package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ninjadiego/go-ai-gateway/internal/models"
)

type fakeCosts struct {
	spent float64
	err   error
}

func (f fakeCosts) MonthlyCost(context.Context, int64) (float64, error) { return f.spent, f.err }

func budgetReq(budget *float64) *http.Request {
	key := &models.APIKey{ID: 1, MonthlyBudgetUSD: budget}
	ctx := context.WithValue(context.Background(), ctxAPIKey, key)
	return httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
}

func TestBudgetGuard(t *testing.T) {
	fifty := 50.0
	cases := []struct {
		name   string
		budget *float64
		costs  fakeCosts
		want   int
	}{
		{"no budget configured", nil, fakeCosts{spent: 1e9}, http.StatusOK},
		{"under budget", &fifty, fakeCosts{spent: 49.99}, http.StatusOK},
		{"exactly at budget", &fifty, fakeCosts{spent: 50}, http.StatusPaymentRequired},
		{"over budget", &fifty, fakeCosts{spent: 80}, http.StatusPaymentRequired},
		{"lookup error fails open", &fifty, fakeCosts{err: errors.New("db down")}, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			BudgetGuard(tc.costs)(okHandler()).ServeHTTP(rec, budgetReq(tc.budget))
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want == http.StatusPaymentRequired && rec.Header().Get("X-Gateway-Spent-USD") == "" {
				t.Error("expected X-Gateway-Spent-USD header on rejection")
			}
		})
	}
}

func TestBudgetGuard_RequiresAuthFirst(t *testing.T) {
	rec := httptest.NewRecorder()
	BudgetGuard(fakeCosts{})(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}
