// Package handlers wires the service layer to HTTP endpoints.
package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ninjadiego/go-ai-gateway/internal/middleware"
	"github.com/ninjadiego/go-ai-gateway/internal/models"
	"github.com/ninjadiego/go-ai-gateway/internal/providers"
	"github.com/ninjadiego/go-ai-gateway/internal/repository"
)

// bgCtx returns a short-lived background context for async DB writes that
// must outlive the request context.
func bgCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

type Proxy struct {
	anthropic *providers.Anthropic
	usage     *repository.UsageRepo
}

func NewProxy(a *providers.Anthropic, u *repository.UsageRepo) *Proxy {
	return &Proxy{anthropic: a, usage: u}
}

// Messages proxies POST /v1/messages to Anthropic, records usage, prices it,
// and returns the upstream response verbatim to the client.
func (p *Proxy) Messages(w http.ResponseWriter, r *http.Request) {
	key, ok := middleware.APIKeyFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthenticated", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	result, err := p.anthropic.Messages(r.Context(), body)
	if err != nil {
		p.recordFailure(key.ID, err.Error())
		http.Error(w, "upstream error: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Record usage (best-effort — don't block the response on a DB failure).
	go p.recordSuccess(key.ID, result)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Gateway-Cost-USD", formatUSD(providers.CostUSD(result.Model, result.Usage)))
	w.Header().Set("X-Gateway-Latency-MS", itoa(result.LatencyMS))
	w.WriteHeader(result.StatusCode)
	_, _ = w.Write(result.Body)
}

func (p *Proxy) recordSuccess(apiKeyID int64, r *providers.ProxyResult) {
	req := &models.Request{
		APIKeyID:         apiKeyID,
		Provider:         "anthropic",
		Model:            r.Model,
		InputTokens:      r.Usage.InputTokens,
		OutputTokens:     r.Usage.OutputTokens,
		CacheReadTokens:  r.Usage.CacheReadInputTokens,
		CacheWriteTokens: r.Usage.CacheCreationInputTokens,
		CostUSD:          providers.CostUSD(r.Model, r.Usage),
		LatencyMS:        r.LatencyMS,
		StatusCode:       r.StatusCode,
	}
	ctx, cancel := bgCtx()
	defer cancel()
	if err := p.usage.RecordRequest(ctx, req); err != nil {
		log.Error().Err(err).Int64("api_key_id", apiKeyID).Msg("record request")
	}
}

func (p *Proxy) recordFailure(apiKeyID int64, msg string) {
	m := msg
	req := &models.Request{
		APIKeyID:     apiKeyID,
		Provider:     "anthropic",
		Model:        "unknown",
		StatusCode:   502,
		ErrorMessage: &m,
	}
	ctx, cancel := bgCtx()
	defer cancel()
	_ = p.usage.RecordRequest(ctx, req)
}

// Tiny helpers kept in this file to avoid an extra pkg for two lines.

func formatUSD(v float64) string {
	buf, _ := json.Marshal(v)
	return string(buf)
}

func itoa(n int) string {
	buf, _ := json.Marshal(n)
	return string(buf)
}
