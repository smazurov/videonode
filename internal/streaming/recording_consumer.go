package streaming

import (
	"log/slog"
	"sync"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	mp4codecs "github.com/bluenviron/mediacommon/v2/pkg/formats/mp4/codecs"

	"github.com/smazurov/videonode/internal/logging"
)

// RecordingConsumer attaches to a stream as a Reader and stream-copies its
// already-encoded access units into fragmented-MP4 HLS on disk (no re-encode).
// It mirrors SRTConsumer but writes to a recMuxer + optional thumbnail track
// instead of an SRT connection. Because it is a live Reader, it pins the
// encoder up for the duration of the recording (defeats the last-reader
// debounce); Stop removes the reader so the normal idle path can run.
type RecordingConsumer struct {
	reader      *Reader
	mux         *recMuxer
	thumbs      *thumbnailWriter
	streamID    string
	recordingID string
	dir         string
	logger      *slog.Logger

	startedAt time.Time

	mu     sync.Mutex
	closed bool
}

// newRecordingConsumer wires a muxer + reader callbacks for the stream's video
// (required) and Opus audio (optional) tracks. A zero thumbCfg disables the
// thumbnail track.
func newRecordingConsumer(
	stream *Stream,
	recordingID, dir string,
	segSec int,
	thumbCfg thumbnailConfig,
	logger *slog.Logger,
) (*RecordingConsumer, error) {
	desc := stream.Description()

	var (
		vCodec  mp4codecs.Codec
		isH265  bool
		haveVid bool

		aCodec     mp4codecs.Codec
		aTimescale uint32

		videoMedia *description.Media
		videoForma format.Format
		audioMedia *description.Media
		audioForma format.Format
	)

	for _, medi := range desc.Medias {
		for _, forma := range medi.Formats {
			switch f := forma.(type) {
			case *format.H264:
				if !haveVid {
					vCodec = &mp4codecs.H264{SPS: f.SPS, PPS: f.PPS}
					isH265, haveVid = false, true
					videoMedia, videoForma = medi, forma
				}
			case *format.H265:
				if !haveVid {
					vCodec = &mp4codecs.H265{VPS: f.VPS, SPS: f.SPS, PPS: f.PPS}
					isH265, haveVid = true, true
					videoMedia, videoForma = medi, forma
				}
			case *format.Opus:
				if aCodec == nil {
					ch := f.ChannelCount
					if ch <= 0 {
						ch = 2
					}
					aCodec = &mp4codecs.Opus{ChannelCount: ch}
					aTimescale = uint32(f.ClockRate())
					audioMedia, audioForma = medi, forma
				}
			}
		}
	}

	if !haveVid {
		return nil, ErrNoSupportedCodecs
	}

	mux, err := newRecMuxer(dir, vCodec, isH265, aCodec, aTimescale, float64(segSec))
	if err != nil {
		return nil, err
	}

	c := &RecordingConsumer{
		mux:         mux,
		streamID:    stream.ID(),
		recordingID: recordingID,
		dir:         dir,
		logger:      logger,
		startedAt:   time.Now(),
	}

	if thumbCfg.fetch != nil {
		tw, terr := newThumbnailWriter(dir, thumbCfg, c.startedAt, logger)
		if terr != nil {
			_ = mux.close()
			return nil, terr
		}
		c.thumbs = tw
		tw.start()
	}

	c.reader = NewReader(stream, recordingID)
	c.setupCallbacks(videoMedia, videoForma, isH265, audioMedia, audioForma)

	return c, nil
}

func (c *RecordingConsumer) setupCallbacks(
	videoMedia *description.Media, _ format.Format, isH265 bool,
	audioMedia *description.Media, _ format.Format,
) {
	c.reader.OnUnit(videoMedia, func(pts, dts int64, au [][]byte) error {
		var keyframe bool
		if isH265 {
			keyframe = h265.IsRandomAccess(au)
		} else {
			keyframe = h264.IsRandomAccess(au)
		}
		off := max(pts-dts, 0)
		return c.mux.writeVideo(dts, int32(off), au, keyframe)
	})

	if audioMedia != nil {
		c.reader.OnUnit(audioMedia, func(pts, _ int64, packets [][]byte) error {
			return c.mux.writeAudio(pts, packets)
		})
	}
}

// Stop detaches the reader, finalizes the playlist + thumbnail track, and is
// idempotent.
func (c *RecordingConsumer) Stop() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	c.reader.Close()
	if c.thumbs != nil {
		c.thumbs.stop()
	}
	if err := c.mux.close(); err != nil {
		c.logger.Warn("recording finalize error",
			logging.KeyStreamID, c.streamID, logging.KeyError, err)
		return err
	}
	return nil
}

// info returns a point-in-time status snapshot.
func (c *RecordingConsumer) info() RecordingInfo {
	c.mu.Lock()
	active := !c.closed
	c.mu.Unlock()
	return RecordingInfo{
		RecordingID: c.recordingID,
		StreamID:    c.streamID,
		Session:     c.recordingID, // recordingID is the session dir name
		Active:      active,
		StartedAt:   c.startedAt.UTC(),
		Segments:    c.mux.segmentCount(),
	}
}
