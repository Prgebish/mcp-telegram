package ratelimit

import (
	"context"

	"github.com/chestnykh/mcp-telegram/internal/config"
	"golang.org/x/time/rate"
)

type Limiter struct {
	limiter *rate.Limiter
}

func New(cfg config.RateConfig) *Limiter {
	return &Limiter{
		limiter: rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst),
	}
}

func (l *Limiter) Wait(ctx context.Context) error {
	return l.limiter.Wait(ctx)
}
