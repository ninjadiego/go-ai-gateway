package service

import (
	"context"

	"github.com/ninjadiego/go-ai-gateway/internal/models"
	"github.com/ninjadiego/go-ai-gateway/internal/repository"
)

type AnalyticsService struct {
	repo *repository.UsageRepo
}

func NewAnalyticsService(repo *repository.UsageRepo) *AnalyticsService {
	return &AnalyticsService{repo: repo}
}

func (s *AnalyticsService) DailyUsage(ctx context.Context, apiKeyID int64, days int) ([]models.DailyUsage, error) {
	if days <= 0 {
		days = 30
	}
	return s.repo.DailyUsageForKey(ctx, apiKeyID, days)
}

func (s *AnalyticsService) MonthlyCost(ctx context.Context, apiKeyID int64) (float64, error) {
	return s.repo.MonthlyCostUSD(ctx, apiKeyID)
}

func (s *AnalyticsService) Overview(ctx context.Context, days int) (*repository.AnalyticsOverview, error) {
	if days <= 0 {
		days = 30
	}
	return s.repo.Analytics(ctx, days)
}
