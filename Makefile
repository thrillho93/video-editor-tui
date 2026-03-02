BINARY = vid
INSTALL_DIR = /home/frazier/bin
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build test install clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) .

test:
	go test ./...

install: build
	cp $(BINARY) $(INSTALL_DIR)/$(BINARY)

clean:
	rm -f $(BINARY)
