VERSION := $(shell git describe --tags 2>/dev/null || echo dev)

.PHONY: build extensions clean

build:
	go build -ldflags "-X main.version=$(VERSION)" -o piglet ./cmd/piglet/

extensions:
	@for p in core agent context code workflow cron eval; do \
		echo "  pack-$$p"; \
		go build -o ~/.config/piglet/extensions/pack-$$p/pack-$$p ./extensions/packs/$$p/; \
	done
	@echo "Packs installed"

clean:
	rm -f piglet
