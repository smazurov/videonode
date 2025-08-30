package types

// RateControlMode represents the rate control strategy
type RateControlMode string

const (
	RateControlCBR RateControlMode = "cbr" // Constant bitrate
	RateControlVBR RateControlMode = "vbr" // Variable bitrate
	RateControlCRF RateControlMode = "crf" // Constant rate factor (quality-based)
	RateControlCQP RateControlMode = "cqp" // Constant quantization parameter
)

// QualityParams represents quality and rate control settings
type QualityParams struct {
	Mode             RateControlMode
	TargetBitrate    *float64 // Mbps
	MaxBitrate       *float64 // Mbps
	MinBitrate       *float64 // Mbps
	BufferSize       *float64 // Mbps
	Quality          *int     // 0-51 for CRF/CQP
	Preset           *string  // fast, medium, slow
	BFrames          *int     // B-frame count
	KeyframeInterval *int     // GOP size
}

// ValidationResults represents the complete validation results
type ValidationResults struct {
	Timestamp      string          `toml:"timestamp" json:"timestamp"`
	FFmpegVersion  string          `toml:"ffmpeg_version" json:"ffmpeg_version"`
	TestDuration   int             `toml:"test_duration" json:"test_duration"`
	TestResolution string          `toml:"test_resolution" json:"test_resolution"`
	H264           CodecValidation `toml:"h264" json:"h264"`
	H265           CodecValidation `toml:"h265" json:"h265"`
}

// CodecValidation represents validation results for a specific codec
type CodecValidation struct {
	Working []string `toml:"working" json:"working"`
	Failed  []string `toml:"failed" json:"failed"`
}
