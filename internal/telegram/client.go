package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/chestnykh/mcp-telegram/internal/config"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/peers"
	"github.com/gotd/td/tg"
)

type Client struct {
	cfg    config.TelegramConfig
	tg     *telegram.Client
	api    *tg.Client
	peers  *peers.Manager
	ready  chan struct{}
	done   chan struct{}
	err    error
	cancel context.CancelFunc
	mu     sync.Mutex
}

func New(cfg config.TelegramConfig) *Client {
	return &Client{
		cfg:   cfg,
		ready: make(chan struct{}),
		done:  make(chan struct{}),
	}
}

func (c *Client) Start(ctx context.Context) error {
	sessionDir := filepath.Dir(c.cfg.SessionPath)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}

	storage := &session.FileStorage{Path: c.cfg.SessionPath}

	c.tg = telegram.NewClient(c.cfg.AppID, c.cfg.APIHash, telegram.Options{
		SessionStorage: storage,
	})

	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	go func() {
		defer close(c.done)
		runErr := c.tg.Run(runCtx, func(ctx context.Context) error {
			api := c.tg.API()
			peerManager := peers.Options{}.Build(api)

			c.mu.Lock()
			c.api = api
			c.peers = peerManager
			c.mu.Unlock()

			close(c.ready)
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
