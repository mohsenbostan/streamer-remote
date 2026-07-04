package commands

import (
	"testing"
)

func TestBlacklistEmptyByDefault(t *testing.T) {
	cfg := testConfig()
	bl := buildBlacklist(cfg)
	actions, _ := ParseCombo("lwin", cfg)
	if reason := bl.Check(actions); reason != "" {
		t.Fatalf("expected no built-in restrictions, got: %s", reason)
	}
}

func TestBlacklistDeniedKey(t *testing.T) {
	cfg := testConfig()
	cfg.Blacklist.DeniedKeys = []string{"f4"}
	bl := buildBlacklist(cfg)
	actions, _ := ParseCombo("f4", cfg)
	if reason := bl.Check(actions); reason == "" {
		t.Fatal("expected f4 to be blocked")
	}
}

func TestBlacklistDeniedComboWithAlias(t *testing.T) {
	cfg := testConfig()
	cfg.Blacklist.DeniedCombos = [][]string{{"alt", "f4"}}
	bl := buildBlacklist(cfg)

	// "lalt" should be caught by a generic "alt" rule via alias grouping.
	actions, _ := ParseCombo("lalt+f4", cfg)
	if reason := bl.Check(actions); reason == "" {
		t.Fatal("expected lalt+f4 to be blocked by the generic alt+f4 rule")
	}

	actions, _ = ParseCombo("f4", cfg)
	if reason := bl.Check(actions); reason != "" {
		t.Fatal("f4 alone must not be blocked by a combo-only rule")
	}
}

func TestCanonicalKey(t *testing.T) {
	if canonicalKey("lctrl") != "ctrl" {
		t.Fatal("expected lctrl to canonicalize to ctrl")
	}
	if canonicalKey("w") != "w" {
		t.Fatal("expected unrelated key to pass through unchanged")
	}
}
