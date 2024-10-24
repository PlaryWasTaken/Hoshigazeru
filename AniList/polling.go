package AniList

import (
	"context"
	"fmt"
	"github.com/machinebox/graphql"
	"log/slog"
	"time"
)

type (
	PollingClient struct {
		ApiFetchDelay time.Duration
		Chan          chan []Media
		started       bool
	}
	PageData struct {
		PageInfo struct {
			hasNextPage bool
		}
		Medias []Media
	}
	Media struct {
		Id             int
		Title          string
		Episodes       *int // This is a pointer because the episodes can be null
		AiringSchedule []EpisodeSchedule
		Description    *string // This is a pointer because the description can be null
		MarkdownDescription *string
		CoverImage     string
		ExternalLinks  []ExternalLinks
	}
	EpisodeSchedule struct {
		AiringAt int
		Episode  int
	}
	ExternalLinks struct {
		Type  string
		Site  string
		Color string
		Url   string
		Icon  string
	}
)

func fetchPage(page int) (*PageData, error) {
	client := graphql.NewClient("https://graphql.anilist.co")
	req := graphql.NewRequest(`
	query ($page: Int) {
		page: Page(perPage: 50, page: $page) {
    			pageInfo {
      				hasNextPage
    			}
    		medias: media(format: TV, sort: POPULARITY_DESC, status_in: [RELEASING, NOT_YET_RELEASED]) {
      			id
      			title {
        			romaji
        			english
      			}
      			episodes
      			airingSchedule(notYetAired: true) {
      		  	nodes {
     			    airingAt
      			    episode
     			   }
     		 	}
      			description (asHtml: false)
      			coverImage {
      			  large
      			}
				externalLinks {
					isDisabled
					icon
					type
      				site
      				color
      				url
				}
    		}
  		}
	}
	`)
	req.Var("page", page)
	ctx := context.WithoutCancel(context.Background())
	var respData map[string]interface{}
	if err := client.Run(ctx, req, &respData); err != nil {
		return nil, err
	}
	var pageData PageData
	// Unmarshal the response into a struct
	if respData["page"] == nil {
		return nil, fmt.Errorf("page not found")
	}
	pageData.PageInfo = struct {
		hasNextPage bool
	}{
		hasNextPage: respData["page"].(map[string]interface{})["pageInfo"].(map[string]interface{})["hasNextPage"].(bool),
	}
	medias := respData["page"].(map[string]interface{})["medias"].([]interface{})
	for _, media := range medias {
		mediaData := media.(map[string]interface{})
		var title string
		if mediaData["title"].(map[string]interface{})["english"] != nil {
			title = mediaData["title"].(map[string]interface{})["english"].(string)
		} else {
			title = mediaData["title"].(map[string]interface{})["romaji"].(string)
		}
		var airingSchedule []EpisodeSchedule
		for _, schedule := range mediaData["airingSchedule"].(map[string]interface{})["nodes"].([]interface{}) {
			scheduleData := schedule.(map[string]interface{})
			airingSchedule = append(airingSchedule, EpisodeSchedule{
				AiringAt: int(scheduleData["airingAt"].(float64)),
				Episode:  int(scheduleData["episode"].(float64)),
			})
		}
		var externalLinks []ExternalLinks
		for _, link := range mediaData["externalLinks"].([]interface{}) {
			linkData := link.(map[string]interface{})
			if linkData["isDisabled"].(bool) {
				continue
			}
			externalLinks = append(externalLinks, ExternalLinks{
				Type:  linkData["type"].(string),
				Site:  linkData["site"].(string),
				Color: linkData["color"].(string),
				Url:   linkData["url"].(string),
				Icon:  linkData["icon"].(string),
			})
		}
		var episodes *int = nil
		if mediaData["episodes"] != nil {
			eps := int(mediaData["episodes"].(float64))
			episodes = &eps
		}
		var description *string = nil
		if mediaData["description"] != nil {
			desc := mediaData["description"].(string)
			description = &desc
		}
		pageData.Medias = append(pageData.Medias, Media{
			Id:             int(mediaData["id"].(float64)),
			Title:          title,
			Episodes:       episodes,
			Description:    description,
			AiringSchedule: airingSchedule,
			CoverImage:     mediaData["coverImage"].(map[string]interface{})["large"].(string),
			ExternalLinks:  externalLinks,
		})

	}

	return &pageData, nil
}

func CreateClient(delay time.Duration) *PollingClient {
	return &PollingClient{
		ApiFetchDelay: delay,
		Chan:          make(chan []Media),
	}
}

func (c *PollingClient) PollApi() ([]Media, error) {
	// This method will be called every `c.ApiFetchDelay` seconds
	var medias []Media
	currentPage := 1
	for {
		slog.Info("Fetching page", slog.Int("page", currentPage))
		page, err := fetchPage(currentPage)
		if err != nil {
			return nil, err
		}
		medias = append(medias, page.Medias...)
		if !page.PageInfo.hasNextPage {
			break
		}
		currentPage++
	}
	c.Chan <- medias
	return medias, nil
}

func (c *PollingClient) Start() {
	// Start polling
	if c.started {
		return
	}
	go func() {
		c.started = true
		for {
			_, err := c.PollApi()
			if err != nil {
				slog.Error("Polling failed", slog.Any("error", err))
			}
			time.Sleep(c.ApiFetchDelay)
		}
	}()
	return
}
