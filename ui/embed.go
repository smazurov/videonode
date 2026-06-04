//go:build ui_embed

// Package ui embeds the frontend assets for the web interface.
package ui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
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

const (
	immutableCacheControl = "public, max-age=31536000, immutable"
	indexCacheControl     = "no-cache"
)

// Handler returns an http.Handler that serves the embedded frontend.
//
// Vite emits content-hashed assets under /assets, so those are served immutable
// and a miss returns a real 404 (never index.html, which would surface as an
// "Unexpected token '<'" MIME error). Every other path that isn't a real file
// falls back to index.html for client-side routing — no filename heuristics.
func Handler() (http.Handler, error) {
	fsys, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil, err
	}

	// embed.FS files have a zero ModTime, so http.ServeContent emits no
	// Last-Modified and never generates an ETag. Hash index.html once so
	// no-cache revalidation can short-circuit to 304. Stable until rebuild.
	indexETag := computeETag(fsys, "index.html")

	fileServer := http.FileServer(http.FS(fsys))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))

		if strings.HasPrefix(p, "/assets/") {
			w.Header().Set("Cache-Control", immutableCacheControl)
			fileServer.ServeHTTP(w, r)
			return
		}

		if f, openErr := fsys.Open(strings.TrimPrefix(p, "/")); openErr == nil {
			stat, statErr := f.Stat()
			_ = f.Close()
			if statErr == nil && !stat.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("Cache-Control", indexCacheControl)
		if indexETag != "" {
			w.Header().Set("ETag", indexETag)
			if r.Header.Get("If-None-Match") == indexETag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}), nil
}

func computeETag(fsys fs.FS, name string) string {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}
