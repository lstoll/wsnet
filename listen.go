package wsnet

import (
	"net"
	"net/http"

	"golang.org/x/net/websocket"
)

type wsServer struct {
	listener net.Listener
	conns    chan *wsConn
}

func Listen(laddr string) (net.Listener, error) {
	listener, err := net.Listen("tcp", laddr)
	if err != nil {
		return nil, err
	}
	wss := &wsServer{
		listener: listener,
		conns:    make(chan *wsConn),
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
	w.conns <- &wsConn{conn: ws, closed: closed}
	<-closed
}
