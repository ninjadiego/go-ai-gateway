package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/ninjadiego/go-ai-gateway/internal/config"
	"github.com/ninjadiego/go-ai-gateway/internal/handlers"
	"github.com/ninjadiego/go-ai-gateway/internal/middleware"
	"github.com/ninjadiego/go-ai-gateway/internal/providers"
	"github.com/ninjadiego/go-ai-gateway/internal/repository"
	"github.com/ninjadiego/go-ai-gateway/internal/service"
)

type Server struct {
	cfg *config.Config
	db  *sql.DB

	apiKeyRepo *repository.APIKeyRepo
	usageRepo  *repository.UsageRepo

	authSvc      *service.AuthService
	analyticsSvc *service.AnalyticsService

	anthropic *providers.Anthropic
}

func New(cfg *config.Config, db *sql.DB) *Server {
	apiKeyRepo := repository.NewAPIKeyRepo(db)
	usageRepo := repository.NewUsageRepo(db)

	return &Server{
		cfg:          cfg,
		db:           db,
		apiKeyRepo:   apiKeyRepo,
		usageRepo:    usageRepo,
		authSvc:      service.NewAuthService(apiKeyRepo),
		analyticsSvc: service.NewAnalyticsService(usageRepo),
		anthropic:    providers.NewAnthropic(cfg.Anthropic, cfg.UpstreamTimeout),
	}
}

func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(120 * time.Second))
	r.Use(middleware.Logger())

	// Public
	r.Get("/health", s.healthHandler)
	r.Get("/ready", s.readyHandler)

	// Tenant-scoped proxy (requires gateway API key)
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.APIKeyAuth(s.authSvc))
		r.Use(middleware.RateLimit(s.authSvc))

		proxy := handlers.NewProxy(s.anthropic, s.usageRepo)
		r.Post("/messages", proxy.Messages)
	})

	// Admin
	r.Route("/admin", func(r chi.Router) {
		r.Use(middleware.AdminAuth(s.cfg.AdminToken))

		admin := handlers.NewAdmin(s.authSvc, s.analyticsSvc)
		r.Post("/keys", admin.CreateKey)
		r.Get("/keys", admin.ListKeys)
		r.Get("/keys/{id}/usage", admin.KeyUsage)
		r.Delete("/keys/{id}", admin.RevokeKey)
		r.Get("/analytics", admin.Analytics)
	})

	return r
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	if err := s.db.PingContext(r.Context()); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unavailable",
			"error":  "database not reachable",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
