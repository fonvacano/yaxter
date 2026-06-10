GO ?= go
OAPI_CODEGEN = go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.4.1
BUF = go run github.com/bufbuild/buf/cmd/buf@v1.47.2

.PHONY: lint test test-integration build clean lint-spec generate

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

lint-spec:
	npx -y @stoplight/spectral-cli lint --fail-severity=error api/openapi.yaml

generate:
	$(OAPI_CODEGEN) -config api/server.cfg.yaml api/openapi.yaml
	$(OAPI_CODEGEN) -config api/client.cfg.yaml api/openapi.yaml
	$(BUF) generate
