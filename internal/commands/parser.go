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
			return Action{}, fmt.Errorf("invalid move token %q (use move:up, move:down, move:left, or move:right)", tok)
		}
		if !isDirection(parts[1]) {
			return Action{}, fmt.Errorf("invalid move direction %q", parts[1])
		}
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
