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
	if !cfg.TextToSpeechEnabled {
		t.Fatal("expected text-to-speech to be enabled by default")
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

func TestUpdateKickChannelPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := Load(path); !errors.Is(err, ErrDefaultCreated) {
		t.Fatalf("expected ErrDefaultCreated, got %v", err)
	}

	if err := UpdateKickChannel(path, "mystreamer"); err != nil {
		t.Fatalf("UpdateKickChannel failed: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to reload cleanly after update, got %v", err)
	}
	if cfg.Kick.Channel != "mystreamer" {
		t.Fatalf("expected saved kick channel, got %+v", cfg.Kick)
	}

	if err := UpdateKickChannel(path, ""); err != nil {
		t.Fatalf("UpdateKickChannel clear failed: %v", err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Kick.Channel != "" {
		t.Fatalf("expected kick channel cleared, got %q", cfg.Kick.Channel)
	}
}

func TestAddAndRemoveRewardAction(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := Load(path); !errors.Is(err, ErrDefaultCreated) {
		t.Fatalf("expected ErrDefaultCreated, got %v", err)
	}

	ra := RewardAction{Action: "alt+f4", RewardTitle: "Rage Quit", Cost: 500, RewardID: "reward-1"}
	if err := AddRewardAction(path, ra); err != nil {
		t.Fatalf("AddRewardAction failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected config to reload cleanly, got %v", err)
	}
	if len(cfg.RewardActions) != 1 || cfg.RewardActions[0] != ra {
		t.Fatalf("expected saved reward action, got %+v", cfg.RewardActions)
	}

	if err := AddRewardAction(path, RewardAction{Action: "lwin", RewardTitle: "Lock Screen", Cost: 1000, RewardID: "reward-2"}); err != nil {
		t.Fatalf("second AddRewardAction failed: %v", err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RewardActions) != 2 {
		t.Fatalf("expected 2 reward actions, got %d", len(cfg.RewardActions))
	}

	if err := RemoveRewardAction(path, "reward-1"); err != nil {
		t.Fatalf("RemoveRewardAction failed: %v", err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RewardActions) != 1 || cfg.RewardActions[0].RewardID != "reward-2" {
		t.Fatalf("expected only reward-2 to remain, got %+v", cfg.RewardActions)
	}

	// Removing an already-gone ID must be a harmless no-op, not an error.
	if err := RemoveRewardAction(path, "reward-1"); err != nil {
		t.Fatalf("expected removing a missing rewardId to be a no-op, got %v", err)
	}
}

func TestRewardProfileSaveDeleteAndActivation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if _, err := Load(path); !errors.Is(err, ErrDefaultCreated) {
		t.Fatalf("expected ErrDefaultCreated, got %v", err)
	}

	speedrun := RewardProfile{
		Name: "Speedrun",
		Rewards: []RewardAction{
			{Action: "alt+f4", RewardTitle: "Rage Quit", Cost: 500},
		},
	}
	if err := SaveRewardProfile(path, speedrun); err != nil {
		t.Fatalf("SaveRewardProfile failed: %v", err)
	}
	chill := RewardProfile{
		Name: "Chill",
		Rewards: []RewardAction{
			{Action: "lwin", RewardTitle: "Lock Screen", Cost: 1000},
		},
	}
	if err := SaveRewardProfile(path, chill); err != nil {
		t.Fatalf("second SaveRewardProfile failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RewardProfiles) != 2 {
		t.Fatalf("expected 2 reward profiles, got %d", len(cfg.RewardProfiles))
	}

	// Saving again under an existing name overwrites rather than duplicating.
	speedrun.Rewards[0].Cost = 750
	if err := SaveRewardProfile(path, speedrun); err != nil {
		t.Fatalf("overwrite SaveRewardProfile failed: %v", err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RewardProfiles) != 2 {
		t.Fatalf("expected overwrite to keep 2 profiles, got %d", len(cfg.RewardProfiles))
	}

	if err := SetActiveRewardProfile(path, "Speedrun"); err != nil {
		t.Fatalf("SetActiveRewardProfile failed: %v", err)
	}
	if err := AddRewardAction(path, RewardAction{Action: "alt+f4", RewardTitle: "Rage Quit", Cost: 750, RewardID: "reward-live"}); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveRewardProfile != "Speedrun" {
		t.Fatalf("expected active profile 'Speedrun', got %q", cfg.ActiveRewardProfile)
	}
	if len(cfg.RewardActions) != 1 || cfg.RewardActions[0].Cost != 750 {
		t.Fatalf("expected the active profile's reward to be live, got %+v", cfg.RewardActions)
	}

	if err := ClearRewardActions(path); err != nil {
		t.Fatalf("ClearRewardActions failed: %v", err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RewardActions) != 0 {
		t.Fatalf("expected rewardActions cleared, got %+v", cfg.RewardActions)
	}

	// Deleting the currently active profile must also clear the pointer to it.
	if err := DeleteRewardProfile(path, "Speedrun"); err != nil {
		t.Fatalf("DeleteRewardProfile failed: %v", err)
	}
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.RewardProfiles) != 1 || cfg.RewardProfiles[0].Name != "Chill" {
		t.Fatalf("expected only 'Chill' to remain, got %+v", cfg.RewardProfiles)
	}
	if cfg.ActiveRewardProfile != "" {
		t.Fatalf("expected activeRewardProfile cleared after deleting it, got %q", cfg.ActiveRewardProfile)
	}

	// Deleting an already-gone name must be a harmless no-op.
	if err := DeleteRewardProfile(path, "Speedrun"); err != nil {
		t.Fatalf("expected deleting a missing profile to be a no-op, got %v", err)
	}
}

// TestLoadFillsDefaultsForFieldsMissingFromOldConfig reproduces the exact
// bug reported after v1.2.0 shipped maxSequenceSteps: a config.yaml
// written by an older release has no such key, so it unmarshals to 0 —
// and validate() rejected 0 outright, crashing the app on startup for
// every existing install. Load must fill in a working default instead.
func TestLoadFillsDefaultsForFieldsMissingFromOldConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	oldFormat := "twitch:\n  channel: \"mystreamer\"\n  clientId: \"abc123\"\n" +
		"prefix: \"rc!\"\n" +
		"maxComboSize: 3\n" +
		"tapHoldMs: 40\n" +
		"maxHoldMs: 3000\n" +
		"maxMoveStep: 300\n"
	// Deliberately no maxSequenceSteps key, simulating a pre-v1.2.0 file.
	if err := os.WriteFile(path, []byte(oldFormat), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected an old config missing a newer field to still load, got: %v", err)
	}
	if cfg.MaxSequenceSteps != 6 {
		t.Fatalf("expected missing maxSequenceSteps to default to 6, got %d", cfg.MaxSequenceSteps)
	}
	if !cfg.TextToSpeechEnabled {
		t.Fatal("expected missing textToSpeechEnabled to default to true")
	}
	// Everything else in the old file must survive untouched.
	if cfg.Twitch.Channel != "mystreamer" || cfg.MaxComboSize != 3 {
		t.Fatalf("expected existing fields to be preserved, got %+v", cfg)
	}
}

func TestValidateRejectsBadLimits(t *testing.T) {
	cfg := &Config{
		Prefix:           "rc!",
		MaxComboSize:     99,
		MaxSequenceSteps: 4,
		TapHoldMs:        40,
		MaxHoldMs:        3000,
		MaxMoveStep:      300,
	}
	if err := cfg.validate(); err == nil {
		t.Fatal("expected error for out-of-range maxComboSize")
	}
}
