package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ninjadiego/go-ai-gateway/internal/models"
)

type fakeTokens struct {
	used int64
	err  error
}

func (f fakeTokens) TokensToday(context.Context, int64) (int64, error) { return f.used, f.err }

func dailyReq(limit int64) *http.Request {
	key := &models.APIKey{ID: 1, DailyTokenLimit: limit}
	ctx := context.WithValue(context.Background(), ctxAPIKey, key)
	return httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
}

func TestDailyTokenGuard(t *testing.T) {
	cases := []struct {
		name   string
		limit  int64
		tokens fakeTokens
		want   int
	}{
		{"no limit configured", 0, fakeTokens{used: 1 << 40}, http.StatusOK},
		{"under limit", 1000, fakeTokens{used: 999}, http.StatusOK},
		{"exactly at limit", 1000, fakeTokens{used: 1000}, http.StatusTooManyRequests},
		{"over limit", 1000, fakeTokens{used: 5000}, http.StatusTooManyRequests},
		{"lookup error fails open", 1000, fakeTokens{err: errors.New("db down")}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			DailyTokenGuard(tc.tokens)(okHandler()).ServeHTTP(rec, dailyReq(tc.limit))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestDailyTokenGuard_ExposesUsageHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	DailyTokenGuard(fakeTokens{used: 250})(okHandler()).ServeHTTP(rec, dailyReq(1000))
	if got := rec.Header().Get("X-Gateway-Daily-Tokens-Used"); got != "250" {
		t.Errorf("used header = %q, want 250", got)
	}
	if got := rec.Header().Get("X-Gateway-Daily-Token-Limit"); got != "1000" {
		t.Errorf("limit header = %q, want 1000", got)
	}
}

func TestDailyTokenGuard_RequiresAuthFirst(t *testing.T) {
	rec := httptest.NewRecorder()
	DailyTokenGuard(fakeTokens{})(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
