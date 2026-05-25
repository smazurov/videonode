// Package models defines API request and response data structures.
package models

import (
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/smazurov/videonode/internal/ffmpeg"
)

// ProcessStatus is the process pool state surfaced on every entity.
// Mirrors process.State; redefined here to avoid an import cycle.
type ProcessStatus string

// ProcessStatus values.
const (
	ProcessStatusIdle     ProcessStatus = "idle"
	ProcessStatusStarting ProcessStatus = "starting"
	ProcessStatusRunning  ProcessStatus = "running"
	ProcessStatusStopping ProcessStatus = "stopping"
	ProcessStatusError    ProcessStatus = "error"
)

// HealthData contains health check response fields.
type HealthData struct {
	Status  string `json:"status" example:"ok" doc:"Service status"`
	Message string `json:"message" example:"API is healthy" doc:"Status message"`
}

// HealthResponse wraps HealthData for API responses.
type HealthResponse struct {
	Body HealthData
}

// EncoderType represents the category of encoder (video or audio).
type EncoderType string

// Encoder type constants.
const (
	VideoEncoder EncoderType = "video"
	AudioEncoder EncoderType = "audio"
)

// EncoderData contains lists of available video and audio encoders.
type EncoderData struct {
	VideoEncoders []EncoderInfo `json:"video_encoders" doc:"Available video encoders"`
	AudioEncoders []EncoderInfo `json:"audio_encoders" doc:"Available audio encoders"`
	Count         int           `json:"count" example:"15" doc:"Total number of encoders"`
}

// EncoderInfo describes a single encoder with its capabilities.
type EncoderInfo struct {
	Type        EncoderType `json:"type" example:"video" doc:"Encoder type"`
	Name        string      `json:"name" example:"libx264" doc:"Encoder name"`
	Description string      `json:"description" example:"H.264 encoder" doc:"Human-readable description"`
	HWAccel     bool        `json:"hwaccel" example:"false" doc:"Whether this is a hardware-accelerated encoder"`
}

// EncodersResponse wraps EncoderData for API responses.
type EncodersResponse struct {
	Body EncoderData
}

// AudioConfigData mirrors pipeline.AudioConfig: ALSA devices + codec + bitrate.
type AudioConfigData struct {
	Devices []string `json:"devices,omitempty" doc:"ALSA device names; one output audio track per entry"`
	Codec   string   `json:"codec,omitempty" example:"opus" doc:"Audio codec (opus, aac, ...)"`
	Bitrate string   `json:"bitrate,omitempty" example:"128k" doc:"Audio bitrate"`
	Filters string   `json:"filters,omitempty" doc:"Optional shared filter chain"`
}

// EncoderConfigData mirrors pipeline.EncoderConfig.
type EncoderConfigData struct {
	Codec       string `json:"codec,omitempty" example:"h264" doc:"Logical codec (h264, h265, av1)"`
	Bitrate     string `json:"bitrate,omitempty" example:"4M" doc:"Video bitrate"`
	GOP         int    `json:"gop,omitempty" example:"120" doc:"Keyframe interval"`
	BFrames     int    `json:"b_frames,omitempty" example:"0" doc:"Number of B-frames"`
	RateControl string `json:"rate_control,omitempty" example:"cbr" doc:"Rate control mode"`
	Preset      string `json:"preset,omitempty" example:"fast" doc:"Encoder preset"`
}

// PublishTargetData mirrors pipeline.PublishTarget: one output destination.
type PublishTargetData struct {
	Type string `json:"type" example:"rtsp" doc:"Output type (rtsp, srt, hls, ...)"`
	URL  string `json:"url" example:"rtsp://example/stream" doc:"Destination URL"`
}

// StreamData is the slim API view of a stream. Upstream is "source:<id>" or
// "composer:<id>"; the encoder dials whichever SCM socket that resolves to.
type StreamData struct {
	StreamID          string              `json:"stream_id" example:"stream-001" doc:"Unique stream identifier"`
	Name              string              `json:"name,omitempty" example:"Main Archive" doc:"Human-readable stream name"`
	Upstream          string              `json:"upstream" example:"source:hdmi0" doc:"Upstream reference: \"source:<id>\" or \"composer:<id>\""`
	Audio             AudioConfigData     `json:"audio,omitzero" doc:"Audio routing"`
	Encoder           EncoderConfigData   `json:"encoder,omitzero" doc:"Encoder configuration"`
	Publish           []PublishTargetData `json:"publish,omitempty" doc:"Output destinations"`
	CustomEncoderArgs string              `json:"custom_encoder_args,omitempty" doc:"User-supplied ffmpeg encoder args (replaces daemon-generated argv from -c:v onward)"`
	Enabled           bool                `json:"enabled" example:"true" doc:"Runtime state — true when the encoder process is intended to run"`
	Status            ProcessStatus       `json:"status,omitempty" example:"running" enum:"idle,starting,running,stopping,error" doc:"Encoder process pool state"`
	RTSPURL           string              `json:"rtsp_url,omitempty" example:"rtsp://localhost:8554/stream-001" doc:"RTSP playback URL"`
	SRTURL            string              `json:"srt_url,omitempty" example:"srt://localhost:6001?streamid=stream-001" doc:"SRT playback URL"`
	CreatedAt         time.Time           `json:"created_at,omitzero" doc:"When the stream spec was created"`
	UpdatedAt         time.Time           `json:"updated_at,omitzero" doc:"When the stream spec was last updated"`
}

// StreamListData contains a list of all configured streams.
type StreamListData struct {
	Streams []StreamData `json:"streams" doc:"List of configured streams"`
	Count   int          `json:"count" example:"2" doc:"Number of streams"`
}

