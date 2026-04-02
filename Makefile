MODULE   := github.com/yousysadmin/jc2aws
VERSION  ?= dev
LDFLAGS  := -s -w -X $(MODULE)/pkg.Version=$(VERSION)
GOFLAGS  := -trimpath
BIN_DIR  := bin

# Binaries
BIN  := $(BIN_DIR)/jc2aws

.PHONY: all build build run-cli run-tui test test-v vet fmt lint clean help

## all: Show help (default)
all: help

## build: Build jc2aws binaries
build:
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/jc2aws

## run: Run in interactive mode
run-tui: build
	$(BIN) -i

## run-cli: Run in CLI mode (pass ARGS="..." for extra flags)
run-cli: build
	$(BIN) $(ARGS)

## test: Run all tests
test:
	go test ./...

## test-v: Run all tests with verbose output
test-v:
	go test -v ./...

## vet: Run go vet
vet:
	go vet ./...

## fmt: Run gofmt on all Go files
fmt:
	gofmt -w .

## lint: Run vet and check formatting
lint: vet
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

## clean: Remove built binaries
clean:
	rm -rf $(BIN_DIR)

## help: Show this help message
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
