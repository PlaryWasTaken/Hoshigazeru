package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/PlaryWasTaken/Hoshigazeru/AniList"
	"github.com/PlaryWasTaken/Hoshigazeru/Client"
	"github.com/gorilla/websocket"
)

// NewHandler uses a private mux so creating another server does not register
// duplicate routes in the process-wide default mux.
func NewHandler(client *Client.Client) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		handler := CreateWebsocketHandler(conn)
		defer handler.Destroy()
		handler.Start()
		id := client.Subscribe(func(released AniList.Media, episode AniList.EpisodeSchedule) {
			handler.SendJSON(map[string]interface{}{"media": released, "episode": episode})
		})
		defer client.Unsubscribe(id)
		ping := &PingHandler{Conn: conn, Client: client, Id: id, WebsocketHandler: handler}
		// Set all read options before the read loop starts.
		conn.SetReadLimit(1024)
		conn.SetPongHandler(func(string) error {
			ping.AcknowledgePing()
			return nil
		})
		ping.StartPings()
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return
			}
			switch string(msg) {
			case "pong":
				ping.AcknowledgePing()
			case "close":
				return
			}
		}
	})
	mux.HandleFunc("/animes", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}
		writeJSON(w, client.GetMedias())
	})
	mux.HandleFunc("/animes/nextRelease", func(w http.ResponseWriter, r *http.Request) {
		if !requireGET(w, r) {
			return
		}
		var next *AniList.Media
		var nextAt int
		now := time.Now().Unix()
		for _, media := range client.GetMedias() {
			for _, episode := range media.AiringSchedule {
				if int64(episode.AiringAt) > now && (next == nil || episode.AiringAt < nextAt) {
					copy := media
					next = &copy
					nextAt = episode.AiringAt
				}
			}
		}
		if next == nil {
			http.Error(w, "no upcoming releases", http.StatusNotFound)
			return
		}
		writeJSON(w, next)
	})
	return mux
}

func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	body, err := json.Marshal(data)
	if err != nil {
		http.Error(w, "could not encode response", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(body); err != nil {
		slog.Debug("Could not write response", slog.Any("error", err))
	}
}

// Start serves until the listener fails or the server is stopped.
func Start(client *Client.Client) error {
	server := &http.Server{
		Addr: ":8080", Handler: NewHandler(client),
		ReadHeaderTimeout: 5 * time.Second, IdleTimeout: time.Minute,
	}
	slog.Info("Listening on :8080")
	return server.ListenAndServe()
}