// StreamListResponse wraps StreamListData for API responses.
type StreamListResponse struct {
	Body StreamListData
}

// StreamRequestData is the create-stream payload.
type StreamRequestData struct {
	StreamID          string              `json:"stream_id" pattern:"^[a-zA-Z0-9_-]+$" minLength:"1" maxLength:"50" example:"my-stream-001" doc:"Stream identifier"`
	Name              string              `json:"name,omitempty" example:"Main Archive" doc:"Human-readable stream name"`
	Upstream          string              `json:"upstream" pattern:"^(source|composer):[a-zA-Z0-9_-]+$" example:"source:hdmi0" doc:"Upstream reference"`
	Audio             AudioConfigData     `json:"audio,omitzero" doc:"Audio routing"`
	Encoder           EncoderConfigData   `json:"encoder,omitzero" doc:"Encoder configuration"`
	Publish           []PublishTargetData `json:"publish,omitempty" doc:"Output destinations"`
	CustomEncoderArgs string              `json:"custom_encoder_args,omitempty" doc:"User-supplied ffmpeg encoder args"`
	Enabled           *bool               `json:"enabled,omitempty" doc:"Initial enabled state (default true)"`
}

// Resolve validates the upstream reference shape.
func (d *StreamRequestData) Resolve(_ huma.Context, prefix *huma.PathBuffer) []error {
	if d.Upstream == "" {
		return []error{&huma.ErrorDetail{
			Location: prefix.With("upstream"),
			Message:  "upstream is required (\"source:<id>\" or \"composer:<id>\")",
			Value:    d.Upstream,
		}}
	}
	return nil
}

// StreamRequest wraps StreamRequestData for API requests.
type StreamRequest struct {
	Body StreamRequestData
}

// StreamUpdateRequestData contains partial-update fields. Pointers/nullables
// distinguish "not set" from "set to zero/null".
type StreamUpdateRequestData struct {
	Name              *string                       `json:"name,omitempty" doc:"Human-readable name"`
	Upstream          *string                       `json:"upstream,omitempty" pattern:"^(source|composer):[a-zA-Z0-9_-]+$" doc:"Upstream reference"`
	Audio             Nullable[AudioConfigData]     `json:"audio,omitzero" doc:"Audio routing (null to clear)"`
	Encoder           Nullable[EncoderConfigData]   `json:"encoder,omitzero" doc:"Encoder configuration (null to clear)"`
	Publish           Nullable[[]PublishTargetData] `json:"publish,omitzero" doc:"Output destinations (null to clear)"`
	CustomEncoderArgs *string                       `json:"custom_encoder_args,omitempty" doc:"User-supplied ffmpeg encoder args"`
	Enabled           *bool                         `json:"enabled,omitempty" doc:"Runtime enabled state"`
}

// StreamUpdateRequest wraps StreamUpdateRequestData for API requests.
type StreamUpdateRequest struct {
	Body StreamUpdateRequestData
}

// StreamResponse wraps StreamData for API responses.
type StreamResponse struct {
	Body StreamData
}

// StreamStatusData contains basic status information about a stream.
type StreamStatusData struct {
	StreamID  string    `json:"stream_id" example:"stream-001" doc:"Unique stream identifier"`
	StartTime time.Time `json:"start_time,omitzero" doc:"When the stream was loaded into memory"`
}

// StreamStatusResponse wraps StreamStatusData for API responses.
type StreamStatusResponse struct {
	Body StreamStatusData
}

// ErrorData contains error information for failed API requests.
type ErrorData struct {
	Status  string `json:"status" example:"error" doc:"Error status"`
	Message string `json:"message" example:"Device not found" doc:"Error message"`
}

// ErrorResponse wraps ErrorData for API responses.
type ErrorResponse struct {
	Body ErrorData
}

// OptionsData contains all available FFmpeg configuration options.
type OptionsData struct {
	Options []ffmpeg.Option `json:"options" doc:"All available FFmpeg options with metadata"`
}

// OptionsResponse wraps OptionsData for API responses.
type OptionsResponse struct {
	Body OptionsData
}

// VersionData contains build and version information about the application.
type VersionData struct {
	Version   string `json:"version" example:"dev" doc:"Application version"`
	GitCommit string `json:"git_commit" example:"abc1234" doc:"Git commit SHA"`
	BuildDate string `json:"build_date" example:"2024-12-15 14:30" doc:"Build timestamp"`
	BuildID   string `json:"build_id" example:"a1b2c3d4" doc:"Unique build identifier"`
	GoVersion string `json:"go_version" example:"go1.21.0" doc:"Go compiler version"`
	Compiler  string `json:"compiler" example:"gc" doc:"Compiler used"`
	Platform  string `json:"platform" example:"linux/amd64" doc:"Platform"`
}

// VersionResponse wraps VersionData for API responses.
type VersionResponse struct {
	Body VersionData
}

// FFmpegCommandData contains the FFmpeg command for a specific stream.
type FFmpegCommandData struct {
	StreamID string `json:"stream_id" example:"stream-001" doc:"Stream identifier"`
	Command  string `json:"command" example:"ffmpeg -f v4l2 -i /dev/video0 ..." doc:"Complete FFmpeg command"`
	IsCustom bool   `json:"is_custom" example:"false" doc:"Whether this is a custom command or auto-generated"`
}

// FFmpegCommandResponse wraps FFmpegCommandData for API responses.
type FFmpegCommandResponse struct {
	Body FFmpegCommandData
}

// FFmpegCommandRequest contains a custom FFmpeg command to set for a stream.
type FFmpegCommandRequest struct {
	Body struct {
		Command string `json:"command" minLength:"1" example:"ffmpeg -f v4l2 -i /dev/video0 ..." doc:"Custom FFmpeg command to use"`
	}
}
