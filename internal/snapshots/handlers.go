package snapshots

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// RegisterAPI mounts the snapshot routes on `mux`. The four endpoints
// are:
//
//	GET /api/sources/{id}/snapshot.jpg
//	GET /api/sources/{id}/preview.mjpg
//	GET /api/composers/{id}/snapshot.jpg
//	GET /api/composers/{id}/preview.mjpg
//
// All four use the same Cache so multiple viewers share a single
// upstream RPC stream per entity.
func RegisterAPI(mux *http.ServeMux, cache *Cache) {
	mux.HandleFunc("GET /api/sources/{id}/snapshot.jpg", snapshotHandler(cache, KindSource))
	mux.HandleFunc("GET /api/sources/{id}/preview.mjpg", previewHandler(cache, KindSource))
	mux.HandleFunc("GET /api/composers/{id}/snapshot.jpg", snapshotHandler(cache, KindComposer))
	mux.HandleFunc("GET /api/composers/{id}/preview.mjpg", previewHandler(cache, KindComposer))
}

func snapshotHandler(cache *Cache, kind Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		entry, err := cache.Get(r.Context(), kind, id)
		if err != nil {
			writeCacheError(w, err)
			return
		}
		etag := fmt.Sprintf(`"frame-%d"`, entry.FrameIdx)
		if match := r.Header.Get("If-None-Match"); match != "" && match == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", strconv.Itoa(len(entry.JPEG)))
		w.Header().Set("ETag", etag)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(entry.JPEG)
	}
}

func previewHandler(cache *Cache, kind Kind) http.HandlerFunc {
	const boundary = "vnframe"
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}

		fps := parseFPS(r.URL.Query().Get("fps"))

		flusher, isFlusher := w.(http.Flusher)
		if !isFlusher {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Probe once before committing headers so a 404 / 503 surfaces
		// as a regular HTTP error instead of a malformed MJPEG body.
		probeCtx, cancel := context.WithTimeout(r.Context(), cache.cfg.RPCTimeout+500*time.Millisecond)
		_, err := cache.Get(probeCtx, kind, id)
		cancel()
		if err != nil {
			writeCacheError(w, err)
			return
		}

		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+boundary)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusOK)

		handle := cache.Subscribe(kind, id, fps)
		defer handle.Close()

		logger := logging.GetLogger("snapshots")
		logger.Debug("preview opened", logging.KeyKind, string(kind), logging.KeyEntityID, id, logging.KeyFPS, fps)
		defer logger.Debug("preview closed", logging.KeyKind, string(kind), logging.KeyEntityID, id)

		var lastIdx uint64
		for {
			select {
			case <-r.Context().Done():
				return
			case entry, more := <-handle.Frames():
				if !more {
					return
				}
				if entry.FrameIdx == lastIdx {
					continue
				}
				lastIdx = entry.FrameIdx
				if err := writePart(w, boundary, entry.JPEG); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// writePart writes one multipart/x-mixed-replace frame.
func writePart(w http.ResponseWriter, boundary string, jpeg []byte) error {
	hdr := "--" + boundary + "\r\n" +
		"Content-Type: image/jpeg\r\n" +
		"Content-Length: " + strconv.Itoa(len(jpeg)) + "\r\n\r\n"
	if _, err := w.Write([]byte(hdr)); err != nil {
		return err
	}
	if _, err := w.Write(jpeg); err != nil {
		return err
	}
	if _, err := w.Write([]byte("\r\n")); err != nil {
		return err
	}
	return nil
}

// parseFPS extracts the ?fps query param; 0 means "use default".
func parseFPS(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}

func writeCacheError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, ErrNoFrame):
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
	case errors.Is(err, context.DeadlineExceeded):
		http.Error(w, err.Error(), http.StatusGatewayTimeout)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
