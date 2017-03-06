package wsnet

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

// Dial gives you a net.Conn that talks to a WS destination.
// Addr should be like "ws://localhost:8080/"
func Dial(addr string, timeout time.Duration) (net.Conn, error) {
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	var hdr = http.Header{}
	if u.User != nil {
		ui := base64.StdEncoding.EncodeToString([]byte(u.User.String()))
		hdr["Authorization"] = []string{"Basic " + ui}
	}
	u.User = nil
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	if err != nil {
		return nil, err
	}
	return &wsConn{conn: conn}, nil
}
