package vtui

import (
	"image"
	"image/draw"
)

// ImageSurface stores straight (non-premultiplied) alpha, while the standard
// library uses premultiplied alpha. This file is the only place that knows
// about the difference.

func unpremultiply(pix []byte) {
	for i := 0; i+3 < len(pix); i += 4 {
		a := uint32(pix[i+3])
		if a == 255 {
			continue
		}
		if a == 0 {
			pix[i], pix[i+1], pix[i+2] = 0, 0, 0
			continue
		}
		pix[i] = byte(uint32(pix[i]) * 255 / a)
		pix[i+1] = byte(uint32(pix[i+1]) * 255 / a)
		pix[i+2] = byte(uint32(pix[i+2]) * 255 / a)
	}
}

func premultiply(pix []byte) {
	for i := 0; i+3 < len(pix); i += 4 {
		a := uint32(pix[i+3])
		if a == 255 {
			continue
		}
		pix[i] = byte(uint32(pix[i]) * a / 255)
		pix[i+1] = byte(uint32(pix[i+1]) * a / 255)
		pix[i+2] = byte(uint32(pix[i+2]) * a / 255)
	}
}

func isOpaque(s *ImageSurface) bool {
	for y := 0; y < s.Height; y++ {
		off := y*s.Stride + 3
		for x := 0; x < s.Width; x++ {
			if s.Pix[off] != 255 {
				return false
			}
			off += 4
		}
	}
	return true
}

// NewImageSurfaceFromImage converts any Go image into a surface. Decoders in
// the application produce image.Image, the rendering layer consumes surfaces.
func NewImageSurfaceFromImage(img image.Image) *ImageSurface {
	if img == nil {
		return nil
	}
	b := img.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), img, b.Min, draw.Src)
	unpremultiply(out.Pix)
	return &ImageSurface{
		Width:  b.Dx(),
		Height: b.Dy(),
		Stride: out.Stride,
		Pix:    out.Pix,
	}
}

// ToRGBA exposes the surface as a standard Go image. A fully opaque surface
// shares its memory, which matters because the GUI backends call this on
// every frame they repaint.
func (s *ImageSurface) ToRGBA() *image.RGBA {
	if !s.Valid() {
		return nil
	}
	if s.Stride == s.Width*4 && isOpaque(s) {
		return &image.RGBA{
			Pix:    s.Pix[:s.Width*s.Height*4],
			Stride: s.Stride,
			Rect:   image.Rect(0, 0, s.Width, s.Height),
		}
	}
	out := image.NewRGBA(image.Rect(0, 0, s.Width, s.Height))
	for y := 0; y < s.Height; y++ {
		copy(out.Pix[y*out.Stride:y*out.Stride+s.Width*4], s.Pix[y*s.Stride:y*s.Stride+s.Width*4])
	}
	premultiply(out.Pix)
	return out
}
