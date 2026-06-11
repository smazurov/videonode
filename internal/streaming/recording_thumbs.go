package streaming

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/smazurov/videonode/internal/logging"
)

// Sprite-sheet storyboard layout (the VOD-industry pattern: YouTube/Mux pack
// ~100 tiles per sheet; WebVTT #xywh cues address tiles).
const (
	spriteCols = 10
	spriteRows = 10
)

// thumbnailFetchFunc returns a freshly-encoded JPEG of the recorded upstream
// (source/composer) plus its dimensions. Supplied by the recording manager,
// backed by the in-memory snapshot cache so no extra decode/encode is incurred
// beyond the downscale below.
type thumbnailFetchFunc func(ctx context.Context) (jpeg []byte, width, height int, err error)

// thumbnailConfig controls the per-recording storyboard track.
type thumbnailConfig struct {
	intervalSec int
	width       int
	fetch       thumbnailFetchFunc
	// mediaStart reports the wall-clock instant of media t=0 (first accepted
	// keyframe). Frames are skipped until it reports ok, so cue offsets line
	// up with the playback timeline instead of consumer-creation time.
	mediaStart func() (time.Time, bool)
}

// thumbCue is one storyboard frame: its time offset and tile position.
type thumbCue struct {
	offsetS float64
	sheet   int // 1-based sprite sheet number
	cell    int // 0-based cell within the sheet (row-major)
}

// thumbnailWriter samples the upstream on a ticker and packs frames into
// 10×10 sprite sheets (sprites/sprite_NNN.jpg) addressed by #xywh cues in an
// append-only thumbnails.vtt — consumed both by Media Chrome's hover preview
// and the filmstrip. The first frame is also written as poster.jpg for list
// previews. Tile dimensions are fixed by the first captured frame.
type thumbnailWriter struct {
	dir    string
	cfg    thumbnailConfig
	logger *slog.Logger

	done chan struct{}
	wg   sync.WaitGroup

	mu       sync.Mutex
	tileW    int
	tileH    int
	sheet    *image.RGBA
	cues     []thumbCue
	vtt      bytes.Buffer     // accumulated cue text, appended per frame
	sizes    map[string]int64 // latest on-disk size per file (sheets get rewritten)
	bytesOut int64
}

func newThumbnailWriter(dir string, cfg thumbnailConfig, logger *slog.Logger) (*thumbnailWriter, error) {
	if cfg.intervalSec <= 0 {
		cfg.intervalSec = 5
	}
	if cfg.width <= 0 {
		cfg.width = 240
	}
	if err := os.MkdirAll(filepath.Join(dir, "sprites"), 0o755); err != nil {
		return nil, fmt.Errorf("create sprites dir: %w", err)
	}
	w := &thumbnailWriter{
		dir:    dir,
		cfg:    cfg,
		logger: logger,
		done:   make(chan struct{}),
		sizes:  make(map[string]int64),
	}
	w.vtt.WriteString("WEBVTT\n\n")
	return w, nil
}

func (t *thumbnailWriter) start() {
	t.wg.Add(1)
	go t.loop()
}

func (t *thumbnailWriter) loop() {
	defer t.wg.Done()
	ticker := time.NewTicker(time.Duration(t.cfg.intervalSec) * time.Second)
	defer ticker.Stop()

	t.capture() // one frame at ~t=0 so the strip is populated immediately
	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			t.capture()
		}
	}
}

func (t *thumbnailWriter) capture() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	mediaT0, started := t.cfg.mediaStart()
	if !started {
		return // nothing recorded yet; a cue here would lead the timeline
	}
	raw, _, _, err := t.cfg.fetch(ctx)
	if err != nil {
		t.logger.Debug("thumbnail fetch failed", logging.KeyError, err)
		return
	}
	frame, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.logger.Debug("thumbnail decode failed", logging.KeyError, err)
		return
	}
	offset := time.Since(mediaT0).Seconds()

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.tileW == 0 {
		t.initTileDims(frame)
		t.writePoster(frame)
	}
	tile := scaleBilinear(frame, t.tileW, t.tileH)

	cell := len(t.cues) % (spriteCols * spriteRows)
	sheetNum := len(t.cues)/(spriteCols*spriteRows) + 1
	if cell == 0 {
		t.sheet = image.NewRGBA(image.Rect(0, 0, spriteCols*t.tileW, spriteRows*t.tileH))
	}
	x := (cell % spriteCols) * t.tileW
	y := (cell / spriteCols) * t.tileH
	draw.Draw(t.sheet, image.Rect(x, y, x+t.tileW, y+t.tileH), tile, image.Point{}, draw.Src)

	if !t.writeSheetLocked(sheetNum, y+t.tileH) {
		return
	}
	t.cues = append(t.cues, thumbCue{offsetS: offset, sheet: sheetNum, cell: cell})
	t.appendCueLocked(offset, sheetNum, x, y)
}

// initTileDims fixes the tile size from the first frame's aspect at the
// configured width. Caller holds t.mu.
func (t *thumbnailWriter) initTileDims(frame image.Image) {
	b := frame.Bounds()
	sw, sh := max(b.Dx(), 1), max(b.Dy(), 1)
	t.tileW = t.cfg.width
	t.tileH = max(int(math.Round(float64(sh)*float64(t.tileW)/float64(sw))), 1)
}

