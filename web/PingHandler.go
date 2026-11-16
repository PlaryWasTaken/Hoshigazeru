package web

import (
	"sync"
	"time"

	"github.com/PlaryWasTaken/Hoshigazeru/Client"
	"github.com/gorilla/websocket"
)

type PingHandler struct {
	WebsocketHandler *WebsocketHandler
	PingsMissed      int
	WaitingForPong   bool
	Client           *Client.Client
	Id               int
	Conn             *websocket.Conn
	mu               sync.Mutex
	startOnce        sync.Once
}

func (p *PingHandler) StartPings() {
	p.startOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			defer p.Client.Unsubscribe(p.Id)
			for {
				select {
				case <-p.WebsocketHandler.Done():
					return
				default:
				}
				p.mu.Lock()
				if p.WaitingForPong {
					p.PingsMissed++
				}
				expired := p.PingsMissed > 3
				p.WaitingForPong = true
				p.mu.Unlock()
				if expired {
					p.WebsocketHandler.Destroy()
					return
				}
				p.WebsocketHandler.SendMessage([]byte("ping"))
				select {
				case <-p.WebsocketHandler.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func (p *PingHandler) AcknowledgePing() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.WaitingForPong = false
	p.PingsMissed = 0
}
