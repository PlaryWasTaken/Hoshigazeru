package Client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/PlaryWasTaken/Hoshigazeru/AniList"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	c := CreateClient(time.Second, time.Hour)
	c.cachePath = filepath.Join(t.TempDir(), "savedMedias.json")
	return c
}

func TestCheckReleasesRemovesEveryDueEpisode(t *testing.T) {
	c := testClient(t)
	now := time.Now().Unix()
	c.Medias = []AniList.Media{{Id: 1, AiringSchedule: []AniList.EpisodeSchedule{
		{Episode: 1, AiringAt: int(now - 2)},
		{Episode: 2, AiringAt: int(now - 1)},
		{Episode: 3, AiringAt: int(now)},
		{Episode: 4, AiringAt: int(now + 3600)},
	}}}
	events := make(chan AniList.EpisodeSchedule, 4)
	id := c.Subscribe(func(_ AniList.Media, episode AniList.EpisodeSchedule) { events <- episode })
	defer c.Unsubscribe(id)
	if releases := c.CheckReleases(); len(releases) != 3 {
		t.Fatalf("got %d releases, want 3", len(releases))
	}
	seen := map[int]bool{}
	for i := 0; i < 3; i++ {
		select {
		case event := <-events:
			seen[event.Episode] = true
		case <-time.After(time.Second):
			t.Fatal("release notification was not delivered")
		}
	}
	if !seen[1] || !seen[2] || !seen[3] {
		t.Fatalf("missing or duplicated notification: %v", seen)
	}
	if schedule := c.GetMedias()[0].AiringSchedule; len(schedule) != 1 || schedule[0].Episode != 4 {
		t.Fatalf("remaining schedule = %v", schedule)
	}
	if len(c.CheckReleases()) != 0 {
		t.Fatal("a due episode was emitted twice")
	}
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved []AniList.Media
	if err := json.Unmarshal(data, &saved); err != nil || len(saved[0].AiringSchedule) != 1 {
		t.Fatalf("cache retained released episodes: %s (%v)", data, err)
	}
}

func TestRefreshPreservesFinalReleasesAndDeduplicates(t *testing.T) {
	c := testClient(t)
	due := AniList.EpisodeSchedule{Episode: 12, AiringAt: int(time.Now().Unix() - 60)}
	c.Medias = []AniList.Media{{Id: 1, AiringSchedule: []AniList.EpisodeSchedule{due}}}
	c.replaceMedias([]AniList.Media{{Id: 2, AiringSchedule: []AniList.EpisodeSchedule{due}}})
	c.mu.Lock()
	first := c.released[releaseKey{1, 12}]
	second := c.released[releaseKey{2, 12}]
	c.mu.Unlock()
	if !first || !second {
		t.Fatal("lost an old final episode or a newly fetched due episode")
	}
	c.replaceMedias([]AniList.Media{{Id: 2, AiringSchedule: []AniList.EpisodeSchedule{due, due}}})
	if len(c.CheckReleases()) != 0 || len(c.GetMedias()[0].AiringSchedule) != 0 {
		t.Fatal("refresh reintroduced an already emitted episode")
	}
}

func TestSnapshotDoesNotAliasSchedules(t *testing.T) {
	c := testClient(t)
	episodes, description, color := 12, "description", "color"
	c.Medias = []AniList.Media{{Id: 1, Episodes: &episodes, Description: &description,
		AiringSchedule: []AniList.EpisodeSchedule{{Episode: 1}},
		ExternalLinks:  []AniList.ExternalLinks{{Color: &color}},
	}}
	snapshot := c.GetMedias()
	snapshot[0].AiringSchedule[0].Episode = 99
	snapshot[0].Title = "modified"
	*snapshot[0].Episodes = 99
	*snapshot[0].Description = "modified"
	*snapshot[0].ExternalLinks[0].Color = "modified"
	if got := c.GetMedias()[0]; got.AiringSchedule[0].Episode != 1 || got.Title != "" || *got.Episodes != 12 || *got.Description != "description" || *got.ExternalLinks[0].Color != "color" {
		t.Fatalf("snapshot mutated client: %+v", got)
	}
}

func TestConcurrentRefreshReadsAndSubscriptions(t *testing.T) {
	c := testClient(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				id := c.Subscribe(func(AniList.Media, AniList.EpisodeSchedule) {})
				c.EmitRelease(AniList.Media{Id: 1}, AniList.EpisodeSchedule{Episode: 1})
				c.GetMedias()
				c.Unsubscribe(id)
				c.CheckReleases()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 20; j++ {
			c.replaceMedias([]AniList.Media{{Id: j}})
		}
	}()
	wg.Wait()
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.Subscribers) != 0 {
		t.Fatal("subscriptions leaked")
	}
}

func TestCacheReplaceAndFailure(t *testing.T) {
	c := testClient(t)
	c.replaceMedias([]AniList.Media{{Id: 1}})
	c.replaceMedias([]AniList.Media{{Id: 2}})
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved []AniList.Media
	if err := json.Unmarshal(data, &saved); err != nil || len(saved) != 1 || saved[0].Id != 2 {
		t.Fatalf("cache replacement failed: %s (%v)", data, err)
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(c.cachePath), ".savedMedias-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("temporary cache files leaked: %v (%v)", files, err)
	}
	c.cachePath = filepath.Join(t.TempDir(), "missing-directory", "savedMedias.json")
	c.replaceMedias([]AniList.Media{{Id: 3}})
	if c.GetMedias()[0].Id != 3 {
		t.Fatal("a cache write failure prevented an in-memory update")
	}
}

func TestParseHtmlToMarkdown(t *testing.T) {
	got := parseHtmlToMarkdown("<b>Bold</b><br><i>Italic</i><br/><u>Underlined</u><BR />End")
	if want := "**Bold**\n*Italic*\n__Underlined__\nEnd"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
