.PHONY = all build test

all: build test

build:
	go build -i ./...

test:
	go test -count 5 -race -v $(go list ./... | grep -v /vendor/)
