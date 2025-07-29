package server

import (
	"github.com/smazurov/videonode/internal/encoders"
)

// Re-export types and functions from encoders package for compatibility
type Encoder = encoders.Encoder
type EncoderList = encoders.EncoderList
type EncoderType = encoders.EncoderType
type EncoderFilter = encoders.EncoderFilter

const (
	VideoEncoder    = encoders.VideoEncoder
	AudioEncoder    = encoders.AudioEncoder
	SubtitleEncoder = encoders.SubtitleEncoder
	Unknown         = encoders.Unknown
)

// Re-export functions from encoders package
var GetFFmpegEncoders = encoders.GetFFmpegEncoders
var FilterEncoders = encoders.FilterEncoders
var IsFFmpegInstalled = encoders.IsFFmpegInstalled
