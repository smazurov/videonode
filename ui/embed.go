//go:build ui_embed

// Package ui embeds the frontend assets for the web interface.
package ui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Embed the frontend build output directly from dist folder
// Build with: go build -tags ui_embed .
// Requires: cd ui && pnpm build

//go:embed all:dist
var distFS embed.FS

// Handler returns an http.Handler that serves the embedded frontend.
func Handler() (http.Handler, error) {
	// Always serve from embedded filesystem
	fsys, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}

	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Clean the path
		p := path.Clean(r.URL.Path)

		// Try to open the file
		f, openErr := fsys.Open(strings.TrimPrefix(p, "/"))
		if openErr == nil {
			defer func() { _ = f.Close() }()
			// Check if it's a file (not a directory)
			stat, statErr := f.Stat()
			if statErr == nil && !stat.IsDir() {
				// File exists, serve it
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// For SPA routing: if file doesn't exist and path doesn't have an extension,
		// serve index.html for client-side routing
		if !strings.Contains(path.Base(p), ".") {
			// Reset the URL path to serve index.html
			r.URL.Path = "/"
		}

		fileServer.ServeHTTP(w, r)
	}), nil
}
