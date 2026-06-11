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

	// onFailure is invoked at most once, off the media path, when a muxer
	// write fails unrecoverably (e.g. disk full). Set by the manager.
	onFailure func(err error)
	failOnce  sync.Once

	mu     sync.Mutex
	closed bool
}

// newRecordingConsumer wires a muxer + reader callbacks for the stream's video
// track. A zero thumbCfg disables the thumbnail track.
//
// Audio is intentionally not recorded yet: the relay's generic handler
// delivers nil access units for Opus (server.go setupGenericHandler), and an
// init.mp4 that declares an audio track which never receives samples stalls
// MSE playback. Record video-only until the relay depacketizes Opus.
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

		videoMedia *description.Media
	)

	for _, medi := range desc.Medias {
		for _, forma := range medi.Formats {
			switch f := forma.(type) {
			case *format.H264:
				if !haveVid {
					vCodec = &mp4codecs.H264{SPS: f.SPS, PPS: f.PPS}
					isH265, haveVid = false, true
					videoMedia = medi
				}
			case *format.H265:
				if !haveVid {
					vCodec = &mp4codecs.H265{VPS: f.VPS, SPS: f.SPS, PPS: f.PPS}
					isH265, haveVid = true, true
					videoMedia = medi
				}
			}
		}
	}

	if !haveVid {
		return nil, ErrNoSupportedCodecs
	}

	mux, err := newRecMuxer(dir, vCodec, isH265, float64(segSec))
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
		// Anchor storyboard offsets to media t=0 (first accepted keyframe),
		// not consumer creation — the encoder can take seconds to spawn.
		thumbCfg.mediaStart = mux.mediaStartTime
		tw, terr := newThumbnailWriter(dir, thumbCfg, logger)
		if terr != nil {
			_ = mux.close()
			return nil, terr
		}
		c.thumbs = tw
		tw.start()
	}

	c.reader = NewReader(stream, recordingID)
	c.setupCallbacks(videoMedia, isH265)

	return c, nil
}

func (c *RecordingConsumer) setupCallbacks(videoMedia *description.Media, isH265 bool) {
	c.reader.OnUnit(videoMedia, func(pts, dts int64, au [][]byte) error {
		var keyframe bool
		if isH265 {
			keyframe = h265.IsRandomAccess(au)
		} else {
			keyframe = h264.IsRandomAccess(au)
		}
		off := max(pts-dts, 0)
		err := c.mux.writeVideo(dts, int32(off), au, keyframe)
		if err != nil {
			// Reader.writeUnit discards callback errors, so surface the
			// failure here exactly once and let the manager finalize.
			c.failOnce.Do(func() {
				c.logger.Error("recording write failed",
					logging.KeyStreamID, c.streamID,
					logging.KeySession, c.recordingID,
					logging.KeyError, err)
				if c.onFailure != nil {
					go c.onFailure(err)
				}
			})
		}
		return err
	})
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

// info returns a point-in-time status snapshot. Size and duration come from
// the muxer's own counters, so status polling never touches the disk.
func (c *RecordingConsumer) info() RecordingInfo {
	c.mu.Lock()
	active := !c.closed
	c.mu.Unlock()
	size := c.mux.bytesWritten()
	if c.thumbs != nil {
		size += c.thumbs.bytesWritten()
	}
	return RecordingInfo{
		RecordingID:     c.recordingID,
		StreamID:        c.streamID,
		Active:          active,
		StartedAt:       c.startedAt.UTC(),
		Segments:        c.mux.segmentCount(),
		SizeBytes:       size,
		DurationSeconds: c.mux.durationSeconds(),
	}
}
