package AniList

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/machinebox/graphql"
)

func TestFetchPageNullableFieldsAndLinks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Variables map[string]int }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Variables["page"] != 2 {
			t.Errorf("page variable = %v", request.Variables)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":{"page":{"pageInfo":{"hasNextPage":true},"medias":[
			{"id":1,"title":{"romaji":"Romaji","english":null},"episodes":null,
			"airingSchedule":{"nodes":[{"airingAt":100,"episode":1}]},"isAdult":true,
			"description":null,"coverImage":{"large":"cover"},
			"externalLinks":[{"type":"STREAMING","site":"Site","color":null,"icon":null,"url":"https://example.com","isDisabled":false},
			{"site":"Disabled","isDisabled":true}]},
			{"id":2,"title":{"romaji":"Fallback","english":"English"},"episodes":12,
			"airingSchedule":{"nodes":[]},"description":"Description","externalLinks":[]},
			{"id":3,"title":{"romaji":"Fallback","english":""}}
		]}}}`))
	}))
	defer server.Close()
	page, err := fetchPageWithClient(context.Background(), graphql.NewClient(server.URL), 2)
	if err != nil {
		t.Fatal(err)
	}
	if !page.PageInfo.HasNextPage || len(page.Medias) != 3 {
		t.Fatalf("unexpected page: %+v", page)
	}
	media := page.Medias[0]
	if media.Title != "Romaji" || media.Episodes != nil || media.Description != nil || !media.IsAdult {
		t.Errorf("nullable fields decoded incorrectly: %+v", media)
	}
	if len(media.ExternalLinks) != 1 || media.ExternalLinks[0].Color != nil || media.ExternalLinks[0].Icon != nil || media.ExternalLinks[0].Url != "https://example.com" {
		t.Errorf("links decoded incorrectly: %+v", media.ExternalLinks)
	}
	if media.AiringSchedule[0].Episode != 1 || media.CoverImage != "cover" {
		t.Errorf("media decoded incorrectly: %+v", media)
	}
	if page.Medias[1].Title != "English" || *page.Medias[1].Episodes != 12 || page.Medias[2].Title != "Fallback" {
		t.Errorf("title or episode fallback failed: %+v", page.Medias)
	}
}

func TestFetchPageErrors(t *testing.T) {
	for _, body := range []string{
		`{"data":{"page":null}}`,
		`{"data":{"page":{"medias":[{"id":"invalid"}]}}}`,
		`{"errors":[{"message":"rate limited"}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(body))
			}))
			defer server.Close()
			if _, err := fetchPageWithClient(context.Background(), graphql.NewClient(server.URL), 1); err == nil {
				t.Fatal("expected decoding or API error")
			}
		})
	}
}

func TestPollPages(t *testing.T) {
	var pages []int
	medias, err := pollPages(func(page int) (*PageData, error) {
		pages = append(pages, page)
		data := &PageData{Medias: []Media{{Id: page}}}
		data.PageInfo.HasNextPage = page < 3
		return data, nil
	})
	if err != nil || !reflect.DeepEqual(pages, []int{1, 2, 3}) || len(medias) != 3 {
		t.Fatalf("medias=%v pages=%v error=%v", medias, pages, err)
	}
	failure := errors.New("upstream failed")
	medias, err = pollPages(func(page int) (*PageData, error) {
		if page == 2 {
			return nil, failure
		}
		data := &PageData{Medias: []Media{{Id: page}}}
		data.PageInfo.HasNextPage = true
		return data, nil
	})
	if !errors.Is(err, failure) || medias != nil {
		t.Fatalf("partial results must not replace the cache: %v, %v", medias, err)
	}
}

func TestFetchPageHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := fetchPageWithClient(ctx, graphql.NewClient("http://127.0.0.1:1"), 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
