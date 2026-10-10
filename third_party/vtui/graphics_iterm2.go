package vtui

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"strconv"
)

// iterm2CacheLimit bounds the encoded pictures kept between frames. A PNG of a
// full-screen picture is a few hundred kilobytes of base64, so the bound is
// small; a cache hit is what keeps panning through one picture cheap.
const iterm2CacheLimit = 24

// iterm2Encoder puts pictures on screen with iTerm2's inline image protocol
// (OSC 1337 ; File=...), also understood by WezTerm, mintty and others. The
// picture becomes part of the cell content, like a sixel: the terminal drops it
// when the cells under it are overwritten, and the layer redraws it whenever
// the text below it was repainted, so no explicit clearing is sent.
//
// Each placement is cropped to its source rectangle, shrunk to the pixel size
// of the cells it covers when the source is larger (so a photograph is not sent
// at full size to be shown in a window), and sent as a PNG that the terminal
// stretches over exactly Cols x Rows cells.
type iterm2Encoder struct {
	cache map[uint64]string
	order []uint64
}

func newITerm2Encoder() *iterm2Encoder {
	return &iterm2Encoder{cache: make(map[uint64]string)}
}

// Reset drops the encoded pictures. iTerm2 keeps nothing on our behalf, so
// there is nothing to tell the terminal.
func (e *iterm2Encoder) Reset() {
	e.cache = make(map[uint64]string)
	e.order = e.order[:0]
}

// Render draws every placement of list. cw and ch are the pixel size of a
// cell, or zero when unknown, in which case pictures are sent at source size.
func (e *iterm2Encoder) Render(sb kittyBuffer, list []ImagePlacement, cw, ch int) {
	for i := range list {
		p := &list[i]
		if !p.Surface.Valid() || p.Cols <= 0 || p.Rows <= 0 {
			continue
		}
		sx, sy, sw, sh := p.Source()
		if sw <= 0 || sh <= 0 {
			continue
		}
		key := nativeCacheKey(p.Surface.Hash(), sx, sy, sw, sh, p.Cols, p.Rows, cw, ch)
		seq, ok := e.cache[key]
		if !ok {
			seq = iterm2Sequence(p, sx, sy, sw, sh, cw, ch)
			if seq == "" {
				continue
			}
			e.cache[key] = seq
			e.order = append(e.order, key)
			if len(e.order) > iterm2CacheLimit {
				delete(e.cache, e.order[0])
				e.order = e.order[1:]
			}
		}
		// The frame buffer's writes cannot fail; the results are dropped
		// explicitly for errcheck.
		_, _ = sb.WriteString("\x1b[")
		sixelWriteCoord(sb, p.Row+1)
		_ = sb.WriteByte(';')
		sixelWriteCoord(sb, p.Col+1)
		_ = sb.WriteByte('H')
		_, _ = sb.WriteString(seq)
	}
}

// iterm2Sequence builds the OSC 1337 File sequence for one placement, or ""
// when the picture cannot be encoded.
func iterm2Sequence(p *ImagePlacement, sx, sy, sw, sh, cw, ch int) string {
	pic := p.Surface.Crop(sx, sy, sw, sh)
	if pic == nil {
		return ""
	}
	if cw > 0 && ch > 0 {
		if dw, dh := p.Cols*cw, p.Rows*ch; pic.Width > dw && pic.Height > dh {
			pic = ScaleSurface(pic, dw, dh)
		}
	}
	if !pic.Valid() {
		return ""
	}
	img := &image.NRGBA{Pix: pic.Pix, Stride: pic.Stride, Rect: image.Rect(0, 0, pic.Width, pic.Height)}
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := enc.Encode(&buf, img); err != nil {
		return ""
	}
	var sb bytes.Buffer
	sb.WriteString("\x1b]1337;File=inline=1;width=")
	sb.WriteString(strconv.Itoa(p.Cols))
	sb.WriteString(";height=")
	sb.WriteString(strconv.Itoa(p.Rows))
	sb.WriteString(";preserveAspectRatio=0;size=")
	sb.WriteString(strconv.Itoa(buf.Len()))
	sb.WriteByte(':')
	sb.WriteString(base64.StdEncoding.EncodeToString(buf.Bytes()))
	sb.WriteByte(0x07)
	return sb.String()
}
