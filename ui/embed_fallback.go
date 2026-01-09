//go:build !ui_embed

// Package ui provides a fallback handler when the frontend is not embedded.
package ui

import (
	"net/http"
)

// Handler returns an http.Handler that redirects to API docs
// when the UI is not built.
func Handler() (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs", http.StatusFound)
	}), nil
}
