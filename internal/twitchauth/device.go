package twitchauth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"streamer-remote/internal/browser"
)

const (
	deviceCodeURL = "https://id.twitch.tv/oauth2/device"
	tokenURL      = "https://id.twitch.tv/oauth2/token"
)

type Authenticator struct {
	ClientID  string
	Scopes    []string
	TokenPath string
	Logger    *slog.Logger

	httpClient *http.Client
}

func New(clientID, tokenPath string, scopes []string, logger *slog.Logger) *Authenticator {
	return &Authenticator{
		ClientID:   clientID,
		Scopes:     scopes,
		TokenPath:  tokenPath,
		Logger:     logger,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// EnsureToken returns a usable access token: from cache, refreshed, or by
// running the interactive device code flow as a last resort.
func (a *Authenticator) EnsureToken(ctx context.Context) (*Token, error) {
	if tok, err := LoadToken(a.TokenPath); err == nil {
		if !tok.ExpiringSoon() {
			return tok, nil
		}
		if refreshed, err := a.refresh(ctx, tok.RefreshToken); err == nil {
			if saveErr := refreshed.Save(a.TokenPath); saveErr != nil {
				a.Logger.Warn("failed to persist refreshed token", "error", saveErr)
			}
			return refreshed, nil
		} else {
			a.Logger.Warn("token refresh failed, starting device login", "error", err)
		}
	}

	tok, err := a.deviceLogin(ctx)
	if err != nil {
		return nil, err
	}
	if err := tok.Save(a.TokenPath); err != nil {
		a.Logger.Warn("failed to persist new token", "error", err)
	}
	return tok, nil
}

type deviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Message      string `json:"message"` // set on "authorization_pending" / "slow_down" errors
}

func (a *Authenticator) deviceLogin(ctx context.Context) (*Token, error) {
	form := url.Values{
		"client_id": {a.ClientID},
		"scopes":    {strings.Join(a.Scopes, " ")},
	}
	var dc deviceCodeResponse
	if err := a.postForm(ctx, deviceCodeURL, form, &dc); err != nil {
		return nil, fmt.Errorf("twitchauth: request device code: %w", err)
	}

	a.Logger.Info("Twitch authorization required",
		"open", dc.VerificationURI, "enter_code", dc.UserCode)
	fmt.Printf("\n>>> Opening your browser. If it doesn't open, go to %s and enter code: %s\n\n", dc.VerificationURI, dc.UserCode)
	if err := browser.Open(dc.VerificationURI); err != nil {
		a.Logger.Debug("could not auto-open browser", "error", err)
	}

	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return nil, fmt.Errorf("twitchauth: device code expired before authorization")
			}
			form := url.Values{
				"client_id":   {a.ClientID},
				"device_code": {dc.DeviceCode},
				"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
				"scopes":      {strings.Join(a.Scopes, " ")},
			}
			var tr tokenResponse
			err := a.postForm(ctx, tokenURL, form, &tr)
			if err != nil {
				if strings.Contains(err.Error(), "authorization_pending") {
					continue
				}
				if strings.Contains(err.Error(), "slow_down") {
					ticker.Reset(interval + 2*time.Second)
					continue
				}
				return nil, fmt.Errorf("twitchauth: poll for token: %w", err)
			}
			a.Logger.Info("Twitch authorization complete")
			return &Token{
				AccessToken:  tr.AccessToken,
				RefreshToken: tr.RefreshToken,
				ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
			}, nil
		}
	}
}

func (a *Authenticator) refresh(ctx context.Context, refreshToken string) (*Token, error) {
	if refreshToken == "" {
		return nil, fmt.Errorf("twitchauth: no refresh token cached")
	}
	form := url.Values{
		"client_id":     {a.ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	}
	var tr tokenResponse
	if err := a.postForm(ctx, tokenURL, form, &tr); err != nil {
		return nil, err
	}
	return &Token{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}

// postForm POSTs form-encoded data and decodes a JSON response into out.
// Non-2xx responses are surfaced as an error containing the response body,
// since Twitch's device-flow "errors" (authorization_pending, slow_down)
// are only distinguishable by message text.
func (a *Authenticator) postForm(ctx context.Context, endpoint string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&errBody)
		if errBody.Message != "" {
			return fmt.Errorf("%s", errBody.Message)
		}
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}
