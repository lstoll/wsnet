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

	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lstoll/wsnet/wsnetpb"
	"github.com/pkg/errors"
)

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

	ErrChan chan error

	servers map[string]*reflectedServer
}

type reflectedServer struct {
	conn      *websocket.Conn
	connWMu   sync.Mutex
	clients   map[uuid.UUID]*reflectedClient
	clientsMu sync.Mutex
}

type reflectedClient struct {
	conn *websocket.Conn
}

// closeClient notifies the server that a client is gone, and removes it from the list
func (r *reflectedServer) closeClient(u uuid.UUID) error {
	r.connWMu.Lock()
	defer r.connWMu.Unlock()
	delete(r.clients, u)
	req := &wsnetpb.Request{
		Token: u[:],
		Msg:   &wsnetpb.Request_ConnectionClose{ConnectionClose: &wsnetpb.ConnectionClose{}},
	}
	return proto2conn(req, r.conn)
}

func (w *WSTunReflector) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	pc := strings.Split(strings.Trim(r.URL.Path, "/ "), "/")
	rt, id := pc[0], pc[1]
	if len(pc) != 2 || rt == "" || id == "" {
		rw.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(rw, "Request path needs to be in form of /[server|client]/<identifier>, got %q", r.URL.Path)
		return
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
			log.Printf("client no server")
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
		w.ErrChan <- err
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
				if err != nil && w.ErrChan != nil {
					if svr != nil {
						for _, c := range svr.clients {
							c.conn.Close()
						}
						conn.Close()
					}
					w.ErrChan <- err
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
	case "client":
		err = w.serveClient(svr, conn)
	}
	// TODO - error handling here?
	if err != nil && w.ErrChan != nil {
		w.ErrChan <- err
	}
	stopHb <- struct{}{}
	conn.Close()
}

func (w *WSTunReflector) serveServer(svr *reflectedServer, conn *websocket.Conn) error {
	// run a read loop, unpack message, find client, send
	for {
		_, b, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		rsp := &wsnetpb.Response{}
		err = proto.Unmarshal(b, rsp)
		fmt.Printf("%s", string(b))
		if err != nil {
			return errors.Wrap(err, "Error unmarshaling response from remote listener")
		}
		u := b2uuid(rsp.Token)
		switch msg := rsp.Msg.(type) {
		case *wsnetpb.Response_ConnectionData:
			cl, ok := svr.clients[u]
			if !ok {
				// TODO - notify the server that the connection is closed
			} else {
				err := cl.conn.WriteMessage(websocket.BinaryMessage, msg.ConnectionData)
				if err != nil {
					cl.conn.Close()
					svr.closeClient(u)
				}
			}
		case *wsnetpb.Response_ConnectionClose:
			err := svr.closeClient(u)
			if err != nil {
				return err
			}
		}
	}
}

