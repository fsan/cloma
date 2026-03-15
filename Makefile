# Binary location
BIN_DIR := bin
CLOMA_BIN := $(BIN_DIR)/cloma

# Go module info
MODULE := github.com/fsan/cloma
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS := -ldflags "-X $(MODULE)/internal/cmd.Version=$(VERSION) -X $(MODULE)/internal/cmd.GitCommit=$(GIT_COMMIT) -X $(MODULE)/internal/cmd.BuildDate=$(BUILD_DATE)"

.DEFAULT_GOAL := help
.PHONY: help build install uninstall clean test run doctor shell stop cloma

help:
	@echo "cloma - Docker Sandbox Manager for Code Agents"
	@echo ""
	@echo "Usage: make <target>"
	@echo ""
	@echo "Building:"
	@echo "  build       Build the cloma binary (bin/cloma)"
	@echo "  install     Install cloma to /usr/local/bin"
	@echo "  uninstall   Remove cloma from /usr/local/bin"
	@echo "  clean       Remove build artifacts"
	@echo "  test        Run tests"
	@echo ""
	@echo "Running:"
	@echo "  run         Run cloma with custom ARGS"
	@echo "  doctor      Run health checks"
	@echo "  shell       Open an interactive shell in the sandbox"
	@echo "  stop        Stop the running sandbox"
	@echo ""
	@echo "Development:"
	@echo "  cloma       Run cloma with custom ARGS (e.g., make cloma ARGS='list')"

# Build targets
build:
	go build $(LDFLAGS) -o $(CLOMA_BIN) ./cmd/cloma

install: build
	@echo "Installing cloma to /usr/local/bin..."
	install -m 755 $(CLOMA_BIN) /usr/local/bin/cloma
	@echo "Installed: /usr/local/bin/cloma"

uninstall:
	@echo "Removing cloma from /usr/local/bin..."
	rm -f /usr/local/bin/cloma
	@echo "Removed: /usr/local/bin/cloma"

clean:
	rm -rf $(BIN_DIR)
	rm -f ./cloma

test:
	go test -v ./...

# Run targets (using Go CLI)
run: build
	./$(CLOMA_BIN) $(ARGS)

doctor: build
	./$(CLOMA_BIN) doctor

shell: build
	./$(CLOMA_BIN) shell

stop: build
	./$(CLOMA_BIN) stop

# Development helper
cloma: build
	./$(CLOMA_BIN) $(ARGS)