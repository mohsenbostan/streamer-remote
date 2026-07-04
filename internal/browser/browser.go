// Package browser opens a URL in the user's default browser. Used to spare
// non-technical streamers from copy-pasting links themselves.
package browser

import "os/exec"

// Open launches url in the default browser. Best-effort: failures are
// returned but should generally just be logged, not treated as fatal.
func Open(url string) error {
	// "start" is a cmd.exe builtin; the empty "" argument is the window
	// title slot it expects before the URL.
	return exec.Command("cmd", "/c", "start", "", url).Start()
}
