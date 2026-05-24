package recording

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streaming"
)

func setupTestLogging(t *testing.T) {
	t.Helper()
	logging.Initialize(logging.Config{Level: "error"})
}

func TestAnnexBMarshal(t *testing.T) {
	tests := []struct {
		name string
		au   [][]byte
		want []byte
	}{
		{
			name: "single NAL",
			au:   [][]byte{{0x65, 0x01, 0x02}},
			want: []byte{0x00, 0x00, 0x00, 0x01, 0x65, 0x01, 0x02},
		},
		{
			name: "multiple NALs",
			au:   [][]byte{{0x67, 0xAA}, {0x68, 0xBB}, {0x65, 0xCC}},
			want: []byte{
				0x00, 0x00, 0x00, 0x01, 0x67, 0xAA,
				0x00, 0x00, 0x00, 0x01, 0x68, 0xBB,
				0x00, 0x00, 0x00, 0x01, 0x65, 0xCC,
			},
		},
		{
			name: "empty AU",
			au:   [][]byte{},
			want: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := annexBMarshal(tt.au)
			if !bytes.Equal(got, tt.want) {
				t.Errorf("annexBMarshal() = %x, want %x", got, tt.want)
			}
		})
	}
}

func TestPrependH264Params(t *testing.T) {
	sps := []byte{0x67, 0x42, 0x00, 0x1e}
	pps := []byte{0x68, 0xce, 0x38, 0x80}
	idr := []byte{0x65, 0x88, 0x80} // NAL type 5 (IDR)

	t.Run("prepends when missing", func(t *testing.T) {
		au := [][]byte{idr}
		result := prependH264Params(au, sps, pps)
		if len(result) != 3 {
			t.Fatalf("expected 3 NALUs, got %d", len(result))
		}
		if h264.NALUType(result[0][0]&0x1F) != h264.NALUTypeSPS {
			t.Error("first NAL should be SPS")
		}
		if h264.NALUType(result[1][0]&0x1F) != h264.NALUTypePPS {
			t.Error("second NAL should be PPS")
		}
	})

	t.Run("skips when present", func(t *testing.T) {
		au := [][]byte{sps, pps, idr}
		result := prependH264Params(au, sps, pps)
		if len(result) != 3 {
			t.Fatalf("expected 3 NALUs (unchanged), got %d", len(result))
		}
	})

	t.Run("skips when params empty", func(t *testing.T) {
		au := [][]byte{idr}
		result := prependH264Params(au, nil, nil)
		if len(result) != 1 {
			t.Fatalf("expected 1 NALU (unchanged), got %d", len(result))
		}
	})
}

func TestPrependH265Params(t *testing.T) {
	// H.265 NAL header is 2 bytes: (type << 1) in first byte
	vps := []byte{(byte(h265.NALUType_VPS_NUT) << 1), 0x01, 0x00}
	sps := []byte{(byte(h265.NALUType_SPS_NUT) << 1), 0x01, 0x00}
	pps := []byte{(byte(h265.NALUType_PPS_NUT) << 1), 0x01, 0x00}
	idr := []byte{(byte(h265.NALUType_IDR_W_RADL) << 1), 0x01, 0x00}

	t.Run("prepends when missing", func(t *testing.T) {
		au := [][]byte{idr}
		result := prependH265Params(au, vps, sps, pps)
		if len(result) != 4 {
			t.Fatalf("expected 4 NALUs, got %d", len(result))
		}
	})

	t.Run("skips when present", func(t *testing.T) {
		au := [][]byte{vps, sps, pps, idr}
		result := prependH265Params(au, vps, sps, pps)
		if len(result) != 4 {
			t.Fatalf("expected 4 NALUs (unchanged), got %d", len(result))
		}
	})
}

func TestWriteSnapshotFile(t *testing.T) {
	setupTestLogging(t)

	tests := []struct {
		name      string
		kind      SnapshotKind
		id        string
		wantPath  string // path prefix after baseDir (everything before the timestamp filename)
		wantPanic bool
	}{
		{
			name:     "source kind",
			kind:     SnapshotKindSource,
			id:       "hdmi-slides",
			wantPath: "sources/hdmi-slides",
		},
		{
			name:     "stream kind",
			kind:     SnapshotKindStream,
			id:       "main-archive",
			wantPath: "streams/main-archive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			payload := []byte("fake-jpeg-bytes")

			rel, err := writeSnapshotFile(payload, tt.kind, tt.id, baseDir)
			if err != nil {
				t.Fatalf("writeSnapshotFile: %v", err)
			}

			// Returned path must be relative and live under <kind>/<id>/.
			if filepath.IsAbs(rel) {
				t.Errorf("returned path %q is absolute, want relative", rel)
			}
			if !strings.HasPrefix(rel, tt.wantPath+string(filepath.Separator)) {
				t.Errorf("returned path %q does not start with %q", rel, tt.wantPath)
			}
			if !strings.HasSuffix(rel, ".jpg") {
				t.Errorf("returned path %q does not end with .jpg", rel)
			}

			// File on disk must exist and carry the exact payload.
			abs := filepath.Join(baseDir, rel)
			got, err := os.ReadFile(abs)
			if err != nil {
				t.Fatalf("read snapshot file: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Errorf("file contents = %q, want %q", got, payload)
			}
		})
	}
}

func TestCaptureKeyframe_NoVideoTrack(t *testing.T) {
	setupTestLogging(t)

	desc := &description.Session{
		Medias: []*description.Media{
			{
				Formats: []format.Format{
					&format.Opus{},
				},
			},
		},
	}
	stream := streaming.NewStream("test-audio", desc, logging.GetLogger("test"))

	_, err := CaptureKeyframe(stream, 100*time.Millisecond)
	if !errors.Is(err, ErrNoVideoTrack) {
		t.Errorf("expected ErrNoVideoTrack, got %v", err)
	}
}

func TestCaptureKeyframe_Timeout(t *testing.T) {
	setupTestLogging(t)

	desc := &description.Session{
		Medias: []*description.Media{
			{
				Formats: []format.Format{
					&format.H264{},
				},
			},
		},
	}
	stream := streaming.NewStream("test-timeout", desc, logging.GetLogger("test"))

	_, err := CaptureKeyframe(stream, 100*time.Millisecond)
	if !errors.Is(err, ErrTimeout) {
		t.Errorf("expected ErrTimeout, got %v", err)
	}
}
