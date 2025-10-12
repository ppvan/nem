.PHONY: help build run test clean dev install fmt vet lint

BINARY_NAME=nem
CMD_PATH=./cmd/api
GO=go
GOFLAGS=-v

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*##"; printf "\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  %-15s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

build: ## Build the binary
	$(GO) build $(GOFLAGS) -o $(BINARY_NAME) $(CMD_PATH)

run:
	$(GO) run $(CMD_PATH)

test:
	$(GO) test -v ./...

test-cover:
	$(GO) test -v -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

bench:
	$(GO) test -bench=. -benchmem ./...

clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.out coverage.html
	$(GO) clean

install: ## Install dependencies
	$(GO) mod download
	$(GO) mod tidy

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

check: fmt vet

all: clean install check test build
