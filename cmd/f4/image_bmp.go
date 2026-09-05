package main

// BMP. Nothing produces it on purpose any more and everything produces it by
// accident, so a file manager had better read it.

import (
	"encoding/binary"
	"fmt"

	"github.com/unxed/vtui"
)

const (
	bmpFileHeaderSize = 14
	bmpInfoHeaderSize = 40
)

func decodeBMP(data []byte) (*vtui.ImageSurface, error) {
	if len(data) < bmpFileHeaderSize+bmpInfoHeaderSize || data[0] != 'B' || data[1] != 'M' {
		return nil, fmt.Errorf("not a BMP image")
	}
	le := binary.LittleEndian

	pixelOffset := int(le.Uint32(data[10:14]))
	headerSize := int(le.Uint32(data[14:18]))
	if headerSize < bmpInfoHeaderSize {
		return nil, fmt.Errorf("unsupported BMP header of %d bytes", headerSize)
	}

	// BMP stores dimensions as signed 32-bit two's-complement fields even
	// though binary.ByteOrder exposes only unsigned readers.
	// #nosec G115 -- the conversion reinterprets the specified signed BMP field.
	width := int(int32(le.Uint32(data[18:22])))
	// #nosec G115 -- the conversion reinterprets the specified signed BMP field.
	height := int(int32(le.Uint32(data[22:26])))
	bits := int(le.Uint16(data[28:30]))
	compression := le.Uint32(data[30:34])
	paletteLen := int(le.Uint32(data[46:50]))

	// A negative height means the rows are stored the right way up, which is
	// the exception rather than the rule.
	topDown := height < 0
	if topDown {
		height = -height
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("the image has no size")
	}
	if int64(width)*int64(height) > imageMaxPixels {
		return nil, fmt.Errorf("the image is too large: %dx%d", width, height)
	}
	if compression != 0 {
		return nil, fmt.Errorf("compressed BMP images are not supported")
	}
	switch bits {
	case 1, 4, 8, 16, 24, 32:
	default:
		return nil, fmt.Errorf("unsupported colour depth of %d bits", bits)
	}

	var palette []qoiPixel
	if bits <= 8 {
		if paletteLen <= 0 || paletteLen > 1<<bits {
			paletteLen = 1 << bits
		}
		at := bmpFileHeaderSize + headerSize
		if at+paletteLen*4 > len(data) {
			return nil, fmt.Errorf("the palette is truncated")
		}
		palette = make([]qoiPixel, paletteLen)
		for i := range palette {
			e := data[at+i*4:]
			palette[i] = qoiPixel{r: e[2], g: e[1], b: e[0], a: 0xFF}
		}
	}

	stride := ((width*bits + 31) / 32) * 4
	if pixelOffset < bmpFileHeaderSize+headerSize || pixelOffset+stride*height > len(data) {
		return nil, fmt.Errorf("the pixel data is truncated")
	}

	pix := make([]byte, width*height*4)
	sawAlpha := false

	for y := 0; y < height; y++ {
		src := pixelOffset + stride*y
		dstRow := y
		if !topDown {
			dstRow = height - 1 - y
		}
		dst := dstRow * width * 4

		for x := 0; x < width; x++ {
			var c qoiPixel
			switch bits {
			case 32:
				e := data[src+x*4:]
				c = qoiPixel{r: e[2], g: e[1], b: e[0], a: e[3]}
				if e[3] != 0 {
					sawAlpha = true
				}
			case 24:
				e := data[src+x*3:]
				c = qoiPixel{r: e[2], g: e[1], b: e[0], a: 0xFF}
			case 16:
				v := le.Uint16(data[src+x*2:])
				c = qoiPixel{
					r: bmpScale5((v >> 10) & 0x1F),
					g: bmpScale5((v >> 5) & 0x1F),
					b: bmpScale5(v & 0x1F),
					a: 0xFF,
				}
			case 8:
				c = bmpPaletteAt(palette, int(data[src+x]))
			case 4:
				b := data[src+x/2]
				if x%2 == 0 {
					b >>= 4
				}
				c = bmpPaletteAt(palette, int(b&0x0F))
			default:
				b := data[src+x/8]
				c = bmpPaletteAt(palette, int((b>>(7-uint(x%8)))&0x01))
			}

			at := dst + x*4
			pix[at], pix[at+1], pix[at+2], pix[at+3] = c.r, c.g, c.b, c.a
		}
	}

	// A thirty two bit BMP with an all zero alpha channel is an opaque
	// picture whose last byte was simply never filled in.
	if bits == 32 && !sawAlpha {
		for i := 3; i < len(pix); i += 4 {
			pix[i] = 0xFF
		}
	}

	surf := vtui.NewImageSurfaceFromPix(width, height, width*4, pix)
	if !surf.Valid() {
		return nil, fmt.Errorf("unsupported image geometry")
	}
	return surf, nil
}

// bmpScale5 stretches a five bit channel over the whole byte.
func bmpScale5(v uint16) byte {
	if v > 0x1F {
		v = 0x1F
	}
	// #nosec G115 -- a clamped five-bit channel scales to at most 255.
	return byte((int(v)*255 + 15) / 31)
}

func bmpPaletteAt(palette []qoiPixel, i int) qoiPixel {
	if i < 0 || i >= len(palette) {
		return qoiPixel{a: 0xFF}
	}
	return palette[i]
}

func init() {
	RegisterImageDecoder(ImageDecoder{
		Name:       "go-bmp",
		Priority:   10,
		Extensions: []string{"bmp", "dib"},
		Decode:     decodeBMP,
	})
}
