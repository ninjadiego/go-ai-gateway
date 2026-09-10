package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// This file translates between the OpenAI Chat Completions wire format and
// the Anthropic Messages API, so clients written against the OpenAI SDK can
// point their base_url at the gateway without code changes.
//
// Scope: text-only conversations, non-streaming. Tool calls, images and
// streaming are not translated yet (see roadmap).

// ChatCompletionRequest is the subset of the OpenAI request we understand.
type ChatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []ChatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	Stop        StopSequences `json:"stop,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
	User        string        `json:"user,omitempty"`
}

// ChatMessage is one turn in an OpenAI-style conversation. Content may be a
// plain string or an array of content parts; only text parts are kept.
type ChatMessage struct {
	Role    string      `json:"role"`
	Content ChatContent `json:"content"`
}

// ChatContent accepts either `"content": "text"` or
// `"content": [{"type":"text","text":"..."}]` and normalises to a string.
type ChatContent string

func (c *ChatContent) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*c = ChatContent(s)
		return nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b, &parts); err != nil {
		return fmt.Errorf("content must be a string or an array of parts")
	}
	var sb strings.Builder
	for _, p := range parts {
		if p.Type == "text" {
			sb.WriteString(p.Text)
		}
	}
	*c = ChatContent(sb.String())
	return nil
}

// StopSequences accepts either a single string or an array of strings.
type StopSequences []string

func (s *StopSequences) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*s = StopSequences{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return fmt.Errorf("stop must be a string or an array of strings")
	}
	*s = StopSequences(many)
	return nil
}

// anthropicMessagesRequest is the outbound body for POST /v1/messages.
type anthropicMessagesRequest struct {
	Model         string             `json:"model"`
	MaxTokens     int                `json:"max_tokens"`
	System        string             `json:"system,omitempty"`
	Messages      []anthropicMessage `json:"messages"`
	Temperature   *float64           `json:"temperature,omitempty"`
	TopP          *float64           `json:"top_p,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
	Metadata      *anthropicMetadata `json:"metadata,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicMetadata struct {
	UserID string `json:"user_id,omitempty"`
}

// DefaultMaxTokens is used when an OpenAI client omits max_tokens, which is
// optional there but required by Anthropic.
const DefaultMaxTokens = 1024

// ErrStreamingNotSupported is returned for `"stream": true` on the
// OpenAI-compatible endpoint until SSE translation is implemented.
var ErrStreamingNotSupported = errors.New("streaming is not supported on /v1/chat/completions yet; use /v1/messages for SSE")

// OpenAIToAnthropic converts an OpenAI chat request into an Anthropic
// Messages request body. Models that are not Claude models (e.g. "gpt-4o")
// are mapped to defaultModel so existing client configs keep working.
func OpenAIToAnthropic(in ChatCompletionRequest, defaultModel string) ([]byte, error) {
	if in.Stream {
		return nil, ErrStreamingNotSupported
	}
	if len(in.Messages) == 0 {
		return nil, errors.New("messages must not be empty")
	}

	out := anthropicMessagesRequest{
		Model:         resolveModel(in.Model, defaultModel),
		MaxTokens:     in.MaxTokens,
		Temperature:   in.Temperature,
		TopP:          in.TopP,
		StopSequences: in.Stop,
	}
	if out.MaxTokens <= 0 {
		out.MaxTokens = DefaultMaxTokens
	}
	if in.User != "" {
		out.Metadata = &anthropicMetadata{UserID: in.User}
	}

	var system []string
	for _, m := range in.Messages {
		switch m.Role {
		case "system", "developer":
			system = append(system, string(m.Content))
		case "user", "assistant":
			// Anthropic requires strictly alternating roles; merge consecutive
			// same-role turns rather than rejecting the request.
			if n := len(out.Messages); n > 0 && out.Messages[n-1].Role == m.Role {
				out.Messages[n-1].Content += "\n\n" + string(m.Content)
				continue
			}
			out.Messages = append(out.Messages, anthropicMessage{Role: m.Role, Content: string(m.Content)})
		default:
			return nil, fmt.Errorf("unsupported message role %q", m.Role)
		}
	}
	out.System = strings.Join(system, "\n\n")

	if len(out.Messages) == 0 {
		return nil, errors.New("at least one user or assistant message is required")
	}
	if out.Messages[0].Role != "user" {
		return nil, errors.New("the first non-system message must have role \"user\"")
	}

	return json.Marshal(out)
}

// resolveModel keeps Claude model names and maps anything else to the default.
func resolveModel(requested, defaultModel string) string {
	if strings.HasPrefix(strings.ToLower(requested), "claude") {
		return requested
	}
	return defaultModel
}

// ChatCompletionResponse mirrors the OpenAI response shape.
type ChatCompletionResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   ChatUsage    `json:"usage"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// anthropicMessagesResponse is the subset of the upstream response we map.
type anthropicMessagesResponse struct {
	ID         string `json:"id"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage Usage `json:"usage"`
}

// AnthropicToOpenAI converts a successful Anthropic Messages response body
// into an OpenAI chat completion. `now` is injected for deterministic tests.
func AnthropicToOpenAI(body []byte, now time.Time) (*ChatCompletionResponse, error) {
	var in anthropicMessagesResponse
	if err := json.Unmarshal(body, &in); err != nil {
		return nil, fmt.Errorf("decode anthropic response: %w", err)
	}

	var text strings.Builder
	for _, c := range in.Content {
		if c.Type == "text" {
			text.WriteString(c.Text)
		}
	}

	// Anthropic counts cached prompt tokens separately; OpenAI clients expect
	// them folded into prompt_tokens.
	prompt := in.Usage.InputTokens + in.Usage.CacheReadInputTokens + in.Usage.CacheCreationInputTokens

	return &ChatCompletionResponse{
		ID:      "chatcmpl-" + strings.TrimPrefix(in.ID, "msg_"),
		Object:  "chat.completion",
		Created: now.Unix(),
		Model:   in.Model,
		Choices: []ChatChoice{{
			Index:        0,
			Message:      ChatMessage{Role: "assistant", Content: ChatContent(text.String())},
			FinishReason: mapStopReason(in.StopReason),
		}},
		Usage: ChatUsage{
			PromptTokens:     prompt,
			CompletionTokens: in.Usage.OutputTokens,
			TotalTokens:      prompt + in.Usage.OutputTokens,
		},
	}, nil
}

// mapStopReason translates Anthropic stop reasons to OpenAI finish reasons.
func mapStopReason(r string) string {
	switch r {
	case "end_turn", "stop_sequence":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return "stop"
	}
}

// OpenAIError is the error envelope OpenAI clients expect.
type OpenAIError struct {
	Error struct {
		Message string  `json:"message"`
		Type    string  `json:"type"`
		Code    *string `json:"code"`
	} `json:"error"`
}

// NewOpenAIError builds an OpenAI-style error body.
func NewOpenAIError(errType, msg string) OpenAIError {
	var e OpenAIError
	e.Error.Type = errType
	e.Error.Message = msg
	return e
}
