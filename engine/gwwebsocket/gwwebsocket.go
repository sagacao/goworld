package gwwebsocket

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/sagacao/goworld/engine/gwlog"
)

type WsServer struct {
	upgrade *websocket.Upgrader

	Handler
}

// ServeHTTP implements the http.Handler interface for a WebSocket
func (s WsServer) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.serveWebSocket(w, req)
}

func (s WsServer) serveWebSocket(w http.ResponseWriter, req *http.Request) {
	conn, err := s.upgrade.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	if conn == nil {
		panic("unexpected nil conn")
	}
	gwlog.Infof("client connect : %v", conn.RemoteAddr())
	s.Handler(conn)
}

// Handler is a simple interface to a WebSocket browser client.
type Handler func(conn *websocket.Conn)

// ServeHTTP implements the http.Handler interface for a WebSocket
func (h Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s := WsServer{
		Handler: h,
		upgrade: &websocket.Upgrader{
			ReadBufferSize:  65535,
			WriteBufferSize: 65535,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
	s.serveWebSocket(w, req)
}
