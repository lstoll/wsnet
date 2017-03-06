package wsnet

import (
	"encoding/base64"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"fmt"

	"sync"

	"net"

	"encoding/gob"

	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

type action int

const (
	actionInit action = iota
	actionData
	actionClose
)

type message struct {
	Token   uuid.UUID
	Action  action
	Payload []byte
}

func sendMessage(c *websocket.Conn, msg *message) error {
	w, err := c.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return errors.Wrap(err, "Error getting socket writer")
	}
	enc := gob.NewEncoder(w)
	err = enc.Encode(msg)
	if err != nil {
		return errors.Wrap(err, "Error encoding message")
	}
	return w.Close()
}

func receiveMessage(c *websocket.Conn) (*message, error) {
	_, r, err := c.NextReader()
	if err != nil {
		return nil, errors.Wrap(err, "Error getting reader for connection")
	}

	m := new(message)
	dec := gob.NewDecoder(r)
	err = dec.Decode(m)
	if err != nil {
		return nil, errors.Wrap(err, "Error decoding message")
	}
	return m, nil
}

// WSTunReflector is a http.Handler that can be used to act as a tunneled
// service reflector. It should be mounted on a HTTP server accessible to both
// the clients that will be dialing it, and the server that will be exporting
// it's service via it's WSTunServer.
//
// This handler must be used on the root (i.e /) path on the server. Both the
// server and the client dialer rely on the URL path to determine the service it
// is connecting to.
type WSTunReflector struct {
	// PingInterval is how often to send a ping packet over the connection. This
	// can be used both to keep it alive, and verify a client is listening. If
	// this is zero value the interval DefaultPingInterval will be used.
	PingInterval time.Duration

	servers   map[string]*reflectedServer
	serversMu sync.RWMutex
}

type reflectedServer struct {
	conn      *websocket.Conn
	connWMu   sync.Mutex
	clients   map[uuid.UUID]*reflectedClient
	clientsMu sync.RWMutex
}

type reflectedClient struct {
	conn *websocket.Conn
}

// closeClient notifies the server that a client is gone, and removes it from the list
func (r *reflectedServer) closeClient(u uuid.UUID) error {
	r.connWMu.Lock()
	defer r.connWMu.Unlock()
	delete(r.clients, u)
	return sendMessage(r.conn, &message{
		Token:  u,
		Action: actionClose,
	})
}

func (w *WSTunReflector) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	pc := strings.Split(strings.Trim(r.URL.Path, "/ "), "/")
	rt, id := pc[0], pc[1]
	if len(pc) != 2 || rt == "" || id == "" {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Request path needs to be in form of /[server|client]/<identifier>, got %q", r.URL.Path)
		return
	}
	if rt == "server" {
		w.serversMu.Lock()
	}
	svr, svrExists := w.servers[id]
	var setServerConn bool
	switch rt {
	case "server":
		if svrExists {
			rw.WriteHeader(http.StatusConflict)
			fmt.Fprintf(rw, "Server already exists at ID %q", id)
			return
		}
		svr = &reflectedServer{
			clients: make(map[uuid.UUID]*reflectedClient),
		}
		setServerConn = true
		if w.servers == nil {
			w.servers = make(map[string]*reflectedServer)
		}
		w.servers[id] = svr
	case "client":
		if !svrExists {
			rw.WriteHeader(http.StatusNotFound)
			fmt.Fprintf(rw, "No server at ID %q", id)
			return
		}
	default:
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(rw, "Remote type not server or client")
		return
	}

	conn, err := upgrader.Upgrade(rw, r, nil)
	if err != nil {
		rw.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(rw, "Error upgrading websocket: %q", err)
		return
	}

	if setServerConn {
		svr.conn = conn
	}

	stopHb := make(chan struct{})
	go func() {
		ticker := time.NewTicker(w.PingInterval)
		for {
			select {
			case <-ticker.C:
				err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(w.PingInterval))
				if err != nil {
					if rt == "server" {
						for _, c := range svr.clients {
							c.conn.Close()
						}
						conn.Close()
					}
				}
			case <-stopHb:
				ticker.Stop()
				return
			}
		}
	}()

	switch rt {
	case "server":
		err = w.serveServer(svr, conn)
		for _, c := range svr.clients {
			c.conn.Close()
		}
		delete(w.servers, id)
		if err != nil {
			log.Printf("WSTunReflector: error returned from handling server\n%+v\n\n", err)
		}
	case "client":
		err = w.serveClient(svr, conn)
		if err != nil {
			if websocket.IsUnexpectedCloseError(errors.Cause(err), websocket.CloseNormalClosure,
				websocket.CloseGoingAway) {
				log.Printf("WSTunReflector: error returned from handling client\n%+v\n\n", err)
			}
		}
	}
	stopHb <- struct{}{}
	conn.Close()
}

