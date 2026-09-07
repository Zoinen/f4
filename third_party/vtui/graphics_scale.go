package vtui

import (
	"math"
	"runtime"
	"sync"
)

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
	// Opaque shrink is the gallery-tile and native-zoom norm: the integer
	// box filter beats the float path on both time and memory.
	if src.Opaque && w < src.Width && h < src.Height {
		if out := scaleSurfaceBox(src, w, h); out != nil {
			return out
		}
	}
	return scaleSurfaceFloat(src, w, h)
}

// scaleSurfaceFloat is the premultiplied, area-weighted resampler. It is the
// reference for every source with alpha or a grow; opaque downscales take the
// integer box filter instead.
func scaleSurfaceFloat(src *ImageSurface, w, h int) *ImageSurface {
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
	if out == nil {
		return nil
	}
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
	out.Opaque = src.Opaque
	return out
}

// scaleRun is one destination run along an axis: the source window it averages
// and the reciprocal that turns a window sum into the average.
type scaleRun struct {
	first int
	count uint32
	inv   uint64 // (1<<32)/count: (sum*inv)>>32 is the exact integer average
}

// buildScaleRuns lays out the source window each destination sample covers
// when shrinking: dst i averages source [i*src/dst, (i+1)*src/dst).
func buildScaleRuns(srcLen, dstLen int) []scaleRun {
	runs := make([]scaleRun, dstLen)
	for i := 0; i < dstLen; i++ {
		x1 := (i * srcLen) / dstLen
		x2 := ((i + 1) * srcLen) / dstLen
		if x2 == x1 {
			x2 = x1 + 1
		}
		count := uint32(x2 - x1)
		// A count of 1 would need the reciprocal 2^32, which overflows a
		// 32-bit word: keep the multiplier 64-bit wide.
		runs[i] = scaleRun{first: x1, count: count, inv: (uint64(1) << 32) / uint64(count)}
	}
	return runs
}

// scaleWorkers bounds the box filter's pass parallelism; goroutines only pay
// above a few hundred rows or columns.
var scaleWorkers = func() int {
	n := runtime.GOMAXPROCS(0)
	if n < 1 {
		return 1
	}
	if n > 16 {
		return 16
	}
	return n
}()

// scaleSerialRows is the smallest axis the box filter splits across workers.
const scaleSerialRows = 256

