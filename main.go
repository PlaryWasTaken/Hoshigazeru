package main

import (
	"encoding/json"
	"github.com/PlaryWasTaken/Hoshigazeru/AniList"
	"github.com/PlaryWasTaken/Hoshigazeru/Client"
	"github.com/gorilla/websocket"
	"log/slog"
	"net/http"
	"time"
)

func main() {
	client := Client.CreateClient(time.Second*5, time.Hour)
	client.Start()

	http.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("New connection")
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		slog.Info("Connection upgraded to websocket")

		id := client.Subscribe(func(released AniList.Media, episode AniList.EpisodeSchedule) {
			slog.Info("Sending release", slog.String("title", released.Title))
			data := map[string]interface{}{
				"media":   released,
				"episode": episode,
			}
			err := conn.WriteJSON(data)
			if err != nil {
				return
			}
		})
		pingHandler := &PingHandler{
			Conn:           conn,
			WaitingForPong: false,
			PingsMissed:    0,
			Client:         client,
			Id:             id,
		}
		pingHandler.StartPings()
		conn.SetCloseHandler(func(code int, text string) error {
			slog.Info("Connection closed", slog.Int("code", code), slog.String("text", text))
			client.Unsubscribe(id)
			_ = conn.Close()
			return nil
		})

		go func() {
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}

				switch string(msg) {
				case "pong":
					pingHandler.AcknowledgePing()
					break
				case "close":
					client.Unsubscribe(id)
					err := conn.Close()
					if err != nil {
						return
					}
					return
				default:
					slog.Info("Received message", slog.String("message", string(msg)))
				}
			}
		}()
	})
	http.HandleFunc("/animes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		marshal, err := json.Marshal(client.Medias)
		if err != nil {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, err = w.Write(marshal)
		if err != nil {
			return
		}
	})
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		return
	}
}

type PingHandler struct {
	Conn           *websocket.Conn
	PingsMissed    int
	WaitingForPong bool
	Client         *Client.Client
	Id             int
}

func (p *PingHandler) StartPings() {
	go func() {
		for {
			if p.WaitingForPong {
				p.PingsMissed++
				slog.Info("Pings missed", slog.Int("missed", p.PingsMissed))
				if p.PingsMissed > 3 {
					_ = p.Conn.Close()
					p.Client.Unsubscribe(p.Id)
					return
				}
			}
			_ = p.Conn.WriteMessage(websocket.TextMessage, []byte("ping"))
			p.WaitingForPong = true

			time.Sleep(time.Second * 5)
		}
	}()
}
func (p *PingHandler) AcknowledgePing() {
	p.WaitingForPong = false
	p.PingsMissed = 0
}
