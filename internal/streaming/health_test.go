package streaming

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/mpeg4audio"
	"github.com/pion/rtp"

	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/metrics"
)

func newHealthTestStream() *Stream {
	medias := []*description.Media{
		{Type: description.MediaTypeVideo, Formats: []format.Format{&format.H265{PayloadTyp: 96}}},
		{Type: description.MediaTypeAudio, Formats: []format.Format{&format.Opus{PayloadTyp: 97, ChannelCount: 2}}},
		{Type: description.MediaTypeAudio, Formats: []format.Format{&format.Opus{PayloadTyp: 98, ChannelCount: 2}}},
	}
	return NewStream("health", &description.Session{Medias: medias}, logging.GetLogger("health-test"))
}

func writeTestUnit(s *Stream, index int, unit [][]byte) {
	medi := s.Description().Medias[index]
	s.WriteUnit(medi, medi.Formats[0], 0, 0, unit)
}

func TestStreamHealth_IndependentTracksAndExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := newHealthTestStream()
		for i := range 3 {
			writeTestUnit(s, i, [][]byte{{0, 0}})
		}
		time.Sleep(metrics.HealthTimeout)
		writeTestUnit(s, 0, [][]byte{{1}, {2}})
		writeTestUnit(s, 2, [][]byte{{0}})
		h := s.Health(time.Now())
		for i, want := range []string{"flowing", "stalled", "flowing"} {
			if h[i].Units.State != want {
				t.Errorf("track %d state = %s, want %s", i, h[i].Units.State, want)
			}
		}
		if h[0].Units.Count != 2 {
			t.Errorf("video units = %d, want 2 (not one per NAL)", h[0].Units.Count)
		}
		time.Sleep(metrics.HealthTimeout)
		for _, track := range s.Health(time.Now()) {
			if track.Units.State != "stalled" {
				t.Errorf("track %d did not expire without further writes", track.Index)
			}
		}
	})
}

func TestStreamHealth_EmptyUnitsAndSilence(t *testing.T) {
	s := newHealthTestStream()
	for _, au := range [][][]byte{nil, {}, {nil}, {{}, nil}} {
		writeTestUnit(s, 1, au)
	}
	if got := s.Health(time.Now())[1].Units; got.State != "never_received" || got.Count != 0 {
		t.Fatalf("empty units counted: %+v", got)
	}
	writeTestUnit(s, 1, [][]byte{{0, 0, 0}})
	if got := s.Health(time.Now())[1].Units; got.State != "flowing" || got.Count != 1 {
		t.Fatalf("silent data not counted: %+v", got)
	}
}

func TestStreamHealth_PacketBeforeDepacketization(t *testing.T) {
	s := newHealthTestStream()
	medi := s.Description().Medias[0]
	sess := &fakeRTPSession{}
	setupFormatHandler(sess, s, nil, medi, medi.Formats[0], logging.GetLogger("health-test"))
	// An unfinished H265 FU contributes transport activity but no complete frame.
	sess.cb(&rtp.Packet{
		Header:  rtp.Header{Version: 2, PayloadType: 96, SequenceNumber: 1, Timestamp: 90, SSRC: 1},
		Payload: []byte{49 << 1, 1, 0x80 | 19, 1},
	})
	h := s.Health(time.Now())[0]
	if h.Packets.Count != 1 || h.Units.Count != 0 {
		t.Fatalf("incomplete RTP health = %+v", h)
	}
}

func TestStreamHealth_GenericFormatIsPacketOnly(t *testing.T) {
	forma := &format.MPEG4Audio{PayloadTyp: 97, Config: &mpeg4audio.AudioSpecificConfig{SampleRate: 48000, ChannelConfig: 2}}
	medi := &description.Media{Type: description.MediaTypeAudio, Formats: []format.Format{forma}}
	s := NewStream("generic", &description.Session{Medias: []*description.Media{medi}}, logging.GetLogger("health-test"))
	sess := &fakeRTPSession{}
	setupFormatHandler(sess, s, nil, medi, forma, logging.GetLogger("health-test"))
	sess.cb(&rtp.Packet{Payload: []byte{1}})
	h := s.Health(time.Now())[0]
	if h.Packets.Count != 1 || h.Units.State != "unsupported" || h.Units.Count != 0 {
		t.Fatalf("packet-only health = %+v", h)
	}
}

func TestStreamHealth_PrecedesFanoutAndResetsWithPublisher(t *testing.T) {
	s := newHealthTestStream()
	r := NewReader(s, "reader")
	defer r.Close()
	r.OnUnit(s.Description().Medias[0], func(_, _ int64, _ [][]byte) error {
		if got := s.Health(time.Now())[0].Units.Count; got != 1 {
			t.Errorf("reader observed unit count %d, want 1", got)
		}
		return nil
	})
	writeTestUnit(s, 0, [][]byte{{1}})
	h := s.Health(time.Now())
	h[0].Units.Count = 999
	if s.Health(time.Now())[0].Units.Count != 1 {
		t.Fatal("snapshot aliases live state")
	}
	replacement := NewStream(s.ID(), s.Description(), s.logger)
	if got := replacement.Health(time.Now())[0].Units.State; got != "never_received" {
		t.Fatalf("replacement inherited old health: %s", got)
	}
}

func TestStreamHealth_ConcurrentSnapshots(t *testing.T) {
	s := newHealthTestStream()
	var wg sync.WaitGroup
	for i := range 3 {
		wg.Go(func() {
			for range 1000 {
				writeTestUnit(s, i, [][]byte{{1}})
			}
		})
	}
	wg.Go(func() {
		for range 1000 {
			_ = s.Health(time.Now())
		}
	})
	wg.Wait()
	for _, h := range s.Health(time.Now()) {
		if h.Units.Count != 1000 {
			t.Errorf("track %d count = %d", h.Index, h.Units.Count)
		}
	}
}

func BenchmarkMediaHealthRecord(b *testing.B) {
	s := newHealthTestStream()
	medi := s.Description().Medias[0]
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		s.health.record(medi, medi.Formats[0], false)
	}
}
