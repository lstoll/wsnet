package wsnet

import (
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// DefaultPingInterval is how often to send a ping packet over the link, if not
// otherwise set.
const DefaultPingInterval = 25 * time.Second

// acceptBacklog is the size of the channel to write connections to.
const acceptBacklog = 50

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// WSServer can be used to serve a network service over websockets. It
// implements both net.Listener and http.Handler. It should be bound on to a
// HTTP server, then passed to the network service you're exporting via this as
// it's net.Listener
type WSServer struct {
	// PingInterval is how often to send a ping packet over the connection. This
	// can be used both to keep it alive, and verify a client is listening. If
	// this is zero value the interval DefaultPingInterval will be used.
	PingInterval time.Duration

	conns chan *wsConn
}

type wsaddr struct{}

func (w *WSServer) Accept() (net.Conn, error) {
	if w.conns == nil {
		w.conns = make(chan *wsConn, acceptBacklog)
	}

	return <-w.conns, nil
}

// Close is a noop, as there is no network connection to close. Closing should
// occur at the HTTP server.
func (w *WSServer) Close() error {
	return nil
}

func (w *WSServer) Addr() net.Addr {
	// TODO - best way to address this in this context?
	return &wsaddr{}
}

func (w *WSServer) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(rw, r, nil)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}
	closed := make(chan struct{})
	wsconn := &wsConn{conn: conn, closedChan: closed}
	stopHb := make(chan struct{})
	if w.PingInterval == 0 {
		w.PingInterval = DefaultPingInterval
	}

	errC := make(chan error)
	go func() {
		ticker := time.NewTicker(w.PingInterval)
		for {
			select {
			case <-ticker.C:
				wsconn.connMu.Lock()
				err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(w.PingInterval))
				if err != nil {
					errC <- err
				}
				wsconn.connMu.Unlock()
			case <-stopHb:
				ticker.Stop()
				return
			}
		}
	}()

	if w.conns == nil {
		w.conns = make(chan *wsConn, acceptBacklog)
	}
	w.conns <- wsconn

	select {
	case <-closed:
	case <-errC:
		// assume client closed
		wsconn.Close()
		return
	}
	stopHb <- struct{}{}
}

func (w *wsaddr) Network() string {
	return "wsnet"
}

func (w *wsaddr) String() string {
	return "wsnet-handler"
}
