package commands

import (
	"testing"

	"streamer-remote/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Prefix:       "rc!",
		MaxComboSize: 3,
		TapHoldMs:    40,
		MaxHoldMs:    3000,
		MaxMoveStep:  300,
	}
}

func TestTrimPrefix(t *testing.T) {
	body, ok := TrimPrefix("rc!w+shift", "rc!")
	if !ok || body != "w+shift" {
		t.Fatalf("got %q, %v", body, ok)
	}
	if _, ok := TrimPrefix("hello world", "rc!"); ok {
		t.Fatal("expected no match for unrelated chat")
	}
	if _, ok := TrimPrefix("!w", "rc!"); ok {
		t.Fatal("single '!' must not match the multi-char prefix")
	}
}

func TestParseComboSingleKey(t *testing.T) {
	actions, err := ParseCombo("w", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != KindKey || actions[0].Name != "w" {
		t.Fatalf("unexpected actions: %+v", actions)
	}
}

func TestParseComboMultiKey(t *testing.T) {
	actions, err := ParseCombo("w+shift", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 || actions[0].Name != "w" || actions[1].Name != "shift" {
		t.Fatalf("unexpected actions: %+v", actions)
	}
}

func TestParseComboTooLong(t *testing.T) {
	if _, err := ParseCombo("w+a+s+d", testConfig()); err == nil {
		t.Fatal("expected error for combo exceeding MaxComboSize")
	}
}

func TestParseComboUnknownKey(t *testing.T) {
	if _, err := ParseCombo("notakey", testConfig()); err == nil {
		t.Fatal("expected error for unknown key")
	}
}

func TestParseClickAndMoveAndScroll(t *testing.T) {
	actions, err := ParseCombo("click:left", testConfig())
	if err != nil || actions[0].Kind != KindClick || actions[0].Name != "left" {
		t.Fatalf("click parse failed: %+v %v", actions, err)
	}

	actions, err = ParseCombo("move:right:9999", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if actions[0].Amount != 300 {
		t.Fatalf("expected move amount clamped to MaxMoveStep 300, got %d", actions[0].Amount)
	}

	actions, err = ParseCombo("scroll:up:999", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if actions[0].Amount != 10 {
		t.Fatalf("expected scroll amount clamped to 10, got %d", actions[0].Amount)
	}
}

func TestParseHoldClamped(t *testing.T) {
	actions, err := ParseCombo("hold:w:999999", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if actions[0].HoldMs != 3000 {
		t.Fatalf("expected hold clamped to MaxHoldMs 3000, got %d", actions[0].HoldMs)
	}
}

func TestEffectiveHoldMs(t *testing.T) {
	actions := []Action{{Kind: KindKey, Name: "w"}, {Kind: KindKey, Name: "shift", HoldMs: 500}}
	if got := EffectiveHoldMs(actions, 40); got != 500 {
		t.Fatalf("expected 500, got %d", got)
	}
}
