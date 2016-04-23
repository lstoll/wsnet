package wsnet

import (
	"bufio"
	"fmt"
	"testing"
	"time"
)

const listenPort = "19546"

func TestEndToEnd(t *testing.T) {
	// Start a server
	server, err := ListenWithKeepalive("127.0.0.1:"+listenPort, 1*time.Millisecond)
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
				if err != nil { // EOF, or worse
					break
				}
				client.Write(line)
			}
		}
	}()

	for i := 0; i < 5; i++ {
		conn, err := Dial("ws://127.0.0.1:"+listenPort, 2*time.Second)
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
