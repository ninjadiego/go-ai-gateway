package middleware

import (
	"context"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
)

// MonthlyCostReader returns the USD already spent by an API key in the
// current calendar month. Satisfied by *service.AnalyticsService.
type MonthlyCostReader interface {
	MonthlyCost(ctx context.Context, apiKeyID int64) (float64, error)
}

// BudgetGuard blocks requests from keys whose month-to-date spend has
// reached their monthly_budget_usd. Keys without a budget are never blocked.
//
// The check is fail-open: if the usage lookup fails we log and let the
// request through, because a database hiccup should not take every tenant
// offline. Must run after APIKeyAuth.
func BudgetGuard(costs MonthlyCostReader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := APIKeyFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusInternalServerError,
					"no_api_key_in_context",
					"budget guard requires APIKeyAuth to run first")
				return
			}

			if key.MonthlyBudgetUSD == nil || costs == nil {
				next.ServeHTTP(w, r)
				return
			}

			spent, err := costs.MonthlyCost(r.Context(), key.ID)
			if err != nil {
				log.Warn().Err(err).Int64("api_key_id", key.ID).
					Msg("budget check failed; allowing request")
				next.ServeHTTP(w, r)
				return
			}

			if spent >= *key.MonthlyBudgetUSD {
				w.Header().Set("X-Gateway-Budget-USD", strconv.FormatFloat(*key.MonthlyBudgetUSD, 'f', 4, 64))
				w.Header().Set("X-Gateway-Spent-USD", strconv.FormatFloat(spent, 'f', 4, 64))
				writeError(w, http.StatusPaymentRequired, "budget_exceeded",
					"monthly budget for this API key has been reached")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
