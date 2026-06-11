package streaming

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

const (
	videoTrackID   = 1
	audioTrackID   = 2
	videoTimescale = 90000 // RTSP H264/H265 RTP clock; relay PTS/DTS arrive in these units.
)

// segInfo is one closed HLS segment for the playlist.
type segInfo struct {
	name string
	dur  float64 // seconds
}

// recMuxer writes fragmented-MP4 HLS — init.mp4 + segNNNNN.m4s + index.m3u8 — to
// a session directory. Segments are cut on video keyframes once the target
// duration elapses, so every segment starts with a sync sample and the playlist
// is playable while still being written (no #EXT-X-ENDLIST until close).
//
// It is fed decoded access units from a streaming.Reader (see RecordingConsumer)
// and stream-copies them: no re-encode. Video is required; Opus audio optional.
type recMuxer struct {
	dir          string
	targetSegSec float64

	vIsH265        bool
	haveAudio      bool
	audioTimescale uint32

	mu sync.Mutex

	started bool // first video keyframe seen
	seq     uint32
	segs    []segInfo
	closed  bool

	// video segment buffer
	vFirstDTS   int64
	vHaveFirst  bool
	vPending    []*fmp4.Sample
	vPendingDTS []int64
	vSegBaseDTS int64
	vSegHasData bool

	// audio segment buffer (accumulated since last cut)
	aFirstDTS   int64
	aHaveFirst  bool
	aPending    []*fmp4.Sample
	aPendingDTS []int64
	aSegBaseDTS int64
	aSegHasData bool
}

// newRecMuxer creates the session dir, writes init.mp4, and returns a muxer
// ready to accept samples. The vCodec is &mp4codecs.H264{SPS,PPS} or
// &mp4codecs.H265{VPS,SPS,PPS}; aCodec is &mp4codecs.Opus{...} or nil.
func newRecMuxer(dir string, vCodec mp4codecs.Codec, isH265 bool, aCodec mp4codecs.Codec, audioTimescale uint32, targetSegSec float64) (*recMuxer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create recording dir: %w", err)
	}
	if targetSegSec <= 0 {
		targetSegSec = 6
	}

	tracks := []*fmp4.InitTrack{{ID: videoTrackID, TimeScale: videoTimescale, Codec: vCodec}}
	if aCodec != nil {
		if audioTimescale == 0 {
			audioTimescale = 48000
		}
		tracks = append(tracks, &fmp4.InitTrack{ID: audioTrackID, TimeScale: audioTimescale, Codec: aCodec})
	}
	init := fmp4.Init{Tracks: tracks}
	if err := marshalToFile(filepath.Join(dir, "init.mp4"), init.Marshal); err != nil {
		return nil, fmt.Errorf("write init.mp4: %w", err)
	}

	return &recMuxer{
		dir:            dir,
		targetSegSec:   targetSegSec,
		vIsH265:        isH265,
		haveAudio:      aCodec != nil,
		audioTimescale: audioTimescale,
	}, nil
}

// marshalToFile creates path and runs the fmp4 marshaler (fmp4.Init.Marshal /
// fmp4.Part.Marshal) against it. *os.File is the io.WriteSeeker the marshaler
// needs for box back-patching.
func marshalToFile(path string, marshal func(io.WriteSeeker) error) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	return marshal(f)
}

// writeVideo appends one video access unit. The dts is in 90kHz units; keyframe
// must be true for random-access (IDR/IRAP) AUs — it gates the start and cuts.
func (m *recMuxer) writeVideo(dts int64, ptsOffset int32, au [][]byte, keyframe bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	if !m.started {
		if !keyframe {
			return nil // drop until the first keyframe so segment 1 is seekable
		}
		m.started = true
	}
	if !m.vHaveFirst {
		m.vFirstDTS = dts
		m.vHaveFirst = true
	}
	rel := dts - m.vFirstDTS

	// Close out the previous sample's duration now that we know the next DTS.
	if n := len(m.vPending); n > 0 {
		m.vPending[n-1].Duration = uint32(rel - m.vPendingDTS[n-1])
	}

	// Cut on a keyframe once the open segment is long enough.
	if keyframe && m.vSegHasData {
		if float64(rel-m.vSegBaseDTS)/videoTimescale >= m.targetSegSec {
			if err := m.flushSegmentLocked(rel); err != nil {
				return err
			}
		}
	}

	var s fmp4.Sample
	var err error
	if m.vIsH265 {
		err = s.FillH265(ptsOffset, au)
	} else {
		err = s.FillH264(ptsOffset, au)
	}
	if err != nil {
		return err
	}
	if !m.vSegHasData {
		m.vSegBaseDTS = rel
		m.vSegHasData = true
	}
	m.vPending = append(m.vPending, &s)
	m.vPendingDTS = append(m.vPendingDTS, rel)
	return nil
}

