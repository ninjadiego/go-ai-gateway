// Package service holds business logic decoupled from HTTP and persistence.
package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/ninjadiego/go-ai-gateway/internal/models"
	"github.com/ninjadiego/go-ai-gateway/internal/repository"
)

type AuthService struct {
	repo *repository.APIKeyRepo
}

func NewAuthService(repo *repository.APIKeyRepo) *AuthService {
	return &AuthService{repo: repo}
}

// CreateKeyInput bundles the parameters accepted by the admin endpoint.
type CreateKeyInput struct {
	UserID           int64
	Name             string
	RateLimitRPM     int
	DailyTokenLimit  int64
	MonthlyBudgetUSD *float64
}

// CreatedKey is returned ONCE upon creation — the raw key is never retrievable
// again. Callers must show it to the user and instruct them to store it safely.
type CreatedKey struct {
	ID     int64
	RawKey string
	Prefix string
	Key    *models.APIKey
}

// Create generates a new cryptographically random API key, stores its hash,
// and returns the raw value to the caller (show-once).
func (s *AuthService) Create(ctx context.Context, in CreateKeyInput) (*CreatedKey, error) {
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if in.RateLimitRPM <= 0 {
		in.RateLimitRPM = 60
	}
	if in.DailyTokenLimit <= 0 {
		in.DailyTokenLimit = 1_000_000
	}

	raw, prefix, hash, err := generateKey()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	id, err := s.repo.Create(ctx, in.UserID, in.Name, hash, prefix,
		in.RateLimitRPM, in.DailyTokenLimit, in.MonthlyBudgetUSD)
	if err != nil {
		return nil, fmt.Errorf("persist key: %w", err)
	}

	return &CreatedKey{
		ID:     id,
		RawKey: raw,
		Prefix: prefix,
	}, nil
}

// Validate hashes the raw key and looks up the matching active record.
func (s *AuthService) Validate(ctx context.Context, rawKey string) (*models.APIKey, error) {
	if len(rawKey) < 20 {
		return nil, errors.New("malformed api key")
	}
	h := hashKey(rawKey)
	key, err := s.repo.GetByHash(ctx, h)
	if err != nil {
		if errors.Is(err, repository.ErrAPIKeyNotFound) {
			return nil, errors.New("api key not found or revoked")
		}
		return nil, err
	}
	return key, nil
}

// List returns the keys owned by a user.
func (s *AuthService) List(ctx context.Context, userID int64) ([]*models.APIKey, error) {
	return s.repo.List(ctx, userID)
}

// Revoke deactivates a key by id.
func (s *AuthService) Revoke(ctx context.Context, id int64) error {
	return s.repo.Revoke(ctx, id)
}

// generateKey produces a key in the format `gw_live_<32 hex chars>` and its
// SHA-256 hash suitable for storage.
func generateKey() (raw, prefix, hash string, err error) {
	const prefixLiteral = "gw_live_"
	buf := make([]byte, 24) // 24 bytes → 48 hex chars (plenty of entropy)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", err
	}
	raw = prefixLiteral + hex.EncodeToString(buf)
	prefix = raw[:min(16, len(raw))]
	hash = hashKey(raw)
	return raw, prefix, hash, nil
}

func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
