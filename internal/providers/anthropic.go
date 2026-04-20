// Package providers contains clients for upstream LLM providers.
package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
	req, err := a.buildRequest(ctx, body)
	if err != nil {
		return nil, err
	}

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

// StreamResult is returned from MessagesStream after the full stream has been
// consumed and forwarded to the client. Usage comes from the `message_delta`
// event that Anthropic emits near the end of the stream.
type StreamResult struct {
	StatusCode int
	Usage      Usage
	Model      string
	LatencyMS  int
}

// MessagesStream forwards a streaming Messages request (SSE) and pipes the
// upstream body verbatim into `out`. While streaming, it also parses
// `message_start` and `message_delta` events to capture model + final usage
// for billing without re-reading the body.
func (a *Anthropic) MessagesStream(ctx context.Context, body []byte, out io.Writer, flush func()) (*StreamResult, error) {
	req, err := a.buildRequest(ctx, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call anthropic: %w", err)
	}
	defer resp.Body.Close()

	// Non-200 streams come back as a regular JSON error — forward unchanged.
	if resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		_, _ = out.Write(errBody)
		return &StreamResult{
			StatusCode: resp.StatusCode,
			LatencyMS:  int(time.Since(start).Milliseconds()),
		}, nil
	}

	result := &StreamResult{StatusCode: resp.StatusCode}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	var eventType, dataLine string

	for scanner.Scan() {
		line := scanner.Text()

		// Forward the raw line + the SSE newline to the caller immediately.
		if _, err := io.WriteString(out, line+"\n"); err != nil {
			return result, fmt.Errorf("write to client: %w", err)
		}
		if flush != nil {
			flush()
		}

		switch {
		case strings.HasPrefix(line, "event: "):
			eventType = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			dataLine = strings.TrimPrefix(line, "data: ")
		case line == "":
			// Blank line = end of SSE event. Parse what we collected.
			a.parseStreamEvent(eventType, dataLine, result)
			eventType, dataLine = "", ""
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read upstream stream: %w", err)
	}

	result.LatencyMS = int(time.Since(start).Milliseconds())
	return result, nil
}

// parseStreamEvent extracts billing info from message_start and message_delta
// events. Silently ignores anything else.
func (a *Anthropic) parseStreamEvent(eventType, data string, result *StreamResult) {
	if data == "" {
		return
	}
	switch eventType {
	case "message_start":
		var msg struct {
			Message MessagesResponse `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &msg); err == nil {
			result.Model = msg.Message.Model
			// message_start includes the input token count but not output yet.
			result.Usage.InputTokens = msg.Message.Usage.InputTokens
			result.Usage.CacheCreationInputTokens = msg.Message.Usage.CacheCreationInputTokens
			result.Usage.CacheReadInputTokens = msg.Message.Usage.CacheReadInputTokens
		}
	case "message_delta":
		var delta struct {
			Usage Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &delta); err == nil {
			result.Usage.OutputTokens = delta.Usage.OutputTokens
		}
	}
}

// buildRequest constructs the HTTP request to Anthropic with correct headers.
func (a *Anthropic) buildRequest(ctx context.Context, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		a.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("x-api-key", a.apiKey)
	return req, nil
}
