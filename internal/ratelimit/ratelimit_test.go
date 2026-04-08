package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/gotd/td/bin"
	tg "github.com/gotd/td/telegram"
)

func TestLimiter_Wait(t *testing.T) {
	l := New(config.RateConfig{RequestsPerSecond: 100, Burst: 10})

	ctx := context.Background()
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestLimiter_WaitCancelled(t *testing.T) {
	// Very slow rate — Wait should block and respect cancellation.
	l := New(config.RateConfig{RequestsPerSecond: 0.001, Burst: 1})

	ctx := context.Background()
	// Exhaust the burst.
	if err := l.Wait(ctx); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := l.Wait(ctx)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestLimiter_Middleware(t *testing.T) {
	l := New(config.RateConfig{RequestsPerSecond: 100, Burst: 10})
	mw := l.Middleware()

	callCount := 0
	inner := tg.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		callCount++
		return nil
	})

	wrapped := mw.Handle(inner)
	ctx := context.Background()

	if err := wrapped.Invoke(ctx, nil, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if callCount != 1 {
		t.Errorf("callCount = %d, want 1", callCount)
	}
}

func TestLimiter_MiddlewareCancelled(t *testing.T) {
	l := New(config.RateConfig{RequestsPerSecond: 0.001, Burst: 1})
	mw := l.Middleware()

	inner := tg.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
		return nil
	})

	wrapped := mw.Handle(inner)

	ctx := context.Background()
	// Exhaust burst.
	if err := wrapped.Invoke(ctx, nil, nil); err != nil {
		t.Fatalf("first Invoke: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := wrapped.Invoke(ctx, nil, nil)
	if err == nil {
		t.Error("expected error when rate limited and context cancelled")
	}
}
