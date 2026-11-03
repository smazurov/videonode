package streaming

import (
	"math"
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
	RelayMS *float64               `json:"relay_ms,omitempty" doc:"Smoothed time from a frame's first packet arriving from the encoder to the frame being written to WebRTC viewers; absent while no WebRTC viewer receives this track"`
}

// relaySmoothing weights each frame into the running relay-lag estimate; at
// 30-50 frames/s it settles within about half a second.
const relaySmoothing = 1.0 / 16

// relayTracker is an exponentially smoothed ingest-to-WebRTC delay. The last
// field holds the most recent egress, so the value can be dropped once nobody
// is reading.
type relayTracker struct {
	ms   float64
	set  bool
	last time.Time
}

func (r *relayTracker) observe(d time.Duration, now time.Time) {
	ms := float64(d) / float64(time.Millisecond)
	if !r.set {
		r.ms, r.set = ms, true
	} else {
		r.ms += (ms - r.ms) * relaySmoothing
	}
	r.last = now
}

type trackActivity struct {
	media          *description.Media
	forma          format.Format
	packets        metrics.Activity
	units          metrics.Activity
	unitsSupported bool
	// Arrival of the first packet of the frame currently being received;
	// packets of one frame share an RTP timestamp.
	frameTS    uint32
	frameStart time.Time
	relay      relayTracker
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

func (h *mediaHealth) track(medi *description.Media, forma format.Format) *trackActivity {
	for i := range h.tracks {
		if h.tracks[i].media == medi && h.tracks[i].forma == forma {
			return &h.tracks[i]
		}
	}
	return nil
}

func (h *mediaHealth) record(medi *description.Media, forma format.Format, unit bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.track(medi, forma)
	if t == nil {
		return
	}
	if unit {
		t.units.Record(time.Now())
	} else {
		t.packets.Record(time.Now())
	}
}

// recordPacket notes one RTP arrival and, on a new RTP timestamp, when the
// frame it starts began arriving.
func (h *mediaHealth) recordPacket(medi *description.Media, forma format.Format, now time.Time, rtpTimestamp uint32) {
	h.mu.Lock()
	defer h.mu.Unlock()
	t := h.track(medi, forma)
	if t == nil {
		return
	}
	t.packets.Record(now)
	if t.frameStart.IsZero() || rtpTimestamp != t.frameTS {
		t.frameTS, t.frameStart = rtpTimestamp, now
	}
}

// recordEgress notes that the frame currently being relayed on medi has been
// written to a WebRTC viewer. Delivery runs synchronously on the ingest
// goroutine, so the current frame is the one that was just sent.
func (h *mediaHealth) recordEgress(medi *description.Media, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.tracks {
		t := &h.tracks[i]
		if t.media == medi && !t.frameStart.IsZero() {
			t.relay.observe(now.Sub(t.frameStart), now)
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
		if t.relay.set && now.Sub(t.relay.last) < metrics.HealthTimeout {
			ms := math.Round(t.relay.ms*10) / 10
			out[i].RelayMS = &ms
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
		s.stream.health.recordPacket(medi, forma, time.Now(), pkt.Timestamp)
		cb(pkt)
	})
}
