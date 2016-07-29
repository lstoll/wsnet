package wsnet

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/websocket"
)

// Dial gives you a net.Conn that talks to a WS destination.
// Addr should be like "ws://localhost:8080/"
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	config, err := websocket.NewConfig(addr, addr)
	if err != nil {
		return nil, err
	}
	if u.User != nil {
		ui := base64.StdEncoding.EncodeToString([]byte(u.User.String()))
		config.Header = http.Header{
			"Authorization": {"Basic " + ui},
		}
	}
	ws, err := websocket.DialConfig(config)
	if err != nil {
		return nil, err
	}
	return &wsConn{conn: ws}, nil
}
