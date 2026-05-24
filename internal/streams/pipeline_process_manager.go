package streams

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/smazurov/videonode/internal/encoders"
	"github.com/smazurov/videonode/internal/encoders/validation"
	"github.com/smazurov/videonode/internal/ffmpeg"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streams/pipeline"
	"github.com/smazurov/videonode/internal/streams/pipelinectl"
	"github.com/smazurov/videonode/internal/types"
)

// buildEncoderPreviewCommand returns the shell command the Pipeline's
// EncoderStage would emit for this spec, without spawning anything.
// Stub-grade for the B1 worktree: only the source-direct path is wired;
// the canvas path will be reintroduced by B9's full service-layer rewrite.
func buildEncoderPreviewCommand(
	spec StreamSpec,
	rtspHost string,
	deviceResolver func(string) string,
	provider types.ValidationProvider,
) (string, error) {
	if spec.ID == "" {
		return "", errors.New("buildEncoderPreviewCommand: spec.ID is required")
	}
	_ = deviceResolver // accepted for signature parity; new model resolves at ApplySource time
	video := pipeline.ProducerFrameSource{Socket: pipeline.SCMSocketPathFor(spec.ID)}

	r := resolveEncoder(spec.FFmpeg.Codec, provider)
	enc := &pipeline.EncoderStage{
		StreamID_: spec.ID,
		Media: pipeline.MediaSource{
			Video: video,
			Audio: pipeline.ALSADirectAudio{Config: audioFromSpec(spec)},
		},
		Cfg: pipeline.EncoderConfig{
			Codec:        spec.FFmpeg.Codec,
			EncoderName:  r.Name,
			GlobalArgs:   r.GlobalArgs,
			VideoFilters: r.VideoFilters,
			Bitrate:      bitrateFromSpec(spec),
		},
		Publish:           publishFromSpec(spec, rtspHost),
		CustomEncoderArgs: spec.CustomFFmpegCommand,
		VNSinkBin:         "vn-sink",
	}
	argv, _, err := enc.Command()
	if err != nil {
		return "", err
	}
	if len(argv) >= 3 && argv[0] == "/bin/sh" {
		return argv[2], nil
	}
	return strings.Join(argv, " "), nil
}

// audioFromSpec lifts the legacy single AudioDevice field into the new
// AudioConfig shape. Multi-device routing arrives with B9.
func audioFromSpec(spec StreamSpec) pipeline.AudioConfig {
	if spec.FFmpeg.AudioDevice == "" {
		return pipeline.AudioConfig{}
	}
	return pipeline.AudioConfig{Devices: []string{spec.FFmpeg.AudioDevice}}
}

func bitrateFromSpec(spec StreamSpec) string {
	if spec.FFmpeg.QualityParams != nil && spec.FFmpeg.QualityParams.TargetBitrate != nil {
		return fmt.Sprintf("%.0fM", *spec.FFmpeg.QualityParams.TargetBitrate)
	}
	return ""
}

func publishFromSpec(spec StreamSpec, rtspHost string) []pipeline.PublishTarget {
	host := rtspHost
	if host == "" {
		host = "127.0.0.1:8554"
	}
	return []pipeline.PublishTarget{{
		Type: "rtsp",
		URL:  fmt.Sprintf("rtsp://%s/%s", host, spec.ID),
	}}
}

// pipelineProcessManager translates StreamSpec into the new
// source+stream Apply calls. Stub-grade for B1; B9 replaces this with
// the SourceService / ComposerService / StreamService split.
type pipelineProcessManager struct {
	pipe          *pipeline.Pipeline
	store         Store
	controlServer *pipelinectl.Manager
	rtspHost      string
	validation    types.ValidationProvider
	logger        logging.Logger
}

// NewPipelineProcessManager constructs a StreamProcessManager backed by
// the new id-keyed pipeline. Canvas specs degrade to single-source mode
// in this stub; multi-input composer wiring lives in B9.
func NewPipelineProcessManager(
	pipe *pipeline.Pipeline,
	store Store,
	controlServer *pipelinectl.Manager,
	rtspPort string,
) StreamProcessManager {
	return &pipelineProcessManager{
		pipe:          pipe,
		store:         store,
		controlServer: controlServer,
		rtspHost:      resolveRTSPHost(rtspPort),
		validation:    NewValidationService(store),
		logger:        logging.GetLogger("pipeline_pm"),
	}
}

type resolvedEncoder struct {
	Name         string
	GlobalArgs   []string
	VideoFilters string
}

func resolveEncoder(codec string, provider types.ValidationProvider) resolvedEncoder {
	if codec == "" {
		codec = "h264"
	}
	if provider != nil {
		if cfg, err := encoders.MapAPICodec(codec, provider); err == nil && cfg != nil {
			r := resolvedEncoder{Name: cfg.EncoderName}
			if cfg.Settings != nil {
				r.GlobalArgs = append([]string(nil), cfg.Settings.GlobalArgs...)
				r.VideoFilters = cfg.Settings.VideoFilters
			}
			return r
		}
	}
	return resolvedEncoder{Name: validation.AutodetectEncoder(codec)}
}

