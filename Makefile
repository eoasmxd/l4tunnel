BIN_DIR := bin
LDFLAGS := -s -w

.PHONY: all build fmt release clean

all: fmt build

build:
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/l4ts ./cmd/l4ts
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/l4tc ./cmd/l4tc

fmt:
	go fmt ./...

release:
	bash ./package.sh

clean:
	rm -rf $(BIN_DIR) packages
