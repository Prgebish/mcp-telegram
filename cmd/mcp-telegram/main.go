package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/chestnykh/mcp-telegram/internal/acl"
	"github.com/chestnykh/mcp-telegram/internal/config"
	"github.com/chestnykh/mcp-telegram/internal/ratelimit"
	tgclient "github.com/chestnykh/mcp-telegram/internal/telegram"
	"github.com/chestnykh/mcp-telegram/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	configPath := flag.String("config", "", "Path to config.yaml (required)")
	flag.Parse()

	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "error: --config flag is required")
		os.Exit(1)
	}

	cfg, err := config.Load(*configPath)
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
	}
	tools.Register(server, deps)

	logger.Info("MCP server starting on stdio")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		logger.Error("MCP server error", "error", err)
		os.Exit(1)
	}
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
