package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/ninjadiego/go-ai-gateway/internal/models"
	"github.com/ninjadiego/go-ai-gateway/internal/service"
)

type ctxKey int

const (
	ctxAPIKey ctxKey = iota
)

// APIKeyAuth validates the `Authorization: Bearer gw_live_...` header.
// On success, the authenticated APIKey is attached to the request context.
func APIKeyAuth(auth *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				writeError(w, http.StatusUnauthorized, "missing_api_key",
					"Authorization header must be 'Bearer gw_live_...'")
				return
			}

			raw := strings.TrimPrefix(h, "Bearer ")
			key, err := auth.Validate(r.Context(), raw)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid_api_key", err.Error())
				return
			}

			ctx := context.WithValue(r.Context(), ctxAPIKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminAuth protects /admin/* routes with a static shared secret.
func AdminAuth(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if h != "Bearer "+token {
				writeError(w, http.StatusUnauthorized, "admin_unauthorized",
					"invalid admin token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// APIKeyFromContext retrieves the authenticated APIKey set by APIKeyAuth.
func APIKeyFromContext(ctx context.Context) (*models.APIKey, bool) {
	k, ok := ctx.Value(ctxAPIKey).(*models.APIKey)
	return k, ok
}