// writePoster saves the first frame as poster.jpg for list-row previews.
// Caller holds t.mu.
func (t *thumbnailWriter) writePoster(frame image.Image) {
	tile := scaleBilinear(frame, t.tileW, t.tileH)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, tile, &jpeg.Options{Quality: 72}); err != nil {
		t.logger.Debug("poster encode failed", logging.KeyError, err)
		return
	}
	t.writeAtomic("poster.jpg", out.Bytes())
}

// writeSheetLocked re-encodes the current sheet to sprites/sprite_NNN.jpg
// (write-temp-rename, so live readers always see a complete JPEG). Only the
// rows filled so far are encoded — #xywh cues never address unfilled rows, so
// a partial-height JPEG is valid and the per-frame encode cost stops growing
// with empty rows. Caller holds t.mu.
func (t *thumbnailWriter) writeSheetLocked(sheetNum, filledH int) bool {
	img := t.sheet.SubImage(image.Rect(0, 0, spriteCols*t.tileW, filledH))
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 72}); err != nil {
		t.logger.Debug("sprite encode failed", logging.KeyError, err)
		return false
	}
	t.writeAtomic(filepath.Join("sprites", fmt.Sprintf("sprite_%03d.jpg", sheetNum)), out.Bytes())
	return true
}

// appendCueLocked appends one #xywh cue (end = start + interval) to the
// accumulated VTT and republishes it. Append-plus-rename keeps the file write
// O(n) but the formatting O(1) per frame. Caller holds t.mu.
func (t *thumbnailWriter) appendCueLocked(offset float64, sheetNum, x, y int) {
	fmt.Fprintf(&t.vtt, "%s --> %s\nsprites/sprite_%03d.jpg#xywh=%d,%d,%d,%d\n\n",
		formatVTTTime(offset), formatVTTTime(offset+float64(t.cfg.intervalSec)),
		sheetNum, x, y, t.tileW, t.tileH)
	t.writeAtomic("thumbnails.vtt", t.vtt.Bytes())
}

func (t *thumbnailWriter) stop() {
	close(t.done)
	t.wg.Wait()
}

func (t *thumbnailWriter) writeAtomic(name string, data []byte) {
	if err := writeFileAtomic(t.dir, name, data); err != nil {
		t.logger.Debug("thumbnail write failed", logging.KeyError, err)
		return
	}
	t.bytesOut += int64(len(data)) - t.sizes[name]
	t.sizes[name] = int64(len(data))
}

// bytesWritten reports the storyboard bytes currently on disk (latest size of
// each written file).
func (t *thumbnailWriter) bytesWritten() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.bytesOut
}

// formatVTTTime renders seconds as HH:MM:SS.mmm for a WebVTT cue.
func formatVTTTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	total := int(sec)
	ms := int(math.Round((sec - float64(total)) * 1000))
	if ms >= 1000 {
		ms = 999
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// scaleBilinear resamples src into a dstW×dstH RGBA image. Dependency-free
// (stdlib only); fine for small, infrequent storyboard frames.
func scaleBilinear(src image.Image, dstW, dstH int) *image.RGBA {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	clamp := func(v, hi int) int {
		if v < 0 {
			return 0
		}
		if v > hi {
			return hi
		}
		return v
	}

	for y := range dstH {
		fy := (float64(y)+0.5)*float64(sh)/float64(dstH) - 0.5
		y0 := int(math.Floor(fy))
		wy := fy - float64(y0)
		y0c := clamp(y0, sh-1)
		y1c := clamp(y0+1, sh-1)
		for x := range dstW {
			fx := (float64(x)+0.5)*float64(sw)/float64(dstW) - 0.5
			x0 := int(math.Floor(fx))
			wx := fx - float64(x0)
			x0c := clamp(x0, sw-1)
			x1c := clamp(x0+1, sw-1)

			r00, g00, b00 := rgb8(src, sb.Min.X+x0c, sb.Min.Y+y0c)
			r10, g10, b10 := rgb8(src, sb.Min.X+x1c, sb.Min.Y+y0c)
			r01, g01, b01 := rgb8(src, sb.Min.X+x0c, sb.Min.Y+y1c)
			r11, g11, b11 := rgb8(src, sb.Min.X+x1c, sb.Min.Y+y1c)

			dst.Pix[dst.PixOffset(x, y)+0] = lerp2(r00, r10, r01, r11, wx, wy)
			dst.Pix[dst.PixOffset(x, y)+1] = lerp2(g00, g10, g01, g11, wx, wy)
			dst.Pix[dst.PixOffset(x, y)+2] = lerp2(b00, b10, b01, b11, wx, wy)
			dst.Pix[dst.PixOffset(x, y)+3] = 255
		}
	}
	return dst
}

func rgb8(img image.Image, x, y int) (float64, float64, float64) {
	r, g, b, _ := img.At(x, y).RGBA()
	return float64(r >> 8), float64(g >> 8), float64(b >> 8)
}

func lerp2(c00, c10, c01, c11, wx, wy float64) uint8 {
	top := c00 + (c10-c00)*wx
	bot := c01 + (c11-c01)*wx
	v := top + (bot-top)*wy
	if v < 0 {
		v = 0
	}
	if v > 255 {
		v = 255
	}
	return uint8(v + 0.5)
}
