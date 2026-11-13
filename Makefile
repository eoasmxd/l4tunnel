export PATH := $(GOPATH)/bin:$(PATH)

all: fmt build

build: l4ts l4tc

fmt:
	go fmt ./...

l4ts:
	go build -o bin/l4ts ./cmd/l4ts

l4tc:
	go build -o bin/l4tc ./cmd/l4tc

clean:
	rm -f ./bin/l4tc
	rm -f ./bin/l4ts
