package telegram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/chestnykh/mcp-telegram/internal/config"
	"github.com/chestnykh/mcp-telegram/internal/ratelimit"
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

			// Warm up peer cache by fetching all dialogs via raw API
			// and feeding entities into peers.Manager via Apply().
			// The dialog iterator doesn't populate peers.Manager,
			// so we use the raw API to get Users/Chats lists.
			if err := warmUpPeerCache(ctx, api, peerManager); err != nil {
				return fmt.Errorf("warm up peer cache: %w", err)
			}

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

// warmUpPeerCache fetches all dialogs via raw API and feeds the returned
// User/Chat/Channel entities into peers.Manager via Apply(). This ensures
// that access hashes are available for user:ID and channel:ID resolution
// immediately after startup.
func warmUpPeerCache(ctx context.Context, api *tg.Client, pm *peers.Manager) error {
	var offsetPeer tg.InputPeerClass = &tg.InputPeerEmpty{}
	offsetDate := 0
	offsetID := 0

	for {
		result, err := api.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetPeer: offsetPeer,
			OffsetDate: offsetDate,
			OffsetID:   offsetID,
			Limit:      100,
		})
		if err != nil {
			return fmt.Errorf("get dialogs: %w", err)
		}

		switch r := result.(type) {
		case *tg.MessagesDialogs:
			// Non-paginated response — full list.
			return pm.Apply(ctx, r.Users, r.Chats)

		case *tg.MessagesDialogsSlice:
			if err := pm.Apply(ctx, r.Users, r.Chats); err != nil {
				return err
			}
			if len(r.Dialogs) == 0 {
				return nil
			}

			// Build offset for next page from the last dialog.
			last := r.Dialogs[len(r.Dialogs)-1]
			dlg, ok := last.(*tg.Dialog)
			if !ok {
				return nil
			}

			// Build offsetPeer from the dialog's Peer (with access hash
			// from the entities we just applied).
			offsetPeer = peerToInputPeer(dlg.Peer, r.Users, r.Chats)

			// Find the TopMessage in the messages list to get offsetDate.
			// Match by message ID, not by peer comparison (which would
			// be fragile interface pointer equality).
			offsetID = dlg.TopMessage
			msgsByID := make(map[int]int) // msg ID -> Date
			for _, msg := range r.Messages {
				if m, ok := msg.(*tg.Message); ok {
					msgsByID[m.ID] = m.Date
				}
			}
			if date, ok := msgsByID[dlg.TopMessage]; ok {
				offsetDate = date
			}

			if len(r.Dialogs) < 100 {
				return nil
			}

		case *tg.MessagesDialogsNotModified:
			return nil

		default:
			return nil
		}
	}
}

// peerToInputPeer converts a tg.PeerClass to tg.InputPeerClass using
// access hashes from the provided user/chat lists.
func peerToInputPeer(p tg.PeerClass, users []tg.UserClass, chats []tg.ChatClass) tg.InputPeerClass {
	switch peer := p.(type) {
	case *tg.PeerUser:
		for _, u := range users {
			if user, ok := u.(*tg.User); ok && user.ID == peer.UserID {
				return &tg.InputPeerUser{
					UserID:     user.ID,
					AccessHash: user.AccessHash,
				}
			}
		}
		return &tg.InputPeerUser{UserID: peer.UserID}

	case *tg.PeerChat:
		return &tg.InputPeerChat{ChatID: peer.ChatID}

	case *tg.PeerChannel:
		for _, c := range chats {
			if ch, ok := c.(*tg.Channel); ok && ch.ID == peer.ChannelID {
				return &tg.InputPeerChannel{
					ChannelID:  ch.ID,
					AccessHash: ch.AccessHash,
				}
			}
		}
		return &tg.InputPeerChannel{ChannelID: peer.ChannelID}

	default:
		return &tg.InputPeerEmpty{}
	}
}
