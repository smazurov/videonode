package recording

import (
	"errors"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h264"
	"github.com/bluenviron/mediacommon/v2/pkg/codecs/h265"
	"github.com/google/uuid"
	"github.com/smazurov/videonode/internal/logging"
	"github.com/smazurov/videonode/internal/streaming"
)

// Errors returned by CaptureKeyframe.
var (
	ErrNoVideoTrack = errors.New("stream has no supported video track")
	ErrTimeout      = errors.New("timeout waiting for keyframe")
)

// CodecType identifies the video codec for FFmpeg's raw demuxer (-f flag).
type CodecType string

// Supported codec types.
const (
	CodecH264 CodecType = "h264"
	CodecH265 CodecType = "hevc"
)

// KeyframeResult holds one captured keyframe in Annex B format.
type KeyframeResult struct {
	Data  []byte
	Codec CodecType
}

// CaptureKeyframe subscribes to a stream, waits for the first IDR keyframe,
// and returns it as Annex B bytes with SPS/PPS/VPS prepended.
func CaptureKeyframe(stream *streaming.Stream, timeout time.Duration) (*KeyframeResult, error) {
	logger := logging.GetLogger("recording")

	// Find the first supported video format in the stream description
	var (
		videoMedia *description.Media
		h264Format *format.H264
		h265Format *format.H265
		codecType  CodecType
	)

	for _, medi := range stream.Description().Medias {
		for _, forma := range medi.Formats {
			switch f := forma.(type) {
			case *format.H264:
				videoMedia = medi
				h264Format = f
				codecType = CodecH264
			case *format.H265:
				videoMedia = medi
				h265Format = f
				codecType = CodecH265
			}
			if videoMedia != nil {
				break
			}
		}
		if videoMedia != nil {
			break
		}
	}

	if videoMedia == nil {
		return nil, ErrNoVideoTrack
	}

	readerID := "snapshot-" + uuid.NewString()[:8]
	reader := streaming.NewReader(stream, readerID)
	defer reader.Close()

	logger.Debug("Snapshot reader attached", "stream_id", stream.ID(), "reader_id", readerID, "codec", string(codecType))

	frameCh := make(chan *KeyframeResult, 1)

	switch codecType {
	case CodecH264:
		reader.OnUnit(videoMedia, func(_, _ int64, au [][]byte) error {
			if !h264.IsRandomAccess(au) {
				return nil
			}

			au = prependH264Params(au, h264Format.SPS, h264Format.PPS)
			data := annexBMarshal(au)

			select {
			case frameCh <- &KeyframeResult{Data: data, Codec: CodecH264}:
			default:
			}
			return nil
		})

	case CodecH265:
		reader.OnUnit(videoMedia, func(_, _ int64, au [][]byte) error {
			if !h265.IsRandomAccess(au) {
				return nil
			}

			au = prependH265Params(au, h265Format.VPS, h265Format.SPS, h265Format.PPS)
			data := annexBMarshal(au)

			select {
			case frameCh <- &KeyframeResult{Data: data, Codec: CodecH265}:
			default:
			}
			return nil
		})
	}

	select {
	case result := <-frameCh:
		logger.Debug("Snapshot keyframe captured", "stream_id", stream.ID(), "codec", string(codecType), "bytes", len(result.Data))
		return result, nil
	case <-time.After(timeout):
		return nil, ErrTimeout
	}
}

// prependH264Params prepends SPS/PPS before the access unit if not already present.
func prependH264Params(au [][]byte, sps, pps []byte) [][]byte {
	if len(sps) == 0 || len(pps) == 0 {
		return au
	}

	hasSPS, hasPPS := false, false
	for _, nalu := range au {
		if len(nalu) > 0 {
			switch h264.NALUType(nalu[0] & 0x1F) {
			case h264.NALUTypeSPS:
				hasSPS = true
			case h264.NALUTypePPS:
				hasPPS = true
			}
		}
	}

	if hasSPS && hasPPS {
		return au
	}

	newAU := make([][]byte, 0, len(au)+2)
	if !hasSPS {
		newAU = append(newAU, sps)
	}
	if !hasPPS {
		newAU = append(newAU, pps)
	}
	return append(newAU, au...)
}

// prependH265Params prepends VPS/SPS/PPS before the access unit if not already present.
func prependH265Params(au [][]byte, vps, sps, pps []byte) [][]byte {
	if len(vps) == 0 || len(sps) == 0 || len(pps) == 0 {
		return au
	}

	hasVPS, hasSPS, hasPPS := false, false, false
	for _, nalu := range au {
		if len(nalu) > 0 {
			naluType := h265.NALUType((nalu[0] >> 1) & 0x3F)
			switch naluType {
			case h265.NALUType_VPS_NUT:
				hasVPS = true
			case h265.NALUType_SPS_NUT:
				hasSPS = true
			case h265.NALUType_PPS_NUT:
				hasPPS = true
			}
		}
	}

	if hasVPS && hasSPS && hasPPS {
		return au
	}

	newAU := make([][]byte, 0, len(au)+3)
	if !hasVPS {
		newAU = append(newAU, vps)
	}
	if !hasSPS {
		newAU = append(newAU, sps)
	}
	if !hasPPS {
		newAU = append(newAU, pps)
	}
	return append(newAU, au...)
}

// annexBMarshal converts an access unit (slice of NAL units) to Annex B byte stream
// by prepending the 4-byte start code (0x00000001) before each NAL unit.
func annexBMarshal(au [][]byte) []byte {
	size := 0
	for _, nalu := range au {
		size += 4 + len(nalu)
	}

	buf := make([]byte, 0, size)
	for _, nalu := range au {
		buf = append(buf, 0x00, 0x00, 0x00, 0x01)
		buf = append(buf, nalu...)
	}
	return buf
}
