.PHONY: help dev build run test test-integration lint clean docker-up docker-down db-migrate db-rollback db-seed

BINARY := gateway
CMD    := ./cmd/gateway

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# ── Development ──────────────────────────────────────────────────────────────
dev: ## Run with hot-reload (needs air: go install github.com/air-verse/air@latest)
	air -c .air.toml

build: ## Build binary
	@echo "▶ Building $(BINARY)..."
	go build -o bin/$(BINARY) $(CMD)

run: build ## Build and run
	./bin/$(BINARY)

# ── Testing ──────────────────────────────────────────────────────────────────
test: ## Run unit tests
	go test -v -race -cover ./internal/...

test-integration: ## Run integration tests (needs running MySQL)
	go test -v -tags=integration ./tests/...

coverage: ## Generate HTML coverage report
	go test -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out -o coverage.html
	@echo "▶ Open coverage.html"

# ── Quality ──────────────────────────────────────────────────────────────────
lint: ## Run linter
	golangci-lint run --timeout 3m

fmt: ## Format code
	gofmt -s -w .
	goimports -w .

# ── Docker ───────────────────────────────────────────────────────────────────
docker-up: ## Start MySQL + gateway in Docker
	docker compose up -d

docker-down: ## Stop Docker containers
	docker compose down

docker-logs: ## Follow gateway logs
	docker compose logs -f gateway

# ── Database ─────────────────────────────────────────────────────────────────
db-migrate: ## Apply database migrations
	migrate -path ./migrations -database "mysql://$$DB_USER:$$DB_PASSWORD@tcp($$DB_HOST:$$DB_PORT)/$$DB_NAME" up

db-rollback: ## Rollback last migration
	migrate -path ./migrations -database "mysql://$$DB_USER:$$DB_PASSWORD@tcp($$DB_HOST:$$DB_PORT)/$$DB_NAME" down 1

db-seed: ## Seed demo data (run after docker-up)
	docker compose exec -T mysql mysql -u $$DB_USER -p$$DB_PASSWORD $$DB_NAME < scripts/seed.sql

# ── Cleanup ──────────────────────────────────────────────────────────────────
clean: ## Remove build artifacts
	rm -rf bin/ tmp/ coverage.out coverage.html
