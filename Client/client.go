package Client

import (
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"github.com/PlaryWasTaken/Hoshigazeru/AniList"
)

type (
	ReleaseHandleFunc func(media AniList.Media, episode AniList.EpisodeSchedule)
	Client            struct {
		ReleasedCheckDelay time.Duration
		Polling            *AniList.PollingClient
		// Use GetMedias to read a snapshot while the client is running.
		Medias      []AniList.Media
		Subscribers map[int]ReleaseHandleFunc
		currentId   int
		mu          sync.RWMutex
		cacheMu     sync.Mutex
		startOnce   sync.Once
		released    map[releaseKey]bool
		cachePath   string
	}
	releaseKey struct{ media, episode int }
	release    struct {
		media   AniList.Media
		episode AniList.EpisodeSchedule
	}
)

var (
	br        = regexp.MustCompile(`(?i)<br\s*/?>`)
	bold      = regexp.MustCompile(`(?i)</?b>`)
	italic    = regexp.MustCompile(`(?i)</?i>`)
	underline = regexp.MustCompile(`(?i)</?u>`)
)

func parseHtmlToMarkdown(description string) string {
	replaced := br.ReplaceAllString(description, "\n")
	replaced = bold.ReplaceAllString(replaced, "**")
	replaced = italic.ReplaceAllString(replaced, "*")
	return underline.ReplaceAllString(replaced, "__")
}

func CreateClient(expiredDelay, apiDelay time.Duration) *Client {
	if expiredDelay <= 0 {
		expiredDelay = 15 * time.Second
	}
	return &Client{
		ReleasedCheckDelay: expiredDelay,
		Subscribers:        make(map[int]ReleaseHandleFunc),
		Polling:            AniList.CreateClient(apiDelay),
		Medias:             []AniList.Media{},
		released:           make(map[releaseKey]bool),
		cachePath:          "savedMedias.json",
	}
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneMedia(media AniList.Media) AniList.Media {
	media.Episodes = clonePointer(media.Episodes)
	media.Description = clonePointer(media.Description)
	media.MarkdownDescription = clonePointer(media.MarkdownDescription)
	media.AiringSchedule = append([]AniList.EpisodeSchedule{}, media.AiringSchedule...)
	media.ExternalLinks = append([]AniList.ExternalLinks{}, media.ExternalLinks...)
	for i := range media.ExternalLinks {
		media.ExternalLinks[i].Color = clonePointer(media.ExternalLinks[i].Color)
		media.ExternalLinks[i].Icon = clonePointer(media.ExternalLinks[i].Icon)
	}
	return media
}

func (c *Client) GetMedias() []AniList.Media {
	c.mu.RLock()
	defer c.mu.RUnlock()
	medias := make([]AniList.Media, len(c.Medias))
	for i, media := range c.Medias {
		medias[i] = cloneMedia(media)
	}
	return medias
}

func (c *Client) Subscribe(fn ReleaseHandleFunc) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.currentId
	c.currentId++
	c.Subscribers[id] = fn
	return id
}

func (c *Client) Unsubscribe(index int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.Subscribers, index)
}

func (c *Client) EmitRelease(media AniList.Media, episode AniList.EpisodeSchedule) {
	c.mu.RLock()
	subscribers := make([]ReleaseHandleFunc, 0, len(c.Subscribers))
	for _, subscriber := range c.Subscribers {
		subscribers = append(subscribers, subscriber)
	}
	c.mu.RUnlock()
	for _, subscriber := range subscribers {
		go subscriber(cloneMedia(media), episode)
	}
}

// Caller holds mu. Build a new schedule so removing due episodes cannot skip
// entries, invalidate a range index, or mutate an in-flight notification.
func (c *Client) checkReleasesLocked(now int64) []release {
	var releases []release
	for i := range c.Medias {
		media := &c.Medias[i]
		upcoming := make([]AniList.EpisodeSchedule, 0, len(media.AiringSchedule))
		var due []AniList.EpisodeSchedule
		for _, schedule := range media.AiringSchedule {
			if int64(schedule.AiringAt) <= now {
				key := releaseKey{media.Id, schedule.Episode}
				if !c.released[key] {
					c.released[key] = true
					due = append(due, schedule)
				}
			} else {
				upcoming = append(upcoming, schedule)
			}
		}
		media.AiringSchedule = upcoming
		for _, episode := range due {
			releases = append(releases, release{cloneMedia(*media), episode})
		}
	}
	return releases
}

func (c *Client) publish(releases []release) []AniList.Media {
	medias := make([]AniList.Media, 0, len(releases))
	for _, release := range releases {
		c.EmitRelease(release.media, release.episode)
		medias = append(medias, release.media)
	}
	if len(releases) > 0 {
		slog.Info("Found releases", slog.Int("released", len(releases)))
	}
	return medias
}

func (c *Client) CheckReleases() []AniList.Media {
	c.mu.Lock()
	releases := c.checkReleasesLocked(time.Now().Unix())
	c.mu.Unlock()
	medias := c.publish(releases)
	if len(releases) > 0 {
		c.saveCache()
	}
	return medias
}

func (c *Client) replaceMedias(fetched []AniList.Media) {
	c.mu.Lock()
	now := time.Now().Unix()
	// Account for episodes that aired while a paginated fetch was in flight,
	// including shows that disappeared from AniList after their final episode.
	releases := c.checkReleasesLocked(now)
	c.Medias = make([]AniList.Media, len(fetched))
	for i, media := range fetched {
		media = cloneMedia(media)
		if media.Description != nil {
			markdown := parseHtmlToMarkdown(*media.Description)
			media.MarkdownDescription = &markdown
		}
		c.Medias[i] = media
	}
	releases = append(releases, c.checkReleasesLocked(now)...)
	c.mu.Unlock()
	c.publish(releases)
	c.saveCache()
}

func (c *Client) saveCache() {
	// Serialize saves and take the snapshot afterwards so an older snapshot
	// cannot overwrite a newer one. A failed write leaves the old cache intact.
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	if err := c.writeCache(); err != nil {
		slog.Error("Could not save local buffer", slog.Any("error", err))
	}
}

func (c *Client) writeCache() error {
	data, err := json.Marshal(c.GetMedias())
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(c.cachePath), ".savedMedias-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), c.cachePath)
}

func (c *Client) Start() {
	c.startOnce.Do(func() {
		data, err := os.ReadFile(c.cachePath)
		if err == nil {
			var medias []AniList.Media
			if err := json.Unmarshal(data, &medias); err != nil {
				slog.Error("Unable to read local buffer", slog.Any("error", err))
			} else {
				c.replaceMedias(medias)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			slog.Error("Unable to read local buffer", slog.Any("error", err))
		}
		go func() {
			for fetched := range c.Polling.Chan {
				c.replaceMedias(fetched)
			}
		}()
		c.Polling.Start()
		go func() {
			ticker := time.NewTicker(c.ReleasedCheckDelay)
			defer ticker.Stop()
			for range ticker.C {
				c.CheckReleases()
			}
		}()
	})
}
