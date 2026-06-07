package streaming

import (
	"io"
	"log/slog"
	"testing"
)

// h265NAL builds a minimal H265 NAL unit with the given nal_unit_type in its
// two-byte header (type occupies bits 1-6 of the first byte).
func h265NAL(naluType byte, payload ...byte) []byte {
	return append([]byte{naluType << 1, 0x01}, payload...)
}

const (
	h265VPSType   = 32
	h265SPSType   = 33
	h265PPSType   = 34
	h265TrailType = 1
)

func TestPrependH265Params(t *testing.T) {
	vps := h265NAL(h265VPSType, 0xaa)
	sps := h265NAL(h265SPSType, 0xbb)
	pps := h265NAL(h265PPSType, 0xcc)
	slice := h265NAL(h265TrailType, 0xdd)

	c := &SRTConsumer{
		h265VPS: vps,
		h265SPS: sps,
		h265PPS: pps,
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	tests := []struct {
		name     string
		au       [][]byte
		wantLen  int
		wantHead []byte // first NAL type expected, 0 = don't check
	}{
		{"no params prepends all three", [][]byte{slice}, 4, []byte{h265VPSType}},
		{"all present unchanged", [][]byte{vps, sps, pps, slice}, 4, []byte{h265VPSType}},
		{"sps only prepends vps and pps", [][]byte{sps, slice}, 4, []byte{h265VPSType}},
		{"empty nalu skipped", [][]byte{{}, slice}, 5, []byte{h265VPSType}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.prependH265Params(tt.au)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantHead != nil {
				if gotType := (got[0][0] >> 1) & 0x3f; gotType != tt.wantHead[0] {
					t.Errorf("head NAL type = %d, want %d", gotType, tt.wantHead[0])
				}
			}
		})
	}
}

func TestPrependH265ParamsPreservesOriginalTail(t *testing.T) {
	c := &SRTConsumer{
		h265VPS: h265NAL(h265VPSType),
		h265SPS: h265NAL(h265SPSType),
		h265PPS: h265NAL(h265PPSType),
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	slice := h265NAL(h265TrailType, 0x99)
	got := c.prependH265Params([][]byte{slice})
	tail := got[len(got)-1]
	if (tail[0]>>1)&0x3f != h265TrailType {
		t.Fatalf("tail NAL type = %d, want %d", (tail[0]>>1)&0x3f, h265TrailType)
	}
}
