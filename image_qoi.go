package main

// QOI, the Quite OK Image format. The whole specification fits on one page,
// which makes a decoder cheaper to carry than a dependency.

import (
	"encoding/binary"
	"fmt"

	"github.com/unxed/vtui"
)

const (
	qoiHeaderSize = 14

	qoiOpIndex = 0x00
	qoiOpDiff  = 0x40
	qoiOpLuma  = 0x80
	qoiOpRun   = 0xC0
	qoiOpRGB   = 0xFE
	qoiOpRGBA  = 0xFF
	qoiTagMask = 0xC0
)

type qoiPixel struct{ r, g, b, a byte }

// hash is the position a pixel takes in the running table of recent colours.
func (p qoiPixel) hash() int {
	return (int(p.r)*3 + int(p.g)*5 + int(p.b)*7 + int(p.a)*11) % 64
}

func decodeQOI(data []byte) (*vtui.ImageSurface, error) {
	if len(data) < qoiHeaderSize || string(data[:4]) != "qoif" {
		return nil, fmt.Errorf("not a QOI image")
	}
	width := int(binary.BigEndian.Uint32(data[4:8]))
	height := int(binary.BigEndian.Uint32(data[8:12]))
	if channels := data[12]; channels != 3 && channels != 4 {
		return nil, fmt.Errorf("unsupported channel count %d", channels)
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("the image has no size")
	}
	if int64(width)*int64(height) > imageMaxPixels {
		return nil, fmt.Errorf("the image is too large: %dx%d", width, height)
	}

	pix := make([]byte, width*height*4)
	var index [64]qoiPixel
	px := qoiPixel{a: 0xFF}
	pos := qoiHeaderSize
	run := 0

	for i := 0; i < len(pix); i += 4 {
		if run > 0 {
			run--
		} else {
			if pos >= len(data) {
				return nil, fmt.Errorf("the image data ends too early")
			}
			tag := data[pos]
			pos++

			switch {
			case tag == qoiOpRGB:
				if pos+3 > len(data) {
					return nil, fmt.Errorf("the image data ends too early")
				}
				px.r, px.g, px.b = data[pos], data[pos+1], data[pos+2]
				pos += 3
			case tag == qoiOpRGBA:
				if pos+4 > len(data) {
					return nil, fmt.Errorf("the image data ends too early")
				}
				px.r, px.g, px.b, px.a = data[pos], data[pos+1], data[pos+2], data[pos+3]
				pos += 4
			case tag&qoiTagMask == qoiOpIndex:
				px = index[tag&0x3F]
			case tag&qoiTagMask == qoiOpDiff:
				// Each difference is two bits wide and biased by two, and
				// it is meant to wrap around.
				px.r += ((tag >> 4) & 0x03) - 2
				px.g += ((tag >> 2) & 0x03) - 2
				px.b += (tag & 0x03) - 2
			case tag&qoiTagMask == qoiOpLuma:
				if pos >= len(data) {
					return nil, fmt.Errorf("the image data ends too early")
				}
				extra := data[pos]
				pos++
				dg := (tag & 0x3F) - 32
				px.r += dg - 8 + ((extra >> 4) & 0x0F)
				px.g += dg
				px.b += dg - 8 + (extra & 0x0F)
			default:
				// A run of the previous colour, one shorter than it says.
				run = int(tag & 0x3F)
			}
			index[px.hash()] = px
		}

		pix[i], pix[i+1], pix[i+2], pix[i+3] = px.r, px.g, px.b, px.a
	}

	surf := vtui.NewImageSurfaceFromPix(width, height, width*4, pix)
	if !surf.Valid() {
		return nil, fmt.Errorf("unsupported image geometry")
	}
	return surf, nil
}

func init() {
	RegisterImageDecoder(ImageDecoder{
		Name:       "go-qoi",
		Priority:   10,
		Extensions: []string{"qoi"},
		Decode:     decodeQOI,
	})
}
