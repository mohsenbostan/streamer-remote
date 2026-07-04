package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoadCreatesDefaultOnFirstRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	_, err := Load(path)
	if !errors.Is(err, ErrDefaultCreated) {
		t.Fatalf("expected ErrDefaultCreated, got %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected default config to be valid on second load, got %v", err)
	}
	if cfg.Prefix != "rc!" {
		t.Fatalf("expected default prefix 'rc!', got %q", cfg.Prefix)
	}
}

func TestTwitchValidateRequiresFields(t *testing.T) {
	if err := (Twitch{}).Validate(); err == nil {
		t.Fatal("expected error for empty twitch config")
	}
	if err := (Twitch{Channel: "foo", ClientID: "bar"}).Validate(); err != nil {
		t.Fatalf("expected valid twitch config to pass, got %v", err)
	}
}

func TestUpdateTwitchFieldsPersistsOnFreshTemplate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := Load(path); !errors.Is(err, ErrDefaultCreated) {
		t.Fatalf("expected ErrDefaultCreated, got %v", err)
	}

	if err := UpdateTwitchFields(path, "mystreamer", "abc123"); err != nil {
		t.Fatalf("UpdateTwitchFields failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to reload cleanly after update, got %v", err)
	}
	if cfg.Twitch.Channel != "mystreamer" || cfg.Twitch.ClientID != "abc123" {
		t.Fatalf("expected saved twitch fields, got %+v", cfg.Twitch)
	}
	if err := cfg.Twitch.Validate(); err != nil {
		t.Fatalf("expected twitch config to validate after setup, got %v", err)
	}
	// The bug this guards against: setup must not need to run twice.
	if err := cfg.Twitch.Validate(); err != nil {
		t.Fatalf("expected setup to be idempotent, got %v", err)
	}
}

// TestUpdateTwitchFieldsAddsMissingKeysToOldFormatFile simulates upgrading
// the binary against a config.yaml from a release that didn't have these
// keys at all (or had them named/placed differently). The fix must add
// the keys rather than silently no-op, or setup loops forever.
func TestUpdateTwitchFieldsAddsMissingKeysToOldFormatFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	oldFormat := "twitch:\n  botUsername: \"\"\n  oauthToken: \"\"\nprefix: \"!\"\n"
	if err := os.WriteFile(path, []byte(oldFormat), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := UpdateTwitchFields(path, "mystreamer", "abc123"); err != nil {
		t.Fatalf("UpdateTwitchFields failed on old-format file: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("expected updated file to still be valid yaml: %v", err)
	}
	if cfg.Twitch.Channel != "mystreamer" || cfg.Twitch.ClientID != "abc123" {
		t.Fatalf("expected new keys to be added, got %+v", cfg.Twitch)
	}
	if cfg.Prefix != "!" {
		t.Fatalf("expected unrelated old field 'prefix' to survive untouched, got %q", cfg.Prefix)
	}
}

func TestValidateRejectsBadLimits(t *testing.T) {
	cfg := &Config{
		Prefix:       "rc!",
		MaxComboSize: 99,
		TapHoldMs:    40,
		MaxHoldMs:    3000,
		MaxMoveStep:  300,
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for out-of-range maxComboSize")
	}
}
