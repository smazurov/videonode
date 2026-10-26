package streaming

import (
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"

	"github.com/smazurov/videonode/internal/metrics"
)

// TrackHealth describes one advertised media track for this publisher.
type TrackHealth struct {
	Index   int                    `json:"index"`
	Type    string                 `json:"type"`
	Codec   string                 `json:"codec"`
	Packets metrics.ActivityHealth `json:"packets"`
	Units   metrics.ActivityHealth `json:"units" doc:"Nonempty access units; one video frame can contain several NAL units"`
}

type trackActivity struct {
	media          *description.Media
	forma          format.Format
	packets        metrics.Activity
	units          metrics.Activity
	unitsSupported bool
}

type mediaHealth struct {
	mu        sync.RWMutex
	startedAt time.Time
	tracks    []trackActivity
}

func newMediaHealth(desc *description.Session) *mediaHealth {
	h := &mediaHealth{startedAt: time.Now()}
	if desc != nil {
		for _, medi := range desc.Medias {
			for _, forma := range medi.Formats {
				_, h264 := forma.(*format.H264)
				_, h265 := forma.(*format.H265)
				_, opus := forma.(*format.Opus)
				h.tracks = append(h.tracks, trackActivity{
					media: medi, forma: forma, unitsSupported: h264 || h265 || opus,
				})
			}
		}
	}
	return h
}

func (h *mediaHealth) record(medi *description.Media, forma format.Format, unit bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.tracks {
		t := &h.tracks[i]
		if t.media == medi && t.forma == forma {
			if unit {
				t.units.Record(time.Now())
			} else {
				t.packets.Record(time.Now())
			}
			return
		}
	}
}

// Health returns detached per-track snapshots, including tracks that have never delivered.
func (s *Stream) Health(now time.Time) []TrackHealth {
	s.health.mu.RLock()
	defer s.health.mu.RUnlock()
	out := make([]TrackHealth, len(s.health.tracks))
	for i, t := range s.health.tracks {
		units := t.units.Snapshot(now)
		if !t.unitsSupported {
			units.State = "unsupported"
		}
		codec, _, _ := strings.Cut(t.forma.RTPMap(), "/")
		out[i] = TrackHealth{
			Index: i, Type: string(t.media.Type), Codec: codec,
			Packets: t.packets.Snapshot(now), Units: units,
		}
	}
	return out
}

// observedRTPSession records ingress even when depacketization or PTS extraction fails.
type observedRTPSession struct {
	rtpSession
	stream *Stream
}

// OnPacketRTP wraps the handler without changing its forwarding or decoding behavior.
func (s observedRTPSession) OnPacketRTP(medi *description.Media, forma format.Format, cb gortsplib.OnPacketRTPFunc) {
	s.rtpSession.OnPacketRTP(medi, forma, func(pkt *rtp.Packet) {
		s.stream.health.record(medi, forma, false)
		cb(pkt)
	})
}
