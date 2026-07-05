package webui

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"streamer-remote/internal/commands"
	"streamer-remote/internal/config"
	"streamer-remote/internal/tts"
)

func testServer(t *testing.T) (*Server, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{
		Prefix:              "rc!",
		TextToSpeechEnabled: true,
		MaxComboSize:        3,
		MaxSequenceSteps:    4,
		TapHoldMs:           40,
		MaxHoldMs:           3000,
		MaxMoveStep:         300,
	}
	if err := cfg.Save(path); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	executor := commands.NewExecutor(logger, 10)
	dispatcher := commands.NewDispatcher(cfg, logger, executor)

	srv := New(context.Background(), path, dispatcher, logger, "v1.2.3", true, NewHub())
	return srv, path
}

func doJSON(t *testing.T, ts *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(data))
	}
	req, err := http.NewRequest(method, ts.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestStatusEndpoint(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodGet, "/api/status", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var status StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status.Version != "v1.2.3" || !status.LocalOnly {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestPauseResumeEndpoints(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	doJSON(t, ts, http.MethodPost, "/api/pause", nil).Body.Close()
	if srv.dispatcher.Enabled() {
		t.Fatal("expected dispatcher to be paused after POST /api/pause")
	}

	doJSON(t, ts, http.MethodPost, "/api/resume", nil).Body.Close()
	if !srv.dispatcher.Enabled() {
		t.Fatal("expected dispatcher to be active after POST /api/resume")
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	srv, path := testServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodGet, "/api/settings", nil)
	var settings Settings
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if settings.Prefix != "rc!" {
		t.Fatalf("expected prefix 'rc!', got %q", settings.Prefix)
	}

	settings.Prefix = "!!"
	settings.ModOnlyMode = true
	settings.TextToSpeechEnabled = false
	settings.Blacklist.DeniedKeys = []string{"lwin"}

	putResp := doJSON(t, ts, http.MethodPut, "/api/settings", settings)
	defer putResp.Body.Close()
	if putResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", putResp.StatusCode)
	}

	if got := srv.dispatcher.Config(); got.Prefix != "!!" || !got.ModOnlyMode || got.TextToSpeechEnabled {
		t.Fatalf("expected live config to reflect the update, got %+v", got)
	}

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Prefix != "!!" || reloaded.TextToSpeechEnabled || len(reloaded.Blacklist.DeniedKeys) != 1 {
		t.Fatalf("expected settings to persist to disk, got %+v", reloaded)
	}
}

func TestTestEndpointDispatchesCommand(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodPost, "/api/test", testRequest{Permission: "broadcaster", Text: "rc!w"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestTestEndpointPublishesTextToSpeech(t *testing.T) {
	srv, _ := testServer(t)
	ch, unsubscribe := srv.hub.subscribe()
	defer unsubscribe()

	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodPost, "/api/test", testRequest{Permission: "broadcaster", Text: "rc-say: hello chat"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	select {
	case e := <-ch:
		if e.Msg != tts.EventMessage || e.Attrs["text"] != "hello chat" {
			t.Fatalf("expected tts event, got %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tts event")
	}
}

func TestTestEndpointRejectsUnknownPermission(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodPost, "/api/test", testRequest{Permission: "wizard", Text: "rc!w"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTwitchLogoutClearsConfigAndAuthState(t *testing.T) {
	srv, path := testServer(t)
	if err := config.UpdateTwitchFields(path, "somechannel", "someclientid"); err != nil {
		t.Fatal(err)
	}
	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	srv.dispatcher.UpdateConfig(reloaded)
	srv.authState.setConnected()

	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodPost, "/api/twitch/logout", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	if got := srv.dispatcher.Config(); got.Twitch.Channel != "" || got.Twitch.ClientID != "" {
		t.Fatalf("expected twitch fields cleared from live config, got %+v", got.Twitch)
	}
	if got, err := config.Load(path); err != nil || got.Twitch.Channel != "" {
		t.Fatalf("expected twitch fields cleared on disk, got %+v (err=%v)", got, err)
	}
	if got := srv.authState.snapshot(); got.State != "idle" {
		t.Fatalf("expected auth state reset to idle, got %+v", got)
	}
}

func TestRewardsListEmptyIsEmptyArrayNotNull(t *testing.T) {
	srv, _ := testServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodGet, "/api/rewards", nil)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("expected '[]' for no rewards, got %q", body)
	}
}

func TestRewardProfilesSaveAndDelete(t *testing.T) {
	srv, path := testServer(t)
	ts := httptest.NewServer(srv.routes())
	defer ts.Close()

	resp := doJSON(t, ts, http.MethodPost, "/api/reward-profiles", saveProfileRequest{Name: "Chill"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	listResp := doJSON(t, ts, http.MethodGet, "/api/reward-profiles", nil)
	defer listResp.Body.Close()
	var list rewardProfilesResponse
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Profiles) != 1 || list.Profiles[0].Name != "Chill" || list.Active != "Chill" {
		t.Fatalf("expected 'Chill' saved and active, got %+v", list)
	}

	reloaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.RewardProfiles) != 1 || reloaded.ActiveRewardProfile != "Chill" {
		t.Fatalf("expected profile persisted to disk, got %+v", reloaded)
	}

	delResp := doJSON(t, ts, http.MethodDelete, "/api/reward-profiles/Chill", nil)
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	reloaded, err = config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.RewardProfiles) != 0 {
		t.Fatalf("expected profile removed, got %+v", reloaded.RewardProfiles)
	}
}
