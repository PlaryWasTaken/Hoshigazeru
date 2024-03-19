package web

import (
	"encoding/json"
	"github.com/PlaryWasTaken/Hoshigazeru/AniList"
	"github.com/PlaryWasTaken/Hoshigazeru/Client"
	"github.com/gorilla/websocket"
	"log/slog"
	"net"
	"net/http"
)

func Start(client *Client.Client) error {
	slog.Info("Starting web server")
	http.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		slog.Info("New connection")
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		slog.Info("Connection upgraded to websocket")
		webSocketHandler := CreateWebsocketHandler(conn)
		webSocketHandler.Start()
		id := client.Subscribe(func(released AniList.Media, episode AniList.EpisodeSchedule) {
			slog.Info("Sending release", slog.String("title", released.Title))
			data := map[string]interface{}{
				"media":   released,
				"episode": episode,
			}
			webSocketHandler.SendJSON(data)
			if err != nil {
				return
			}
		})
		pingHandler := &PingHandler{
			Conn:             conn,
			WaitingForPong:   false,
			PingsMissed:      0,
			Client:           client,
			Id:               id,
			WebsocketHandler: webSocketHandler,
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
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		slog.Error("Error starting web server", slog.Any("error", err))
		return err
	} else {
		slog.Info("Listening on :8080")
		go func() {
			err := http.Serve(l, nil)
			if err != nil {
				slog.Error("Error serving", slog.Any("error", err)) // Eh should be properly handled, but idc
			}
		}()
		return nil
	}
}
