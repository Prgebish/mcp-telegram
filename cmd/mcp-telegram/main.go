package main

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/chestnykh/mcp-telegram/internal/acl"
	"github.com/chestnykh/mcp-telegram/internal/config"
	"github.com/chestnykh/mcp-telegram/internal/ratelimit"
	tgclient "github.com/chestnykh/mcp-telegram/internal/telegram"
	"github.com/chestnykh/mcp-telegram/internal/tools"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/term"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "auth":
		runAuth()
	case "serve":
		runServe()
	case "--config":
		// Backward compat: ./mcp-telegram --config path
		os.Args = append([]string{os.Args[0], "serve"}, os.Args[1:]...)
		runServe()
	case "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  mcp-telegram auth  --config config.yaml   Authenticate with Telegram
  mcp-telegram serve --config config.yaml   Start MCP server (default)`)
}

// --- auth command ---

func runAuth() {
	configPath := findFlag("--config", os.Args[2:])
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config flag is required")
		os.Exit(1)
	}

	// Auth only needs telegram credentials, not the full config with ACL.
	tgCfg, err := config.LoadTelegram(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sessionDir := filepath.Dir(tgCfg.SessionPath)
	if err := os.MkdirAll(sessionDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "error creating session directory: %v\n", err)
		os.Exit(1)
	}

	storage := &session.FileStorage{Path: tgCfg.SessionPath}
	client := telegram.NewClient(tgCfg.AppID, tgCfg.APIHash, telegram.Options{
		SessionStorage: storage,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	reader := bufio.NewReader(os.Stdin)

	err = client.Run(ctx, func(ctx context.Context) error {
		// Check if already authenticated — skip login flow if so.
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return fmt.Errorf("check auth status: %w", err)
		}
		if status.Authorized {
			user := status.User
			fmt.Printf("Already authenticated as %s %s (@%s)\n", user.FirstName, user.LastName, user.Username)
			fmt.Printf("Session: %s\n", tgCfg.SessionPath)
			return nil
		}

		fmt.Print("Phone number (e.g. +79001234567): ")
		phone, _ := reader.ReadString('\n')
		phone = strings.TrimSpace(phone)
		if phone == "" {
			return fmt.Errorf("phone number is required")
		}

		flow := auth.NewFlow(
			terminalAuth{phone: phone, reader: reader},
			auth.SendCodeOptions{},
		)

		if err := flow.Run(ctx, client.Auth()); err != nil {
			return err
		}

		fmt.Println("Authentication successful!")
		fmt.Printf("Session saved to: %s\n", tgCfg.SessionPath)
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// terminalAuth implements auth.UserAuthenticator for interactive terminal login.
type terminalAuth struct {
	phone  string
	reader *bufio.Reader
}

func (a terminalAuth) Phone(_ context.Context) (string, error) {
	return a.phone, nil
}

func (a terminalAuth) Code(_ context.Context, _ *tg.AuthSentCode) (string, error) {
	fmt.Print("Enter code from Telegram: ")
	code, _ := a.reader.ReadString('\n')
	return strings.TrimSpace(code), nil
}

func (a terminalAuth) Password(_ context.Context) (string, error) {
	fmt.Print("Enter 2FA password: ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // newline after hidden input
	if err != nil {
		return "", fmt.Errorf("read password: %w", err)
	}
	return string(password), nil
}

func (a terminalAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

func (a terminalAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up not supported")
}

// --- serve command ---

func runServe() {
	configPath := findFlag("--config", os.Args[2:])
	if configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config flag is required")
		os.Exit(1)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	logger := setupLogging(cfg.Logging)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	checker, err := acl.NewChecker(cfg.ACL)
	if err != nil {
		logger.Error("failed to initialize ACL", "error", err)
		os.Exit(1)
	}

	limiter := ratelimit.New(cfg.Limits.Rate)

	client := tgclient.New(cfg.Telegram, limiter)
	logger.Info("connecting to Telegram...")
	if err := client.Start(ctx); err != nil {
		logger.Error("failed to start Telegram client", "error", err)
		os.Exit(1)
	}
	defer client.Stop()
	logger.Info("connected to Telegram")

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "mcp-telegram",
		Version: "0.1.0",
	}, nil)

	deps := &tools.Deps{
		Client: client,
		ACL:    checker,
		Limits: cfg.Limits,
		Media:  cfg.Media,
	}
	tools.Register(server, deps)

	logger.Info("MCP server starting on stdio")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		logger.Error("MCP server error", "error", err)
		os.Exit(1)
	}
}

// --- helpers ---

func findFlag(name string, args []string) string {
	for i, arg := range args {
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func setupLogging(cfg config.LoggingConfig) *slog.Logger {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	writer := os.Stderr
	if cfg.File != "" {
		f, err := os.OpenFile(cfg.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: cannot open log file %s: %v, using stderr\n", cfg.File, err)
		} else {
			writer = f
		}
	}

	logger := slog.New(slog.NewTextHandler(writer, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)
	return logger
}
