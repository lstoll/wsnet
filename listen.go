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

func Listen(laddr string) (net.Listener, error) {
	return ListenWithKeepalive(laddr, 0)
}

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
	http.Handle("/", websocket.Handler(wss.wsHandler))
	go http.Serve(listener, nil)
	return wss, nil
}

func (w *wsServer) Accept() (net.Conn, error) {
	return <-w.conns, nil
}

func (w *wsServer) Close() error {
	return w.listener.Close()
}

func (w *wsServer) Addr() net.Addr {
	// This is still legit enough? Maybe?
	return w.listener.Addr()
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
