package wsnet

import (
	"net"
	"time"

	"golang.org/x/net/websocket"
)

// Dial gives you a net.Conn that talks to a WS destination.
// Addr should be like "ws://localhost:8080/"
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	conn, err := websocket.Dial(addr, "", "http://localhost/")
	if err != nil {
		return nil, err
	}
	return &wsConn{conn: conn}, nil
}
