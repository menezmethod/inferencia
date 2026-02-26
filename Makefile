.PHONY: build run test test-v test-coverage integration lint clean fmt vet openapi smoke-prod check-sensitive

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

# Coverage: run tests with -coverprofile, then print summary. Unit tests use Ginkgo/Gomega.
test-coverage:
	go test -count=1 -coverprofile=coverage.out $(PKG)
	go tool cover -func=coverage.out

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

# Integration tests: build app, start it, run Ginkgo integration suite and (if Node present) Newman. Must pass in CI.
integration:
	@./scripts/run-integration-and-newman.sh

# Smoke test your deployment (required: INFERENCIA_SMOKE_BASE_URL; optional: INFERENCIA_E2E_API_KEY for /v1/models)
smoke-prod:
	@./scripts/smoke-prod.sh

# Blocklist check: set SENSITIVE_BLOCKLIST (newline-separated patterns) to enable; when unset, passes. CI runs this + gitleaks.
check-sensitive:
	@./scripts/check-sensitive-data.sh
