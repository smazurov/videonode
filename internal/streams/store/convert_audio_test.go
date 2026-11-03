package store

import (
	"os"
	"strings"
	"testing"
)

func TestStreamAudio_OffsetRoundTrip(t *testing.T) {
	v := V2Stream{
		ID:       "s",
		Upstream: "source:x",
		Audio:    V2AudioConfig{Devices: []string{"hw:Lyra,0"}, Codec: "opus", OffsetMS: -120},
	}
	got := pipelineStreamToV2(pipelineStreamFromV2(v))
	if got.Audio.OffsetMS != -120 || got.Audio.Devices[0] != "hw:Lyra,0" {
		t.Errorf("audio did not round-trip: %+v", got.Audio)
	}
}

func TestStreamAudio_OffsetPersistsAsOffsetMS(t *testing.T) {
	repo, path := setupTestStore(t)
	if err := repo.AddV2Stream(V2Stream{ID: "s", Upstream: "source:x", Audio: V2AudioConfig{OffsetMS: 80}}); err != nil {
		t.Fatalf("add: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(raw), "offset_ms = 80") {
		t.Errorf("expected offset_ms in TOML, got:\n%s", raw)
	}
}
