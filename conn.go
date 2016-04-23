package wsnet

import (
	"fmt"
	"net"
	"time"

	"golang.org/x/net/websocket"
)

const (
	frameTypeData      byte = 0x00
	frameTypeKeepalive byte = 0x01
)

type wsConn struct {
	conn   *websocket.Conn
	rbuf   []byte
	closed chan struct{} // Used for notifing that we're closed.
}

func (w *wsConn) Read(b []byte) (n int, err error) {
	if len(w.rbuf) == 0 {
		in := []byte{}
		err = websocket.Message.Receive(w.conn, &in)
		if err != nil {
			return
		}
		switch in[0] {
		case frameTypeData:
			w.rbuf = in[1:]
		case frameTypeKeepalive:
			// Do nothing, it's just a make work frame
		default:
			panic(fmt.Sprintf("Received a websocket frame with an unknown type: %#x", in[0]))
		}
	}
	n = copy(b, w.rbuf)
	w.rbuf = w.rbuf[n:]
	return
}

func (w *wsConn) Write(b []byte) (n int, err error) {
	err = websocket.Message.Send(w.conn, append([]byte{frameTypeData}, b...))
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
