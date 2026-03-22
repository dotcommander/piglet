EXTENSIONS_DIR := $(HOME)/.config/piglet/extensions

EXTENSION_NAMES := safeguard rtk autotitle clipboard skill memory subagent lsp

.PHONY: build extensions install-extensions clean $(addprefix extensions-,$(EXTENSION_NAMES))

VERSION := $(shell git describe --tags 2>/dev/null || echo dev)

build:
	go build -ldflags "-X main.version=$(VERSION)" -o piglet ./cmd/piglet/

extensions: $(addprefix extensions-,$(EXTENSION_NAMES))

define EXT_RULE
extensions-$(1):
	@mkdir -p $(EXTENSIONS_DIR)/$(1)
	go build -o $(EXTENSIONS_DIR)/$(1)/$(1) ./$(1)/cmd/
	cp $(1)/cmd/manifest.yaml $(EXTENSIONS_DIR)/$(1)/
endef

$(foreach ext,$(EXTENSION_NAMES),$(eval $(call EXT_RULE,$(ext))))

install-extensions: extensions
	@echo "Extensions installed to $(EXTENSIONS_DIR)"

clean:
	rm -f piglet