// scaleSurfaceBox downsamples an opaque source with a separable box filter:
// integer prefix sums per row and a sliding window per column, no per-pixel
// float math and no premultiply pass. It averages the same pixel windows as
// the float path (sRGB output matches within a few units); alpha needs the
// premultiply float path.
func scaleSurfaceBox(src *ImageSurface, w, h int) *ImageSurface {
	if !src.Valid() || w <= 0 || h <= 0 {
		return nil
	}
	sw, sh := src.Width, src.Height
	out := NewImageSurface(w, h)
	if out == nil {
		return nil
	}
	xr := buildScaleRuns(sw, w)
	yr := buildScaleRuns(sh, h)

	// Horizontal pass: average each source row into w columns. Prefix sums in
	// uint32 keep window sums exact; the averages fit uint16. Rows are
	// independent, so they split across workers, one prefix bank each.
	tmp := make([]uint16, w*sh*3)
	workers := scaleWorkers
	if sh < scaleSerialRows {
		workers = 1
	}
	rowPass := func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			row := y * src.Stride
			tRow := y * w * 3
			for cx := range xr {
				r := &xr[cx]
				x2 := r.first + int(r.count)
				var rSum, gSum, bSum uint32
				for x := r.first; x < x2; x++ {
					o := row + x*4
					rSum += uint32(src.Pix[o])
					gSum += uint32(src.Pix[o+1])
					bSum += uint32(src.Pix[o+2])
				}
				o := tRow + cx*3
				tmp[o] = uint16((uint64(rSum) * r.inv) >> 32)
				tmp[o+1] = uint16((uint64(gSum) * r.inv) >> 32)
				tmp[o+2] = uint16((uint64(bSum) * r.inv) >> 32)
			}
		}
	}
	if workers == 1 {
		rowPass(0, sh)
	} else {
		var wg sync.WaitGroup
		rowsPer := (sh + workers - 1) / workers
		for wkr := 0; wkr < workers; wkr++ {
			y0 := wkr * rowsPer
			y1 := y0 + rowsPer
			if y1 > sh {
				y1 = sh
			}
			if y0 >= y1 {
				continue
			}
			wg.Add(1)
			go func(y0, y1 int) {
				defer wg.Done()
				rowPass(y0, y1)
			}(y0, y1)
		}
		wg.Wait()
	}

	// Vertical pass: slide one window over the tmp rows keeping a running
	// column sum, so every byte is read once (each row added and subtracted
	// once). Output columns are independent, so each worker takes a band.
	if w >= scaleSerialRows && workers > 1 {
		var wg sync.WaitGroup
		colBanks := make([]uint32, w*3*workers)
		colsPer := (w + workers - 1) / workers
		for wkr := 0; wkr < workers; wkr++ {
			x0 := wkr * colsPer
			x1 := x0 + colsPer
			if x1 > w {
				x1 = w
			}
			if x0 >= x1 {
				continue
			}
			bank := wkr * 3 * w
			wg.Add(1)
			go func(x0, x1, bank int) {
				defer wg.Done()
				n := x1 - x0
				colR := colBanks[bank : bank+n]
				colG := colBanks[bank+w : bank+w+n]
				colB := colBanks[bank+2*w : bank+2*w+n]
				curY1, curY2 := 0, 0
				for cy := range yr {
					r := &yr[cy]
					nextY2 := r.first + int(r.count)
					for y := curY2; y < nextY2; y++ {
						row := y * w * 3
						for i := 0; i < n; i++ {
							o := row + (x0+i)*3
							colR[i] += uint32(tmp[o])
							colG[i] += uint32(tmp[o+1])
							colB[i] += uint32(tmp[o+2])
						}
					}
					for y := curY1; y < r.first; y++ {
						row := y * w * 3
						for i := 0; i < n; i++ {
							o := row + (x0+i)*3
							colR[i] -= uint32(tmp[o])
							colG[i] -= uint32(tmp[o+1])
							colB[i] -= uint32(tmp[o+2])
						}
					}
					curY1, curY2 = r.first, nextY2
					inv := r.inv
					o := cy*out.Stride + x0*4
					for i := 0; i < n; i++ {
						out.Pix[o+i*4] = uint8((uint64(colR[i]) * inv) >> 32)
						out.Pix[o+i*4+1] = uint8((uint64(colG[i]) * inv) >> 32)
						out.Pix[o+i*4+2] = uint8((uint64(colB[i]) * inv) >> 32)
						out.Pix[o+i*4+3] = 255
					}
				}
			}(x0, x1, bank)
		}
		wg.Wait()
	} else {
		colR := make([]uint32, w)
		colG := make([]uint32, w)
		colB := make([]uint32, w)
		curY1, curY2 := 0, 0
		for cy := range yr {
			r := &yr[cy]
			nextY2 := r.first + int(r.count)
			for y := curY2; y < nextY2; y++ {
				row := y * w * 3
				for x := 0; x < w; x++ {
					o := row + x*3
					colR[x] += uint32(tmp[o])
					colG[x] += uint32(tmp[o+1])
					colB[x] += uint32(tmp[o+2])
				}
			}
			for y := curY1; y < r.first; y++ {
				row := y * w * 3
				for x := 0; x < w; x++ {
					o := row + x*3
					colR[x] -= uint32(tmp[o])
					colG[x] -= uint32(tmp[o+1])
					colB[x] -= uint32(tmp[o+2])
				}
			}
			curY1, curY2 = r.first, nextY2
			inv := r.inv
			o := cy * out.Stride
			for x := 0; x < w; x++ {
				out.Pix[o+x*4] = uint8((uint64(colR[x]) * inv) >> 32)
				out.Pix[o+x*4+1] = uint8((uint64(colG[x]) * inv) >> 32)
				out.Pix[o+x*4+2] = uint8((uint64(colB[x]) * inv) >> 32)
				out.Pix[o+x*4+3] = 255
			}
		}
	}
	out.Opaque = true
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
