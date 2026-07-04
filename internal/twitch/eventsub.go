package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/coder/websocket"

	"streamer-remote/internal/backoff"
)

const eventSubURL = "wss://eventsub.wss.twitch.tv/ws?keepalive_timeout_seconds=15"

// ChatEvent is one chat message read from EventSub, translated into a
// transport-agnostic shape the rest of the app understands.
type ChatEvent struct {
	ChatterLogin string
	Badges       map[string]bool
	Text         string
}

// RedemptionEvent is one Channel Points redemption read from EventSub.
// RedemptionID is needed to later mark it FULFILLED or CANCELED via Helix.
type RedemptionEvent struct {
	RedemptionID string
	RewardID     string
	UserLogin    string
}

// TokenProvider returns a currently-valid access token, refreshing it if
// necessary. Called on every (re)connect so a long-running process always
// authenticates with a fresh token.
type TokenProvider func(ctx context.Context) (string, error)

type Client struct {
	ClientID      string
	Channel       string
	TokenProvider TokenProvider
	Logger        *slog.Logger

	mu                sync.Mutex
	broadcasterUserID string
	ownUserID         string
}

// BroadcasterUserID returns the resolved channel ID, or "" before the
// first successful connection. Safe to call once redemption/chat events
// have started arriving, since resolving IDs always happens before any
// event is emitted.
func (c *Client) BroadcasterUserID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.broadcasterUserID
}

// Run connects to EventSub and streams chat messages and Channel Points
// redemptions until ctx is cancelled. It reconnects with backoff on any
// error, including the periodic "please reconnect" notice Twitch sends
// for load balancing.
func (c *Client) Run(ctx context.Context, chatOut chan<- ChatEvent, redemptionOut chan<- RedemptionEvent) {
	bo := backoff.New(time.Second, time.Minute)

	for ctx.Err() == nil {
		if err := c.resolveIDs(ctx); err != nil {
			c.Logger.Error("failed to resolve twitch user ids", "error", err)
			c.waitBackoff(ctx, bo)
			continue
		}

		err := c.runSession(ctx, chatOut, redemptionOut)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.Logger.Error("twitch connection lost", "error", err)
		}
		c.waitBackoff(ctx, bo)
	}
}

func (c *Client) waitBackoff(ctx context.Context, bo *backoff.Backoff) {
	wait := bo.Next()
	c.Logger.Warn("reconnecting to twitch", "in", wait)
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

func (c *Client) resolveIDs(ctx context.Context) error {
	c.mu.Lock()
	resolved := c.broadcasterUserID != "" && c.ownUserID != ""
	c.mu.Unlock()
	if resolved {
		return nil
	}

	token, err := c.TokenProvider(ctx)
	if err != nil {
		return fmt.Errorf("get token: %w", err)
	}
	helix := NewHelixClient(c.ClientID, token)

	broadcasterID, err := helix.GetUserID(ctx, c.Channel)
	if err != nil {
		return fmt.Errorf("resolve channel %q: %w", c.Channel, err)
	}
	ownID, err := helix.GetOwnUserID(ctx)
	if err != nil {
		return fmt.Errorf("resolve token owner: %w", err)
	}

	c.mu.Lock()
	c.broadcasterUserID = broadcasterID
	c.ownUserID = ownID
	c.mu.Unlock()
	return nil
}

// runSession owns one WebSocket connection end to end: connect, wait for
// welcome, subscribe, then read notifications until the connection drops.
func (c *Client) runSession(ctx context.Context, chatOut chan<- ChatEvent, redemptionOut chan<- RedemptionEvent) error {
	conn, _, err := websocket.Dial(ctx, eventSubURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()

	keepalive := 15 * time.Second

	for {
		readCtx, cancel := context.WithTimeout(ctx, keepalive+10*time.Second)
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var env struct {
			Metadata struct {
				MessageType      string `json:"message_type"`
				SubscriptionType string `json:"subscription_type"`
			} `json:"metadata"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			c.Logger.Warn("failed to parse eventsub message", "error", err)
			continue
		}

		switch env.Metadata.MessageType {
		case "session_welcome":
			var p struct {
				Session struct {
					ID                      string `json:"id"`
					KeepaliveTimeoutSeconds int    `json:"keepalive_timeout_seconds"`
				} `json:"session"`
			}
			if err := json.Unmarshal(env.Payload, &p); err != nil {
				return fmt.Errorf("parse welcome: %w", err)
			}
			if p.Session.KeepaliveTimeoutSeconds > 0 {
				keepalive = time.Duration(p.Session.KeepaliveTimeoutSeconds) * time.Second
			}
			token, err := c.TokenProvider(ctx)
			if err != nil {
				return fmt.Errorf("get token for subscription: %w", err)
			}
			helix := NewHelixClient(c.ClientID, token)
			if err := helix.CreateChatSubscription(ctx, c.broadcasterUserID, c.ownUserID, p.Session.ID); err != nil {
				return fmt.Errorf("create chat subscription: %w", err)
			}
			if err := helix.CreateRedemptionSubscription(ctx, c.broadcasterUserID, p.Session.ID); err != nil {
				return fmt.Errorf("create redemption subscription: %w", err)
			}
			c.Logger.Info("connected to twitch chat", "channel", c.Channel)

		case "session_keepalive":
			// no-op: receiving anything resets the read deadline next loop

		case "session_reconnect":
			// Twitch asks us to move to a new session ahead of a planned
			// disconnect. Rather than juggling two live sockets, close
			// cleanly and let Run's outer loop redial from scratch.
			c.Logger.Info("twitch requested reconnect")
			return nil

		case "notification":
			switch env.Metadata.SubscriptionType {
			case "channel.chat.message":
				c.handleChatNotification(env.Payload, chatOut)
			case "channel.channel_points_custom_reward_redemption.add":
				c.handleRedemptionNotification(env.Payload, redemptionOut)
			}
		}
	}
}

func (c *Client) handleChatNotification(payload json.RawMessage, out chan<- ChatEvent) {
	var p struct {
		Event struct {
			ChatterUserLogin string `json:"chatter_user_login"`
			Message          struct {
				Text string `json:"text"`
			} `json:"message"`
			Badges []struct {
				SetID string `json:"set_id"`
			} `json:"badges"`
		} `json:"event"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		c.Logger.Warn("failed to parse chat notification", "error", err)
		return
	}
	badges := make(map[string]bool, len(p.Event.Badges))
	for _, b := range p.Event.Badges {
		badges[b.SetID] = true
	}
	event := ChatEvent{
		ChatterLogin: p.Event.ChatterUserLogin,
		Badges:       badges,
		Text:         p.Event.Message.Text,
	}
	select {
	case out <- event:
	default:
		c.Logger.Warn("chat event queue full, dropping message", "user", event.ChatterLogin)
	}
}

func (c *Client) handleRedemptionNotification(payload json.RawMessage, out chan<- RedemptionEvent) {
	var p struct {
		Event struct {
			ID        string `json:"id"`
			UserLogin string `json:"user_login"`
			Reward    struct {
				ID string `json:"id"`
			} `json:"reward"`
		} `json:"event"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		c.Logger.Warn("failed to parse redemption notification", "error", err)
		return
	}
	event := RedemptionEvent{
		RedemptionID: p.Event.ID,
		RewardID:     p.Event.Reward.ID,
		UserLogin:    p.Event.UserLogin,
	}
	select {
	case out <- event:
	default:
		c.Logger.Warn("redemption event queue full, dropping redemption", "user", event.UserLogin)
	}
}
