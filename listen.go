package wsnet

import (
	"net"
	"net/http"
	"time"

	"golang.org/x/net/websocket"
)

type wsServer struct {
	listener     net.Listener
	conns        chan *wsConn
	pingInterval time.Duration
}

type wsaddr struct{}

// Listen will start a HTTP server on the given listener, and return the
// net.Listener for the websocket handler on "/"
func Listen(laddr string) (net.Listener, error) {
	return ListenWithKeepalive(laddr, 0)
}

// ListenWithKeepalive will start a HTTP server on the given listener, and
// return the net.Listener for the websocket handler on "/". It will send
// traffic down the connection every pingInterval to keep it alive.
func ListenWithKeepalive(laddr string, pingInterval time.Duration) (net.Listener, error) {
	listener, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}
	wss := &wsServer{
		listener:     listener,
		conns:        make(chan *wsConn),
		pingInterval: pingInterval,
	}
	serveMux := http.NewServeMux()
	serveMux.Handle("/", websocket.Handler(wss.wsHandler))
	go func() {
		err = http.Serve(listener, serveMux)
		if err != nil {
			panic(err)
		}
	}()
	return wss, nil
}

// Handler returns a net.Listener that will be a listener for traffic received
// over the websocket http.Handler
func Handler() (net.Listener, http.Handler) {
	return HandlerWithKeepalive(0)
}

// HandlerWithKeepalive returns a net.Listener that will be a listener for
// traffic received over the websocket http.Handler. It will send traffic down
// the connection every pingInterval to keep it alive.
func HandlerWithKeepalive(pingInterval time.Duration) (net.Listener, http.Handler) {
	wss := &wsServer{
		conns:        make(chan *wsConn),
		pingInterval: pingInterval,
	}
	return wss, websocket.Handler(wss.wsHandler)
}

func (w *wsServer) Accept() (net.Conn, error) {
	return <-w.conns, nil
}

func (w *wsServer) Close() error {
	if w.listener != nil {
		return w.listener.Close()
	}
	return nil
}

func (w *wsServer) Addr() net.Addr {
	// This is still legit enough? Maybe?
	if w.listener != nil {
		return w.listener.Addr()
	}
	return &wsaddr{}
}

func (w *wsServer) wsHandler(ws *websocket.Conn) {
	closed := make(chan struct{})
	wsconn := &wsConn{conn: ws, closed: closed}
	stopHb := make(chan struct{})
	if w.pingInterval > 0 {
		go func() {
			for {
				time.Sleep(w.pingInterval)
				select {
				case <-stopHb:
					return
				default:
				}
				websocket.Message.Send(wsconn.conn, []byte{frameTypeKeepalive})
			}
		}()
	}
	w.conns <- wsconn
	<-closed
	stopHb <- struct{}{}
}

func (w *wsaddr) Network() string {
	return "wsnet"
}

func (w *wsaddr) String() string {
	return "wsnet-handler"
}
