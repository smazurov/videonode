package streaming

import (
	"bytes"
	"testing"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/smazurov/videonode/internal/logging"
)

// fakeRTPSession captures the callback registered by a format handler and
// answers PacketPTS with a fixed value so the handler's decode path runs.
type fakeRTPSession struct {
	cb  gortsplib.OnPacketRTPFunc
	pts int64
}

func (f *fakeRTPSession) OnPacketRTP(_ *description.Media, _ format.Format, cb gortsplib.OnPacketRTPFunc) {
	f.cb = cb
}

func (f *fakeRTPSession) PacketPTS(_ *description.Media, _ *rtp.Packet) (int64, bool) {
	return f.pts, true
}

func TestSetupFormatHandler_OpusDeliversAccessUnits(t *testing.T) {
	forma := &format.Opus{PayloadTyp: 96, ChannelCount: 2}
	medi := &description.Media{Type: description.MediaTypeAudio, Formats: []format.Format{forma}}
	stream := NewStream("opus", &description.Session{Medias: []*description.Media{medi}}, logging.GetLogger("streaming-test"))

	var gotPTS int64
	var gotAU [][]byte
	calls := 0
	r := NewReader(stream, "unit-test")
	defer r.Close()
	r.OnUnit(medi, func(pts int64, _ int64, au [][]byte) error {
		calls++
		gotPTS, gotAU = pts, au
		return nil
	})

	sess := &fakeRTPSession{pts: 4321}
	setupFormatHandler(sess, stream, nil, medi, forma, logging.GetLogger("streaming-test"))
	if sess.cb == nil {
		t.Fatal("handler did not register an RTP callback")
	}

	frame := []byte{0xfc, 0x01, 0x02, 0x03}
	sess.cb(&rtp.Packet{
		Header:  rtp.Header{Version: 2, PayloadType: 96, SequenceNumber: 1, Timestamp: 960, SSRC: 1},
		Payload: frame,
	})

	if calls != 1 {
		t.Fatalf("unit callback calls = %d, want 1", calls)
	}
	if gotPTS != 4321 {
		t.Errorf("pts = %d, want 4321", gotPTS)
	}
	if len(gotAU) != 1 {
		t.Fatalf("access units = %v, want exactly one Opus frame", gotAU)
	}
	if !bytes.Equal(gotAU[0], frame) {
		t.Errorf("access unit = %x, want %x", gotAU[0], frame)
	}
}
