.PHONY: build run test lint clean fmt vet openapi smoke-prod check-sensitive

BINARY := inferencia
PKG    := ./...
# Set at build time for releases: make build VERSION=1.0.0
VERSION ?= dev
# Optional: git rev-parse --short HEAD for commit in version info
VERSION_LDFLAGS := -ldflags "-s -w -X github.com/menezmethod/inferencia/internal/version.Version=$(VERSION)"

openapi:
	cp docs/openapi.yaml internal/openapi/spec.yaml

build: openapi
	go build $(VERSION_LDFLAGS) -o $(BINARY) ./cmd/inferencia

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

# Fail if blocklisted strings (e.g. real prod URLs) appear in repo. CI runs this + gitleaks.
check-sensitive:
	@./scripts/check-sensitive-data.sh
