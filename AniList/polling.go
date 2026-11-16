package AniList

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/machinebox/graphql"
)

type (
	PollingClient struct {
		ApiFetchDelay time.Duration
		Chan          chan []Media
		startOnce     sync.Once
	}
	PageData struct {
		PageInfo struct{ HasNextPage bool }
		Medias   []Media
	}
	Media struct {
		Id                  int
		Title               string
		Episodes            *int
		AiringSchedule      []EpisodeSchedule
		Description         *string
		MarkdownDescription *string
		CoverImage          string
		ExternalLinks       []ExternalLinks
		IsAdult             bool
	}
	EpisodeSchedule struct {
		AiringAt int
		Episode  int
	}
	ExternalLinks struct {
		Type  string
		Site  string
		Color *string
		Url   string
		Icon  *string
	}
)

const mediaQuery = `
query ($page: Int) {
  page: Page(perPage: 50, page: $page) {
    pageInfo { hasNextPage }
    medias: media(type: ANIME, format: TV, sort: POPULARITY_DESC, status_in: [RELEASING, NOT_YET_RELEASED]) {
      id
      title { romaji english }
      episodes
      airingSchedule(notYetAired: true) { nodes { airingAt episode } }
      isAdult
      description(asHtml: false)
      coverImage { large }
      externalLinks { isDisabled icon type site color url }
    }
  }
}`

// Decode nullable API fields directly instead of asserting types from untyped maps.
type apiMedia struct {
	Id    int
	Title struct {
		Romaji  string
		English *string
	}
	Episodes       *int
	AiringSchedule struct{ Nodes []EpisodeSchedule }
	IsAdult        bool
	Description    *string
	CoverImage     struct{ Large string }
	ExternalLinks  []struct {
		ExternalLinks
		IsDisabled bool
	}
}

func fetchPage(page int) (*PageData, error) {
	return fetchPageWithClient(context.Background(), graphql.NewClient("https://graphql.anilist.co"), page)
}

func fetchPageWithClient(ctx context.Context, client *graphql.Client, page int) (*PageData, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req := graphql.NewRequest(mediaQuery)
	req.Var("page", page)
	var response struct {
		Page *struct {
			PageInfo struct{ HasNextPage bool }
			Medias   []apiMedia
		}
	}
	if err := client.Run(ctx, req, &response); err != nil {
		return nil, err
	}
	if response.Page == nil {
		return nil, fmt.Errorf("page not found")
	}
	data := &PageData{Medias: make([]Media, 0, len(response.Page.Medias))}
	data.PageInfo.HasNextPage = response.Page.PageInfo.HasNextPage
	for _, media := range response.Page.Medias {
		title := media.Title.Romaji
		if media.Title.English != nil && *media.Title.English != "" {
			title = *media.Title.English
		}
		links := make([]ExternalLinks, 0, len(media.ExternalLinks))
		for _, link := range media.ExternalLinks {
			if !link.IsDisabled {
				links = append(links, link.ExternalLinks)
			}
		}
		schedule := media.AiringSchedule.Nodes
		if schedule == nil {
			schedule = []EpisodeSchedule{}
		}
		data.Medias = append(data.Medias, Media{
			Id: media.Id, Title: title, Episodes: media.Episodes,
			AiringSchedule: schedule, Description: media.Description,
			CoverImage: media.CoverImage.Large, ExternalLinks: links, IsAdult: media.IsAdult,
		})
	}
	return data, nil
}

func CreateClient(delay time.Duration) *PollingClient {
	if delay <= 0 {
		delay = time.Hour
	}
	return &PollingClient{ApiFetchDelay: delay, Chan: make(chan []Media)}
}

func pollPages(fetch func(int) (*PageData, error)) ([]Media, error) {
	medias := []Media{}
	for currentPage := 1; ; currentPage++ {
		slog.Info("Fetching page", slog.Int("page", currentPage))
		page, err := fetch(currentPage)
		if err != nil {
			return nil, err
		}
		medias = append(medias, page.Medias...)
		if !page.PageInfo.HasNextPage {
			return medias, nil
		}
	}
}

// PollApi fetches a complete snapshot without requiring a channel consumer.
func (c *PollingClient) PollApi() ([]Media, error) {
	return pollPages(fetchPage)
}

func (c *PollingClient) Start() {
	c.startOnce.Do(func() {
		go func() {
			for {
				medias, err := c.PollApi()
				if err != nil {
					slog.Error("Polling failed", slog.Any("error", err))
				} else {
					c.Chan <- medias
				}
				time.Sleep(c.ApiFetchDelay)
			}
		}()
	})
}
