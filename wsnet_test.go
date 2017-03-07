package wsnet

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"

	"github.com/lstoll/grpce/helloproto"
)

var cases = []struct {
	name     string
	listener func() (addr string, lis net.Listener, err error)
	dialer   func(addr string) (net.Conn, error)
}{
	{
		name: "Custom handler",
		listener: func() (string, net.Listener, error) {
			hl, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return "", nil, err
			}
			sh := &WSServer{PingInterval: 1 * time.Millisecond}
			serveMux := http.NewServeMux()
			serveMux.Handle("/wsnet", sh)
			go func() {
				err = http.Serve(hl, serveMux)
				if err != nil {
					panic(err)
				}
			}()
			return hl.Addr().String(), sh, nil
		},
		dialer: func(addr string) (net.Conn, error) { return Dial("ws://"+addr+"/wsnet", 2*time.Second) },
	},
	{
		name: "Handler + http auth",
		listener: func() (string, net.Listener, error) {
			hl, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return "", nil, err
			}
			sh := &WSServer{PingInterval: 1 * time.Millisecond}
			auth := func(fn http.Handler) http.HandlerFunc {
				return func(w http.ResponseWriter, r *http.Request) {
					user, pass, _ := r.BasicAuth()
					if user == "testu" && pass == "password" {
						fn.ServeHTTP(w, r)
						return
					}
					w.Header().Set("WWW-Authenticate", "Basic")
					http.Error(w, "Unauthorized.", 401)
				}
			}
			serveMux := http.NewServeMux()
			serveMux.Handle("/wsnet", auth(sh))
			go func() {
				err = http.Serve(hl, serveMux)
				if err != nil {
					panic(err)
				}
			}()
			return hl.Addr().String(), sh, nil
		},
		dialer: func(addr string) (net.Conn, error) { return Dial("ws://testu:password@"+addr+"/wsnet", 2*time.Second) },
	},
	{
		name: "Reflected server",
		listener: func() (string, net.Listener, error) {
			// Start the reflector
			hl, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				return "", nil, err
			}
			rh := &WSTunReflector{PingInterval: 1 * time.Millisecond}
			serveMux := http.NewServeMux()
			serveMux.Handle("/", rh)
			go func() {
				err = http.Serve(hl, serveMux)
				if err != nil {
					panic(err)
				}
			}()
			// Create a listener pointing to the reflector
			lis, err := NewTunListener("ws://" + hl.Addr().String())
			if err != nil {
				return "", nil, err
			}
			return lis.Addr().String(), lis, nil
		},
		dialer: func(addr string) (net.Conn, error) { return Dial(addr, 2*time.Second) },
	},
}

func TestE2ESimple(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, server, err := tc.listener()
			defer server.Close()
			if server == nil {
				t.Fatal("couldn't start listening: ", err)
			}
			go func() {
				for {
					client, err := server.Accept()
					if err != nil {
						t.Fatalf(err.Error())
					}
					if client == nil {
						t.Fatal("couldn't accept: ", err)
					}
					b := bufio.NewReader(client)

					for {
						line, err := b.ReadBytes('\n')
						if err != nil {
							break
						}
						client.Write(line)
					}
				}
			}()

			for i := 0; i < 5; i++ {
				conn, err := tc.dialer(addr)
				if err != nil {
					t.Fatalf("Error dialing addr %q [%v]", addr, err)
				}
				for x := 0; x < 10; x++ {
					_, err := fmt.Fprintf(conn, "PING\n")
					if err != nil {
						t.Fatalf("Error writing to connection [%v]", err)
					}
					resp, err := bufio.NewReader(conn).ReadString('\n')
					if err != nil {
						t.Fatalf("Error reading from connection [%v]", err)
					}
					if resp != "PING\n" {
						t.Fatalf("Expected %q, got %q", "PING\n", resp)
					}
					time.Sleep(5 * time.Millisecond)
				}
				conn.Close()
			}
		})
	}
}

type hs struct{}

func (h *hs) HelloWorld(ctx context.Context, req *helloproto.HelloRequest) (*helloproto.HelloResponse, error) {
	return &helloproto.HelloResponse{
		Message:    fmt.Sprintf("Hello, %s!", req.Name),
		ServerName: "some server",
	}, nil
}

func TestE2Egrpc(t *testing.T) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr, lis, err := tc.listener()
			if err != nil {
				t.Fatalf("Error getting first listener [%v]", err)
			}
			s := grpc.NewServer()
			helloproto.RegisterHelloServer(s, &hs{})
			t.Log("Starting server")
			go s.Serve(lis)
			for x := 0; x < 10; x++ {
				t.Log("Starting client")
				conn, err := grpc.Dial(addr,
					grpc.WithDialer(func(addr string, timeout time.Duration) (net.Conn, error) {
						return tc.dialer(addr)
					}),
					grpc.WithInsecure(),
				)
				if err != nil {
					t.Fatalf("Error connecting to server: %v", err)
				}
				c := helloproto.NewHelloClient(conn)
				for i := 0; i < 1000; i++ {
					_, err := c.HelloWorld(context.Background(), &helloproto.HelloRequest{Name: "Handshakin"})
					if err != nil {
						t.Fatalf("Error calling RPC: %v", err)
					}
				}
				conn.Close()
			}
			s.Stop()
			lis.Close()
		})
	}
}