func (w *WSTunReflector) serveClient(svr *reflectedServer, conn *websocket.Conn) error {
	// create an id for this client
	u := uuid.New()
	svr.clientsMu.Lock()
	svr.clients[u] = &reflectedClient{conn: conn}
	svr.clientsMu.Unlock()
	defer svr.closeClient(u)

	// Start a session
	req := &wsnetpb.Request{
		Token: u[:],
		Msg: &wsnetpb.Request_ConnectionInit{
			ConnectionInit: &wsnetpb.ConnectionInit{},
		},
	}
	d, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	svr.connWMu.Lock()
	log.Printf("WSTunReflector: writing to list server: %#v", req.Msg)

	err = svr.conn.WriteMessage(websocket.BinaryMessage, d)
	svr.connWMu.Unlock()

	// run a read loop, send to server
	for {
		log.Print("reflect: read from client")
		_, b, err := conn.ReadMessage()
		log.Print("reflect: data has been read from client")

		if err != nil {
			return err
		}
		log.Print("reflect: creating request method")

		req := &wsnetpb.Request{
			Token: u[:],
			Msg: &wsnetpb.Request_ConnectionData{
				ConnectionData: b,
			},
		}
		d, err := proto.Marshal(req)
		if err != nil {
			return err
		}
		log.Print("reflect: acquiring server conn lock")

		log.Print("reflect: writing message to listen host")

		svr.connWMu.Lock()
		err = svr.conn.WriteMessage(websocket.BinaryMessage, d)
		svr.connWMu.Unlock()
		if err != nil {
			return err
		}
		log.Print("reflect: data written")
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

func (w *wsTunServer) writeMessage(rsp *wsnetpb.Response) error {
	w.wsconnWMu.Lock()
	defer w.wsconnWMu.Unlock()

	d, err := proto.Marshal(rsp)
	if err != nil {
		return errors.Wrap(err, "Error marshaling proto response")
	}

	err = w.wsconn.WriteMessage(websocket.BinaryMessage, d)
	if err != nil {
		return errors.Wrap(err, "Error writing to websocket")
	}
	return nil
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
	log.Print("wsTunServer: connRead start")
	_, b, err := w.wsconn.ReadMessage()

	req := &wsnetpb.Request{}
	err = proto.Unmarshal(b, req)

	if err != nil {
		return err
	}

	log.Printf("wsTunServer: received data token %#v msg %#v", req.Token, req.Msg)

	log.Print("wsTunServer: connRead data read")

	//log.Print("wsTunServer: acquire conns mu")
	//w.connsMu.Lock()
	//defer w.connsMu.Unlock()

	u := b2uuid(req.Token)
	switch msg := req.Msg.(type) {
	case *wsnetpb.Request_ConnectionInit:
		log.Print("wsTunServer: connection init")
		conn := &tunConn{
			clientID: u,
			svr:      w,
		}
		w.conns[u] = conn
		w.acceptC <- conn
	case *wsnetpb.Request_ConnectionClose:
		conn, ok := w.conns[u]
		if ok {
			conn.err = ErrClosed
			delete(w.conns, u)
		}
	case *wsnetpb.Request_ConnectionData:
		conn, ok := w.conns[u]
		if ok {
			conn.recvMu.Lock()
			conn.recvBuffer = append(conn.recvBuffer, msg.ConnectionData...)
			conn.recvMu.Unlock()
		} else {
			// Notify the server that conn is closed
			msg := &wsnetpb.Response{
				Token: req.Token,
				Msg:   &wsnetpb.Response_ConnectionClose{},
			}

			d, err := proto.Marshal(msg)
			if err != nil {
				return err
			}

			err = w.wsconn.WriteMessage(websocket.BinaryMessage, d)
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
	recvBuffer []byte
	recvMu     sync.Mutex
	// stick something to return on next read/write
	err error
}

var (
	ErrClosed = errors.New("connection closed")
)

func (t *tunConn) Read(b []byte) (n int, err error) {
	log.Print("tunConn: read called")
	if t.err != nil {
		return 0, t.err
	}

	t.recvMu.Lock()
	defer t.recvMu.Unlock()
	if t.err != nil {
		return 0, err
	}
	if len(t.recvBuffer) == 0 {
		return 0, nil
	}
	copied := copy(b, t.recvBuffer)
	t.recvBuffer = t.recvBuffer[copied:]
	return copied, nil
}

func (t *tunConn) Write(b []byte) (n int, err error) {
	log.Print("tunConn: write called")
	if t.err != nil {
		return 0, t.err
	}

	msg := &wsnetpb.Response{
		Token: t.clientID[:],
		Msg: &wsnetpb.Response_ConnectionData{
			ConnectionData: b,
		},
	}

	err = t.svr.writeMessage(msg)

	return len(b), nil
}

func (t *tunConn) Close() error {
	msg := &wsnetpb.Response{
		Token: t.clientID[:],
		Msg: &wsnetpb.Response_ConnectionClose{
			ConnectionClose: &wsnetpb.ConnectionClose{},
		},
	}

	err := t.svr.writeMessage(msg)
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
