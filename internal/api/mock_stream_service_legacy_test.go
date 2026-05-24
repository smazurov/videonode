//go:build !planv2_tests

// Legacy mockStreamService implementation retained until B9 (service
// rewrite) lands. The post-rewrite streams_test.go uses a slimmer
// mockStreamAPISvc behind the planv2_tests tag; meanwhile
// encoders_test.go still talks to the monolithic StreamService and
// needs this mock in the default build.
package api

import (
	"context"

	"github.com/smazurov/videonode/internal/devices"
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/types"
)

type mockStreamService struct {
	streams            map[string]*streams.Stream
	streamSpecs        map[string]*streams.StreamSpec
	lastUpdate         *streams.StreamUpdateParams
	validationProvider types.ValidationProvider
}

func (m *mockStreamService) CreateStream(_ context.Context, _ streams.StreamCreateParams) (*streams.Stream, error) {
	return nil, nil
}

func (m *mockStreamService) UpdateStream(_ context.Context, streamID string, params streams.StreamUpdateParams) (*streams.Stream, error) {
	m.lastUpdate = &params
	if spec, ok := m.streamSpecs[streamID]; ok {
		spec.FFmpeg.Codec = params.Codec
		spec.FFmpeg.InputFormat = params.InputFormat
		spec.FFmpeg.Resolution = params.Resolution
		spec.FFmpeg.FPS = params.FPS
		spec.FFmpeg.AudioDevice = params.AudioDevice
		spec.FFmpeg.Options = params.Options
		spec.FFmpeg.QualityParams = params.QualityParams
		spec.CustomFFmpegCommand = params.CustomFFmpegCommand
		spec.TestMode = params.TestMode
		spec.Canvas = params.Canvas
		spec.Perspective = params.Perspective
		spec.Vision = params.Vision
	}
	return m.streams[streamID], nil
}

func (m *mockStreamService) SetEnabled(_ context.Context, streamID string, enabled bool) (bool, error) {
	if s, ok := m.streams[streamID]; ok {
		s.Enabled = enabled
	}
	return enabled, nil
}

func (m *mockStreamService) UpdatePartial(_ context.Context, streamID string, patch func(*streams.StreamSpec) error) (*streams.Stream, error) {
	spec, ok := m.streamSpecs[streamID]
	if !ok {
		return nil, &streams.StreamError{Code: streams.ErrCodeStreamNotFound}
	}
	if err := patch(spec); err != nil {
		return nil, err
	}
	return m.streams[streamID], nil
}

func (m *mockStreamService) DeleteStream(_ context.Context, _ string) error  { return nil }
func (m *mockStreamService) RestartStream(_ context.Context, _ string) error { return nil }
func (m *mockStreamService) ReleaseCanvas(_ context.Context, _ string) error { return nil }
func (m *mockStreamService) EngageCanvas(_ context.Context, _ string) error  { return nil }

func (m *mockStreamService) GetStream(_ context.Context, streamID string) (*streams.Stream, error) {
	s, ok := m.streams[streamID]
	if !ok {
		return nil, &streams.StreamError{Code: streams.ErrCodeStreamNotFound}
	}
	return s, nil
}

func (m *mockStreamService) GetStreamSpec(_ context.Context, streamID string) (*streams.StreamSpec, error) {
	spec, ok := m.streamSpecs[streamID]
	if !ok {
		return nil, &streams.StreamError{Code: streams.ErrCodeStreamNotFound}
	}
	return spec, nil
}

func (m *mockStreamService) ListStreams(_ context.Context) ([]streams.Stream, error) {
	result := make([]streams.Stream, 0, len(m.streams))
	for _, s := range m.streams {
		result = append(result, *s)
	}
	return result, nil
}

func (m *mockStreamService) ListStreamsWithSpecs(_ context.Context) ([]streams.StreamWithSpec, error) {
	out := make([]streams.StreamWithSpec, 0, len(m.streams))
	for id, s := range m.streams {
		var spec streams.StreamSpec
		if sp, ok := m.streamSpecs[id]; ok && sp != nil {
			spec = *sp
		}
		out = append(out, streams.StreamWithSpec{Stream: *s, Spec: spec})
	}
	return out, nil
}

func (m *mockStreamService) GetFFmpegCommand(_ context.Context, _ string, _ string) (string, bool, error) {
	return "", false, nil
}

func (m *mockStreamService) BroadcastDeviceDiscovery(_ string, _ devices.DeviceInfo, _ string) {
}

func (m *mockStreamService) LoadStreamsFromConfig() error                    { return nil }
func (m *mockStreamService) GetProcessManager() streams.StreamProcessManager { return nil }
func (m *mockStreamService) ValidationProvider() types.ValidationProvider {
	return m.validationProvider
}

func (m *mockStreamService) StartPipeline(_ context.Context) (bool, error) { return false, nil }
func (m *mockStreamService) StopPipeline(_ context.Context) (bool, error)  { return false, nil }
func (m *mockStreamService) PipelineEnabled() bool                         { return true }
