package providers

import "strings"

// Price describes per-million-token costs in USD.
type Price struct {
	Input      float64
	Output     float64
	CacheWrite float64
	CacheRead  float64
}

// anthropicPricing lists USD per 1M tokens as of 2026.
// Keep in sync with https://www.anthropic.com/pricing.
var anthropicPricing = map[string]Price{
	"claude-opus-4-7":    {Input: 15.00, Output: 75.00, CacheWrite: 18.75, CacheRead: 1.50},
	"claude-opus-4-6":    {Input: 15.00, Output: 75.00, CacheWrite: 18.75, CacheRead: 1.50},
	"claude-opus-4":      {Input: 15.00, Output: 75.00, CacheWrite: 18.75, CacheRead: 1.50},
	"claude-sonnet-4-6":  {Input: 3.00, Output: 15.00, CacheWrite: 3.75, CacheRead: 0.30},
	"claude-sonnet-4":    {Input: 3.00, Output: 15.00, CacheWrite: 3.75, CacheRead: 0.30},
	"claude-haiku-4-5":   {Input: 0.80, Output: 4.00, CacheWrite: 1.00, CacheRead: 0.08},
	"claude-haiku-4":     {Input: 0.80, Output: 4.00, CacheWrite: 1.00, CacheRead: 0.08},
}

// CostUSD returns the total cost in USD for the given usage on the given model.
// Falls back to sonnet pricing for unknown models so we never drop billing.
func CostUSD(model string, u Usage) float64 {
	p, ok := anthropicPricing[canonicalModel(model)]
	if !ok {
		p = anthropicPricing["claude-sonnet-4-6"]
	}
	const perMillion = 1_000_000.0
	return (float64(u.InputTokens)*p.Input +
		float64(u.OutputTokens)*p.Output +
		float64(u.CacheCreationInputTokens)*p.CacheWrite +
		float64(u.CacheReadInputTokens)*p.CacheRead) / perMillion
}

// canonicalModel strips dated suffixes like "-20241022" so we match the table.
func canonicalModel(m string) string {
	m = strings.ToLower(m)
	for i := len(m) - 1; i >= 0; i-- {
		if m[i] == '-' && i+1 < len(m) && m[i+1] >= '0' && m[i+1] <= '9' && len(m[i+1:]) == 8 {
			return m[:i]
		}
	}
	return m
}
