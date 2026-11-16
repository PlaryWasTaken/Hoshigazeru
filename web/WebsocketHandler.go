package web

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type MsgType int

const (
	TextMessage MsgType = iota
	JSONMessage
	BinaryMessage
)

type (
	Message struct {
		Type MsgType
		Data interface{}
	}
	WebsocketHandler struct {
		Conn      *websocket.Conn
		MsgChan   chan Message
		done      chan struct{}
		startOnce sync.Once
		closeOnce sync.Once
	}
)

func CreateWebsocketHandler(conn *websocket.Conn) *WebsocketHandler {
	return &WebsocketHandler{
		Conn:    conn,
		MsgChan: make(chan Message, 20),
		done:    make(chan struct{}),
	}
}

func (w *WebsocketHandler) Done() <-chan struct{} { return w.done }

func (w *WebsocketHandler) Start() {
	w.startOnce.Do(func() {
		go func() {
			defer w.Destroy()
			for {
				select {
				case <-w.done:
					return
				case msg := <-w.MsgChan:
					if err := w.Conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
						return
					}
					var err error
					switch msg.Type {
					case TextMessage:
						err = w.Conn.WriteMessage(websocket.TextMessage, msg.Data.([]byte))
					case JSONMessage:
						err = w.Conn.WriteJSON(msg.Data)
					case BinaryMessage:
						err = w.Conn.WriteMessage(websocket.BinaryMessage, msg.Data.([]byte))
					}
					if err != nil {
						return
					}
				}
			}
		}()
	})
}

func (w *WebsocketHandler) send(msg Message) {
	select {
	case <-w.done:
		return
	default:
	}
	select {
	case <-w.done:
	case w.MsgChan <- msg:
	default:
		// Disconnect a slow consumer instead of blocking release delivery.
		w.Destroy()
	}
}

func (w *WebsocketHandler) SendMessage(msg []byte) {
	w.send(Message{Type: TextMessage, Data: msg})
}

func (w *WebsocketHandler) SendJSON(msg interface{}) {
	w.send(Message{Type: JSONMessage, Data: msg})
}

func (w *WebsocketHandler) Destroy() {
	w.closeOnce.Do(func() {
		close(w.done)
		_ = w.Conn.Close()
	})
}
