.PHONY = all build test

all: build test

build: wsnetpb/tun.pb.go
	go build -i ./...

test:
	go test -v $(go list ./... | grep -v /vendor/)

wsnetpb/tun.pb.go: $(GOPATH)/bin/protoc-gen-go wsnetpb/tun.proto
	PATH=$(GOPATH)/bin:$(PATH) protoc --go_out=. **/*.proto

$(GOPATH)/bin/protoc-gen-go:
	go get -u github.com/golang/protobuf/protoc-gen-go