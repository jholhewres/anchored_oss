.PHONY: build test run lint clean

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS = -X main.versionFlag=$(VERSION)
BINARY = bin/anchored-oss

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/server

test:
	go test -count=1 -race ./...

run: build
	./$(BINARY)

lint:
	go vet ./...

clean:
	rm -rf bin/

.PHONY: db-up db-down db-reset

db-up:
	docker compose up -d postgres
	@echo "Waiting for postgres..."
	@sleep 2

db-down:
	docker compose down

db-reset:
	docker compose down -v

.PHONY: docker-build docker-up docker-down docker-reset bootstrap

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
