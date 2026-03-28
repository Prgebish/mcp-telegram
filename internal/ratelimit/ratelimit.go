package ratelimit

import (
	"context"

	"github.com/chestnykh/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"golang.org/x/time/rate"
)

// Limiter wraps rate.Limiter and provides a telegram.Middleware
// that rate-limits every Telegram RPC call.
type Limiter struct {
	limiter *rate.Limiter
}

func New(cfg config.RateConfig) *Limiter {
	return &Limiter{
		limiter: rate.NewLimiter(rate.Limit(cfg.RequestsPerSecond), cfg.Burst),
	}
}

// Wait blocks until the rate limiter allows one event.
func (l *Limiter) Wait(ctx context.Context) error {
	return l.limiter.Wait(ctx)
}

// Middleware returns a telegram.Middleware that rate-limits every RPC call.
func (l *Limiter) Middleware() telegram.Middleware {
	return telegram.MiddlewareFunc(func(next tg.Invoker) telegram.InvokeFunc {
		return func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			if err := l.limiter.Wait(ctx); err != nil {
				return err
			}
			return next.Invoke(ctx, input, output)
		}
	})
}
