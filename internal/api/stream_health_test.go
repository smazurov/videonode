package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/smazurov/videonode/internal/api/models"
	"github.com/smazurov/videonode/internal/metrics"
	"github.com/smazurov/videonode/internal/streaming"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

type healthStreamService struct{ StreamService }

func (healthStreamService) Get(_ context.Context, id string) (*pipeline.Stream, error) {
	return &pipeline.Stream{ID: id, Upstream: "source:cam"}, nil
}

func (healthStreamService) List(context.Context) ([]pipeline.Stream, error) {
	return []pipeline.Stream{{ID: "s1", Upstream: "source:cam"}}, nil
}

func (healthStreamService) EncoderStatus(string) models.ProcessStatus {
	return models.ProcessStatusRunning
}

type restHealthProvider struct {
	streaming.HealthProvider
	snapshot streaming.StreamHealth
}

func (p restHealthProvider) Snapshot(string) streaming.StreamHealth { return p.snapshot }

func TestStreamREST_IncludesHealthOnInitialLoad(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Millisecond)
	provider := restHealthProvider{snapshot: streaming.StreamHealth{
		SampledAt: now, StallAfterMS: 5000,
		Encoder: metrics.EncoderHealth{
			State: "running", StartedAtUS: 123,
			FFmpeg: metrics.FFmpegHealth{
				Progress: metrics.ActivityHealth{State: "stalled", Count: 10},
				Frames:   metrics.ActivityHealth{State: "stalled", Count: 300},
				Stats:    metrics.FFmpegStreamMetrics{FPS: 30, Speed: 1},
			},
		},
		Tracks: []streaming.TrackHealth{},
	}}
	_, testAPI := humatest.New(t)
	s := &Server{api: testAPI, streamService: healthStreamService{}, options: &Options{StreamHealth: provider}}
	s.registerStreamRoutes()

	for _, path := range []string{"/api/streams/s1", "/api/streams"} {
		t.Run(path, func(t *testing.T) {
			response := testAPI.Get(path)
			if response.Code != http.StatusOK {
				t.Fatalf("status=%d: %s", response.Code, response.Body.String())
			}
			var item models.StreamData
			if path == "/api/streams" {
				var list models.StreamListData
				if err := json.Unmarshal(response.Body.Bytes(), &list); err != nil {
					t.Fatal(err)
				}
				if len(list.Streams) != 1 {
					t.Fatal("missing stream")
				}
				item = list.Streams[0]
			} else if err := json.Unmarshal(response.Body.Bytes(), &item); err != nil {
				t.Fatal(err)
			}
			if item.Health == nil || item.Health.Encoder.FFmpeg.Progress.State != "stalled" ||
				item.Health.Encoder.FFmpeg.Stats.FPS != 30 || !item.Health.SampledAt.Equal(now) {
				t.Fatalf("initial REST health = %+v", item.Health)
			}
		})
	}
}
