package providers

import (
	"math"
	"testing"
)

func TestCostUSD_KnownModel(t *testing.T) {
	// 1M input + 1M output tokens for sonnet-4-6 → $3 + $15 = $18
	got := CostUSD("claude-sonnet-4-6", Usage{
		InputTokens:  1_000_000,
		OutputTokens: 1_000_000,
	})
	want := 18.00
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("cost = %v, want %v", got, want)
	}
}

func TestCostUSD_WithCache(t *testing.T) {
	// 100k cache write + 500k cache read on sonnet → $0.375 + $0.15 = $0.525
	got := CostUSD("claude-sonnet-4-6", Usage{
		CacheCreationInputTokens: 100_000,
		CacheReadInputTokens:     500_000,
	})
	want := 0.525
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("cache cost = %v, want %v", got, want)
	}
}

func TestCostUSD_UnknownModelFallsBackToSonnet(t *testing.T) {
	got := CostUSD("claude-martian-99", Usage{InputTokens: 1_000_000})
	want := 3.00
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("fallback cost = %v, want %v", got, want)
	}
}

func TestCanonicalModel_StripsDate(t *testing.T) {
	tests := []struct{ in, want string }{
		{"claude-sonnet-4-6-20251001", "claude-sonnet-4-6"},
		{"claude-haiku-4-5", "claude-haiku-4-5"},
		{"CLAUDE-OPUS-4", "claude-opus-4"},
	}
	for _, tt := range tests {
		if got := canonicalModel(tt.in); got != tt.want {
			t.Errorf("canonicalModel(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
