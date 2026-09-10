package middleware

import (
	"context"
	"net/http"
	"strconv"

	"github.com/rs/zerolog/log"
)

// DailyTokenReader returns the tokens (input + output) an API key has
// consumed on the current UTC date. Satisfied by *service.AnalyticsService.
type DailyTokenReader interface {
	TokensToday(ctx context.Context, apiKeyID int64) (int64, error)
}

// DailyTokenGuard blocks requests from keys that have already consumed
// their daily_token_limit today. A limit of zero or less disables the check.
//
// Like BudgetGuard it fails open on lookup errors and must run after
// APIKeyAuth. The check is based on the daily_usage rollup, which is
// written asynchronously after each response, so a burst of concurrent
// requests can overshoot the limit slightly; that is an accepted trade-off
// for keeping the DB off the hot path.
func DailyTokenGuard(tokens DailyTokenReader) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := APIKeyFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusInternalServerError,
					"no_api_key_in_context",
					"daily token guard requires APIKeyAuth to run first")
				return
			}

			if key.DailyTokenLimit <= 0 || tokens == nil {
				next.ServeHTTP(w, r)
				return
			}

			used, err := tokens.TokensToday(r.Context(), key.ID)
			if err != nil {
				log.Warn().Err(err).Int64("api_key_id", key.ID).
					Msg("daily token check failed; allowing request")
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-Gateway-Daily-Token-Limit", strconv.FormatInt(key.DailyTokenLimit, 10))
			w.Header().Set("X-Gateway-Daily-Tokens-Used", strconv.FormatInt(used, 10))

			if used >= key.DailyTokenLimit {
				writeError(w, http.StatusTooManyRequests, "daily_token_limit_exceeded",
					"daily token limit for this API key has been reached; resets at 00:00 UTC")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
