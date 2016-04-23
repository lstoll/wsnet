# wsnet

[![Build Status](https://travis-ci.org/lstoll/wsnet.svg?branch=master)](https://travis-ci.org/lstoll/wsnet)

Dial/Listen, but tunneled over websockets. Designed to make grpc work on Heroku.

![A gif because why not](https://cdn.lstoll.net/screen/screencast_2016-04-22_19-35-31.gif)

## Usage

Use wherever you'd use `net.Listen` and `net.Dial`.

Theres also a `ListenWithKeepalive` variant - this can be provided with a duration. This will
then regularly send small frames down the wire, which can be used to prevent the heroku router
from terminating the connection

## Examples

grpc server:

```
lis, err := wsnet.Listen("127.0.0.1:8080")
if err != nil {
	log.Fatalf("failed to listen: %v", err)
}
s := grpc.NewServer()
pb.RegisterGreeterServer(s, &server{})
s.Serve(lis)
```

grpc client:

```
conn, err := grpc.Dial("ws://127.0.0.1:8080", grpc.WithInsecure(), grpc.WithDialer(wsnet.Dial))
if err != nil {
	log.Fatalf("did not connect: %v", err)
}
defer conn.Close()
c := pb.NewGreeterClient(conn)
r, err := c.SayHello(context.Background(), &pb.HelloRequest{Name: defaultName})
if err != nil {
	log.Fatalf("could not greet: %v", err)
}
log.Printf("Greeting: %s", r.Message)

```
