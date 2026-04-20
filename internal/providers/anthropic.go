// Package providers contains clients for upstream LLM providers.
package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ninjadiego/go-ai-gateway/internal/config"
)

// Anthropic is a minimal client for the Anthropic Messages API.
//
// We intentionally do not depend on the official SDK to keep the surface
// area small and make the proxy behaviour explicit.
type Anthropic struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewAnthropic(cfg config.AnthropicConfig, timeout time.Duration) *Anthropic {
	return &Anthropic{
		apiKey:  cfg.APIKey,
		baseURL: cfg.BaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

// MessagesResponse is the subset of Anthropic's response we care about.
// We forward the raw body to the caller and parse Usage for cost tracking.
type MessagesResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Model   string `json:"model"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage Usage `json:"usage"`
}

// Usage reports token counts for pricing. Includes cache tokens when
// prompt caching is used.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens,omitempty"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens,omitempty"`
}

// ProxyResult carries the raw upstream response back to the caller so it
// can be forwarded verbatim, alongside parsed usage for billing.
type ProxyResult struct {
	StatusCode int
	Body       []byte
	Usage      Usage
	Model      string
	LatencyMS  int
}

// Messages forwards the request body to POST /v1/messages and returns the
// raw response. The caller is responsible for writing it back to the client.
func (a *Anthropic) Messages(ctx context.Context, body []byte) (*ProxyResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", a.apiKey)

	start := time.Now()
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call anthropic: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	result := &ProxyResult{
		StatusCode: resp.StatusCode,
		Body:       respBody,
		LatencyMS:  int(time.Since(start).Milliseconds()),
	}

	// Parse usage only on success — error bodies have a different shape.
	if resp.StatusCode < 300 {
		var parsed MessagesResponse
		if err := json.Unmarshal(respBody, &parsed); err == nil {
			result.Usage = parsed.Usage
			result.Model = parsed.Model
		}
	}

	return result, nil
}
