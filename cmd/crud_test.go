package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeAPI is a tiny in-memory REST server used to exercise the source/composer/
// stream CRUD subcommands without standing up the real daemon.
type fakeAPI struct {
	t         *testing.T
	gets      map[string]any
	lastWrite struct {
		method string
		path   string
		body   string
	}
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	f := &fakeAPI{t: t, gets: map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "videonode" || p != "videonode" {
			http.Error(w, "auth required", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.lastWrite.method = r.Method
		f.lastWrite.path = r.URL.Path
		f.lastWrite.body = string(body)

		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			v, ok := f.gets[r.URL.Path]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(v)
		case http.MethodPost:
			_, _ = w.Write([]byte(`{"created":true}`))
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	return f, srv
}

func TestCreateSourceCmd_List(t *testing.T) {
	api, srv := newFakeAPI(t)
	defer srv.Close()
	api.gets["/api/sources"] = []map[string]any{{"id": "cam-a"}, {"id": "cam-b"}}

	cmd := CreateSourceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list", "--api-url", srv.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "cam-a") || !strings.Contains(out.String(), "cam-b") {
		t.Errorf("expected list output to contain cam-a + cam-b; got:\n%s", out.String())
	}
}

func TestCreateComposerCmd_Get(t *testing.T) {
	api, srv := newFakeAPI(t)
	defer srv.Close()
	api.gets["/api/composers/main"] = map[string]any{"id": "main", "canvas": map[string]int{"w": 1920, "h": 1080}}

	cmd := CreateComposerCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"get", "main", "--api-url", srv.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "1920") {
		t.Errorf("expected canvas width in output; got:\n%s", out.String())
	}
}

func TestCreateStreamCmd_Create(t *testing.T) {
	api, srv := newFakeAPI(t)
	defer srv.Close()

	cmd := CreateStreamCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(`{"stream_id":"archive","upstream":"composer:main"}`))
	cmd.SetArgs([]string{"create", "--api-url", srv.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if api.lastWrite.method != "POST" || api.lastWrite.path != "/api/streams" {
		t.Errorf("expected POST /api/streams, got %s %s", api.lastWrite.method, api.lastWrite.path)
	}
	if !strings.Contains(api.lastWrite.body, "archive") {
		t.Errorf("body missing stream_id: %s", api.lastWrite.body)
	}
}

func TestCreateSourceCmd_Delete(t *testing.T) {
	api, srv := newFakeAPI(t)
	defer srv.Close()

	cmd := CreateSourceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"delete", "cam-a", "--api-url", srv.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if api.lastWrite.method != "DELETE" || api.lastWrite.path != "/api/sources/cam-a" {
		t.Errorf("expected DELETE /api/sources/cam-a, got %s %s", api.lastWrite.method, api.lastWrite.path)
	}
}
