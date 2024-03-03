package Client

import (
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
		Conn    *websocket.Conn
		MsgChan chan Message
	}
)

func CreateWebsocketHandler(conn *websocket.Conn) *WebsocketHandler {
	return &WebsocketHandler{
		Conn:    conn,
		MsgChan: make(chan Message),
	}
}

func (w *WebsocketHandler) Start() {
	go func() {
		for {
			msg := <-w.MsgChan
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
	}()
}
func (w *WebsocketHandler) SendMessage(msg []byte) {
	w.MsgChan <- Message{
		Type: TextMessage,
		Data: msg,
	}
}
func (w *WebsocketHandler) SendJSON(msg interface{}) {
	w.MsgChan <- Message{
		Type: JSONMessage,
		Data: msg,
	}
}
