package main

import (
	"log"

	"golang.org/x/net/context"

	"github.com/lstoll/wsnet"
	pb "github.com/lstoll/wsnet/helloworld"
	"google.golang.org/grpc"
)

const (
	address     = "ws://127.0.0.1:8080"
	defaultName = "world"
)

func main() {
	// Set up a connection to the server.
	log.Println("Dialing....")
	conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithDialer(wsnet.Dial))
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	log.Println("Calling....")
	defer conn.Close()
	c := pb.NewGreeterClient(conn)
	r, err := c.SayHello(context.Background(), &pb.HelloRequest{Name: defaultName})
	if err != nil {
		log.Fatalf("could not greet: %v", err)
	}
	log.Printf("Greeting: %s", r.Message)
}
