package main

import "github.com/unxed/vtui"

// Rotation and mirroring of decoded pictures. This is deliberately plain Go
// over the RGBA bytes of an ImageSurface: the graphics backends only know how
// to ship a rectangle of pixels, so anything the viewer wants to change about
// the orientation has to be baked into the pixels before they are sent.
//
// Every function here returns a fresh, tightly packed surface and never
// touches the source, so the viewer can keep the decoded picture around and
// rebuild the shown one whenever the orientation changes.

// CopySurface returns a tightly packed copy of a surface.
func CopySurface(src *vtui.ImageSurface) *vtui.ImageSurface {
	if !src.Valid() {
		return nil
	}
	dst := vtui.NewImageSurface(src.Width, src.Height)
	line := src.Width * 4
	for y := 0; y < src.Height; y++ {
		s := y * src.Stride
		d := y * dst.Stride
		copy(dst.Pix[d:d+line], src.Pix[s:s+line])
	}
	return dst
}

// RotateSurface turns a picture clockwise by the given angle, which is taken
// modulo a full turn and has to be a multiple of ninety degrees; any other
// angle would need resampling and is treated as no rotation at all. A quarter
// turn swaps the width and the height.
func RotateSurface(src *vtui.ImageSurface, degrees int) *vtui.ImageSurface {
	if !src.Valid() {
		return nil
	}
	deg := ((degrees % 360) + 360) % 360
	if deg == 0 || deg%90 != 0 {
		return CopySurface(src)
	}

	w, h := src.Width, src.Height
	dstW, dstH := w, h
	if deg == 90 || deg == 270 {
		dstW, dstH = h, w
	}
	dst := vtui.NewImageSurface(dstW, dstH)

	for y := 0; y < h; y++ {
		row := y * src.Stride
		for x := 0; x < w; x++ {
			var dx, dy int
			switch deg {
			case 90:
				dx, dy = h-1-y, x
			case 180:
				dx, dy = w-1-x, h-1-y
			default:
				dx, dy = y, w-1-x
			}
			s := row + x*4
			d := dy*dst.Stride + dx*4
			copy(dst.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return dst
}

// FlipSurface mirrors a picture across the vertical axis, the horizontal one,
// or both.
func FlipSurface(src *vtui.ImageSurface, horizontal, vertical bool) *vtui.ImageSurface {
	if !src.Valid() {
		return nil
	}
	if !horizontal && !vertical {
		return CopySurface(src)
	}

	dst := vtui.NewImageSurface(src.Width, src.Height)
	for y := 0; y < src.Height; y++ {
		dy := y
		if vertical {
			dy = src.Height - 1 - y
		}
		for x := 0; x < src.Width; x++ {
			dx := x
			if horizontal {
				dx = src.Width - 1 - x
			}
			s := y*src.Stride + x*4
			d := dy*dst.Stride + dx*4
			copy(dst.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return dst
}

// TransformSurface applies a rotation and then a mirroring. The order matters:
// the flip is meant as "mirror what I see", so it works on the already turned
// picture rather than on the decoded one.
func TransformSurface(src *vtui.ImageSurface, degrees int, horizontal, vertical bool) *vtui.ImageSurface {
	if !src.Valid() {
		return nil
	}
	out := RotateSurface(src, degrees)
	if horizontal || vertical {
		out = FlipSurface(out, horizontal, vertical)
	}
	return out
}
