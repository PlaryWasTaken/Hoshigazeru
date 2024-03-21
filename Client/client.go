package Client

import (
	"encoding/json"
	"fmt"
	"github.com/PlaryWasTaken/Hoshigazeru/AniList"
	"log/slog"
	"os"
	"time"
)

type (
	ReleaseHandleFunc func(media AniList.Media, episode AniList.EpisodeSchedule)
	Client            struct {
		ReleasedCheckDelay time.Duration
		//ReleasedChan       chan AniList.Media
		Polling     *AniList.PollingClient
		Medias      []AniList.Media
		Subscribers map[int]ReleaseHandleFunc
		currentId   int
	}
)

func fastRemove[T any](slice []T, i int) []T {
	slice[i] = slice[len(slice)-1]
	return slice[:len(slice)-1]
}

func CreateClient(expiredDelay time.Duration, apiDelay time.Duration) *Client {
	return &Client{
		ReleasedCheckDelay: expiredDelay,
		Subscribers:        make(map[int]ReleaseHandleFunc),
		Polling:            AniList.CreateClient(apiDelay),
		currentId:          0,
	}
}
func (c *Client) Subscribe(fn ReleaseHandleFunc) int {
	c.Subscribers[c.currentId] = fn
	c.currentId += 1
	return c.currentId - 1
}
func (c *Client) Unsubscribe(index int) {
	fmt.Println(c.Subscribers)
	delete(c.Subscribers, index)
	fmt.Println(c.Subscribers)
}
func (c *Client) EmitRelease(media AniList.Media, episode AniList.EpisodeSchedule) {
	for _, subscriber := range c.Subscribers {
		go subscriber(media, episode)
	}
}

func (c *Client) CheckReleases() []AniList.Media {
	slog.Debug("Checking releases")
	var releases []AniList.Media
	var newList []AniList.Media
	for _, media := range c.Medias {
		mediaPtr := &media
		for i, schedule := range mediaPtr.AiringSchedule {
			if time.Now().Unix() > int64(schedule.AiringAt) {
				mediaPtr.AiringSchedule = fastRemove(media.AiringSchedule, i)
				c.EmitRelease(*mediaPtr, schedule)
				releases = append(releases, *mediaPtr)
			}
		}
		newList = append(newList, *mediaPtr)
	}
	c.Medias = newList
	if len(releases) > 0 {
		slog.Info(fmt.Sprintf("Found %d releases", len(releases)), slog.Int("released", len(releases)), slog.Any("releases", releases))
	}
	return releases
}

func (c *Client) Start() {
	c.Polling.Start()
	file, _ := os.ReadFile("savedMedias.json")
	if file != nil {
		var data []AniList.Media
		err := json.Unmarshal(file, &data)
		if err != nil {
			slog.Error("Unable to read local buffer, skipping to initialization")
		} else {
			c.Medias = data
			c.CheckReleases()
		}
	} else {
		slog.Info("Local buffer not yet created, skipping")
	}
	go func() {
		for {
			fetched := <-c.Polling.Chan
			c.Medias = fetched
			// Creates local buffer of medias for release checking at startup
			create, err := os.Create("savedMedias.json")
			if err != nil {
				slog.Error("Could not create local buffer")
				os.Exit(1)
				return
			}
			saved, err := json.Marshal(c.Medias)
			if err != nil {
				return
			}
			_, err = create.Write(saved)
			if err != nil {
				return
			}
			slog.Info("Written to local buffer")
		}
	}()
	go func() {
		for {
			c.CheckReleases()
			time.Sleep(c.ReleasedCheckDelay)
		}
	}()
}
