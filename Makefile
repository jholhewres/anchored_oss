.PHONY: build test run lint clean
.PHONY: db-up db-down db-reset
.PHONY: docker-build docker-up docker-down docker-reset bootstrap
.PHONY: web-build web-dev web-clean build-prod test-integration
.PHONY: sync-install verify-install

# Canonical server installer. The embedded copy (served at /install-oss) must
# stay byte-identical so the dashboard and curl|sh paths never drift.
INSTALL_CANONICAL = install/install.sh
INSTALL_EMBEDDED  = internal/web/install/anchored-oss.sh

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

# test-integration runs Postgres-backed tests guarded by the `integration`
# build tag. Requires a pgvector-enabled Postgres; point ANCHORED_TEST_DSN at
# it (tests skip when unset), e.g.:
#   docker run -d --name pgvec -e POSTGRES_PASSWORD=anchored -e POSTGRES_USER=anchored \
#     -e POSTGRES_DB=anchored_oss -p 55433:5432 pgvector/pgvector:pg16
#   ANCHORED_TEST_DSN=postgres://anchored:anchored@localhost:55433/anchored_oss?sslmode=disable make test-integration
test-integration:
	go test -count=1 -tags integration ./internal/store/... ./internal/curation/...

run: build
	./$(BINARY)

lint:
	go vet ./...

# sync-install copies the canonical installer to the embedded location so a
# single edit propagates. Run after editing install/install.sh.
sync-install:
	cp $(INSTALL_CANONICAL) $(INSTALL_EMBEDDED)
	@echo "synced $(INSTALL_CANONICAL) -> $(INSTALL_EMBEDDED)"

# verify-install fails if the embedded installer has drifted from canonical.
# Wire into CI to catch out-of-sync copies.
verify-install:
	@diff -u $(INSTALL_CANONICAL) $(INSTALL_EMBEDDED) \
		&& echo "install scripts in sync" \
		|| (echo "ERROR: $(INSTALL_EMBEDDED) drifted from $(INSTALL_CANONICAL); run 'make sync-install'" && exit 1)

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
