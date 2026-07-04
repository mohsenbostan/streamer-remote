// Package twitch talks to Twitch's Helix REST API and EventSub WebSocket
// to receive chat messages in real time. IRC still works today, but
// Twitch's own docs now point new bots at EventSub + Helix, so that's
// what this package implements.
// See https://dev.twitch.tv/docs/eventsub/handling-websocket-events/.
package twitch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func bytesReader(b []byte) io.Reader {
	if b == nil {
		return http.NoBody
	}
	return bytes.NewReader(b)
}

const helixBase = "https://api.twitch.tv/helix"

type HelixClient struct {
	ClientID    string
	AccessToken string
	httpClient  *http.Client
}

func NewHelixClient(clientID, accessToken string) *HelixClient {
	return &HelixClient{
		ClientID:    clientID,
		AccessToken: accessToken,
		httpClient:  &http.Client{Timeout: 15 * time.Second},
	}
}

func (h *HelixClient) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody []byte
	if body != nil {
		var err error
		reqBody, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, helixBase+path, bytesReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", h.ClientID)
	req.Header.Set("Authorization", "Bearer "+h.AccessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("helix %s %s: status %d: %s", method, path, resp.StatusCode, errBody.Message)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// GetUserID resolves a Twitch login name to its numeric user ID.
func (h *HelixClient) GetUserID(ctx context.Context, login string) (string, error) {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := h.do(ctx, http.MethodGet, "/users?login="+login, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("helix: no user found for login %q", login)
	}
	return resp.Data[0].ID, nil
}

// GetOwnUserID resolves the user ID of the account that authorized the
// current access token (no login parameter queries the token owner).
func (h *HelixClient) GetOwnUserID(ctx context.Context) (string, error) {
	var resp struct {
		Data []struct {
			ID    string `json:"id"`
			Login string `json:"login"`
		} `json:"data"`
	}
	if err := h.do(ctx, http.MethodGet, "/users", nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("helix: could not resolve token owner")
	}
	return resp.Data[0].ID, nil
}

// CreateChatSubscription subscribes a WebSocket session to a channel's
// chat messages via the channel.chat.message EventSub type.
func (h *HelixClient) CreateChatSubscription(ctx context.Context, broadcasterUserID, userID, sessionID string) error {
	body := map[string]any{
		"type":    "channel.chat.message",
		"version": "1",
		"condition": map[string]string{
			"broadcaster_user_id": broadcasterUserID,
			"user_id":             userID,
		},
		"transport": map[string]string{
			"method":     "websocket",
			"session_id": sessionID,
		},
	}
	return h.do(ctx, http.MethodPost, "/eventsub/subscriptions", body, nil)
}

// CreateRedemptionSubscription subscribes a WebSocket session to every
// Channel Points redemption on the channel. Redemptions for rewards we
// don't manage are simply ignored by the dispatcher, so one broad
// subscription is simpler than tracking per-reward subscriptions.
func (h *HelixClient) CreateRedemptionSubscription(ctx context.Context, broadcasterUserID, sessionID string) error {
	body := map[string]any{
		"type":    "channel.channel_points_custom_reward_redemption.add",
		"version": "1",
		"condition": map[string]string{
			"broadcaster_user_id": broadcasterUserID,
		},
		"transport": map[string]string{
			"method":     "websocket",
			"session_id": sessionID,
		},
	}
	return h.do(ctx, http.MethodPost, "/eventsub/subscriptions", body, nil)
}

// CreateCustomReward creates a Channel Points reward the app will listen
// for redemptions of. Requires the broadcaster to be a Twitch Affiliate
// or Partner; Twitch returns an error for other accounts, surfaced as-is
// so the caller can show it to the streamer.
func (h *HelixClient) CreateCustomReward(ctx context.Context, broadcasterUserID, title string, cost int) (rewardID string, err error) {
	body := map[string]any{
		"title":                  title,
		"cost":                   cost,
		"prompt":                 "Triggers automatically — no need to type anything.",
		"is_user_input_required": false,
	}
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	path := "/channel_points/custom_rewards?broadcaster_id=" + url.QueryEscape(broadcasterUserID)
	if err := h.do(ctx, http.MethodPost, path, body, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("helix: reward creation returned no data")
	}
	return resp.Data[0].ID, nil
}

// DeleteCustomReward removes a reward previously created by this app. Per
// Twitch's rules, a reward can only be deleted by the same Client ID that
// created it.
func (h *HelixClient) DeleteCustomReward(ctx context.Context, broadcasterUserID, rewardID string) error {
	path := fmt.Sprintf("/channel_points/custom_rewards?broadcaster_id=%s&id=%s",
		url.QueryEscape(broadcasterUserID), url.QueryEscape(rewardID))
	return h.do(ctx, http.MethodDelete, path, nil, nil)
}

// UpdateRedemptionStatus marks a redemption FULFILLED (the action ran) or
// CANCELED (refunds the viewer's points; used when the remote is paused
// or the action is blacklisted).
func (h *HelixClient) UpdateRedemptionStatus(ctx context.Context, broadcasterUserID, rewardID, redemptionID, status string) error {
	path := fmt.Sprintf("/channel_points/custom_rewards/redemptions?broadcaster_id=%s&reward_id=%s&id=%s",
		url.QueryEscape(broadcasterUserID), url.QueryEscape(rewardID), url.QueryEscape(redemptionID))
	body := map[string]string{"status": status}
	return h.do(ctx, http.MethodPatch, path, body, nil)
}
