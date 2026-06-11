package streaming

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// RegisterRecordingFiles mounts static playback serving for recordings under
// baseDir:
//
//	GET /api/streams/{stream_id}/recordings/{session}/{path...}
//
// serving init.mp4, segNNNNN.m4s, index.m3u8, thumbnails.vtt/json, and
// thumbs/*.jpg with the right content types. Paths are confined to baseDir.
func RegisterRecordingFiles(mux *http.ServeMux, baseDir string) {
	mux.HandleFunc("GET /api/streams/{stream_id}/recordings/{session}/{path...}",
		recordingFileHandler(baseDir))
}

func recordingFileHandler(baseDir string) http.HandlerFunc {
	root, err := filepath.Abs(baseDir)
	if err != nil {
		root = baseDir
	}
	return func(w http.ResponseWriter, r *http.Request) {
		streamID := r.PathValue("stream_id")
		session := r.PathValue("session")
		rel := r.PathValue("path")
		if streamID == "" || session == "" || rel == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}

		// Confine to root/<stream>/<session>/<rel> with no traversal.
		full := filepath.Join(root, streamID, session, filepath.Clean("/"+rel))
		if !strings.HasPrefix(full, root+string(os.PathSeparator)) {
			http.Error(w, "invalid path", http.StatusBadRequest)
			return
		}

		f, err := os.Open(full)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		defer func() { _ = f.Close() }()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		if ct := recordingContentType(full); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		// Playlists, the growing VTT index, and sprite sheets (the current one
		// is rewritten in place every interval, and VTT cue URLs carry no cache
		// bust) must revalidate; segments and the one-shot poster are immutable.
		switch {
		case strings.Contains(rel, "sprites/"):
			w.Header().Set("Cache-Control", "no-cache")
		default:
			switch strings.ToLower(filepath.Ext(full)) {
			case ".m3u8", ".vtt", ".json":
				w.Header().Set("Cache-Control", "no-cache")
			default:
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
		}

		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	}
}

func recordingContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".m4s":
		return "video/iso.segment"
	case ".mp4":
		return "video/mp4"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".vtt":
		return "text/vtt"
	case ".json":
		return "application/json"
	default:
		return ""
	}
}
