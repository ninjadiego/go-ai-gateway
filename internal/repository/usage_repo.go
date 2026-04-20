package repository

import (
	"context"
	"database/sql"
	"time"

	"github.com/ninjadiego/go-ai-gateway/internal/models"
)

type UsageRepo struct {
	db *sql.DB
}

func NewUsageRepo(db *sql.DB) *UsageRepo {
	return &UsageRepo{db: db}
}

// RecordRequest inserts an entry into `requests` AND upserts the daily rollup.
// Runs in a single transaction so either both succeed or neither does.
func (r *UsageRepo) RecordRequest(ctx context.Context, req *models.Request) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO requests
		    (api_key_id, provider, model, input_tokens, output_tokens,
		     cache_read_tokens, cache_write_tokens, cost_usd, latency_ms,
		     status_code, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.APIKeyID, req.Provider, req.Model, req.InputTokens, req.OutputTokens,
		req.CacheReadTokens, req.CacheWriteTokens, req.CostUSD, req.LatencyMS,
		req.StatusCode, req.ErrorMessage)
	if err != nil {
		return err
	}

	// Only bill successful requests into the rollup.
	if req.StatusCode < 300 {
		_, err = tx.ExecContext(ctx, `
			INSERT INTO daily_usage
			    (api_key_id, usage_date, request_count,
			     total_input_tokens, total_output_tokens, total_cost_usd)
			VALUES (?, CURRENT_DATE, 1, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
			    request_count       = request_count + 1,
			    total_input_tokens  = total_input_tokens  + VALUES(total_input_tokens),
			    total_output_tokens = total_output_tokens + VALUES(total_output_tokens),
			    total_cost_usd      = total_cost_usd      + VALUES(total_cost_usd)
		`, req.APIKeyID, req.InputTokens, req.OutputTokens, req.CostUSD)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DailyUsageForKey returns the last N days of usage for a key, newest first.
func (r *UsageRepo) DailyUsageForKey(ctx context.Context, apiKeyID int64, days int) ([]models.DailyUsage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT api_key_id, usage_date, request_count,
		       total_input_tokens, total_output_tokens, total_cost_usd
		  FROM daily_usage
		 WHERE api_key_id = ?
		   AND usage_date >= DATE_SUB(CURRENT_DATE, INTERVAL ? DAY)
		 ORDER BY usage_date DESC
	`, apiKeyID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []models.DailyUsage
	for rows.Next() {
		d := models.DailyUsage{}
		if err := rows.Scan(&d.APIKeyID, &d.UsageDate, &d.RequestCount,
			&d.TotalInputTokens, &d.TotalOutputTokens, &d.TotalCostUSD); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MonthlyCostUSD returns the current calendar month spend for an API key.
func (r *UsageRepo) MonthlyCostUSD(ctx context.Context, apiKeyID int64) (float64, error) {
	var cost sql.NullFloat64
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(total_cost_usd), 0)
		  FROM daily_usage
		 WHERE api_key_id = ?
		   AND usage_date >= DATE_FORMAT(CURRENT_DATE, '%Y-%m-01')
	`, apiKeyID).Scan(&cost)
	if err != nil {
		return 0, err
	}
	return cost.Float64, nil
}

// AnalyticsOverview returns high-level metrics for the last N days.
type AnalyticsOverview struct {
	TotalRequests int64     `json:"total_requests"`
	TotalCostUSD  float64   `json:"total_cost_usd"`
	ActiveKeys    int       `json:"active_keys"`
	AsOf          time.Time `json:"as_of"`
}

func (r *UsageRepo) Analytics(ctx context.Context, days int) (*AnalyticsOverview, error) {
	ov := &AnalyticsOverview{AsOf: time.Now().UTC()}
	err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(request_count), 0),
		       COALESCE(SUM(total_cost_usd), 0)
		  FROM daily_usage
		 WHERE usage_date >= DATE_SUB(CURRENT_DATE, INTERVAL ? DAY)
	`, days).Scan(&ov.TotalRequests, &ov.TotalCostUSD)
	if err != nil {
		return nil, err
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM api_keys WHERE is_active = TRUE`,
	).Scan(&ov.ActiveKeys)
	if err != nil {
		return nil, err
	}
	return ov, nil
}
