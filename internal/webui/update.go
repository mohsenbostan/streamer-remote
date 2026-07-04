package webui

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"streamer-remote/internal/update"
)

const updateRepo = "mohsenbostan/streamer-remote"

type updateInfoDTO struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
}

func (s *Server) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	rel, err := update.FetchLatest(ctx, updateRepo)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, updateInfoDTO{
		Current:   s.version,
		Latest:    rel.TagName,
		Available: update.IsNewer(s.version, rel.TagName),
	})
}

// handleApplyUpdate downloads and installs the update, then relaunches.
// It responds before restarting so the dashboard can show a message
// first — the browser tab will briefly lose connection while the new
// process comes up.
func (s *Server) handleApplyUpdate(w http.ResponseWriter, _ *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)

	rel, err := update.FetchLatest(ctx, updateRepo)
	if err != nil {
		cancel()
		writeError(w, http.StatusBadGateway, err)
		return
	}
	assetURL, ok := rel.AssetURL()
	if !ok {
		cancel()
		writeError(w, http.StatusBadGateway, fmt.Errorf("release %s has no Windows build attached", rel.TagName))
		return
	}

	w.WriteHeader(http.StatusAccepted)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	go func() {
		defer cancel()
		exePath, err := os.Executable()
		if err != nil {
			s.logger.Error("update failed: could not locate running executable", "error", err)
			return
		}
		newPath := exePath + ".new"
		if err := update.Download(ctx, assetURL, newPath); err != nil {
			s.logger.Error("update download failed", "error", err)
			return
		}
		s.logger.Info("installing update and restarting", "version", rel.TagName)
		if err := update.Apply(newPath); err != nil {
			s.logger.Error("update install failed", "error", err)
		}
	}()
}
