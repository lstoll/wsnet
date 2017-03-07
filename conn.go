package wsnet

import (
	"net"
	"time"

	"sync"

	"io"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

type wsConn struct {
	conn       *websocket.Conn
	connRMu    sync.Mutex
	connWMu    sync.Mutex
	closedChan chan struct{} // Used for notifing that we're closed.
	recvBuffer []byte
	// The addr to return for this connection.
	addr string
}

func (w *wsConn) Read(b []byte) (int, error) {
	if len(w.recvBuffer) > 0 {
		copied := copy(b, w.recvBuffer)
		w.recvBuffer = w.recvBuffer[copied:]
		return copied, nil
	}

	w.connRMu.Lock()
	defer w.connRMu.Unlock()

	_, msg, err := w.conn.ReadMessage()
	if err != nil {
		if websocket.IsCloseError(errors.Cause(err),
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
		) {
			return 0, io.EOF
		}
		return 0, errors.Wrap(err, "Error reading from underlying websocket")
	}

	copied := copy(b, msg)
	if copied < len(msg) {
		w.recvBuffer = msg[copied:]
	}
	return copied, nil
}

func (w *wsConn) Write(b []byte) (int, error) {
	w.connWMu.Lock()
	defer w.connWMu.Unlock()
	n := len(b)
	err := w.conn.WriteMessage(websocket.BinaryMessage, b)
	if err != nil {
		if websocket.IsCloseError(errors.Cause(err),
			websocket.CloseNormalClosure,
			websocket.CloseGoingAway,
		) {
			return 0, io.EOF
		}
		return 0, errors.Wrap(err, "Error writing to underlying websocket")
	}
	return n, err
}

func (w *wsConn) Close() error {
	w.connWMu.Lock()
	defer w.connWMu.Unlock()
	_ = w.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	// TODO - we're meant to wait for the server to close now. Maybe set an
	// error to return for all read/writes, then wait for the close in a
	// goroutine?
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
	if w.addr != "" {
		return &wsaddr{network: "wsnet", addr: w.addr}
	}
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
