package vtui

import "math"

// resampleTap is one source sample contributing to a destination sample.
type resampleTap struct {
	idx    int
	weight float32
}

// buildResampleTaps precomputes the source samples for every destination
// index along one axis. Shrinking uses area averaging, which is what keeps a
// downscaled photo free of aliasing; growing uses linear interpolation.
func buildResampleTaps(srcLen, dstLen int) [][]resampleTap {
	taps := make([][]resampleTap, dstLen)

	if dstLen < srcLen {
		scale := float64(srcLen) / float64(dstLen)
		for i := 0; i < dstLen; i++ {
			from := float64(i) * scale
			to := from + scale
			first := int(from)
			last := int(math.Ceil(to)) - 1
			if last >= srcLen {
				last = srcLen - 1
			}
			list := make([]resampleTap, 0, last-first+1)
			total := 0.0
			for s := first; s <= last; s++ {
				lo := math.Max(from, float64(s))
				hi := math.Min(to, float64(s+1))
				if hi <= lo {
					continue
				}
				list = append(list, resampleTap{idx: s, weight: float32(hi - lo)})
				total += hi - lo
			}
			if total > 0 {
				for j := range list {
					list[j].weight = float32(float64(list[j].weight) / total)
				}
			}
			taps[i] = list
		}
		return taps
	}

	for i := 0; i < dstLen; i++ {
		pos := (float64(i)+0.5)*float64(srcLen)/float64(dstLen) - 0.5
		if pos < 0 {
			pos = 0
		}
		s0 := int(pos)
		if s0 > srcLen-1 {
			s0 = srcLen - 1
		}
		s1 := s0 + 1
		if s1 > srcLen-1 {
			s1 = srcLen - 1
		}
		frac := pos - float64(s0)
		if s0 == s1 || frac <= 0 {
			taps[i] = []resampleTap{{idx: s0, weight: 1}}
			continue
		}
		taps[i] = []resampleTap{
			{idx: s0, weight: float32(1 - frac)},
			{idx: s1, weight: float32(frac)},
		}
	}
	return taps
}

func clampToByte(v float32) byte {
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return byte(v + 0.5)
}

// ScaleSurface resamples src to exactly w x h pixels. The filtering runs on
// premultiplied values, otherwise transparent pixels would bleed their colour
// into their neighbours. When the size already matches, src is returned
// unchanged, so callers must not write into the result.
func ScaleSurface(src *ImageSurface, w, h int) *ImageSurface {
	if !src.Valid() || w <= 0 || h <= 0 {
		return nil
	}
	if w == src.Width && h == src.Height {
		return src
	}

	xt := buildResampleTaps(src.Width, w)
	yt := buildResampleTaps(src.Height, h)

	tmp := make([]float32, w*src.Height*4)
	for y := 0; y < src.Height; y++ {
		rowOff := y * src.Stride
		dstOff := y * w * 4
		for x := 0; x < w; x++ {
			var pr, pg, pb, pa float32
			for _, t := range xt[x] {
				o := rowOff + t.idx*4
				a := float32(src.Pix[o+3])
				af := a / 255
				pr += float32(src.Pix[o]) * af * t.weight
				pg += float32(src.Pix[o+1]) * af * t.weight
				pb += float32(src.Pix[o+2]) * af * t.weight
				pa += a * t.weight
			}
			o := dstOff + x*4
			tmp[o], tmp[o+1], tmp[o+2], tmp[o+3] = pr, pg, pb, pa
		}
	}

	out := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		dstOff := y * out.Stride
		for x := 0; x < w; x++ {
			var pr, pg, pb, pa float32
			for _, t := range yt[y] {
				o := (t.idx*w + x) * 4
				pr += tmp[o] * t.weight
				pg += tmp[o+1] * t.weight
				pb += tmp[o+2] * t.weight
				pa += tmp[o+3] * t.weight
			}
			o := dstOff + x*4
			if pa <= 0 {
				continue
			}
			scale := 255 / pa
			out.Pix[o] = clampToByte(pr * scale)
			out.Pix[o+1] = clampToByte(pg * scale)
			out.Pix[o+2] = clampToByte(pb * scale)
			out.Pix[o+3] = clampToByte(pa)
		}
	}
	return out
}

// FitInside returns the largest size with the aspect ratio of srcW x srcH that
// still fits into boxW x boxH. Both the viewer and the thumbnails need it.
func FitInside(srcW, srcH, boxW, boxH int) (int, int) {
	if srcW <= 0 || srcH <= 0 || boxW <= 0 || boxH <= 0 {
		return 0, 0
	}
	w := boxW
	h := srcH * boxW / srcW
	if h > boxH {
		h = boxH
		w = srcW * boxH / srcH
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}
