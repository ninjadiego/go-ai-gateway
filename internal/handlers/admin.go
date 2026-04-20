package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/ninjadiego/go-ai-gateway/internal/repository"
	"github.com/ninjadiego/go-ai-gateway/internal/service"
)

type Admin struct {
	auth      *service.AuthService
	analytics *service.AnalyticsService
}

func NewAdmin(auth *service.AuthService, analytics *service.AnalyticsService) *Admin {
	return &Admin{auth: auth, analytics: analytics}
}

// ─── POST /admin/keys ───────────────────────────────────────────────────────

type createKeyRequest struct {
	Name             string   `json:"name"`
	UserID           int64    `json:"user_id"`
	RateLimitRPM     int      `json:"rate_limit_rpm"`
	DailyTokenLimit  int64    `json:"daily_token_limit"`
	MonthlyBudgetUSD *float64 `json:"monthly_budget_usd,omitempty"`
}

type createKeyResponse struct {
	ID      int64  `json:"id"`
	APIKey  string `json:"api_key"`
	Prefix  string `json:"prefix"`
	Message string `json:"message"`
}

func (a *Admin) CreateKey(w http.ResponseWriter, r *http.Request) {
	var in createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, errMsg("invalid_json", err.Error()))
		return
	}

	created, err := a.auth.Create(r.Context(), service.CreateKeyInput{
		UserID:           in.UserID,
		Name:             in.Name,
		RateLimitRPM:     in.RateLimitRPM,
		DailyTokenLimit:  in.DailyTokenLimit,
		MonthlyBudgetUSD: in.MonthlyBudgetUSD,
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errMsg("create_failed", err.Error()))
		return
	}

	writeJSON(w, http.StatusCreated, createKeyResponse{
		ID:      created.ID,
		APIKey:  created.RawKey,
		Prefix:  created.Prefix,
		Message: "Store this key securely — it will not be shown again.",
	})
}

// ─── GET /admin/keys ────────────────────────────────────────────────────────

func (a *Admin) ListKeys(w http.ResponseWriter, r *http.Request) {
	userID, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	keys, err := a.auth.List(r.Context(), userID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errMsg("list_failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

// ─── GET /admin/keys/{id}/usage ─────────────────────────────────────────────

func (a *Admin) KeyUsage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errMsg("bad_id", "invalid key id"))
		return
	}
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))

	daily, err := a.analytics.DailyUsage(r.Context(), id, days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errMsg("usage_failed", err.Error()))
		return
	}
	monthly, err := a.analytics.MonthlyCost(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errMsg("usage_failed", err.Error()))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"api_key_id":         id,
		"daily_usage":        daily,
		"month_to_date_cost": monthly,
	})
}

// ─── DELETE /admin/keys/{id} ────────────────────────────────────────────────

func (a *Admin) RevokeKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errMsg("bad_id", "invalid key id"))
		return
	}
	if err := a.auth.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, repository.ErrAPIKeyNotFound) {
			writeJSON(w, http.StatusNotFound, errMsg("not_found", "api key not found"))
			return
		}
		writeJSON(w, http.StatusInternalServerError, errMsg("revoke_failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

// ─── GET /admin/analytics ───────────────────────────────────────────────────

func (a *Admin) Analytics(w http.ResponseWriter, r *http.Request) {
	days, _ := strconv.Atoi(r.URL.Query().Get("days"))
	ov, err := a.analytics.Overview(r.Context(), days)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errMsg("analytics_failed", err.Error()))
		return
	}
	writeJSON(w, http.StatusOK, ov)
}

// ─── helpers ────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

func errMsg(kind, msg string) map[string]map[string]string {
	return map[string]map[string]string{
		"error": {"type": kind, "message": msg},
	}
}
