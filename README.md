# Go AI Gateway

> A self-hosted API gateway for LLM providers, written in Go.
> Multi-tenant API keys, per-key rate limits and monthly budgets, cost tracking per request (including prompt-cache tokens), SSE streaming pass-through and usage analytics.

[![CI](https://github.com/ninjadiego/go-ai-gateway/actions/workflows/ci.yml/badge.svg)](https://github.com/ninjadiego/go-ai-gateway/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?logo=go)](https://go.dev/)
[![MySQL](https://img.shields.io/badge/MySQL-8.0-4479A1?logo=mysql&logoColor=white)](https://www.mysql.com/)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?logo=docker&logoColor=white)](https://www.docker.com/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

---

## Why this exists

Every team that integrates an LLM into a product runs into the same problems within a few weeks:

- **Cost surprises.** A buggy retry loop burns hundreds of dollars overnight before anyone notices.
- **One shared provider key.** Marketing, Support and Engineering all use it, so nobody knows who spent what.
- **No per-team limits.** One noisy service can exhaust the provider's rate limit for everyone.
- **Cache tokens billed wrong.** Anthropic prompt caching reports separate token counts; most home-grown trackers ignore them and over-estimate cost.

**Go AI Gateway** sits between your applications and the provider. Each app gets its own `gw_live_...` key with a requests-per-minute limit and an optional monthly budget in USD. Every request is priced, logged and rolled up per day, and the response carries the cost back to the caller in a header.

---

## Architecture

```
┌─────────────┐        ┌──────────────────────────────────────┐        ┌──────────────┐
│  Your app   │        │  Go AI Gateway  (chi router)         │        │              │
│  (any lang) │ ─────▶ │                                      │ ─────▶ │  Anthropic   │
│             │        │  APIKeyAuth  → SHA-256 lookup        │        │  Messages    │
│  Bearer     │ ◀───── │  RateLimit   → token bucket per key  │ ◀───── │  API         │
│  gw_live_…  │        │  BudgetGuard → 402 when over budget  │        │              │
└─────────────┘        │  DailyTokens → 429 when over quota   │        └──────────────┘
                       │  Proxy       → JSON or SSE stream    │
                       │  Pricing     → USD per model/tokens  │
                       │              │                        │
                       │              ▼  (async, best-effort)  │
                       │      ┌──────────────┐                 │
                       │      │   MySQL 8    │  api_keys        │
                       │      │              │  requests        │
                       │      │              │  daily_usage     │
                       │      └──────────────┘                 │
                       └──────────────────────────────────────┘
```

Request flow for `POST /v1/messages`:

1. `APIKeyAuth` hashes the bearer token and looks up an active key. The raw key is never stored.
2. `RateLimit` applies a per-key token bucket (`rate_limit_rpm`).
3. `BudgetGuard` compares month-to-date spend with `monthly_budget_usd` and answers `402 Payment Required` once it is reached, and `DailyTokenGuard` answers `429` once today's tokens reach `daily_token_limit`. Both fail open on DB errors so a database hiccup never takes every tenant down.
4. `Proxy` forwards the body verbatim to Anthropic. If the body has `"stream": true`, events are piped to the client as they arrive and usage is captured from the final `message_delta` event.
5. Usage is priced with the model's per-million-token table (input, output, cache write, cache read) and recorded asynchronously, so a slow DB write never delays the response.

---

## Features

**Implemented**

- Drop-in proxy for Anthropic `POST /v1/messages` — point the official SDK at the gateway and nothing else changes
- Streaming (SSE) pass-through with inline usage capture
- Multi-tenant API keys (`gw_live_...`), stored as SHA-256 hashes, show-once on creation
- Per-key rate limiting (requests per minute, token bucket)
- Per-key monthly budget enforcement in USD
- Per-key daily token limit (input + output), reset at 00:00 UTC
- Per-request cost tracking, including Anthropic prompt-cache tokens, returned in `X-Gateway-Cost-USD`
- Daily usage rollups and admin analytics (top models, p50/p95/p99 latency)
- Structured JSON logging (zerolog), request IDs, panic recovery
- Graceful shutdown with connection draining
- `/health` and `/ready` endpoints for Kubernetes probes
- Multi-stage Docker build, non-root runtime image, `docker compose` for local dev
- CI on GitHub Actions: `go vet`, race-enabled tests with coverage, `golangci-lint`, build

**Admin API** (protected by `ADMIN_TOKEN`)

| Method | Path                        | Purpose                                     |
|--------|-----------------------------|---------------------------------------------|
| POST   | `/admin/keys`               | Create a key with limits (returns raw key once) |
| GET    | `/admin/keys?user_id=`      | List keys for a user                        |
| GET    | `/admin/keys/{id}/usage`    | Daily usage and month-to-date cost          |
| DELETE | `/admin/keys/{id}`          | Revoke a key                                |
| GET    | `/admin/analytics?days=`    | Global overview: requests, cost, latency    |

---

## Quickstart

```bash
git clone https://github.com/ninjadiego/go-ai-gateway.git
cd go-ai-gateway

cp .env.example .env            # set ANTHROPIC_API_KEY and ADMIN_TOKEN
docker compose up -d            # MySQL 8 + gateway on :8080

# Create a tenant key with 60 rpm and a USD 50 monthly budget
curl -s -X POST http://localhost:8080/admin/keys \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"user_id":1,"name":"my-app","rate_limit_rpm":60,"monthly_budget_usd":50}'
# → {"id":1,"api_key":"gw_live_...","prefix":"gw_live_ab12cd34","message":"Store this key securely — it will not be shown again."}

# Use it exactly like the Anthropic API
curl -i http://localhost:8080/v1/messages \
  -H "Authorization: Bearer gw_live_..." \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-6","max_tokens":256,"messages":[{"role":"user","content":"Hello!"}]}'
# → HTTP/1.1 200 OK
# → X-Gateway-Cost-USD: 0.000411
# → X-Gateway-Latency-MS: 842
```

### From code

```python
from anthropic import Anthropic

client = Anthropic(api_key="gw_live_...", base_url="http://localhost:8080")
msg = client.messages.create(
    model="claude-sonnet-4-6",
    max_tokens=256,
    messages=[{"role": "user", "content": "Hello!"}],
)
```

```go
req, _ := http.NewRequest(http.MethodPost, "http://localhost:8080/v1/messages", body)
req.Header.Set("Authorization", "Bearer gw_live_...")
req.Header.Set("Content-Type", "application/json")
resp, err := http.DefaultClient.Do(req)
```

---

## Project structure

```
go-ai-gateway/
├── cmd/gateway/          # main.go — config, DB, HTTP server, graceful shutdown
├── internal/
│   ├── config/           # env-based configuration (fails fast on missing vars)
│   ├── server/           # chi router and route wiring
│   ├── handlers/         # proxy (JSON + SSE) and admin endpoints
│   ├── middleware/       # api-key auth, admin auth, rate limit, budget guard, logging
│   ├── providers/        # Anthropic client + pricing table
│   ├── service/          # auth (key generation/validation) and analytics
│   ├── repository/       # MySQL data access
│   ├── models/           # domain types
│   └── database/         # connection pool
├── migrations/           # SQL migrations (up/down), auto-applied by docker compose
├── scripts/              # seed data
├── .github/workflows/    # CI
├── docker-compose.yml · Dockerfile · Makefile
```

---

## Development

```bash
make test         # unit tests with -race and coverage
make lint         # golangci-lint
make build        # binary in bin/gateway
make docker-up    # MySQL + gateway
make db-migrate   # apply migrations (needs golang-migrate)
```

Unit tests need no database or network: middleware is tested with `httptest` and fakes, and the Anthropic client is tested against a local fake upstream (`internal/providers/anthropic_test.go`).

Requirements: Go 1.23+, MySQL 8 (or Docker), an Anthropic API key.

---

## Pricing model

Cost is computed per request from the model name (dated suffixes such as `-20250514` are normalised) and the four token counters Anthropic returns. Unknown models fall back to Sonnet pricing so a request is never left unbilled.

| Model             | Input ($/1M) | Output ($/1M) | Cache write | Cache read |
|-------------------|-------------:|--------------:|------------:|-----------:|
| claude-opus-4-x   | 15.00        | 75.00         | 18.75       | 1.50       |
| claude-sonnet-4-x | 3.00         | 15.00         | 3.75        | 0.30       |
| claude-haiku-4-x  | 0.80         | 4.00          | 1.00        | 0.08       |

Keep `internal/providers/pricing.go` in sync with https://www.anthropic.com/pricing.

---

## Design decisions

- **No official SDK for the upstream call.** A 200-line client keeps the proxy behaviour explicit (headers, streaming, error pass-through) and avoids pulling a large dependency into the hot path.
- **Hash the key, not encrypt it.** Keys are random 192-bit values; a SHA-256 lookup is enough and there is nothing to leak if the table is dumped.
- **Async usage writes.** Billing rows are written in a goroutine with its own timeout, so the caller's latency is the provider's latency, not the provider's plus MySQL's.
- **Fail open on the budget check, fail closed on auth.** Losing money for a minute is recoverable; letting anonymous traffic through is not.
- **In-memory rate limiter.** Correct for a single instance and simple to reason about. Horizontal scaling needs a Redis-backed limiter (see roadmap).

---

## Roadmap

- [ ] OpenAI-compatible provider (`/v1/chat/completions`)
- [ ] Redis-backed rate limiting for multiple gateway instances
- [ ] Webhook alert when a key crosses 80 % of its budget
- [ ] Prometheus `/metrics` endpoint
- [ ] OpenAPI spec for the admin API
- [ ] Integration test suite against MySQL in CI (service container)

---

## License

MIT © 2026 Diego Peña

Built as a portfolio project to show production-style Go and LLM integration. Feedback and contributions are welcome.
