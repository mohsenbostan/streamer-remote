// Package kick reads a Kick channel's public chat in real time over its
// Pusher WebSocket — the same one kick.com's own web player connects to.
// Reading a channel's public chat needs no authentication at all, so
// unlike Twitch this package has no OAuth flow: give it a channel slug
// and it streams chat messages.
//
// Kick's own developer API only offers chat via webhooks to a publicly
// reachable URL (see https://docs.kick.com/events/webhooks), which a
// local desktop app doesn't have. The public Pusher feed used here is
// undocumented but has been stable for years and is what every
// third-party Kick chat bot relies on for real-time delivery.
package kick

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"streamer-remote/internal/backoff"
)

const (
	pusherURL  = "wss://ws-us2.pusher.com/app/32cbd69e4b950bf97679?protocol=7&client=js&version=8.4.0-rc2&flash=false"
	channelAPI = "https://kick.com/api/v2/channels/"
)

// ChatEvent is one chat message read from Kick, translated into a
// transport-agnostic shape the rest of the app understands.
type ChatEvent struct {
	Username string
	Badges   map[string]bool
	Text     string
}

// Client streams one Kick channel's public chat until Run's context is
// cancelled, reconnecting with backoff on any error.
type Client struct {
	Channel string // Kick channel slug, e.g. "xqc"
	Logger  *slog.Logger

	connected atomic.Bool
}

// Connected reports whether the WebSocket session is currently live and
// subscribed (i.e. actually receiving chat events, not just attempting
// to reconnect).
func (c *Client) Connected() bool {
	return c.connected.Load()
}

// Run resolves Channel to its chatroom ID and streams chat messages into
// chatOut until ctx is cancelled.
func (c *Client) Run(ctx context.Context, chatOut chan<- ChatEvent) {
	bo := backoff.New(time.Second, time.Minute)

	for ctx.Err() == nil {
		chatroomID, err := c.resolveChatroomID(ctx)
		if err != nil {
			c.Logger.Error("failed to resolve kick chatroom", "channel", c.Channel, "error", err)
			c.waitBackoff(ctx, bo)
			continue
		}

		err = c.runSession(ctx, chatroomID, chatOut)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			c.Logger.Error("kick connection lost", "error", err)
		}
		c.waitBackoff(ctx, bo)
	}
}

func (c *Client) waitBackoff(ctx context.Context, bo *backoff.Backoff) {
	wait := bo.Next()
	c.Logger.Warn("reconnecting to kick", "in", wait)
	select {
	case <-ctx.Done():
	case <-time.After(wait):
	}
}

// resolveChatroomID looks up the numeric chatroom ID for Channel. Kick's
// official public API (api.kick.com) does not expose chatroom IDs, so
// this uses the same channel endpoint the kick.com website itself loads
// channel data from.
func (c *Client) resolveChatroomID(ctx context.Context) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, channelAPI+c.Channel, nil)
	if err != nil {
		return 0, err
	}
	// A browser-like User-Agent is required: Kick's edge rejects the
	// default Go User-Agent on this endpoint.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	httpClient := http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("kick: channel %q: status %d", c.Channel, resp.StatusCode)
	}

	var body struct {
		Chatroom struct {
			ID int `json:"id"`
		} `json:"chatroom"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("kick: parse channel %q: %w", c.Channel, err)
	}
	if body.Chatroom.ID == 0 {
		return 0, fmt.Errorf("kick: channel %q has no chatroom", c.Channel)
	}
	return body.Chatroom.ID, nil
}

// runSession owns one WebSocket connection end to end: connect, wait for
// Pusher's handshake, subscribe to the chatroom, then read events until
// the connection drops.
func (c *Client) runSession(ctx context.Context, chatroomID int, chatOut chan<- ChatEvent) error {
	conn, _, err := websocket.Dial(ctx, pusherURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.CloseNow()
	defer c.connected.Store(false)

	activityTimeout := 120 * time.Second

	for {
		readCtx, cancel := context.WithTimeout(ctx, activityTimeout+10*time.Second)
		_, data, err := conn.Read(readCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var env struct {
			Event string `json:"event"`
			Data  string `json:"data"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			c.Logger.Warn("failed to parse kick pusher message", "error", err)
			continue
		}

		switch env.Event {
		case "pusher:connection_established":
			var p struct {
				ActivityTimeout int `json:"activity_timeout"`
			}
			if err := json.Unmarshal([]byte(env.Data), &p); err == nil && p.ActivityTimeout > 0 {
				activityTimeout = time.Duration(p.ActivityTimeout) * time.Second
			}
			if err := c.subscribe(ctx, conn, chatroomID); err != nil {
				return fmt.Errorf("subscribe: %w", err)
			}

		case "pusher:ping":
			if err := writeJSON(ctx, conn, map[string]string{"event": "pusher:pong"}); err != nil {
				return fmt.Errorf("pong: %w", err)
			}

		case "pusher_internal:subscription_succeeded":
			c.connected.Store(true)
			c.Logger.Info("connected to kick chat", "channel", c.Channel)

		case "pusher:error":
			return fmt.Errorf("pusher error: %s", env.Data)

		case "App\\Events\\ChatMessageEvent":
			c.handleChatMessage(env.Data, chatOut)
		}
	}
}

func (c *Client) subscribe(ctx context.Context, conn *websocket.Conn, chatroomID int) error {
	return writeJSON(ctx, conn, map[string]any{
		"event": "pusher:subscribe",
		"data": map[string]string{
			"channel": "chatrooms." + strconv.Itoa(chatroomID) + ".v2",
		},
	})
}

func writeJSON(ctx context.Context, conn *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, data)
}

func (c *Client) handleChatMessage(data string, out chan<- ChatEvent) {
	var msg struct {
		Content string `json:"content"`
		Sender  struct {
			Username string `json:"username"`
			Identity struct {
				Badges []struct {
					Type string `json:"type"`
				} `json:"badges"`
			} `json:"identity"`
		} `json:"sender"`
	}
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		c.Logger.Warn("failed to parse kick chat message", "error", err)
		return
	}

	badges := make(map[string]bool, len(msg.Sender.Identity.Badges))
	for _, b := range msg.Sender.Identity.Badges {
		badges[b.Type] = true
	}
	event := ChatEvent{
		Username: msg.Sender.Username,
		Badges:   badges,
		Text:     msg.Content,
	}
	select {
	case out <- event:
	default:
		c.Logger.Warn("kick chat event queue full, dropping message", "user", event.Username)
	}
}
