package commands

import (
	"testing"

	"streamer-remote/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Prefix:           "rc!",
		MaxComboSize:     3,
		MaxSequenceSteps: 4,
		TapHoldMs:        40,
		MaxHoldMs:        3000,
		MaxMoveStep:      300,
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

func TestParseMoveXY(t *testing.T) {
	actions, err := ParseCombo("move:50:-30", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	a := actions[0]
	if a.Kind != KindMove || a.Name != "xy" || a.Amount != 50 || a.Amount2 != -30 {
		t.Fatalf("unexpected xy move action: %+v", a)
	}
}

func TestParseMoveXYClamped(t *testing.T) {
	actions, err := ParseCombo("move:9999:-9999", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	a := actions[0]
	if a.Amount != 300 || a.Amount2 != -300 {
		t.Fatalf("expected dx/dy clamped to +/-MaxMoveStep 300, got dx=%d dy=%d", a.Amount, a.Amount2)
	}
}

func TestParseSequenceSingleComboIsOneStep(t *testing.T) {
	steps, err := ParseSequence("w+shift", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || len(steps[0].Actions) != 2 {
		t.Fatalf("unexpected steps: %+v", steps)
	}
}

func TestParseSequenceWithWait(t *testing.T) {
	steps, err := ParseSequence("alt+f10,wait:800,enter", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d: %+v", len(steps), steps)
	}
	if len(steps[0].Actions) != 2 || steps[0].Actions[0].Name != "alt" || steps[0].Actions[1].Name != "f10" {
		t.Fatalf("unexpected first step: %+v", steps[0])
	}
	if len(steps[1].Actions) != 0 || steps[1].HoldMs != 800 {
		t.Fatalf("expected a pure 800ms wait step, got %+v", steps[1])
	}
	if len(steps[2].Actions) != 1 || steps[2].Actions[0].Name != "enter" {
		t.Fatalf("unexpected third step: %+v", steps[2])
	}
}

func TestParseSequenceWaitClampedToMaxHoldMs(t *testing.T) {
	steps, err := ParseSequence("wait:999999", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if steps[0].HoldMs != 3000 {
		t.Fatalf("expected wait clamped to MaxHoldMs 3000, got %d", steps[0].HoldMs)
	}
}

func TestParseSequenceTooManySteps(t *testing.T) {
	if _, err := ParseSequence("w,a,s,d,enter", testConfig()); err == nil {
		t.Fatal("expected error for sequence exceeding MaxSequenceSteps (4)")
	}
}

func TestParseSequenceEmptyStep(t *testing.T) {
	if _, err := ParseSequence("w,,enter", testConfig()); err == nil {
		t.Fatal("expected error for an empty step between commas")
	}
}

func TestParseComboOEMKeys(t *testing.T) {
	cases := map[string]string{
		"`":         "grave/tilde/console-toggle key",
		"console":   "friendly alias for the same key",
		"semicolon": ";",
		"comma":     ",",
		"period":    ".",
		"slash":     "/",
	}
	for tok, desc := range cases {
		if _, err := ParseCombo(tok, testConfig()); err != nil {
			t.Errorf("expected %q (%s) to parse, got error: %v", tok, desc, err)
		}
	}
}

// TestParseSequenceTypingText guards the exact scenario a game console
// command needs: tapping letters one after another (comma-separated
// steps), not holding them all down at once (which '+' would do).
func TestParseSequenceTypingText(t *testing.T) {
	cfg := testConfig()
	cfg.MaxSequenceSteps = 8
	steps, err := ParseSequence("console,q,u,i,t,enter", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 6 {
		t.Fatalf("expected 6 sequential taps, got %d: %+v", len(steps), steps)
	}
	// Each step is a single tap, in order; "console" is a friendly alias
	// for the same key as "`" (aliases aren't canonicalized, they just
	// resolve to the same virtual-key code — input.KeyDown looks up
	// whatever name it's given directly).
	for i, want := range []string{"console", "q", "u", "i", "t", "enter"} {
		if len(steps[i].Actions) != 1 || steps[i].Actions[0].Name != want {
			t.Fatalf("step %d: expected single tap of %q, got %+v", i, want, steps[i])
		}
	}
}

func TestParseSequenceInvalidWaitDuration(t *testing.T) {
	if _, err := ParseSequence("wait:0", testConfig()); err == nil {
		t.Fatal("expected error for a non-positive wait duration")
	}
	if _, err := ParseSequence("wait:notanumber", testConfig()); err == nil {
		t.Fatal("expected error for a non-numeric wait duration")
	}
}
