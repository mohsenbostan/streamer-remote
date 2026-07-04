// Package webui serves the dashboard: the built frontend (embedded into
// the binary, so it's still a single .exe) plus a small JSON/SSE API the
// dashboard uses to configure and monitor the app.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// staticFS strips the "dist" prefix so paths match what the browser
// requests (e.g. "/assets/index.js" instead of "/dist/assets/index.js").
func staticFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Only possible if the frontend wasn't built before `go build`;
		// fail loudly rather than silently serving nothing.
		panic("webui: frontend not built — run 'npm run build' in web/ first: " + err.Error())
	}
	return sub
}
