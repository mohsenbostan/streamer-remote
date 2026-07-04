package tts

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const Prefix = "rc-say:"

type Player struct {
	logger *slog.Logger
	queue  chan string
}

func NewPlayer(logger *slog.Logger, queueSize int) *Player {
	return &Player{
		logger: logger,
		queue:  make(chan string, queueSize),
	}
}

func Message(text string) (string, bool) {
	body, ok := strings.CutPrefix(text, Prefix)
	if !ok {
		return "", false
	}
	body = strings.TrimSpace(body)
	return body, body != ""
}

func (p *Player) Say(text string) {
	select {
	case p.queue <- text:
	default:
		p.logger.Warn("text-to-speech queue full, dropping message")
	}
}

func (p *Player) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case text := <-p.queue:
			p.speak(ctx, text)
		}
	}
}

func (p *Player) speak(ctx context.Context, text string) {
	timeout := time.Duration(10+len(text)/12) * time.Second
	speakCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	script := `$voice = New-Object System.Speech.Synthesis.SpeechSynthesizer; $voice.Speak($args[0])`
	cmd := exec.CommandContext(speakCtx, "powershell", "-NoProfile", "-Command", "Add-Type -AssemblyName System.Speech; "+script, text)
	if err := cmd.Run(); err != nil && speakCtx.Err() == nil {
		p.logger.Warn("text-to-speech failed", "error", err)
	}
}
