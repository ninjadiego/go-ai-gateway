# Go AI Gateway

> Production-ready API gateway for LLM providers (Anthropic, OpenAI), written in Go.
> Multi-tenant API keys, rate limiting, cost tracking, prompt caching, and usage analytics.

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://go.dev/)
[![MySQL](https://img.shields.io/badge/MySQL-8.0-4479A1?logo=mysql&logoColor=white)](https://www.mysql.com/)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

---

## Why this exists

Every company integrating LLMs (Claude, GPT-4) runs into the same problems:

- **Cost surprises.** A buggy prompt loop burns $500 overnight before anyone notices.
- **No per-team limits.** Marketing, Support, and Engineering all share one API key — impossible to track who spent what.
- **Rate-limit whack-a-mole.** When you hit Anthropic's limits, all your services fail at once.
- **No caching.** You pay full price for the same system prompt 10,000 times a day when Anthropic's prompt caching could cut 90% of that.

**Go AI Gateway** sits between your applications and the LLM provider. It gives you per-tenant API keys, enforces budgets, tracks every dollar, and uses Anthropic's prompt caching automatically.

---

## Architecture

```
┌─────────────┐       ┌──────────────────────────────┐       ┌──────────────┐
│ Your App    │       │  Go AI Gateway               │       │              │
│ (any lang)  ├──────▶│  ┌────────────────────────┐  ├──────▶│  Anthropic   │
│             │       │  │ Auth middleware        │  │       │  Claude API  │
│   Bearer    │       │  │ Rate limiter           │  │       │              │
│   gw_xxx... │       │  │ Cost tracker           │  │       └──────────────┘
└─────────────┘       │  │ Prompt cache optimizer │  │
                      │  └───────────┬────────────┘  │       ┌──────────────┐
                      │              │                │       │  OpenAI      │
                      │              ▼                ├──────▶│  GPT-4 API   │
                      │      ┌──────────────┐         │       │  (optional)  │
                      │      │   MySQL 8    │         │       └──────────────┘
                      │      │  • api_keys  │         │
                      │      │  • requests  │         │
                      │      │  • usage     │         │
                      │      └──────────────┘         │
                      └──────────────────────────────┘
```

## Features

### Core
- [x] **Provider-agnostic proxy** — drop-in replacement for Anthropic `/v1/messages`
- [x] **Multi-tenant API keys** (`gw_live_...`) with per-key limits
- [x] **Rate limiting** — requests/minute + tokens/day enforced at gateway level
- [x] **Cost tracking** — every request priced in USD by model
- [x] **Anthropic prompt caching** — automatic `cache_control` injection for system prompts

### Operations
- [x] **Structured logging** (JSON, zerolog)
- [x] **Graceful shutdown** with connection draining
- [x] **Health checks** (`/health`, `/ready`) for Kubernetes
- [x] **OpenAPI 3 spec** served at `/docs`
- [x] **Prometheus metrics** at `/metrics`

### Admin
- [x] `POST /admin/keys` — create API key with limits
- [x] `GET /admin/keys/:id/usage` — daily/monthly cost breakdown
- [x] `GET /admin/analytics` — top models, latency p50/p95/p99

---

## Quickstart (60 seconds)

```bash
# 1. Clone
git clone https://github.com/ninjadiego/go-ai-gateway.git
cd go-ai-gateway

# 2. Set your Anthropic key
cp .env.example .env
echo "ANTHROPIC_API_KEY=sk-ant-..." >> .env

# 3. Start everything (MySQL + gateway)
docker compose up -d

# 4. Create your first gateway key
curl -X POST http://localhost:8080/admin/keys \
  -H "Content-Type: application/json" \
  -d '{"name":"my-app","rate_limit_rpm":60,"monthly_budget_usd":50}'
# → { "api_key": "gw_live_abc123...", "id": 1 }

# 5. Use it exactly like the Anthropic API
curl http://localhost:8080/v1/messages \
  -H "Authorization: Bearer gw_live_abc123..." \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-sonnet-4-6",
    "max_tokens": 1024,
    "messages": [{"role":"user","content":"Hello!"}]
  }'
```

---

## Usage from code

### Python (drop-in Anthropic SDK replacement)

```python
from anthropic import Anthropic

# Point the SDK at your gateway instead of api.anthropic.com
client = Anthropic(
    api_key="gw_live_abc123...",
    base_url="http://localhost:8080",
)

message = client.messages.create(
    model="claude-sonnet-4-6",
    max_tokens=1024,
    messages=[{"role": "user", "content": "Hello!"}],
)
```

### Go

```go
req, _ := http.NewRequest("POST", "http://localhost:8080/v1/messages", body)
req.Header.Set("Authorization", "Bearer gw_live_abc123...")
req.Header.Set("Content-Type", "application/json")
resp, _ := http.DefaultClient.Do(req)
```

---

## Project structure

```
go-ai-gateway/
├── cmd/gateway/          # main.go — entry point
├── internal/
│   ├── config/           # env-based configuration
│   ├── server/           # HTTP server, routing, graceful shutdown
│   ├── handlers/         # request handlers (proxy, admin, analytics)
│   ├── middleware/       # auth, rate limit, logging, cost tracking
│   ├── providers/        # LLM provider clients (Anthropic, OpenAI)
│   ├── models/           # domain types
│   ├── repository/       # data access (MySQL)
│   ├── service/          # business logic
│   └── database/         # connection + migrations runner
├── migrations/           # SQL migrations (up/down)
├── scripts/              # dev utilities (seed data, load test)
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── README.md
```

---

## Development

```bash
make dev          # Run locally with hot-reload (requires air)
make test         # Run unit tests
make test-integration  # Run integration tests (requires MySQL)
make lint         # Run golangci-lint
make docs         # Regenerate OpenAPI spec
make db-migrate   # Apply migrations
make db-seed      # Seed demo data
```

### Requirements

- Go 1.22+
- MySQL 8.0+ (or use Docker Compose)
- Anthropic API key

---

## Pricing model

The gateway automatically prices every request based on the model and token usage:

| Model              | Input ($/1M tok) | Output ($/1M tok) | Cache write | Cache read |
|--------------------|------------------|-------------------|-------------|------------|
| claude-opus-4      | $15.00           | $75.00            | $18.75      | $1.50      |
| claude-sonnet-4-6  | $3.00            | $15.00            | $3.75       | $0.30      |
| claude-haiku-4-5   | $0.80            | $4.00             | $1.00       | $0.08      |

Daily rollups are stored in `daily_usage` — admin endpoints query this table for fast analytics.

---

## Roadmap

- [ ] OpenAI provider (GPT-4, GPT-4o)
- [ ] Gemini provider
- [ ] Streaming responses (SSE passthrough)
- [ ] Redis-backed rate limiting (for horizontal scaling)
- [ ] Webhook alerts when budget thresholds are hit
- [ ] Admin web UI (React)

---

## License

MIT © 2026 Diego Peña

Built as a portfolio project to showcase production Go + LLM integration. Feedback and contributions welcome.
