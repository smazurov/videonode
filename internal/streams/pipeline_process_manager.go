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
		OwnerStreamID: spec.ID,
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
// at it. When spec.Upstream is set, the upstream is honored verbatim and
// the implicit same-id Source is skipped — the v2 EntityStore owns the
// referenced source / composer.
func (m *pipelineProcessManager) applySpec(spec StreamSpec) error {
	upstream := spec.Upstream
	if upstream == "" {
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
		upstream = pipeline.SourceIDFor(spec.ID)
	}

	r := resolveEncoder(spec.FFmpeg.Codec, m.validation)
	stream := pipeline.Stream{
		ID:       spec.ID,
		Name:     spec.Name,
		Upstream: upstream,
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

// Start spawns or reapplies the per-stream pipeline (source + encoder)
// using the stored StreamSpec. Legacy entry point — B9's split moves
// callers toward ApplySource / ApplyStream directly.
func (m *pipelineProcessManager) Start(streamID string) error {
	spec, ok := m.store.GetStream(streamID)
	if !ok {
		return fmt.Errorf("stream %s not found", streamID)
	}
	return m.applySpec(spec)
}

// Stop tears down the per-stream pipeline (encoder + source).
func (m *pipelineProcessManager) Stop(streamID string) error {
	if err := m.pipe.DeleteStream(streamID); err != nil {
		m.logger.Warn("Stop: DeleteStream failed", "stream_id", streamID, "error", err)
	}
	return m.pipe.DeleteSource(streamID)
}

// Restart reapplies the spec, which the pool treats as a respawn.
func (m *pipelineProcessManager) Restart(streamID string) error {
	return m.Start(streamID)
}

// GetStatus reports the supervised encoder process state for a stream.
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

// StartAll applies every stored stream spec to the pipeline.
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

// StopAll stops every supervised process in the pool.
func (m *pipelineProcessManager) StopAll() {
	m.pipe.Pool().StopAll()
}

// IsRunning reports whether the stream's encoder process is currently up.
func (m *pipelineProcessManager) IsRunning(streamID string) bool {
	return m.pipe.Pool().IsRunning(pipeline.EncoderIDFor(streamID))
}

// IsCrashed reports whether the stream's encoder is in the error state.
func (m *pipelineProcessManager) IsCrashed(streamID string) bool {
	info := m.pipe.Pool().GetStatus(pipeline.EncoderIDFor(streamID))
	return info.State == "error"
}

// CaptureSourceSnapshot pulls a JPEG snapshot from the source by id.
func (m *pipelineProcessManager) CaptureSourceSnapshot(sourceID string) ([]byte, error) {
	return m.CaptureRawSnapshot(sourceID)
}

// CaptureRawSnapshot dials the source's gRPC Snapshot RPC and converts
// the returned NV12 frame to JPEG.
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

// ApplySource forwards a canonical pipeline.Source to the supervised
// pipeline. Idempotent; safe to call on updates.
func (m *pipelineProcessManager) ApplySource(src pipeline.Source) error {
	return m.pipe.ApplySource(src)
}

// ApplyComposer forwards a canonical pipeline.Composer to the supervised
// pipeline. Idempotent; safe to call on updates.
func (m *pipelineProcessManager) ApplyComposer(c pipeline.Composer) error {
	return m.pipe.ApplyComposer(c)
}

// ApplyStream forwards a canonical pipeline.Stream to the supervised
// pipeline. The encoder is (re)spawned with the resolved upstream
// (source or composer SCM socket).
func (m *pipelineProcessManager) ApplyStream(s pipeline.Stream) error {
	return m.pipe.ApplyStream(s)
}

// DeleteSource stops and forgets the producer process for src.
func (m *pipelineProcessManager) DeleteSource(id string) error {
	return m.pipe.DeleteSource(id)
}

// DeleteComposer stops and forgets the composer process.
func (m *pipelineProcessManager) DeleteComposer(id string) error {
	return m.pipe.DeleteComposer(id)
}

// DeleteStreamEntity stops and forgets the encoder process. Named
// "Entity" to avoid collision with the legacy StreamProcessManager.Stop
// (which also exists on this type and is what the legacy StreamService
// drives).
func (m *pipelineProcessManager) DeleteStreamEntity(id string) error {
	return m.pipe.DeleteStream(id)
}
