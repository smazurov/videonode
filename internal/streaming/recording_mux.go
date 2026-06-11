package streaming

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediacommon/v2/pkg/formats/fmp4"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"
)

const (
	videoTrackID   = 1
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
// and stream-copies them: no re-encode. Video only — the relay does not
// depacketize Opus yet (see newRecordingConsumer).
type recMuxer struct {
	dir          string
	targetSegSec float64

	vIsH265 bool

	mu sync.Mutex

	started   bool      // first video keyframe seen
	startWall time.Time // wall clock at the first accepted keyframe (media t=0)
	seq       uint32
	segs      []segInfo
	closed    bool
	failed    error // first unrecoverable write error; muxer stops accepting
	bytesOut  int64 // bytes written so far (init + segments)

	// video segment buffer
	vFirstDTS   int64
	vHaveFirst  bool
	vPending    []*fmp4.Sample
	vLastRel    int64 // rebased DTS of the last buffered sample
	vSegBaseDTS int64
	vSegHasData bool
}

// newRecMuxer creates the session dir, writes init.mp4, and returns a muxer
// ready to accept samples. The vCodec is &mp4codecs.H264{SPS,PPS} or
// &mp4codecs.H265{VPS,SPS,PPS}.
func newRecMuxer(dir string, vCodec mp4codecs.Codec, isH265 bool, targetSegSec float64) (*recMuxer, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create recording dir: %w", err)
	}
	if targetSegSec <= 0 {
		targetSegSec = 6
	}

	tracks := []*fmp4.InitTrack{{ID: videoTrackID, TimeScale: videoTimescale, Codec: vCodec}}
	init := fmp4.Init{Tracks: tracks}
	initBytes, err := marshalToFile(filepath.Join(dir, "init.mp4"), init.Marshal)
	if err != nil {
		return nil, fmt.Errorf("write init.mp4: %w", err)
	}

	return &recMuxer{
		dir:          dir,
		targetSegSec: targetSegSec,
		vIsH265:      isH265,
		bytesOut:     initBytes,
	}, nil
}

// marshalToFile creates path, runs the fmp4 marshaler (fmp4.Init.Marshal /
// fmp4.Part.Marshal) against it, and reports the bytes written. *os.File is
// the io.WriteSeeker the marshaler needs for box back-patching.
func marshalToFile(path string, marshal func(io.WriteSeeker) error) (size int64, err error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	if err := marshal(f); err != nil {
		return 0, err
	}
	return f.Seek(0, io.SeekEnd)
}

// sampleDur clamps a rebased-DTS delta into a sane uint32 sample duration.
// H265 arrives with DTS=PTS (no extractor), so B-frame reordering can make
// deltas negative; an unclamped cast would wrap to ~13h durations.
func sampleDur(delta int64) uint32 {
	if delta < 0 {
		return 0
	}
	return uint32(min(delta, int64(^uint32(0))))
}

// writeVideo appends one video access unit. The dts is in 90kHz units; keyframe
// must be true for random-access (IDR/IRAP) AUs — it gates the start and cuts.
func (m *recMuxer) writeVideo(dts int64, ptsOffset int32, au [][]byte, keyframe bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	if m.failed != nil {
		return m.failed
	}
	if !m.started {
		if !keyframe {
			return nil // drop until the first keyframe so segment 1 is seekable
		}
		m.started = true
		m.startWall = time.Now()
	}
	if !m.vHaveFirst {
		m.vFirstDTS = dts
		m.vHaveFirst = true
	}
	rel := dts - m.vFirstDTS

	// Close out the previous sample's duration now that we know the next DTS.
	if n := len(m.vPending); n > 0 {
		m.vPending[n-1].Duration = sampleDur(rel - m.vLastRel)
	}

	// Cut on a keyframe once the open segment is long enough.
	if keyframe && m.vSegHasData {
		if float64(rel-m.vSegBaseDTS)/videoTimescale >= m.targetSegSec {
			if err := m.flushSegmentLocked(); err != nil {
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
	m.vLastRel = rel
	return nil
}

// flushSegmentLocked writes the buffered samples as one .m4s part and rewrites
// the playlist. On write failure the buffers are dropped and the muxer latches
// failed — better to lose the open segment than to buffer the bitstream in RAM
// forever on a full disk. Caller holds m.mu.
func (m *recMuxer) flushSegmentLocked() error {
	if len(m.vPending) == 0 {
		return nil
	}
	// Trailing video sample: no successor yet, fall back to the prior delta.
	fixTrailingDuration(m.vPending, videoTimescale/30)

	m.seq++
	name := fmt.Sprintf("seg%05d.m4s", m.seq)

	tracks := []*fmp4.PartTrack{{
		ID:       videoTrackID,
		BaseTime: uint64(m.vSegBaseDTS),
		Samples:  m.vPending,
	}}
	var dur float64
	for _, s := range m.vPending {
		dur += float64(s.Duration) / videoTimescale
	}

	m.vPending, m.vSegHasData = nil, false

	part := fmp4.Part{SequenceNumber: m.seq, Tracks: tracks}
	size, err := marshalToFile(filepath.Join(m.dir, name), part.Marshal)
	if err != nil {
		m.failed = fmt.Errorf("write %s: %w", name, err)
		return m.failed
	}
	m.bytesOut += size
	m.segs = append(m.segs, segInfo{name: name, dur: dur})

	return m.writePlaylistLocked(false)
}

// fixTrailingDuration sets the last sample's Duration when it has no successor.
func fixTrailingDuration(samples []*fmp4.Sample, fallback int64) {
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
		samples[n-1].Duration = sampleDur(fallback)
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

	return writeFileAtomic(m.dir, "index.m3u8", []byte(b.String()))
}

// close flushes the open segment and finalizes the playlist with #EXT-X-ENDLIST.
func (m *recMuxer) close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil
	}
	m.closed = true
	if m.failed != nil {
		// Finalize what made it to disk so the partial session stays playable.
		return m.writePlaylistLocked(true)
	}
	if len(m.vPending) > 0 {
		if err := m.flushSegmentLocked(); err != nil {
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

// bytesWritten reports the bytes persisted so far (init + closed segments).
func (m *recMuxer) bytesWritten() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.bytesOut
}

// durationSeconds reports the media duration: closed segments plus the span of
// the open one.
func (m *recMuxer) durationSeconds() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total float64
	for _, s := range m.segs {
		total += s.dur
	}
	if m.vSegHasData {
		total += float64(m.vLastRel-m.vSegBaseDTS) / videoTimescale
	}
	return total
}

// mediaStartTime reports the wall-clock instant of media t=0 (the first
// accepted keyframe); ok is false until recording actually started.
func (m *recMuxer) mediaStartTime() (time.Time, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startWall, m.started
}

// writeFileAtomic writes dir/name via a temp file + rename so concurrent
// readers never observe a partial file.
func writeFileAtomic(dir, name string, data []byte) error {
	tmp := filepath.Join(dir, filepath.Dir(name), "."+filepath.Base(name)+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, filepath.Dir(name), filepath.Base(name)))
}
