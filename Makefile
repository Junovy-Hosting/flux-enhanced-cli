.PHONY: build install test clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME)"

build:
	go build $(LDFLAGS) -o flux-enhanced-cli .

build-dev:
	go build -o flux-enhanced-cli .

install: build
	cp flux-enhanced-cli ~/.local/bin/ || cp flux-enhanced-cli /usr/local/bin/

test:
	go test ./...

clean:
	rm -f flux-enhanced-cli

version:
	@echo "Version: $(VERSION)"
	@echo "Build Time: $(BUILD_TIME)"