func (w *WSTunReflector) serveServer(svr *reflectedServer, conn *websocket.Conn) error {
	// run a read loop, unpack message, find client, send
	for {
		msg, err := receiveMessage(conn)
		if err != nil {
			return err
		}
		switch msg.Action {
		case actionData:
			cl, ok := svr.clients[msg.Token]
			if !ok {
				// TODO - notify the server that the connection is closed
			} else {
				err := cl.conn.WriteMessage(websocket.BinaryMessage, msg.Payload)
				if err != nil {
					cl.conn.Close()
					svr.closeClient(msg.Token)
				}
			}
		case actionClose:
			// cl, ok := svr.clients[msg.Token]
			// if ok {
			// 	cl.conn.Close()
			// }
			err := svr.closeClient(msg.Token)
			if err != nil {
				return err
			}
		}
	}
}

func (w *WSTunReflector) serveClient(svr *reflectedServer, conn *websocket.Conn) error {
	// always gracefully close the connection
	defer conn.Close()

	// create an id for this client
	u := uuid.New()
	svr.clientsMu.Lock()
	svr.clients[u] = &reflectedClient{conn: conn}
	svr.clientsMu.Unlock()
	defer svr.closeClient(u)

	// Start a session
	svr.connWMu.Lock()
	err := sendMessage(svr.conn, &message{Token: u, Action: actionInit})
	svr.connWMu.Unlock()
	if err != nil {
		return err
	}

	// run a read loop, send to server
	for {
		_, b, err := conn.ReadMessage()
		if err != nil {
			return errors.Wrap(err, "Error reading message from client connection")
		}

		svr.connWMu.Lock()
		err = sendMessage(svr.conn, &message{Token: u, Action: actionData, Payload: b})
		svr.connWMu.Unlock()
		if err != nil {
			return errors.Wrap(err, "Error writing message to server connection")
		}
	}
}

// wsTunServer is a net.Listener that will dial-out to the reflector, and then
// act as a listener
type wsTunServer struct {
	// PingInterval is how often to send a ping packet over the connection. This
	// can be used both to keep it alive, and verify a client is listening. If
	// this is zero value the interval DefaultPingInterval will be used.
	PingInterval time.Duration

	wsconn    *websocket.Conn
	wsconnWMu sync.Mutex
	// addr is our address that clients should use. Should be <url>/client/<sessid>
	addr       string
	acceptC    chan *tunConn
	acceptErrC chan error

	close chan struct{}

	conns   map[uuid.UUID]*tunConn
	connsMu sync.Mutex
}

func (w *wsTunServer) writeMessage(m *message) error {
	w.wsconnWMu.Lock()
	defer w.wsconnWMu.Unlock()
	return sendMessage(w.wsconn, m)
}

// NewTunListener will create a net.Listener that exports its connections at
// Addr() via the remote reflector at addr. Addr should be a websocket URL to
// the remote hosts base (i.e wss:// or ws://). There should be no path
// component as the service should be exported at the root. If it exists, it
// will be overwritten
func NewTunListener(addr string) (net.Listener, error) {
	// generate a random string to be our unique identifier
	u, err := url.Parse(addr)
	if err != nil {
		return nil, err
	}
	var hdr = http.Header{}
	uu := u.User
	if u.User != nil {
		ui := base64.StdEncoding.EncodeToString([]byte(u.User.String()))
		hdr["Authorization"] = []string{"Basic " + ui}
	}
	u.User = nil
	// append path with new unique id
	id, err := uuid.NewUUID()
	if err != nil {
		return nil, err
	}
	u.Path = "/server/" + id.String()
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), hdr)
	if err != nil {
		return nil, err
	}

	// re-add user info, needed to strip it for websocket
	u.User = uu
	u.Path = "/client/" + id.String()

	ts := &wsTunServer{
		// TODO - expose?
		PingInterval: DefaultPingInterval,
		wsconn:       conn,
		addr:         u.String(),
		conns:        make(map[uuid.UUID]*tunConn),
		acceptC:      make(chan *tunConn),
		acceptErrC:   make(chan error),
		close:        make(chan struct{}),
	}
	go ts.pollConn()
	return ts, nil
}

