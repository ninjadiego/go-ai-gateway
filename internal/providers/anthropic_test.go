package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ninjadiego/go-ai-gateway/internal/config"
)

// fakeUpstream stands in for api.anthropic.com so the client can be tested
// without network access or a real API key.
func fakeUpstream(t *testing.T, status int, body string, assert func(r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if assert != nil {
			assert(r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func newClient(baseURL string) *Anthropic {
	return NewAnthropic(config.AnthropicConfig{APIKey: "sk-test", BaseURL: baseURL}, 5*time.Second)
}

func TestMessages_ForwardsRequestAndParsesUsage(t *testing.T) {
	const upstreamBody = `{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-6",
		"content":[{"type":"text","text":"Hi"}],
		"usage":{"input_tokens":12,"output_tokens":5,"cache_read_input_tokens":100}}`

	srv := fakeUpstream(t, http.StatusOK, upstreamBody, func(r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %s, want /v1/messages", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "sk-test" {
			t.Errorf("x-api-key = %q, want sk-test", got)
		}
		if got := r.Header.Get("anthropic-version"); got == "" {
			t.Error("anthropic-version header missing")
		}
	})
	defer srv.Close()

	res, err := newClient(srv.URL).Messages(context.Background(), []byte(`{"model":"claude-sonnet-4-6"}`))
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if res.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %q", res.Model)
	}
	if res.Usage.InputTokens != 12 || res.Usage.OutputTokens != 5 || res.Usage.CacheReadInputTokens != 100 {
		t.Errorf("usage = %+v", res.Usage)
	}
	if !strings.Contains(string(res.Body), `"id":"msg_1"`) {
		t.Errorf("body was not forwarded verbatim: %s", res.Body)
	}
}

func TestMessages_PassesThroughUpstreamErrors(t *testing.T) {
	srv := fakeUpstream(t, http.StatusTooManyRequests,
		`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`, nil)
	defer srv.Close()

	res, err := newClient(srv.URL).Messages(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("upstream 4xx must not be a transport error: %v", err)
	}
	if res.StatusCode != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", res.StatusCode)
	}
	if res.Usage.InputTokens != 0 || res.Model != "" {
		t.Errorf("usage should not be parsed from error bodies: %+v", res)
	}
}

func TestMessages_ReturnsErrorWhenUpstreamUnreachable(t *testing.T) {
	// Port 1 is never listening.
	_, err := newClient("http://127.0.0.1:1").Messages(context.Background(), []byte(`{}`))
	if err == nil {
		t.Fatal("expected a transport error")
	}
}