// writeAudio appends Opus packets (one fMP4 sample each). The pts is in the
// audio timescale; packets in a call are spaced by a default 20ms Opus frame.
func (m *recMuxer) writeAudio(pts int64, packets [][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || !m.haveAudio || !m.started {
		return nil
	}
	frameDur := int64(m.audioTimescale / 50) // 20ms
	for i, pkt := range packets {
		dts := pts + int64(i)*frameDur
		if !m.aHaveFirst {
			m.aFirstDTS = dts
			m.aHaveFirst = true
		}
		rel := dts - m.aFirstDTS
		if n := len(m.aPending); n > 0 {
			m.aPending[n-1].Duration = uint32(rel - m.aPendingDTS[n-1])
		}
		payload := make([]byte, len(pkt))
		copy(payload, pkt)
		if !m.aSegHasData {
			m.aSegBaseDTS = rel
			m.aSegHasData = true
		}
		m.aPending = append(m.aPending, &fmp4.Sample{Payload: payload})
		m.aPendingDTS = append(m.aPendingDTS, rel)
	}
	return nil
}

// flushSegmentLocked writes the buffered samples as one .m4s part and rewrites
// the playlist. Caller holds m.mu.
func (m *recMuxer) flushSegmentLocked(_ int64) error {
	if len(m.vPending) == 0 {
		return nil
	}
	// Trailing video sample: no successor yet, fall back to the prior delta.
	fixTrailingDuration(m.vPending, m.vPendingDTS, videoTimescale/30)

	m.seq++
	name := fmt.Sprintf("seg%05d.m4s", m.seq)

	tracks := []*fmp4.PartTrack{{
		ID:       videoTrackID,
		BaseTime: uint64(m.vSegBaseDTS),
		Samples:  m.vPending,
	}}
	if m.haveAudio && len(m.aPending) > 0 {
		fixTrailingDuration(m.aPending, m.aPendingDTS, int64(m.audioTimescale/50))
		tracks = append(tracks, &fmp4.PartTrack{
			ID:       audioTrackID,
			BaseTime: uint64(m.aSegBaseDTS),
			Samples:  m.aPending,
		})
	}

	part := fmp4.Part{SequenceNumber: m.seq, Tracks: tracks}
	if err := marshalToFile(filepath.Join(m.dir, name), part.Marshal); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	var dur float64
	for _, s := range m.vPending {
		dur += float64(s.Duration) / videoTimescale
	}
	m.segs = append(m.segs, segInfo{name: name, dur: dur})

	m.vPending, m.vPendingDTS, m.vSegHasData = nil, nil, false
	m.aPending, m.aPendingDTS, m.aSegHasData = nil, nil, false

	return m.writePlaylistLocked(false)
}

// fixTrailingDuration sets the last sample's Duration when it has no successor.
func fixTrailingDuration(samples []*fmp4.Sample, _ []int64, fallback int64) {
	n := len(samples)
	if n == 0 {
		return
	}
	if samples[n-1].Duration != 0 {
		return
	}
	if n >= 2 && samples[n-2].Duration != 0 {
		samples[n-1].Duration = samples[n-2].Duration
	} else {
		samples[n-1].Duration = uint32(fallback)
	}
}

func (m *recMuxer) writePlaylistLocked(end bool) error {
	maxDur := m.targetSegSec
	for _, s := range m.segs {
		if s.dur > maxDur {
			maxDur = s.dur
		}
	}

	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:7\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(maxDur)))
	b.WriteString("#EXT-X-MEDIA-SEQUENCE:0\n")
	b.WriteString("#EXT-X-MAP:URI=\"init.mp4\"\n")
	for _, s := range m.segs {
		fmt.Fprintf(&b, "#EXTINF:%.6f,\n%s\n", s.dur, s.name)
	}
	if end {
		b.WriteString("#EXT-X-ENDLIST\n")
	}

	// Write-temp-rename so a concurrent player never reads a half-written playlist.
	tmp := filepath.Join(m.dir, ".index.m3u8.tmp")
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(m.dir, "index.m3u8"))
}

// close flushes the open segment and finalizes the playlist with #EXT-X-ENDLIST.
func (m *recMuxer) close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if len(m.vPending) > 0 {
		if err := m.flushSegmentLocked(0); err != nil {
			return err
		}
	}
	return m.writePlaylistLocked(true)
}

// segmentCount reports how many segments have been written (for status).
func (m *recMuxer) segmentCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.segs)
}
