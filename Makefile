GO ?= go

.PHONY: lint test test-integration build clean

lint:
	golangci-lint run ./...

test:
	$(GO) test -race -short ./...

test-integration:
	$(GO) test -race ./...

build:
	$(GO) build -o bin/api ./cmd/api
	$(GO) build -o bin/worker ./cmd/worker

clean:
	rm -rf bin
