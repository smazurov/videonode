package store

import (
	"github.com/smazurov/videonode/internal/streams"
	"github.com/smazurov/videonode/internal/streams/pipeline"
)

// sourceFromV2 converts a persisted V2Source to its canonical shape.
func sourceFromV2(v V2Source) streams.Source {
	out := streams.Source{
		ID:        v.ID,
		Device:    v.Device,
		TestMode:  v.TestMode,
		Pipe:      v.Pipe,
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
	if v.Format != nil {
		out.Format = &pipeline.SourceFormat{
			FourCC: v.Format.FourCC,
			Width:  v.Format.Width,
			Height: v.Format.Height,
			FPS:    v.Format.FPS,
		}
	}
	return out
}

// sourceToV2 converts a canonical pipeline.Source to its persisted form.
func sourceToV2(s streams.Source) V2Source {
	out := V2Source{
		ID:        s.ID,
		Device:    s.Device,
		TestMode:  s.TestMode,
		Pipe:      s.Pipe,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
	if s.Format != nil {
		out.Format = &V2SourceFormat{
			FourCC: s.Format.FourCC,
			Width:  s.Format.Width,
			Height: s.Format.Height,
			FPS:    s.Format.FPS,
		}
	}
	return out
}

func composerFromV2(v V2Composer) streams.Composer {
	c := streams.Composer{
		ID:        v.ID,
		Canvas:    streams.ComposerCanvasDims{W: v.Canvas.W, H: v.Canvas.H, FPS: v.Canvas.FPS, Background: v.Canvas.Background},
		CreatedAt: v.CreatedAt,
		UpdatedAt: v.UpdatedAt,
	}
	if len(v.Inputs) > 0 {
		c.Inputs = make([]streams.ComposerInput, len(v.Inputs))
		for i, in := range v.Inputs {
			c.Inputs[i] = streams.ComposerInput{Ref: in.Ref}
			if in.Effect != nil {
				e := streams.ComposerEffect{
					Type:      in.Effect.Type,
					Corners:   in.Effect.Corners,
					SnapshotW: in.Effect.SnapshotW,
					SnapshotH: in.Effect.SnapshotH,
				}
				c.Inputs[i].Effect = &e
			}
		}
	}
	if len(v.Layout) > 0 {
		c.Layout = make([]streams.ComposerLayoutSlot, len(v.Layout))
		for i, l := range v.Layout {
			slot := streams.ComposerLayoutSlot{Input: l.Input, X: l.X, Y: l.Y, W: l.W, H: l.H, Rotation: l.Rotation, AspectRatioMode: l.AspectRatioMode}
			if l.Crop != nil {
				slot.Crop = &pipeline.CropConfig{X: l.Crop.X, Y: l.Crop.Y, Scale: l.Crop.Scale}
			}
			c.Layout[i] = slot
		}
	}
	return c
}

func composerToV2(c streams.Composer) V2Composer {
	v := V2Composer{
		ID:        c.ID,
		Canvas:    V2CanvasDims{W: c.Canvas.W, H: c.Canvas.H, FPS: c.Canvas.FPS, Background: c.Canvas.Background},
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
	if len(c.Inputs) > 0 {
		v.Inputs = make([]V2ComposerInput, len(c.Inputs))
		for i, in := range c.Inputs {
			v.Inputs[i] = V2ComposerInput{Ref: in.Ref}
			if in.Effect != nil {
				e := V2Effect{
					Type:      in.Effect.Type,
					Corners:   in.Effect.Corners,
					SnapshotW: in.Effect.SnapshotW,
					SnapshotH: in.Effect.SnapshotH,
				}
				v.Inputs[i].Effect = &e
			}
		}
	}
	if len(c.Layout) > 0 {
		v.Layout = make([]V2LayoutSlot, len(c.Layout))
		for i, l := range c.Layout {
			slot := V2LayoutSlot{Input: l.Input, X: l.X, Y: l.Y, W: l.W, H: l.H, Rotation: l.Rotation, AspectRatioMode: l.AspectRatioMode}
			if l.Crop != nil {
				slot.Crop = &V2CropConfig{X: l.Crop.X, Y: l.Crop.Y, Scale: l.Crop.Scale}
			}
			v.Layout[i] = slot
		}
	}
	return v
}

func pipelineStreamFromV2(v V2Stream) streams.PipelineStream {
	s := streams.PipelineStream{
		ID:                v.ID,
		Name:              v.Name,
		Upstream:          v.Upstream,
		CustomEncoderArgs: v.CustomEncoderArgs,
		CreatedAt:         v.CreatedAt,
		UpdatedAt:         v.UpdatedAt,
	}
	s.Audio.Devices = append([]string(nil), v.Audio.Devices...)
	s.Audio.Codec = v.Audio.Codec
	s.Audio.Bitrate = v.Audio.Bitrate
	s.Audio.Filters = v.Audio.Filters

	s.Encoder.Codec = v.Encoder.Codec
	s.Encoder.Bitrate = v.Encoder.Bitrate
	s.Encoder.GOP = v.Encoder.GOP
	s.Encoder.BFrames = v.Encoder.BFrames
	s.Encoder.RateControl = v.Encoder.RateControl
	s.Encoder.Preset = v.Encoder.Preset

	return s
}

func pipelineStreamToV2(s streams.PipelineStream) V2Stream {
	v := V2Stream{
		ID:                s.ID,
		Name:              s.Name,
		Upstream:          s.Upstream,
		CustomEncoderArgs: s.CustomEncoderArgs,
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
	}
	v.Audio.Devices = append([]string(nil), s.Audio.Devices...)
	v.Audio.Codec = s.Audio.Codec
	v.Audio.Bitrate = s.Audio.Bitrate
	v.Audio.Filters = s.Audio.Filters

	v.Encoder.Codec = s.Encoder.Codec
	v.Encoder.Bitrate = s.Encoder.Bitrate
	v.Encoder.GOP = s.Encoder.GOP
	v.Encoder.BFrames = s.Encoder.BFrames
	v.Encoder.RateControl = s.Encoder.RateControl
	v.Encoder.Preset = s.Encoder.Preset

	return v
}
