package web

import (
	"github.com/PlaryWasTaken/Hoshigazeru/Client"
	"github.com/gorilla/websocket"
	"log/slog"
	"time"
)

type PingHandler struct {
	WebsocketHandler *WebsocketHandler
	PingsMissed      int
	WaitingForPong   bool
	Client           *Client.Client
	Id               int
	Conn             *websocket.Conn
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
			p.WebsocketHandler.MsgChan <- Message{
				Type: TextMessage,
				Data: []byte("ping"),
			}
			p.WaitingForPong = true

			time.Sleep(time.Second * 5)
		}
	}()
}
func (p *PingHandler) AcknowledgePing() {
	p.WaitingForPong = false
	p.PingsMissed = 0
}
