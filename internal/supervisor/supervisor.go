// Package supervisor wires the app's subsystems together and keeps them
// running: any subsystem that panics or exits unexpectedly is restarted
// with backoff rather than taking the whole process down.
package supervisor

import (
	"bufio"
	"context"
	"log/slog"
	"os"
	"time"

	"streamer-remote/internal/backoff"
	"streamer-remote/internal/commands"
	"streamer-remote/internal/config"
	"streamer-remote/internal/twitch"
	"streamer-remote/internal/twitchauth"
)

const (
	chatQueueSize   = 64
	executorQueue   = 32
	tokenCachePath  = "twitch_token.json"
	twitchAuthScope = "user:read:chat"
)

type Options struct {
	Config    *config.Config
	Logger    *slog.Logger
	LocalOnly bool

	// Console is the shared stdin reader. Optional: if nil, a fresh one
	// over os.Stdin is created. Callers that already read stdin themselves
	// (e.g. an interactive setup wizard) must pass that same reader so the
	// two don't race over the same file descriptor.
	Console *bufio.Reader
}

// Run blocks until ctx is cancelled, running every enabled subsystem.
func Run(ctx context.Context, opts Options) error {
	cfg, logger := opts.Config, opts.Logger
	console := opts.Console
	if console == nil {
		console = bufio.NewReader(os.Stdin)
	}

	executor := commands.NewExecutor(logger, executorQueue)
	dispatcher := commands.NewDispatcher(cfg, logger, executor)

	go supervise(ctx, logger, "executor", func(ctx context.Context) { executor.Run(ctx) })
	go supervise(ctx, logger, "cooldown-cleanup", dispatcher.RunCooldownCleanup)
	go supervise(ctx, logger, "local-console", func(ctx context.Context) { runLocalConsole(ctx, logger, dispatcher, console) })

	if !opts.LocalOnly {
		if err := cfg.Twitch.Validate(); err != nil {
			return err
		}

		auth := twitchauth.New(cfg.Twitch.ClientID, tokenCachePath, []string{twitchAuthScope}, logger)
		if _, err := auth.EnsureToken(ctx); err != nil {
			return err
		}

		client := &twitch.Client{
			ClientID: cfg.Twitch.ClientID,
			Channel:  cfg.Twitch.Channel,
			Logger:   logger,
			TokenProvider: func(ctx context.Context) (string, error) {
				tok, err := auth.EnsureToken(ctx)
				if err != nil {
					return "", err
				}
				return tok.AccessToken, nil
			},
		}

		chatEvents := make(chan twitch.ChatEvent, chatQueueSize)
		go supervise(ctx, logger, "twitch-eventsub", func(ctx context.Context) { client.Run(ctx, chatEvents) })
		go supervise(ctx, logger, "twitch-forwarder", func(ctx context.Context) {
			forwardTwitchEvents(ctx, chatEvents, dispatcher)
		})
	} else {
		logger.Info("running in local-only mode: no Twitch connection will be made")
	}

	<-ctx.Done()
	return nil
}

func forwardTwitchEvents(ctx context.Context, events <-chan twitch.ChatEvent, dispatcher *commands.Dispatcher) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			dispatcher.Handle(commands.ChatMessage{
				Username:   e.ChatterLogin,
				Permission: commands.PermissionFromBadges(e.Badges),
				Text:       e.Text,
			})
		}
	}
}

// supervise runs fn, recovering panics and restarting it with backoff if
// it panics or returns before ctx is cancelled.
func supervise(ctx context.Context, logger *slog.Logger, name string, fn func(context.Context)) {
	bo := backoff.New(500*time.Millisecond, 30*time.Second)
	for ctx.Err() == nil {
		runOnce(ctx, logger, name, fn)
		if ctx.Err() != nil {
			return
		}
		wait := bo.Next()
		logger.Warn("subsystem exited, restarting", "subsystem", name, "in", wait)
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func runOnce(ctx context.Context, logger *slog.Logger, name string, fn func(context.Context)) {
	defer func() {
		if r := recover(); r != nil {
			logger.Error("recovered panic in subsystem", "subsystem", name, "panic", r)
		}
	}()
	fn(ctx)
}
