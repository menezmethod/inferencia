.PHONY: build run test lint clean fmt vet openapi smoke-prod

BINARY := inferencia
PKG    := ./...

openapi:
	cp docs/openapi.yaml internal/openapi/spec.yaml

build: openapi
	go build -o $(BINARY) ./cmd/inferencia

run: build
	./$(BINARY) -config config.yaml

test:
	go test -race -count=1 $(PKG)

test-v:
	go test -race -count=1 -v $(PKG)

lint: vet
	@which golangci-lint > /dev/null 2>&1 || echo "Install golangci-lint: https://golangci-lint.run/usage/install/"
	golangci-lint run $(PKG)

vet:
	go vet $(PKG)

fmt:
	gofmt -s -w .

clean:
	rm -f $(BINARY)
	go clean

# Smoke test your deployment (required: INFERENCIA_SMOKE_BASE_URL; optional: INFERENCIA_E2E_API_KEY for /v1/models)
smoke-prod:
	@./scripts/smoke-prod.sh
