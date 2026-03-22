VERSION := $(shell git describe --tags 2>/dev/null || echo dev)
EXTENSIONS_REPO := $(HOME)/go/src/piglet-extensions

.PHONY: build extensions clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o piglet ./cmd/piglet/

extensions:
	$(MAKE) -C $(EXTENSIONS_REPO) extensions

clean:
	rm -f piglet
