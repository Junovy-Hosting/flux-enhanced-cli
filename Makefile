.PHONY: build install test clean

build:
	go build -o flux-reconcile .

install: build
	cp flux-reconcile ~/.local/bin/ || cp flux-reconcile /usr/local/bin/

test:
	go test ./...

clean:
	rm -f flux-reconcile