func (w *wsTunServer) pollConn() {
	// loop the connection.
	for {
		select {
		case <-w.close:
			return
		default:
			err := w.connRead()
			if err != nil {
				w.acceptErrC <- err
				return
			}
		}
	}
}

func (w *wsTunServer) connRead() error {
	msg, err := receiveMessage(w.wsconn)
	if err != nil {
		return err
	}

	w.connsMu.Lock()
	defer w.connsMu.Unlock()

	switch msg.Action {
	case actionInit:
		conn := &tunConn{
			clientID:  msg.Token,
			svr:       w,
			recvBytes: make(chan []byte, 1024),
			closeC:    make(chan struct{}),
		}
		w.conns[msg.Token] = conn
		w.acceptC <- conn
	case actionClose:
		conn, ok := w.conns[msg.Token]
		if ok {
			conn.err = ErrClosed
			conn.closeC <- struct{}{}
			delete(w.conns, msg.Token)
		}
	case actionData:
		conn, ok := w.conns[msg.Token]
		if ok {
			conn.recvBytes <- msg.Payload
		} else {
			// Notify the server that conn is closed
			w.wsconnWMu.Lock()
			sendMessage(w.wsconn, &message{Token: msg.Token, Action: actionClose})
			w.wsconnWMu.Unlock()
			if err != nil {
				return err
			}
		}
	default:
		return errors.New("Internal error: Received unknown message type from server")
	}
	return nil
}

func (w *wsTunServer) Accept() (net.Conn, error) {
	select {
	case c := <-w.acceptC:
		return c, nil
	case err := <-w.acceptErrC:
		return nil, err
	}
}

func (w *wsTunServer) Close() error {
	//
	return nil
}

func (w *wsTunServer) Addr() net.Addr {
	return &wsaddr{
		network: "wstun",
		addr:    w.addr,
	}
}

// tunConn is a net.Conn implementation for a tunneled connection.
// It basically watches some channels for read and writes directly.
type tunConn struct {
	// clientID is used to uniquely identify this client, is passed back and
	// forth for multiplexing
	clientID uuid.UUID
	// Pass in the whole server instance to use it's conn and lock
	svr        *wsTunServer
	recvBytes  chan []byte
	recvBuffer []byte
	// stick something to return on next read/write
	err    error
	closeC chan struct{}
}

var (
	ErrClosed = errors.New("connection closed")
)

func (t *tunConn) Read(b []byte) (n int, err error) {
	if len(t.recvBuffer) > 0 {
		copied := copy(b, t.recvBuffer)
		t.recvBuffer = t.recvBuffer[copied:]
		return copied, nil
	}

	select {
	case msg := <-t.recvBytes:
		copied := copy(b, msg)
		if copied < len(msg) {
			t.recvBuffer = msg[copied:]
		}
		return copied, nil
	case <-t.closeC:
		return 0, t.err
	}
}

func (t *tunConn) Write(b []byte) (n int, err error) {
	if t.err != nil {
		return 0, t.err
	}

	err = t.svr.writeMessage(&message{Token: t.clientID, Action: actionData, Payload: b})
	if err != nil {
		return 0, err
	}

	return len(b), nil
}

func (t *tunConn) Close() error {
	err := t.svr.writeMessage(&message{Token: t.clientID, Action: actionClose})
	if err != nil {
		return err
	}

	delete(t.svr.conns, t.clientID)

	t.err = ErrClosed

	return nil
}

// TODO - all of these things

func (t *tunConn) LocalAddr() net.Addr {
	return nil
}

func (t *tunConn) RemoteAddr() net.Addr {
	return nil
}

func (t *tunConn) SetDeadline(tm time.Time) error {
	return nil
}
func (t *tunConn) SetReadDeadline(tm time.Time) error {
	return nil
}

func (t *tunConn) SetWriteDeadline(tm time.Time) error {
	return nil
}

func b2uuid(u []byte) uuid.UUID {
	r := [16]byte{}
	copy(r[:], u)
	return uuid.UUID(r)
}

func proto2conn(msg proto.Message, conn *websocket.Conn) error {
	d, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	return conn.WriteMessage(websocket.BinaryMessage, d)
}
