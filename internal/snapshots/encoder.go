package snapshots

import (
	"fmt"

	"github.com/smazurov/videonode/internal/ffmpeg"
)

// FFmpegEncoder is the default Encoder. It spawns ffmpeg per call to
// transcode raw NV12 to JPEG. Cheap at 1-Hz cadence; if preview rates rise
// above a few Hz, swap for a long-lived encoder process.
type FFmpegEncoder struct{}

// EncodeJPEG dispatches on the frame's Format.
func (FFmpegEncoder) EncodeJPEG(f Frame) ([]byte, error) {
	switch f.Format {
	case FormatNV12:
		return ffmpeg.EncodeNV12ToJPEG(f.Bytes, f.Width, f.Height)
	default:
		return nil, fmt.Errorf("snapshots: unsupported format %v", f.Format)
	}
}
