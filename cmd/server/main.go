package main

import (
	"log"

	"golang.org/x/net/context"

	"github.com/lstoll/wsnet"
	pb "github.com/lstoll/wsnet/helloworld"

	"google.golang.org/grpc"
)

type server struct{}

// SayHello implements helloworld.GreeterServer
func (s *server) SayHello(ctx context.Context, in *pb.HelloRequest) (*pb.HelloReply, error) {
	return &pb.HelloReply{Message: "Hello " + in.Name}, nil
}

func main() {
	log.Println("Starting....")
	lis, err := wsnet.Listen("127.0.0.1:8080")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	log.Println("Listening....")
	s := grpc.NewServer()
	pb.RegisterGreeterServer(s, &server{})
	s.Serve(lis)
}
