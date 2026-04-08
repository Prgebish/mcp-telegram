package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram TelegramConfig `yaml:"telegram"`
	ACL      ACLConfig      `yaml:"acl"`
	Limits   LimitsConfig   `yaml:"limits"`
	Media    MediaConfig    `yaml:"media"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type TelegramConfig struct {
	AppID       int    `yaml:"app_id"`
	APIHash     string `yaml:"api_hash"`
	SessionPath string `yaml:"session_path"`
}

type ACLConfig struct {
	Chats []ChatRule `yaml:"chats"`
}

type ChatRule struct {
	Match       string       `yaml:"match"`
	Permissions []Permission `yaml:"permissions"`
}

type Permission string

const (
	PermRead     Permission = "read"
	PermSend     Permission = "send"
	PermDraft    Permission = "draft"
	PermMarkRead Permission = "mark_read"
)

type LimitsConfig struct {
	MaxMessagesPerRequest int        `yaml:"max_messages_per_request"`
	MaxDialogsPerRequest  int        `yaml:"max_dialogs_per_request"`
	Rate                  RateConfig `yaml:"rate"`
}

type RateConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             int     `yaml:"burst"`
}

type MediaConfig struct {
	Download          []string `yaml:"download"`            // media types to auto-download: photo, document, video, voice, audio
	Directory         string   `yaml:"directory"`           // where to save downloaded media
	AllowedUploadDirs []string `yaml:"allowed_upload_dirs"` // directories from which tg_send can read files
}

func (m *MediaConfig) ShouldDownload(mediaType string) bool {
	for _, t := range m.Download {
		if t == mediaType {
			return true
		}
	}
	return false
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	File  string `yaml:"file"`
}

// LoadTelegram loads only the telegram section from config.
// Used by the auth command which doesn't need ACL or other settings.
func LoadTelegram(path string) (*TelegramConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Telegram.SessionPath == "" {
		home, _ := os.UserHomeDir()
		cfg.Telegram.SessionPath = home + "/.config/mcp-telegram/session.json"
	} else {
		cfg.Telegram.SessionPath = expandTilde(cfg.Telegram.SessionPath)
	}

	if cfg.Telegram.AppID == 0 {
		return nil, fmt.Errorf("telegram.app_id is required")
	}
	if cfg.Telegram.APIHash == "" {
		return nil, fmt.Errorf("telegram.api_hash is required")
	}

	return &cfg.Telegram, nil
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	applyDefaults(&cfg)

	if err := validate(&cfg); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	return &cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Telegram.SessionPath == "" {
		home, _ := os.UserHomeDir()
		cfg.Telegram.SessionPath = home + "/.config/mcp-telegram/session.json"
	} else {
		cfg.Telegram.SessionPath = expandTilde(cfg.Telegram.SessionPath)
	}
	if cfg.Limits.MaxMessagesPerRequest == 0 {
		cfg.Limits.MaxMessagesPerRequest = 50
	}
	if cfg.Limits.MaxDialogsPerRequest == 0 {
		cfg.Limits.MaxDialogsPerRequest = 100
	}
	if cfg.Limits.Rate.RequestsPerSecond == 0 {
		cfg.Limits.Rate.RequestsPerSecond = 2.0
	}
	if cfg.Limits.Rate.Burst == 0 {
		cfg.Limits.Rate.Burst = 3
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Media.Directory != "" {
		cfg.Media.Directory = expandTilde(cfg.Media.Directory)
	}
	for i, dir := range cfg.Media.AllowedUploadDirs {
		cfg.Media.AllowedUploadDirs[i] = expandTilde(dir)
	}
}

var validPermissions = map[Permission]bool{
	PermRead:     true,
	PermSend:     true,
	PermDraft:    true,
	PermMarkRead: true,
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

func validate(cfg *Config) error {
	if cfg.Telegram.AppID == 0 {
		return fmt.Errorf("telegram.app_id is required")
	}
	if cfg.Telegram.APIHash == "" {
		return fmt.Errorf("telegram.api_hash is required")
	}
	if len(cfg.ACL.Chats) == 0 {
		return fmt.Errorf("acl.chats must contain at least one entry")
	}
	for i, rule := range cfg.ACL.Chats {
		if rule.Match == "" {
			return fmt.Errorf("acl.chats[%d].match is required", i)
		}
		if !isValidMatch(rule.Match) {
			return fmt.Errorf("acl.chats[%d].match %q: must start with @, +, or be user:/chat:/channel: prefixed", i, rule.Match)
		}
		if len(rule.Permissions) == 0 {
			return fmt.Errorf("acl.chats[%d].permissions must not be empty", i)
		}
		for _, p := range rule.Permissions {
			if !validPermissions[p] {
				return fmt.Errorf("acl.chats[%d].permissions: unknown permission %q", i, p)
			}
		}
	}
	if !validLogLevels[cfg.Logging.Level] {
		return fmt.Errorf("logging.level %q is invalid, must be one of: debug, info, warn, error", cfg.Logging.Level)
	}
	if cfg.Limits.MaxMessagesPerRequest < 0 {
		return fmt.Errorf("limits.max_messages_per_request must be positive")
	}
	if cfg.Limits.MaxDialogsPerRequest < 0 {
		return fmt.Errorf("limits.max_dialogs_per_request must be positive")
	}
	if cfg.Limits.Rate.RequestsPerSecond <= 0 {
		return fmt.Errorf("limits.rate.requests_per_second must be positive")
	}
	if cfg.Limits.Rate.Burst <= 0 {
		return fmt.Errorf("limits.rate.burst must be positive")
	}
	validMediaTypes := map[string]bool{
		"photo": true, "document": true, "video": true, "voice": true, "audio": true,
	}
	for _, mt := range cfg.Media.Download {
		if !validMediaTypes[mt] {
			return fmt.Errorf("media.download: unknown media type %q (valid: photo, document, video, voice, audio)", mt)
		}
	}
	if len(cfg.Media.Download) > 0 && cfg.Media.Directory == "" {
		return fmt.Errorf("media.directory is required when media.download is set")
	}
	return nil
}

func expandTilde(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home + path[1:]
	}
	return path
}

func isValidMatch(m string) bool {
	if strings.HasPrefix(m, "@") || strings.HasPrefix(m, "+") {
		return len(m) > 1
	}
	for _, prefix := range []string{"user:", "chat:", "channel:"} {
		if strings.HasPrefix(m, prefix) {
			return len(m) > len(prefix)
		}
	}
	return false
}
