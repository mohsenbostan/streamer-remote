// Package supervisor wires the app's subsystems together and keeps them
// running: any subsystem that panics or exits unexpectedly is restarted
// with backoff rather than taking the whole process down.
package supervisor

import (
	"context"
	"log/slog"
	"time"

	"streamer-remote/internal/backoff"
	"streamer-remote/internal/commands"
	"streamer-remote/internal/config"
	"streamer-remote/internal/tts"
	"streamer-remote/internal/twitch"
	"streamer-remote/internal/twitchauth"
)

const (
	chatQueueSize       = 64
	redemptionQueueSize = 32
	executorQueue       = 32
	textToSpeechQueue   = 8

	// TokenCachePath is shared with the dashboard, which needs its own
	// Authenticator to manage Channel Points rewards independently of
	// whether the Twitch subsystem is currently running.
	TokenCachePath = "twitch_token.json"
)

// TwitchAuthScopes are requested unconditionally on every login: a modest
// upfront ask (read chat, read/manage Channel Points redemptions) so a
// streamer who later sets up reward-only actions never has to re-auth.
var TwitchAuthScopes = []string{
	"user:read:chat",
	"channel:read:redemptions",
	"channel:manage:redemptions",
}

// Core holds the subsystems that always run, regardless of whether Twitch
// is connected: the input executor and the dispatcher that gates chat/
// redemption/dashboard-originated commands. It's the shared foundation
// the web dashboard's "quick test" and the Twitch pipeline both feed into.
type Core struct {
	Dispatcher *commands.Dispatcher
	executor   *commands.Executor
}

// NewCore builds and starts the always-on subsystems.
func NewCore(ctx context.Context, cfg *config.Config, logger *slog.Logger) *Core {
	executor := commands.NewExecutor(logger, executorQueue)
	dispatcher := commands.NewDispatcher(cfg, logger, executor)

	go supervise(ctx, logger, "executor", func(ctx context.Context) { executor.Run(ctx) })
	go supervise(ctx, logger, "cooldown-cleanup", dispatcher.RunCooldownCleanup)

	return &Core{Dispatcher: dispatcher, executor: executor}
}

// TwitchSession is a running Twitch connection, startable and stoppable
// independently of Core so the dashboard can connect Twitch on demand
// (after the streamer finishes setup) without restarting the app.
type TwitchSession struct {
	Client *twitch.Client
	cancel context.CancelFunc
}

// StartTwitch begins streaming chat and Channel Points redemptions into
// dispatcher. Call Stop when done. The caller is responsible for having
// already ensured a usable token exists (see twitchauth.Authenticator) —
// this does not itself run interactive auth.
func StartTwitch(parentCtx context.Context, cfg *config.Config, logger *slog.Logger, dispatcher *commands.Dispatcher, auth *twitchauth.Authenticator) *TwitchSession {
	ctx, cancel := context.WithCancel(parentCtx)

	tokenProvider := func(ctx context.Context) (string, error) {
		tok, err := auth.EnsureToken(ctx)
		if err != nil {
			return "", err
		}
		return tok.AccessToken, nil
	}

	client := &twitch.Client{
		ClientID:      cfg.Twitch.ClientID,
		Channel:       cfg.Twitch.Channel,
		Logger:        logger,
		TokenProvider: tokenProvider,
	}

	chatEvents := make(chan twitch.ChatEvent, chatQueueSize)
	redemptionEvents := make(chan twitch.RedemptionEvent, redemptionQueueSize)
	speaker := tts.NewPlayer(logger, textToSpeechQueue)
	go supervise(ctx, logger, "twitch-eventsub", func(ctx context.Context) { client.Run(ctx, chatEvents, redemptionEvents) })
	go supervise(ctx, logger, "text-to-speech", speaker.Run)
	go supervise(ctx, logger, "twitch-chat-forwarder", func(ctx context.Context) {
		forwardTwitchEvents(ctx, chatEvents, dispatcher, speaker)
	})
	go supervise(ctx, logger, "twitch-redemption-forwarder", func(ctx context.Context) {
		forwardRedemptions(ctx, logger, redemptionEvents, dispatcher, client, cfg.Twitch.ClientID, tokenProvider)
	})

	return &TwitchSession{Client: client, cancel: cancel}
}

// Stop disconnects the Twitch session. Safe to call once.
func (s *TwitchSession) Stop() {
	s.cancel()
}

func forwardTwitchEvents(ctx context.Context, events <-chan twitch.ChatEvent, dispatcher *commands.Dispatcher, speaker *tts.Player) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			if text, ok := tts.Message(e.Text); ok {
				if dispatcher.Config().TextToSpeechEnabled {
					speaker.Say(text)
				}
				continue
			}
			dispatcher.Handle(commands.ChatMessage{
				Username:   e.ChatterLogin,
				Permission: commands.PermissionFromBadges(e.Badges),
				Text:       e.Text,
			})
		}
	}
}

// forwardRedemptions runs each Channel Points redemption through the
// dispatcher and reports the outcome back to Twitch: FULFILLED so it
// leaves the streamer's redemption queue, or CANCELED so the viewer's
// points are refunded if the remote was paused or the action is
// blacklisted.
func forwardRedemptions(
	ctx context.Context,
	logger *slog.Logger,
	events <-chan twitch.RedemptionEvent,
	dispatcher *commands.Dispatcher,
	client *twitch.Client,
	clientID string,
	tokenProvider twitch.TokenProvider,
) {
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			result := dispatcher.HandleRedemption(e.RewardID, e.UserLogin)
			if result == commands.RedemptionIgnored {
				continue
			}

			status := "FULFILLED"
			if result == commands.RedemptionRefunded {
				status = "CANCELED"
			}
			broadcasterID := client.BroadcasterUserID()
			token, err := tokenProvider(ctx)
			if err != nil {
				logger.Error("could not update redemption status: no token", "error", err)
				continue
			}
			helix := twitch.NewHelixClient(clientID, token)
			if err := helix.UpdateRedemptionStatus(ctx, broadcasterID, e.RewardID, e.RedemptionID, status); err != nil {
				logger.Error("failed to update redemption status", "status", status, "error", err)
			}
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
