package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ninjadiego/go-ai-gateway/internal/models"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAdminAuth(t *testing.T) {
	h := AdminAuth("s3cret")(okHandler())

	cases := []struct {
		name   string
		header string
		want   int
	}{
		{"valid token", "Bearer s3cret", http.StatusOK},
		{"wrong token", "Bearer nope", http.StatusUnauthorized},
		{"missing header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic s3cret", http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/admin/keys", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			if tc.want != http.StatusOK {
				var body errorBody
				if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
					t.Fatalf("error body is not JSON: %v", err)
				}
				if body.Error.Type != "admin_unauthorized" {
					t.Errorf("error type = %q, want admin_unauthorized", body.Error.Type)
				}
			}
		})
	}
}

func TestAPIKeyAuth_RejectsMissingOrMalformedHeader(t *testing.T) {
	// A nil AuthService is safe here: the middleware must reject the request
	// before it ever touches the service.
	h := APIKeyAuth(nil)(okHandler())

	for _, header := range []string{"", "Basic abc", "gw_live_abc"} {
		req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status = %d, want 401", header, rec.Code)
		}
	}
}

func TestRateLimit_EnforcesPerKeyRPM(t *testing.T) {
	h := RateLimit(nil)(okHandler())

	withKey := func(id int64, rpm int) *http.Request {
		key := &models.APIKey{ID: id, RateLimitRPM: rpm}
		ctx := context.WithValue(context.Background(), ctxAPIKey, key)
		return httptest.NewRequest(http.MethodPost, "/v1/messages", nil).WithContext(ctx)
	}

	// Key 1 has a burst of 2: two requests pass, the third is throttled.
	for i, want := range []int{200, 200, 429} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, withKey(1, 2))
		if rec.Code != want {
			t.Fatalf("request %d: status = %d, want %d", i+1, rec.Code, want)
		}
	}

	// Key 2 is independent and still has its full budget.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withKey(2, 2))
	if rec.Code != http.StatusOK {
		t.Fatalf("second key should not be throttled, got %d", rec.Code)
	}
}

func TestRateLimit_RequiresAuthFirst(t *testing.T) {
	h := RateLimit(nil)(okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 when no key in context", rec.Code)
	}
}

func TestAPIKeyFromContext(t *testing.T) {
	if _, ok := APIKeyFromContext(context.Background()); ok {
		t.Fatal("expected no key in empty context")
	}
	key := &models.APIKey{ID: 7}
	ctx := context.WithValue(context.Background(), ctxAPIKey, key)
	got, ok := APIKeyFromContext(ctx)
	if !ok || got.ID != 7 {
		t.Fatalf("got %+v ok=%v, want key 7", got, ok)
	}
}