// applySpec applies one stream spec to the pipeline. Stub: one Source
// per stream id (TestMode lifts from spec.TestMode), one Stream pointing
// at it.
func (m *pipelineProcessManager) applySpec(spec StreamSpec) error {
	src := pipeline.Source{
		ID:       spec.ID,
		Device:   "",
		TestMode: spec.TestMode,
	}
	if !spec.TestMode {
		src.Device = spec.Device
	}
	if err := m.pipe.ApplySource(src); err != nil {
		return fmt.Errorf("applySpec: ApplySource: %w", err)
	}

	r := resolveEncoder(spec.FFmpeg.Codec, m.validation)
	stream := pipeline.Stream{
		ID:       spec.ID,
		Name:     spec.Name,
		Upstream: pipeline.SourceIDFor(spec.ID),
		Audio:    audioFromSpec(spec),
		Encoder: pipeline.EncoderConfig{
			Codec:        spec.FFmpeg.Codec,
			EncoderName:  r.Name,
			GlobalArgs:   r.GlobalArgs,
			VideoFilters: r.VideoFilters,
			Bitrate:      bitrateFromSpec(spec),
		},
		Publish:           publishFromSpec(spec, m.rtspHost),
		CustomEncoderArgs: spec.CustomFFmpegCommand,
		CreatedAt:         spec.CreatedAt,
		UpdatedAt:         spec.UpdatedAt,
	}
	return m.pipe.ApplyStream(stream)
}

func (m *pipelineProcessManager) Start(streamID string) error {
	spec, ok := m.store.GetStream(streamID)
	if !ok {
		return fmt.Errorf("stream %s not found", streamID)
	}
	return m.applySpec(spec)
}

func (m *pipelineProcessManager) Stop(streamID string) error {
	if err := m.pipe.DeleteStream(streamID); err != nil {
		m.logger.Warn("Stop: DeleteStream failed", "stream_id", streamID, "error", err)
	}
	return m.pipe.DeleteSource(streamID)
}

func (m *pipelineProcessManager) Restart(streamID string) error {
	return m.Start(streamID)
}

func (m *pipelineProcessManager) GetStatus(streamID string) (*ProcessInfo, error) {
	encID := pipeline.EncoderIDFor(streamID)
	info := m.pipe.Pool().GetStatus(encID)
	return &ProcessInfo{
		StreamID:     streamID,
		State:        ProcessState(info.State),
		PID:          info.PID,
		StartedAt:    info.StartedAt,
		RestartCount: info.RestartCount,
		LastError:    info.LastError,
	}, nil
}

func (m *pipelineProcessManager) StartAll() error {
	if m.store == nil {
		return nil
	}
	all := m.store.GetAllStreams()
	var errs []error
	for id, spec := range all {
		if err := m.applySpec(spec); err != nil {
			m.logger.Warn("StartAll: apply failed", "stream_id", id, "error", err)
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (m *pipelineProcessManager) StopAll() {
	m.pipe.Pool().StopAll()
}

func (m *pipelineProcessManager) IsRunning(streamID string) bool {
	return m.pipe.Pool().IsRunning(pipeline.EncoderIDFor(streamID))
}

func (m *pipelineProcessManager) IsCrashed(streamID string) bool {
	info := m.pipe.Pool().GetStatus(pipeline.EncoderIDFor(streamID))
	return info.State == "error"
}

func (m *pipelineProcessManager) CaptureRawSnapshot(sourceStreamID string) ([]byte, error) {
	if m.controlServer == nil {
		return nil, fmt.Errorf("no control server for snapshot")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	resp, err := m.controlServer.Snapshot(ctx, sourceStreamID)
	if err != nil {
		return nil, err
	}
	return ffmpeg.EncodeNV12ToJPEG(resp.GetNv12(), int(resp.GetWidth()), int(resp.GetHeight()))
}

// OwnedBy / CanvasOwner are no-op stubs in the B1 worktree — the new
// model has no implicit ownership (sources are independent entities).
// B9 will surface "which streams reference this source" via the
// composer/stream registries.
func (m *pipelineProcessManager) OwnedBy(string) string     { return "" }
func (m *pipelineProcessManager) CanvasOwner(string) string { return "" }

// PushComposerPerspective is a stub — composer effect routing moves to
// B6's /api/composers/{id}/inputs/{ref}/effect endpoint.
func (m *pipelineProcessManager) PushComposerPerspective(
	string, string, *ffmpeg.PerspectiveConfig,
) (bool, error) {
	return false, nil
}
