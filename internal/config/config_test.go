package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
telegram:
  app_id: 12345
  api_hash: abc123
acl:
  chats:
    - match: "@alice"
      permissions: [read, draft]
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Telegram.AppID != 12345 {
		t.Errorf("app_id = %d, want 12345", cfg.Telegram.AppID)
	}
	if cfg.Telegram.APIHash != "abc123" {
		t.Errorf("api_hash = %q, want %q", cfg.Telegram.APIHash, "abc123")
	}
	if len(cfg.ACL.Chats) != 1 {
		t.Fatalf("len(chats) = %d, want 1", len(cfg.ACL.Chats))
	}
	if cfg.ACL.Chats[0].Match != "@alice" {
		t.Errorf("match = %q, want %q", cfg.ACL.Chats[0].Match, "@alice")
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
telegram:
  app_id: 1
  api_hash: x
acl:
  chats:
    - match: "@test"
      permissions: [read]
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Limits.MaxMessagesPerRequest != 50 {
		t.Errorf("max_messages = %d, want 50", cfg.Limits.MaxMessagesPerRequest)
	}
	if cfg.Limits.MaxDialogsPerRequest != 100 {
		t.Errorf("max_dialogs = %d, want 100", cfg.Limits.MaxDialogsPerRequest)
	}
	if cfg.Limits.Rate.RequestsPerSecond != 2.0 {
		t.Errorf("rps = %f, want 2.0", cfg.Limits.Rate.RequestsPerSecond)
	}
	if cfg.Limits.Rate.Burst != 3 {
		t.Errorf("burst = %d, want 3", cfg.Limits.Rate.Burst)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("level = %q, want %q", cfg.Logging.Level, "info")
	}
}

func TestLoad_EnvExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	t.Setenv("TEST_APP_ID", "99999")
	t.Setenv("TEST_API_HASH", "secrethash")

	yaml := `
telegram:
  app_id: ${TEST_APP_ID}
  api_hash: ${TEST_API_HASH}
acl:
  chats:
    - match: "@test"
      permissions: [read]
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Telegram.AppID != 99999 {
		t.Errorf("app_id = %d, want 99999", cfg.Telegram.AppID)
	}
	if cfg.Telegram.APIHash != "secrethash" {
		t.Errorf("api_hash = %q, want %q", cfg.Telegram.APIHash, "secrethash")
	}
}

func TestLoad_MissingAppID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
telegram:
  api_hash: x
acl:
  chats:
    - match: "@test"
      permissions: [read]
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for missing app_id")
	}
}

func TestLoad_EmptyChats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
telegram:
  app_id: 1
  api_hash: x
acl:
  chats: []
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for empty chats")
	}
}

func TestLoad_InvalidPermission(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
telegram:
  app_id: 1
  api_hash: x
acl:
  chats:
    - match: "@test"
      permissions: [read, delete]
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for unknown permission 'delete'")
	}
}

func TestLoad_InvalidMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
telegram:
  app_id: 1
  api_hash: x
acl:
  chats:
    - match: "justtext"
      permissions: [read]
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid match format")
	}
}

func TestLoad_TildeExpansion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
telegram:
  app_id: 1
  api_hash: x
  session_path: ~/.config/mcp-telegram/session.json
acl:
  chats:
    - match: "@test"
      permissions: [read]
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.Telegram.SessionPath == "~/.config/mcp-telegram/session.json" {
		t.Error("tilde should be expanded in session_path")
	}
	home, _ := os.UserHomeDir()
	expected := home + "/.config/mcp-telegram/session.json"
	if cfg.Telegram.SessionPath != expected {
		t.Errorf("session_path = %q, want %q", cfg.Telegram.SessionPath, expected)
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	yaml := `
telegram:
  app_id: 1
  api_hash: x
acl:
  chats:
    - match: "@test"
      permissions: [read]
logging:
  level: trace
`
	if err := os.WriteFile(path, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Error("expected error for invalid log level")
	}
}
