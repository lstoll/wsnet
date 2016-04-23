package main

import (
	"bufio"
	"log"
	"time"

	"github.com/lstoll/wsnet"
)

const listenPort = "8080"

func main() {
	// Start a server
	server, err := wsnet.ListenWithKeepalive("127.0.0.1:"+listenPort, 1*time.Millisecond)
	//defer server.Close()
	if server == nil {
		log.Fatal("couldn't start listening: ", err)
	}
	for {
		client, err := server.Accept()
		if client == nil {
			log.Fatal("couldn't accept: ", err)
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
}
