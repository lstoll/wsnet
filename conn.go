package wsnet

import (
	"net"
	"time"

	"sync"

	"github.com/gorilla/websocket"
)

type wsConn struct {
	conn       *websocket.Conn
	connMu     sync.Mutex
	closedChan chan struct{} // Used for notifing that we're closed.
	recvBuffer []byte
}

func (w *wsConn) Read(b []byte) (n int, err error) {
	if len(w.recvBuffer) > 0 {
		copied := copy(b, w.recvBuffer)
		w.recvBuffer = w.recvBuffer[copied:]
		return copied, nil
	}

	w.connMu.Lock()
	defer w.connMu.Unlock()

	_, msg, err := w.conn.ReadMessage()
	if err != nil {
		return 0, err
	}

	copied := copy(b, msg)
	if copied < len(msg) {
		w.recvBuffer = msg[copied:]
	}
	return copied, nil
}

func (w *wsConn) Write(b []byte) (n int, err error) {
	w.connMu.Lock()
	defer w.connMu.Unlock()
	n = len(b)
	err = w.conn.WriteMessage(websocket.BinaryMessage, b)
	return
}

func (w *wsConn) Close() error {
	w.connMu.Lock()
	defer w.connMu.Unlock()
	if w.closedChan != nil {
		select {
		case w.closedChan <- struct{}{}:
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
