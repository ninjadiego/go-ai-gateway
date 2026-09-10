package providers

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func decodeOpenAIReq(t *testing.T, raw string) ChatCompletionRequest {
	t.Helper()
	var req ChatCompletionRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return req
}

func TestOpenAIToAnthropic_BasicTranslation(t *testing.T) {
	req := decodeOpenAIReq(t, `{
		"model": "gpt-4o",
		"messages": [
			{"role": "system", "content": "Be brief."},
			{"role": "user", "content": "Hi"},
			{"role": "assistant", "content": "Hello!"},
			{"role": "user", "content": [{"type":"text","text":"How are "},{"type":"text","text":"you?"}]}
		],
		"temperature": 0.2,
		"stop": "END",
		"user": "u-42"
	}`)

	body, err := OpenAIToAnthropic(req, "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("output is not JSON: %v", err)
	}
	if out["model"] != "claude-sonnet-4-6" {
		t.Errorf("gpt-4o should map to the default model, got %v", out["model"])
	}
	if out["system"] != "Be brief." {
		t.Errorf("system = %v", out["system"])
	}
	if out["max_tokens"] != float64(DefaultMaxTokens) {
		t.Errorf("max_tokens should default to %d, got %v", DefaultMaxTokens, out["max_tokens"])
	}
	if out["temperature"] != 0.2 {
		t.Errorf("temperature = %v", out["temperature"])
	}
	if stops, _ := out["stop_sequences"].([]any); len(stops) != 1 || stops[0] != "END" {
		t.Errorf("stop_sequences = %v", out["stop_sequences"])
	}
	msgs := out["messages"].([]any)
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	last := msgs[2].(map[string]any)
	if last["role"] != "user" || last["content"] != "How are you?" {
		t.Errorf("content parts were not concatenated: %v", last)
	}
	if meta, _ := out["metadata"].(map[string]any); meta["user_id"] != "u-42" {
		t.Errorf("metadata.user_id = %v", out["metadata"])
	}
}

func TestOpenAIToAnthropic_KeepsClaudeModelAndMergesConsecutiveRoles(t *testing.T) {
	req := decodeOpenAIReq(t, `{
		"model": "claude-haiku-4-5",
		"max_tokens": 50,
		"messages": [
			{"role": "user", "content": "part one"},
			{"role": "user", "content": "part two"}
		]
	}`)
	body, err := OpenAIToAnthropic(req, "claude-sonnet-4-6")
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	s := string(body)
	if !strings.Contains(s, `"model":"claude-haiku-4-5"`) {
		t.Errorf("claude model should be preserved: %s", s)
	}
	if !strings.Contains(s, `"max_tokens":50`) {
		t.Errorf("explicit max_tokens should be kept: %s", s)
	}
	if !strings.Contains(s, `"content":"part one\n\npart two"`) {
		t.Errorf("consecutive user turns should be merged: %s", s)
	}
}

func TestOpenAIToAnthropic_Rejections(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want error
	}{
		{"stream", `{"model":"x","stream":true,"messages":[{"role":"user","content":"hi"}]}`, ErrStreamingNotSupported},
		{"empty", `{"model":"x","messages":[]}`, nil},
		{"only system", `{"model":"x","messages":[{"role":"system","content":"s"}]}`, nil},
		{"assistant first", `{"model":"x","messages":[{"role":"assistant","content":"a"}]}`, nil},
		{"bad role", `{"model":"x","messages":[{"role":"tool","content":"a"}]}`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := OpenAIToAnthropic(decodeOpenAIReq(t, tc.raw), "claude-sonnet-4-6")
			if err == nil {
				t.Fatal("expected an error")
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAnthropicToOpenAI(t *testing.T) {
	body := `{"id":"msg_abc","type":"message","role":"assistant","model":"claude-sonnet-4-6",
		"content":[{"type":"text","text":"Hello "},{"type":"text","text":"world"}],
		"stop_reason":"max_tokens",
		"usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":90}}`

	now := time.Unix(1_700_000_000, 0)
	res, err := AnthropicToOpenAI([]byte(body), now)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if res.ID != "chatcmpl-abc" || res.Object != "chat.completion" || res.Created != now.Unix() {
		t.Errorf("envelope = %+v", res)
	}
	if len(res.Choices) != 1 || res.Choices[0].Message.Content != "Hello world" {
		t.Errorf("choices = %+v", res.Choices)
	}
	if res.Choices[0].FinishReason != "length" {
		t.Errorf("finish_reason = %q, want length", res.Choices[0].FinishReason)
	}
	if res.Usage.PromptTokens != 100 || res.Usage.CompletionTokens != 4 || res.Usage.TotalTokens != 104 {
		t.Errorf("usage = %+v (cache tokens must fold into prompt_tokens)", res.Usage)
	}
}

func TestMapStopReason(t *testing.T) {
	for in, want := range map[string]string{
		"end_turn": "stop", "stop_sequence": "stop", "max_tokens": "length", "tool_use": "tool_calls", "": "stop",
	} {
		if got := mapStopReason(in); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
}
