package middleware

import (
	"net/http"
	"sync"

	"golang.org/x/time/rate"

	"github.com/ninjadiego/go-ai-gateway/internal/service"
)

// RateLimit enforces per-API-key requests-per-minute limits using a token bucket.
// Limiters are created lazily and cached in memory.
//
// For horizontal scaling, swap this for a Redis-backed limiter (see roadmap).
func RateLimit(auth *service.AuthService) func(http.Handler) http.Handler {
	var (
		mu       sync.Mutex
		limiters = make(map[int64]*rate.Limiter)
	)

	getLimiter := func(keyID int64, rpm int) *rate.Limiter {
		mu.Lock()
		defer mu.Unlock()

		l, ok := limiters[keyID]
		if !ok {
			// rpm → rate per second with burst equal to rpm.
			l = rate.NewLimiter(rate.Limit(float64(rpm)/60.0), rpm)
			limiters[keyID] = l
		}
		return l
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key, ok := APIKeyFromContext(r.Context())
			if !ok {
				writeError(w, http.StatusInternalServerError,
					"no_api_key_in_context",
					"rate limiter requires APIKeyAuth to run first")
				return
			}

			l := getLimiter(key.ID, key.RateLimitRPM)
			if !l.Allow() {
				writeError(w, http.StatusTooManyRequests, "rate_limit_exceeded",
					"requests per minute exceeded for this API key")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
