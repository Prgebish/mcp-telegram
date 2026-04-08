package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/Prgebish/mcp-telegram/internal/config"
	"github.com/Prgebish/mcp-telegram/internal/ratelimit"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

type Client struct {
	cfg     config.TelegramConfig
	limiter *ratelimit.Limiter
	tg      *telegram.Client
	api     *tg.Client
	peers   *peers.Manager
	ready   chan struct{}
	done    chan struct{}
	err     error
	cancel  context.CancelFunc
	mu      sync.Mutex
}

func New(cfg config.TelegramConfig, limiter *ratelimit.Limiter) *Client {
	return &Client{
		cfg:     cfg,
		limiter: limiter,
		ready:   make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (c *Client) Start(ctx context.Context) error {
	sessionDir := filepath.Dir(c.cfg.SessionPath)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	storage := &session.FileStorage{Path: c.cfg.SessionPath}

	// peerManager is created inside Run callback (needs *tg.Client),
	// but UpdateHandler is set before Run. We use an atomic pointer
	// so the update handler can forward to peerManager.UpdateHook
	// once it's initialized.
	var pmRef atomic.Pointer[peers.Manager]

	// updateHandler extracts entities from Telegram updates and feeds
	// them into peers.Manager via Apply(). This keeps the peer cache
	// up to date for the lifetime of the process.
	updateHandler := telegram.UpdateHandlerFunc(func(ctx context.Context, u tg.UpdatesClass) error {
		pm := pmRef.Load()
		if pm == nil {
			return nil // Not initialized yet.
		}
		users, chats := extractEntities(u)
		if len(users) > 0 || len(chats) > 0 {
			return pm.Apply(ctx, users, chats)
		}
		return nil
	})

	opts := telegram.Options{
		SessionStorage: storage,
		UpdateHandler:  updateHandler,
	}
	if c.limiter != nil {
		opts.Middlewares = []telegram.Middleware{c.limiter.Middleware()}
	}
	c.tg = telegram.NewClient(c.cfg.AppID, c.cfg.APIHash, opts)

	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	go func() {
		defer close(c.done)
		runErr := c.tg.Run(runCtx, func(ctx context.Context) error {
			api := c.tg.API()
			peerManager := peers.Options{}.Build(api)
			pmRef.Store(peerManager)

			// Init peers.Manager (loads self user).
			if err := peerManager.Init(ctx); err != nil {
				return fmt.Errorf("init peers: %w", err)
			}

			// No warm-up: peers are resolved lazily when tools request them.
			// This avoids FLOOD_WAIT from loading all dialogs at startup.

			c.mu.Lock()
			c.api = api
			c.peers = peerManager
			c.mu.Unlock()

			close(c.ready)
			c.enforceSessionPermissions()
			<-ctx.Done()
			return ctx.Err()
		})
		if runErr != nil && runErr != context.Canceled {
			c.mu.Lock()
			c.err = runErr
			c.mu.Unlock()
		}
	}()

	select {
	case <-c.ready:
		return nil
	case <-c.done:
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.err != nil {
			return fmt.Errorf("telegram client failed to start: %w", c.err)
		}
		return fmt.Errorf("telegram client exited unexpectedly")
	case <-ctx.Done():
		cancel()
		return ctx.Err()
	}
}

func (c *Client) API() *tg.Client {
	<-c.ready
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.api
}

func (c *Client) Peers() *peers.Manager {
	<-c.ready
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.peers
}

func (c *Client) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	<-c.done
}

func (c *Client) enforceSessionPermissions() {
	_ = os.Chmod(c.cfg.SessionPath, 0600)
}

// extractEntities pulls Users and Chats from a Telegram update container.
func extractEntities(u tg.UpdatesClass) ([]tg.UserClass, []tg.ChatClass) {
	switch v := u.(type) {
	case *tg.Updates:
		return v.Users, v.Chats
	case *tg.UpdatesCombined:
		return v.Users, v.Chats
	default:
		return nil, nil
	}
}

