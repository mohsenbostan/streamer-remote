// Package twitchauth implements Twitch's OAuth Device Code Flow, the
// variant meant for CLI/limited-input apps: no client secret, no local
// redirect server, just a code the streamer enters at a Twitch URL once.
// See https://dev.twitch.tv/docs/authentication/getting-tokens-oauth/.
package twitchauth

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        []string  `json:"scope"`
}

// ExpiringSoon reports whether the token needs refreshing before use.
func (t *Token) ExpiringSoon() bool {
	return time.Now().Add(10 * time.Minute).After(t.ExpiresAt)
}

// HasScopes reports whether the token was granted every scope in required.
// A cached token from before a feature that needs a new scope was added
// will fail this, which is the signal to re-run the device code flow
// rather than just refreshing (refreshing can't add scopes).
func (t *Token) HasScopes(required []string) bool {
	granted := make(map[string]bool, len(t.Scope))
	for _, s := range t.Scope {
		granted[s] = true
	}
	for _, s := range required {
		if !granted[s] {
			return false
		}
	}
	return true
}

func LoadToken(path string) (*Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Token
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("twitchauth: parse token cache: %w", err)
	}
	return &t, nil
}

func (t *Token) Save(path string) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("twitchauth: encode token: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("twitchauth: save token cache: %w", err)
	}
	return nil
}
