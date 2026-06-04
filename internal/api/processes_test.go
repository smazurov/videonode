package api

import (
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/smazurov/videonode/internal/events"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// stubProcessesProvider records the id passed to RestartProcess and returns a
// configurable error so the route's denormalize + error mapping can be
// asserted without a real supervised pipeline.
type stubProcessesProvider struct {
	views      []pipeline.ProcessView
	restartErr error
	lastID     string
}

func (s *stubProcessesProvider) Snapshot() []pipeline.ProcessView { return s.views }
func (s *stubProcessesProvider) RestartProcess(id string) error {
	s.lastID = id
	return s.restartErr
}

// The API speaks "source:"; the pool speaks "producer:". The restart route
// must denormalize before reaching the provider.
func TestRestartProcessRoute_DenormalizesSourcePrefix(t *testing.T) {
	prov := &stubProcessesProvider{}
	_, api := humatest.New(t)
	RegisterProcessesRoutes(api, prov)

	resp := api.Post("/api/processes/source:hdmi0/restart")
	if resp.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body=%s; want 204", resp.Code, resp.Body.String())
	}
	if prov.lastID != "producer:hdmi0" {
		t.Errorf("provider got id %q; want producer:hdmi0", prov.lastID)
	}
}

// composer/encoder ids pass through unchanged.
func TestRestartProcessRoute_PassesThroughComposerEncoder(t *testing.T) {
	for _, id := range []string{"composer:main", "encoder:stream-001"} {
		prov := &stubProcessesProvider{}
		_, api := humatest.New(t)
		RegisterProcessesRoutes(api, prov)

		if resp := api.Post("/api/processes/" + id + "/restart"); resp.Code != http.StatusNoContent {
			t.Fatalf("%s: status %d", id, resp.Code)
		}
		if prov.lastID != id {
			t.Errorf("provider got id %q; want %q", prov.lastID, id)
		}
	}
}

func TestRestartProcessRoute_NotFoundMapsTo404(t *testing.T) {
	prov := &stubProcessesProvider{restartErr: pipeline.ErrNoSuchProcess}
	_, api := humatest.New(t)
	RegisterProcessesRoutes(api, prov)

	if resp := api.Post("/api/processes/encoder:ghost/restart"); resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d; want 404", resp.Code)
	}
}

func TestRestartProcessRoute_GenericErrorMapsTo500(t *testing.T) {
	prov := &stubProcessesProvider{restartErr: errors.New("boom")}
	_, api := humatest.New(t)
	RegisterProcessesRoutes(api, prov)

	if resp := api.Post("/api/processes/composer:main/restart"); resp.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; want 500", resp.Code)
	}
}

// GET /api/processes must surface "source:" ids (not the internal "producer:")
// so the list round-trips with the restart endpoint.
func TestProcessEntries_NormalizeProducerID(t *testing.T) {
	got := toProcessEntries([]pipeline.ProcessView{{ID: "producer:hdmi0", Kind: "producer"}})
	if len(got) != 1 || got[0].ID != "source:hdmi0" || got[0].Kind != "source" {
		t.Fatalf("toProcessEntries normalize = %+v; want id=source:hdmi0 kind=source", got)
	}
}

// The SSE process push must apply the same edge normalization as the REST
// list so both key on identical ids/kinds.
func TestNormalizeProcessesEvent(t *testing.T) {
	in := events.ProcessesEvent{
		Timestamp: "2025-01-27T10:30:00Z",
		Processes: []events.ProcessInfo{
			{ID: "producer:hdmi0", Kind: "producer", State: "running"},
			{ID: "composer:main", Kind: "composer", State: "running"},
			{ID: "encoder:stream-001", Kind: "encoder", State: "idle"},
		},
	}
	got := normalizeProcessesEvent(in)
	if got.Timestamp != in.Timestamp {
		t.Errorf("timestamp = %q; want %q", got.Timestamp, in.Timestamp)
	}
	want := []struct{ id, kind string }{
		{"source:hdmi0", "source"},
		{"composer:main", "composer"},
		{"encoder:stream-001", "encoder"},
	}
	for i, w := range want {
		if got.Processes[i].ID != w.id || got.Processes[i].Kind != w.kind {
			t.Errorf("row %d = (%q,%q); want (%q,%q)", i,
				got.Processes[i].ID, got.Processes[i].Kind, w.id, w.kind)
		}
	}
	// Source rows must not be mutated in place.
	if in.Processes[0].ID != "producer:hdmi0" {
		t.Errorf("input mutated: %q", in.Processes[0].ID)
	}
}

func TestNormalizeProcessIDRoundTrip(t *testing.T) {
	cases := map[string]string{
		"producer:hdmi0":     "source:hdmi0",
		"composer:main":      "composer:main",
		"encoder:stream-001": "encoder:stream-001",
	}
	for raw, want := range cases {
		if got := normalizeProcessID(raw); got != want {
			t.Errorf("normalizeProcessID(%q) = %q; want %q", raw, got, want)
		}
		if got := denormalizeProcessID(normalizeProcessID(raw)); got != raw {
			t.Errorf("round-trip(%q) = %q; want %q", raw, got, raw)
		}
	}
}
