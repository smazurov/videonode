package streaming

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

// minimal valid-ish H264 SPS/PPS (baseline 16x16) so codecs.Fill can derive
// dimensions for the init segment.
var (
	testSPS = []byte{0x67, 0x42, 0xc0, 0x0a, 0xa6, 0x11, 0x11, 0xe8, 0x40, 0x00, 0x00, 0x03, 0x00, 0x40, 0x00, 0x00, 0x0c, 0x23, 0xc6, 0x0c, 0x65, 0x80}
	testPPS = []byte{0x68, 0xce, 0x3c, 0x80}
)

func feedGOPs(t *testing.T, m *recMuxer, gops, framesPerGOP int, frameDur int64) {
	t.Helper()
	var dts int64
	for range gops {
		for f := range framesPerGOP {
			keyframe := f == 0
			var au [][]byte
			if keyframe {
				// IDR NAL (type 5) — IsRandomAccess keys off this.
				au = [][]byte{{0x65, 0x88, 0x84, 0x00}}
			} else {
				// non-IDR slice (type 1)
				au = [][]byte{{0x41, 0x9a, 0x00, 0x00}}
			}
			if err := m.writeVideo(dts, 0, au, keyframe); err != nil {
				t.Fatalf("writeVideo: %v", err)
			}
			dts += frameDur
		}
	}
}

func TestRecMuxer_SegmentsAndPlaylist(t *testing.T) {
	dir := t.TempDir()
	m, err := newRecMuxer(dir, &mp4codecs.H264{SPS: testSPS, PPS: testPPS}, false, 1 /*sec*/)
	if err != nil {
		t.Fatalf("newRecMuxer: %v", err)
	}

	// init.mp4 written eagerly.
	if _, err := os.Stat(filepath.Join(dir, "init.mp4")); err != nil {
		t.Fatalf("init.mp4 not written: %v", err)
	}

	// 4 GOPs, each 30 frames @ 1/30s = 1s. With a 1s target, each keyframe
	// after the first cuts a segment → at least 3 segments before close.
	feedGOPs(t, m, 4, 30, videoTimescale/30)
	if err := m.close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	segs, err := filepath.Glob(filepath.Join(dir, "seg*.m4s"))
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) < 3 {
		t.Fatalf("expected >=3 segments, got %d", len(segs))
	}

	pl, err := os.ReadFile(filepath.Join(dir, "index.m3u8"))
	if err != nil {
		t.Fatalf("read playlist: %v", err)
	}
	s := string(pl)
	for _, want := range []string{"#EXTM3U", "#EXT-X-VERSION:7", `#EXT-X-MAP:URI="init.mp4"`, "#EXTINF:", "#EXT-X-ENDLIST"} {
		if !strings.Contains(s, want) {
			t.Errorf("playlist missing %q:\n%s", want, s)
		}
	}
	if got := strings.Count(s, "#EXTINF:"); got != len(segs) {
		t.Errorf("playlist EXTINF count %d != segment files %d", got, len(segs))
	}
}

func TestRecMuxer_DropsUntilFirstKeyframe(t *testing.T) {
	dir := t.TempDir()
	m, err := newRecMuxer(dir, &mp4codecs.H264{SPS: testSPS, PPS: testPPS}, false, 1)
	if err != nil {
		t.Fatalf("newRecMuxer: %v", err)
	}
	// Non-keyframe before any keyframe must be dropped (no panic, no segment).
	if err := m.writeVideo(0, 0, [][]byte{{0x41, 0x9a, 0x00}}, false); err != nil {
		t.Fatalf("writeVideo: %v", err)
	}
	if m.started {
		t.Fatal("muxer started before first keyframe")
	}
	_ = m.close()
}

func TestFormatVTTTime(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "00:00:00.000"},
		{5.5, "00:00:05.500"},
		{65.25, "00:01:05.250"},
		{3661.001, "01:01:01.001"},
	}
	for _, c := range cases {
		if got := formatVTTTime(c.in); got != c.want {
			t.Errorf("formatVTTTime(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
