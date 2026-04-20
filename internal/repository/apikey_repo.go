package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/ninjadiego/go-ai-gateway/internal/models"
)

var ErrAPIKeyNotFound = errors.New("api key not found")

type APIKeyRepo struct {
	db *sql.DB
}

func NewAPIKeyRepo(db *sql.DB) *APIKeyRepo {
	return &APIKeyRepo{db: db}
}

// Create inserts a new API key row. keyHash is the SHA-256 hex digest of the
// raw key string (which is shown to the user ONCE and never stored).
func (r *APIKeyRepo) Create(ctx context.Context, userID int64, name, keyHash, keyPrefix string,
	rateLimitRPM int, dailyTokenLimit int64, monthlyBudget *float64,
) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO api_keys
		    (user_id, name, key_hash, key_prefix, rate_limit_rpm,
		     daily_token_limit, monthly_budget_usd)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, userID, name, keyHash, keyPrefix, rateLimitRPM, dailyTokenLimit, monthlyBudget)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// GetByHash returns the active API key matching the given SHA-256 hash.
// Updates last_used_at in a best-effort manner (ignoring errors).
func (r *APIKeyRepo) GetByHash(ctx context.Context, keyHash string) (*models.APIKey, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, key_hash, key_prefix,
		       rate_limit_rpm, daily_token_limit, monthly_budget_usd,
		       is_active, last_used_at, expires_at, created_at
		  FROM api_keys
		 WHERE key_hash = ?
		   AND is_active = TRUE
		   AND (expires_at IS NULL OR expires_at > NOW())
		 LIMIT 1
	`, keyHash)

	k := &models.APIKey{}
	err := row.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &k.KeyPrefix,
		&k.RateLimitRPM, &k.DailyTokenLimit, &k.MonthlyBudgetUSD,
		&k.IsActive, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAPIKeyNotFound
	}
	if err != nil {
		return nil, err
	}

	// Best-effort: update last_used_at asynchronously in caller's context.
	go func() {
		_, _ = r.db.Exec(`UPDATE api_keys SET last_used_at = ? WHERE id = ?`, time.Now(), k.ID)
	}()

	return k, nil
}

// List returns all keys for a user, newest first.
func (r *APIKeyRepo) List(ctx context.Context, userID int64) ([]*models.APIKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, key_hash, key_prefix,
		       rate_limit_rpm, daily_token_limit, monthly_budget_usd,
		       is_active, last_used_at, expires_at, created_at
		  FROM api_keys
		 WHERE user_id = ?
		 ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*models.APIKey
	for rows.Next() {
		k := &models.APIKey{}
		if err := rows.Scan(&k.ID, &k.UserID, &k.Name, &k.KeyHash, &k.KeyPrefix,
			&k.RateLimitRPM, &k.DailyTokenLimit, &k.MonthlyBudgetUSD,
			&k.IsActive, &k.LastUsedAt, &k.ExpiresAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// Revoke soft-deletes an API key by flipping is_active to FALSE.
func (r *APIKeyRepo) Revoke(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE api_keys SET is_active = FALSE WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrAPIKeyNotFound
	}
	return nil
}
