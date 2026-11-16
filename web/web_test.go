package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PlaryWasTaken/Hoshigazeru/AniList"
	"github.com/PlaryWasTaken/Hoshigazeru/Client"
	"github.com/gorilla/websocket"
)

func TestAnimeEndpoints(t *testing.T) {
	now := int(time.Now().Unix())
	for _, test := range []struct {
		name   string
		medias []AniList.Media
		status int
		id     int
	}{
		{name: "empty", status: http.StatusNotFound},
		{name: "empty schedules", medias: []AniList.Media{{Id: 1}}, status: http.StatusNotFound},
		{name: "past only", medias: []AniList.Media{{Id: 1, AiringSchedule: []AniList.EpisodeSchedule{{AiringAt: now - 60}}}}, status: http.StatusNotFound},
		{name: "empty first and unsorted schedules", status: http.StatusOK, id: 3, medias: []AniList.Media{
			{Id: 1},
			{Id: 2, AiringSchedule: []AniList.EpisodeSchedule{{AiringAt: now + 300}}},
			{Id: 3, AiringSchedule: []AniList.EpisodeSchedule{{AiringAt: now + 900}, {AiringAt: now - 60}, {AiringAt: now + 60}}},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := Client.CreateClient(time.Second, time.Hour)
			client.Medias = test.medias
			handler := NewHandler(client)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/animes/nextRelease", nil))
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body)
			}
			if test.status == http.StatusOK {
				var media AniList.Media
				if err := json.Unmarshal(response.Body.Bytes(), &media); err != nil || media.Id != test.id {
					t.Fatalf("next release = %s (%v)", response.Body, err)
				}
			}
		})
	}
	client := Client.CreateClient(time.Second, time.Hour)
	handler := NewHandler(client)
	// Constructing another handler must not panic on duplicate route registration.
	NewHandler(client)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/animes", nil))
	if response.Code != http.StatusOK || response.Body.String() != "[]" || response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected empty list: %d %s", response.Code, response.Body)
	}
	for _, path := range []string{"/animes", "/animes/nextRelease"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET" {
			t.Fatalf("method restriction failed for %s", path)
		}
	}
}

func TestWebsocketReleaseAndDisconnect(t *testing.T) {
	client := Client.CreateClient(time.Second, time.Hour)
	handler := NewHandler(client)
	returned := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(returned)
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"/releases", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil || string(msg) != "ping" {
		t.Fatalf("initial heartbeat = %q (%v)", msg, err)
	}
	if err := conn.WriteMessage(websocket.TextMessage, []byte("pong")); err != nil {
		t.Fatal(err)
	}
	client.EmitRelease(AniList.Media{Id: 42}, AniList.EpisodeSchedule{Episode: 2})
	var event struct {
		Media   AniList.Media
		Episode AniList.EpisodeSchedule
	}
	if err := conn.ReadJSON(&event); err != nil || event.Media.Id != 42 || event.Episode.Episode != 2 {
		t.Fatalf("unexpected event: %+v (%v)", event, err)
	}
	// Abrupt socket loss must run cleanup without waiting for a close frame.
	conn.Close()
	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("disconnected handler did not return")
	}
	client.EmitRelease(AniList.Media{Id: 42}, AniList.EpisodeSchedule{Episode: 3})
}

func websocketPair(t *testing.T) (*WebsocketHandler, *websocket.Conn) {
	t.Helper()
	ready := make(chan *WebsocketHandler, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		handler := CreateWebsocketHandler(conn)
		ready <- handler
		<-handler.Done()
	}))
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	var handler *WebsocketHandler
	select {
	case handler = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("websocket was not accepted")
	}
	t.Cleanup(handler.Destroy)
	return handler, conn
}

func TestWebsocketQueueOverflowAndConcurrentDestroy(t *testing.T) {
	handler, _ := websocketPair(t)
	// Without a writer consuming the queue, the next send must disconnect.
	for i := 0; i <= cap(handler.MsgChan); i++ {
		handler.SendJSON(map[string]int{"id": i})
	}
	select {
	case <-handler.Done():
	default:
		t.Fatal("full queue did not disconnect the slow consumer")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler.SendJSON("after disconnect")
			handler.SendMessage([]byte("ping"))
			handler.Destroy()
		}()
	}
	wg.Wait()
}

func TestPingTimeoutClosesConnection(t *testing.T) {
	handler, conn := websocketPair(t)
	client := Client.CreateClient(time.Second, time.Hour)
	id := client.Subscribe(func(AniList.Media, AniList.EpisodeSchedule) {})
	ping := &PingHandler{WebsocketHandler: handler, Client: client, Id: id, Conn: handler.Conn,
		WaitingForPong: true, PingsMissed: 4}
	ping.StartPings()
	select {
	case <-handler.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeat timeout did not close the connection")
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("peer connection was not closed")
	}
}
