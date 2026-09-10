package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ninjadiego/go-ai-gateway/internal/config"
)

// newTestServer builds a router without a live database. Only routes that
// never touch the DB (health, auth rejections) are exercised here; anything
// that needs MySQL belongs in the integration suite.
func newTestServer() http.Handler {
	cfg := &config.Config{
		AdminToken: "admin-test-token",
		Anthropic:  config.AnthropicConfig{APIKey: "sk-test", BaseURL: "http://127.0.0.1:0"},
	}
	return New(cfg, nil).Router()
}

func TestHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Errorf("unexpected body: %s", rec.Body.String())
	}
}

func TestProtectedRoutesRejectAnonymous(t *testing.T) {
	srv := newTestServer()
	cases := []struct {
		method, path string
	}{
		{http.MethodPost, "/v1/messages"},
		{http.MethodGet, "/admin/keys"},
		{http.MethodPost, "/admin/keys"},
		{http.MethodGet, "/admin/analytics"},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}

func TestUnknownRouteIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestServer().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}
