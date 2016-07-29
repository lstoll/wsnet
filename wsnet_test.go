package wsnet

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestEndToEnd(t *testing.T) {
	// Start a server
	for rn, tc := range [...]struct {
		listener func(addr string) (net.Listener, error)
		dialer   func(addr string) (net.Conn, error)
	}{
		{
			// Straight up listener/dialer
			listener: func(addr string) (net.Listener, error) { return ListenWithKeepalive(addr, 1*time.Millisecond) },
			dialer:   func(addr string) (net.Conn, error) { return Dial("ws://"+addr, 2*time.Second) },
		},
		{
			// Custom handler stuff
			listener: func(addr string) (net.Listener, error) {
				hl, err := net.Listen("tcp", addr)
				if err != nil {
					return nil, err
				}
				lis, h := HandlerWithKeepalive(2 * time.Second)
				serveMux := http.NewServeMux()
				serveMux.Handle("/wsnet", h)
				go func() {
					err = http.Serve(hl, serveMux)
					if err != nil {
						panic(err)
					}
				}()
				return lis, nil
			},
			dialer: func(addr string) (net.Conn, error) { return Dial("ws://"+addr+"/wsnet", 2*time.Second) },
		},
	} {
		// get someone to allocate us a port
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := lis.Addr().String()
		lis.Close()

		t.Logf("Starting case %d", rn)
		server, err := tc.listener(addr)
		defer server.Close()
		if server == nil {
			t.Fatal("couldn't start listening: ", err)
		}
		go func() {
			for {
				client, err := server.Accept()
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
				t.Fatal(err)
			}
			for x := 0; x < 10; x++ {
				fmt.Fprintf(conn, "PING\n")
				resp, err := bufio.NewReader(conn).ReadString('\n')
				if err != nil {
					t.Fatal(err)
				}
				if resp != "PING\n" {
					t.Fatalf("Expected %q, got %q", "PING\n", resp)
				}
				time.Sleep(5 * time.Millisecond)
			}
			conn.Close()
		}
	}
}
