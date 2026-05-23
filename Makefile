.PHONY: build test run lint clean
.PHONY: db-up db-down db-reset
.PHONY: docker-build docker-up docker-down docker-reset bootstrap
.PHONY: web-build web-dev web-clean build-prod test-integration

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
VERSION_PKG = github.com/jholhewres/anchored_oss/internal/version
LDFLAGS = -X $(VERSION_PKG).Version=$(VERSION)
BINARY = bin/anchored-oss

# Default build: uses whatever SPA artifacts are currently in
# internal/web/dist/. The committed stub keeps `go build` working without
# Node.js. For a production bundle run `make build-prod` or build the
# Docker image (which always rebuilds the frontend).
build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/server

build-prod: web-build build

test:
	go test -count=1 -race ./...

# test-integration runs Postgres-backed store tests guarded by the
# `integration` build tag. Bring up the database first with `make db-up`.
test-integration:
	go test -count=1 -race -tags integration ./internal/store/...

run: build
	./$(BINARY)

lint:
	go vet ./...

clean:
	rm -rf bin/

web-build:
	cd web && npm install --no-audit --no-fund && npm run build

web-dev:
	cd web && npm run dev

web-clean:
	rm -rf web/node_modules internal/web/dist/assets
	find internal/web/dist -mindepth 1 -name 'index.html' -prune -o -delete

db-up:
	docker compose up -d postgres
	@echo "Waiting for postgres..."
	@sleep 2

db-down:
	docker compose down

db-reset:
	docker compose down -v

docker-build:
	docker compose build

docker-up:
	docker compose up -d
	@echo "Server running at http://localhost:8080"

docker-down:
	docker compose down

docker-reset:
	docker compose down -v

bootstrap: build
	./$(BINARY) -bootstrap
