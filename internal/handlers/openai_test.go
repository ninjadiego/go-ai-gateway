package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ninjadiego/go-ai-gateway/internal/config"
	"github.com/ninjadiego/go-ai-gateway/internal/middleware"
	"github.com/ninjadiego/go-ai-gateway/internal/models"
	"github.com/ninjadiego/go-ai-gateway/internal/providers"
)

// These tests cover the request-validation paths, which never reach the
// upstream or the database. The happy path is exercised by the providers
// translation tests plus the integration suite.

func newTestProxy() *Proxy {
	a := providers.NewAnthropic(config.AnthropicConfig{APIKey: "sk-test", BaseURL: "http://127.0.0.1:1"}, time.Second)
	return NewProxy(a, nil, "claude-sonnet-4-6")
}

func authedRequest(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	ctx := middleware.ContextWithAPIKey(req.Context(), &models.APIKey{ID: 1})
	return req.WithContext(ctx)
}

func TestChatCompletions_RejectsUnauthenticated(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestProxy().ChatCompletions(rec, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestChatCompletions_ValidationErrorsUseOpenAIEnvelope(t *testing.T) {
	cases := []struct {
		name string
		body string
		want int
	}{
		{"invalid json", `{`, http.StatusBadRequest},
		{"no messages", `{"model":"gpt-4o","messages":[]}`, http.StatusBadRequest},
		{"streaming", `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"hi"}]}`, http.StatusNotImplemented},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newTestProxy().ChatCompletions(rec, authedRequest(tc.body))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			var e providers.OpenAIError
			if err := json.NewDecoder(rec.Body).Decode(&e); err != nil || e.Error.Type == "" {
				t.Fatalf("body is not an OpenAI error envelope: %s", rec.Body.String())
			}
		})
	}
}

func TestUpstreamErrorMessage(t *testing.T) {
	if got := upstreamErrorMessage([]byte(`{"type":"error","error":{"type":"x","message":"boom"}}`)); got != "boom" {
		t.Errorf("got %q", got)
	}
	if got := upstreamErrorMessage([]byte(`not json`)); got != "not json" {
		t.Errorf("fallback got %q", got)
	}
}
