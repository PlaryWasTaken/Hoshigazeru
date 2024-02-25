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

		id := client.Subscribe(func(released AniList.Media) {
			slog.Info("Sending release", slog.String("title", released.Title))
			err := conn.WriteJSON(released)
			if err != nil {
				return
			}
		})

		conn.SetCloseHandler(func(code int, text string) error {
			slog.Info("Connection closed", slog.Int("code", code), slog.String("text", text))
			client.Unsubscribe(id)
			conn.Close()
			return nil
		})

		go func() {
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					return
				}
				slog.Info("Received message", slog.String("message", string(msg)))
				switch string(msg) {
				case "ping":
					err := conn.WriteMessage(websocket.TextMessage, []byte("pong"))
					if err != nil {
						return
					}
					break
				case "close":
					client.Unsubscribe(id)
					err := conn.Close()
					if err != nil {
						return
					}
					return
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
