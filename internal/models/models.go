package models

import "time"

// APIKey represents a tenant-scoped gateway key.
type APIKey struct {
	ID               int64
	UserID           int64
	Name             string
	KeyHash          string
	KeyPrefix        string
	RateLimitRPM     int
	DailyTokenLimit  int64
	MonthlyBudgetUSD *float64
	IsActive         bool
	LastUsedAt       *time.Time
	ExpiresAt        *time.Time
	CreatedAt        time.Time
}

// Request is a single proxied call to an LLM provider.
type Request struct {
	ID               int64
	APIKeyID         int64
	Provider         string
	Model            string
	InputTokens      int
	OutputTokens     int
	CacheReadTokens  int
	CacheWriteTokens int
	CostUSD          float64
	LatencyMS        int
	StatusCode       int
	ErrorMessage     *string
	CreatedAt        time.Time
}

// DailyUsage is a per-key daily rollup for fast analytics.
type DailyUsage struct {
	APIKeyID          int64
	UsageDate         time.Time
	RequestCount      int64
	TotalInputTokens  int64
	TotalOutputTokens int64
	TotalCostUSD      float64
}

// User owns one or more API keys.
type User struct {
	ID           int64
	Email        string
	PasswordHash string
	Name         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
