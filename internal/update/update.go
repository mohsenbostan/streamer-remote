// Package update implements self-updating from GitHub Releases: no git
// dependency, just the public REST API and a plain HTTPS asset download.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const assetName = "streamer-remote-windows-amd64.exe"

type Release struct {
	TagName string  `json:"tag_name"`
	Assets  []Asset `json:"assets"`
}

type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// FetchLatest queries GitHub's public releases API for repo ("owner/name").
func FetchLatest(ctx context.Context, repo string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update: unexpected status %d from GitHub", resp.StatusCode)
	}
	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("update: parse release info: %w", err)
	}
	return &rel, nil
}

// AssetURL returns the download URL for this platform's build, if present.
func (r *Release) AssetURL() (string, bool) {
	for _, a := range r.Assets {
		if a.Name == assetName {
			return a.BrowserDownloadURL, true
		}
	}
	return "", false
}

// IsNewer reports whether latestTag (e.g. "v1.2.0") is newer than
// currentVersion (e.g. "v1.1.0" or "dev"). "dev" builds never report an
// update as available, since there's no meaningful version to compare.
func IsNewer(currentVersion, latestTag string) bool {
	if currentVersion == "dev" || currentVersion == "" {
		return false
	}
	cur, ok1 := parseSemver(currentVersion)
	latest, ok2 := parseSemver(latestTag)
	if !ok1 || !ok2 {
		return currentVersion != latestTag
	}
	for i := range cur {
		if latest[i] != cur[i] {
			return latest[i] > cur[i]
		}
	}
	return false
}

func parseSemver(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(strings.SplitN(p, "-", 2)[0]) // tolerate "1.2.3-rc1"
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// Download streams url to destPath.
func Download(ctx context.Context, url, destPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update: download failed with status %d", resp.StatusCode)
	}

	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("update: save download: %w", err)
	}
	return nil
}

// Apply replaces the currently running executable with newExePath and
// relaunches it, then exits the current process. It never returns on
// success. Windows won't let you overwrite a running exe's bytes in
// place, but it will let you rename it out of the way, which is the
// standard trick self-updating Windows binaries use.
func Apply(newExePath string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("update: self-update is only implemented for Windows")
	}

	currentExe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("update: locate running executable: %w", err)
	}
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return fmt.Errorf("update: resolve running executable path: %w", err)
	}

	oldPath := currentExe + ".old"
	_ = os.Remove(oldPath) // leftover from a previous update; fine if it doesn't exist

	if err := os.Rename(currentExe, oldPath); err != nil {
		return fmt.Errorf("update: move current executable aside: %w", err)
	}
	if err := os.Rename(newExePath, currentExe); err != nil {
		// best-effort rollback so we don't leave the streamer with no exe at all
		_ = os.Rename(oldPath, currentExe)
		return fmt.Errorf("update: install new executable: %w", err)
	}

	cmd := exec.Command(currentExe, os.Args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("update: relaunch new executable: %w", err)
	}

	os.Exit(0)
	return nil // unreachable
}

// CleanupOldBinary removes a leftover ".old" file from a previous update,
// if any. Safe to call unconditionally on every startup.
func CleanupOldBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	_ = os.Remove(exe + ".old")
}
