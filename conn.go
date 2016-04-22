package wsnet

import (
	"net"
	"time"

	"golang.org/x/net/websocket"
)

type wsConn struct {
	conn   *websocket.Conn
	rbuf   []byte
	closed chan struct{} // Used for notifing that we're closed.
}

func (w *wsConn) Read(b []byte) (n int, err error) {
	if len(w.rbuf) == 0 {
		err = websocket.Message.Receive(w.conn, &w.rbuf)
	}
	n = copy(b, w.rbuf)
	w.rbuf = w.rbuf[n:]
	return
}

func (w *wsConn) Write(b []byte) (n int, err error) {
	err = websocket.Message.Send(w.conn, b)
	n = len(b)
	return
}

func (w *wsConn) Close() error {
	if w.closed != nil {
		select {
		case w.closed <- struct{}{}:
		default:
		}
	}
	return w.conn.Close()
}

func (w *wsConn) LocalAddr() net.Addr {
	return w.conn.LocalAddr()
}

func (w *wsConn) RemoteAddr() net.Addr {
	return w.conn.RemoteAddr()
}

func (w *wsConn) SetDeadline(t time.Time) error {
	return w.SetDeadline(t)
}
func (w *wsConn) SetReadDeadline(t time.Time) error {
	return w.conn.SetReadDeadline(t)
}

func (w *wsConn) SetWriteDeadline(t time.Time) error {
	return w.conn.SetWriteDeadline(t)
}
