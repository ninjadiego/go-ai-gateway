package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ninjadiego/go-ai-gateway/internal/middleware"
	"github.com/ninjadiego/go-ai-gateway/internal/providers"
)

// ChatCompletions serves POST /v1/chat/completions in the OpenAI wire format
// and fulfils it with Anthropic. Clients built on the OpenAI SDK only need to
// change base_url and api_key.
//
// Billing goes through the same path as /v1/messages: the upstream usage is
// priced with the Claude model that actually served the request.
func (p *Proxy) ChatCompletions(w http.ResponseWriter, r *http.Request) {
	key, ok := middleware.APIKeyFromContext(r.Context())
	if !ok {
		writeOpenAIError(w, http.StatusUnauthorized, "authentication_error", "unauthenticated")
		return
	}

	var in providers.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON: "+err.Error())
		return
	}

	body, err := providers.OpenAIToAnthropic(in, p.defaultModel)
	if err != nil {
		if errors.Is(err, providers.ErrStreamingNotSupported) {
			writeOpenAIError(w, http.StatusNotImplemented, "invalid_request_error", err.Error())
			return
		}
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	result, err := p.anthropic.Messages(r.Context(), body)
	if err != nil {
		p.recordFailure(key.ID, err.Error())
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "upstream error: "+err.Error())
		return
	}

	// Upstream rejected the request: surface it in OpenAI's envelope with the
	// same status code so client retry logic behaves as it would against OpenAI.
	if result.StatusCode >= 300 {
		writeOpenAIError(w, result.StatusCode, "api_error", upstreamErrorMessage(result.Body))
		return
	}

	out, err := providers.AnthropicToOpenAI(result.Body, time.Now())
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}

	go p.recordSuccess(key.ID, result)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Gateway-Cost-USD", formatUSD(providers.CostUSD(result.Model, result.Usage)))
	w.Header().Set("X-Gateway-Latency-MS", itoa(result.LatencyMS))
	w.Header().Set("X-Gateway-Upstream-Model", result.Model)
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

// upstreamErrorMessage extracts Anthropic's error message, falling back to
// the raw body so nothing useful is lost.
func upstreamErrorMessage(body []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return string(body)
}

func writeOpenAIError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(providers.NewOpenAIError(errType, msg))
}
