package commands

import (
	"fmt"
	"strconv"
	"strings"

	"streamer-remote/internal/config"
	"streamer-remote/internal/input"
)

// Reserved control words: never treated as key/mouse combos, always
// available regardless of blacklist config, gated on Moderator+ by the
// dispatcher so a mod can kill or restore the remote instantly.
const (
	ControlPause  = "pause"
	ControlResume = "resume"
)

// TrimPrefix strips the configured trigger prefix from a raw chat message.
// It returns ok=false if the message doesn't start with it.
func TrimPrefix(text, prefix string) (string, bool) {
	if !strings.HasPrefix(text, prefix) {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(text, prefix)), true
}

// ParseSequence splits a command body on ',' into an ordered list of
// Steps, each either a combo (parsed by ParseCombo) or a "wait:<ms>"
// pure delay — e.g. "alt+f10,wait:800,enter" opens a menu, gives it time
// to animate in, then confirms. A single combo with no ',' is just a
// one-step sequence.
func ParseSequence(body string, cfg *config.Config) ([]Step, error) {
	rawSteps := strings.Split(body, ",")
	if len(rawSteps) > cfg.MaxSequenceSteps {
		return nil, fmt.Errorf("sequence too long: %d steps, max is %d", len(rawSteps), cfg.MaxSequenceSteps)
	}

	steps := make([]Step, 0, len(rawSteps))
	for _, raw := range rawSteps {
		raw = strings.TrimSpace(strings.ToLower(raw))
		if raw == "" {
			return nil, fmt.Errorf("empty step in sequence")
		}

		if ms, ok := strings.CutPrefix(raw, "wait:"); ok {
			n, err := strconv.Atoi(ms)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid wait duration in %q", raw)
			}
			if n > cfg.MaxHoldMs {
				n = cfg.MaxHoldMs
			}
			steps = append(steps, Step{HoldMs: n})
			continue
		}

		actions, err := ParseCombo(raw, cfg)
		if err != nil {
			return nil, err
		}
		steps = append(steps, Step{Actions: actions, HoldMs: EffectiveHoldMs(actions, cfg.TapHoldMs)})
	}
	return steps, nil
}

// ParseCombo splits a command body on '+' into individual actions.
func ParseCombo(body string, cfg *config.Config) ([]Action, error) {
	tokens := strings.Split(strings.ToLower(body), "+")
	if len(tokens) == 0 || (len(tokens) == 1 && tokens[0] == "") {
		return nil, fmt.Errorf("empty command")
	}
	if len(tokens) > cfg.MaxComboSize {
		return nil, fmt.Errorf("combo too long: %d keys, max is %d", len(tokens), cfg.MaxComboSize)
	}

	actions := make([]Action, 0, len(tokens))
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			return nil, fmt.Errorf("empty token in combo")
		}
		action, err := parseToken(tok, cfg)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, nil
}

func parseToken(tok string, cfg *config.Config) (Action, error) {
	parts := strings.Split(tok, ":")

	switch parts[0] {
	case "click":
		if len(parts) != 2 || !input.IsMouseButtonKnown(parts[1]) {
			return Action{}, fmt.Errorf("invalid click token %q (use click:left, click:right, or click:middle)", tok)
		}
		return Action{Kind: KindClick, Name: parts[1]}, nil

	case "move":
		if len(parts) < 2 {
			return Action{}, fmt.Errorf("invalid move token %q (use move:up, move:down, move:left, move:right, or move:<dx>:<dy>)", tok)
		}
		if isDirection(parts[1]) {
			amount := int32(20)
			if len(parts) == 3 {
				n, err := strconv.Atoi(parts[2])
				if err != nil || n <= 0 {
					return Action{}, fmt.Errorf("invalid move amount in %q", tok)
				}
				amount = int32(n)
			}
			if amount > int32(cfg.MaxMoveStep) {
				amount = int32(cfg.MaxMoveStep)
			}
			return Action{Kind: KindMove, Name: parts[1], Amount: amount}, nil
		}

		// move:<dx>:<dy> — an explicit offset on both axes in one step,
		// e.g. move:50:-30 moves right 50px and up 30px at the same time.
		if len(parts) != 3 {
			return Action{}, fmt.Errorf("invalid move token %q (use move:up, move:down, move:left, move:right, or move:<dx>:<dy>)", tok)
		}
		dx, err := strconv.Atoi(parts[1])
		if err != nil {
			return Action{}, fmt.Errorf("invalid move dx in %q", tok)
		}
		dy, err := strconv.Atoi(parts[2])
		if err != nil {
			return Action{}, fmt.Errorf("invalid move dy in %q", tok)
		}
		return Action{Kind: KindMove, Name: "xy", Amount: clampMove(dx, cfg), Amount2: clampMove(dy, cfg)}, nil

	case "scroll":
		if len(parts) < 2 || (parts[1] != "up" && parts[1] != "down") {
			return Action{}, fmt.Errorf("invalid scroll token %q (use scroll:up or scroll:down)", tok)
		}
		amount := int32(1)
		if len(parts) == 3 {
			n, err := strconv.Atoi(parts[2])
			if err != nil || n <= 0 {
				return Action{}, fmt.Errorf("invalid scroll amount in %q", tok)
			}
			amount = int32(n)
		}
		const maxScrollNotches = 10
		if amount > maxScrollNotches {
			amount = maxScrollNotches
		}
		return Action{Kind: KindScroll, Name: parts[1], Amount: amount}, nil

	case "hold":
		if len(parts) != 3 || !input.IsKeyKnown(parts[1]) {
			return Action{}, fmt.Errorf("invalid hold token %q (use hold:<key>:<ms>)", tok)
		}
		ms, err := strconv.Atoi(parts[2])
		if err != nil || ms <= 0 {
			return Action{}, fmt.Errorf("invalid hold duration in %q", tok)
		}
		if ms > cfg.MaxHoldMs {
			ms = cfg.MaxHoldMs
		}
		return Action{Kind: KindKey, Name: parts[1], HoldMs: ms}, nil

	default:
		if len(parts) != 1 || !input.IsKeyKnown(tok) {
			return Action{}, fmt.Errorf("unknown key %q", tok)
		}
		return Action{Kind: KindKey, Name: tok}, nil
	}
}

func isDirection(s string) bool {
	return s == "up" || s == "down" || s == "left" || s == "right"
}

// clampMove bounds a signed pixel offset to +/-cfg.MaxMoveStep.
func clampMove(n int, cfg *config.Config) int32 {
	max := int32(cfg.MaxMoveStep)
	v := int32(n)
	if v > max {
		return max
	}
	if v < -max {
		return -max
	}
	return v
}
