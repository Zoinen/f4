package main

// Port of the contrast machinery from far2l's utils/src/colorspace.cpp.
//
// Only the pieces AdjustContrastLevels needs are here: sRGB <-> CIE L*a*b*,
// CIEDE2000, the WCAG ratio and ComputeContrast itself. The structure follows
// the original closely — same thresholds, same iteration counts, same step
// clamping, same truncation back to 8 bits — so that a colour corrected by f4
// lands on the same value far2l would have produced for it.
//
// Note that far2l's contract is *not* "reach WCAG 4.5". It is "reach ΔE2000 30,
// or failing that ΔE2000 20, by moving L* only". A pair can come out of the
// correction with a poor WCAG ratio and still be considered done; yellow on
// light grey is the usual example. Keep that in mind before tightening any
// assertion about the result.

import "math"

// rgbF holds sRGB components in 0..1, matching far2l's RGB struct.
type rgbF struct{ R, G, B float64 }

// labF holds CIE L*a*b*: L in 0..100, a and b roughly -128..127.
type labF struct{ L, A, B float64 }

// ContrastLevel mirrors far2l's enum of the same name.
type ContrastLevel int

const (
	ContrastGood ContrastLevel = iota
	ContrastWarning
	ContrastBad
)

func (l ContrastLevel) String() string {
	switch l {
	case ContrastGood:
		return "Good"
	case ContrastWarning:
		return "Borderline"
	case ContrastBad:
		return "Poor"
	}
	return "Unknown"
}

// toRGBF splits a packed 0xRRGGBB value into 0..1 components.
func toRGBF(c uint32) rgbF {
	return rgbF{
		R: float64((c>>16)&0xFF) / 255.0,
		G: float64((c>>8)&0xFF) / 255.0,
		B: float64(c&0xFF) / 255.0,
	}
}

// toRGB24 packs components back into 0xRRGGBB. far2l's toIRGB truncates rather
// than rounds, and the difference is visible on the last unit, so truncate too.
func toRGB24(c rgbF) uint32 {
	return uint32(c.R*255.0)<<16 | uint32(c.G*255.0)<<8 | uint32(c.B*255.0)
}

func clamp01(v float64) float64 {
	return math.Min(math.Max(v, 0.0), 1.0)
}

// --- sRGB <-> L*a*b* -------------------------------------------------------

func invGamma(x float64) float64 {
	if x <= 0.04045 {
		return x / 12.92
	}
	return math.Pow((x+0.055)/1.055, 2.4)
}

func fwdGamma(x float64) float64 {
	if x <= 0.0031308 {
		return 12.92 * x
	}
	return 1.055*math.Pow(x, 1.0/2.4) - 0.055
}

func rgbToLAB(c rgbF) labF {
	r, g, b := invGamma(c.R), invGamma(c.G), invGamma(c.B)

	// sRGB -> XYZ (D65), normalised by the D65 white point.
	x := (r*0.4124 + g*0.3576 + b*0.1805) / 0.95047
	y := r*0.2126 + g*0.7152 + b*0.0722
	z := (r*0.0193 + g*0.1192 + b*0.9505) / 1.08883

	f := func(t float64) float64 {
		if t > 0.008856 {
			return math.Cbrt(t)
		}
		return 7.787*t + 16.0/116.0
	}

	return labF{
		L: 116.0*f(y) - 16.0,
		A: 500.0 * (f(x) - f(y)),
		B: 200.0 * (f(y) - f(z)),
	}
}

func labToRGB(l labF) rgbF {
	fy := (l.L + 16.0) / 116.0
	fx := fy + l.A/500.0
	fz := fy - l.B/200.0

	invf := func(t float64) float64 {
		t3 := t * t * t
		if t3 > 0.008856 {
			return t3
		}
		return (t - 16.0/116.0) / 7.787
	}

	x := invf(fx) * 0.95047
	y := invf(fy)
	z := invf(fz) * 1.08883

	r := 3.2406*x - 1.5372*y - 0.4986*z
	g := -0.9689*x + 1.8758*y + 0.0415*z
	b := 0.0557*x - 0.2040*y + 1.0570*z

	return rgbF{clamp01(fwdGamma(r)), clamp01(fwdGamma(g)), clamp01(fwdGamma(b))}
}

// --- WCAG ------------------------------------------------------------------

func srgbToLinear(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}

func relativeLuminance(c rgbF) float64 {
	return 0.2126*srgbToLinear(c.R) + 0.7152*srgbToLinear(c.G) + 0.0722*srgbToLinear(c.B)
}

func wcagComputeContrast(fg, bg rgbF) float64 {
	hi, lo := relativeLuminance(fg), relativeLuminance(bg)
	if hi < lo {
		hi, lo = lo, hi
	}
	return (hi + 0.05) / (lo + 0.05)
}

// --- CIEDE2000 -------------------------------------------------------------

func deltaE2000(c1, c2 labF) float64 {
	C1 := math.Hypot(c1.A, c1.B)
	C2 := math.Hypot(c2.A, c2.B)
	Cbar := (C1 + C2) * 0.5

	Cbar7 := math.Pow(Cbar, 7.0)
	G := 0.5 * (1.0 - math.Sqrt(Cbar7/(Cbar7+math.Pow(25.0, 7.0))))

	a1p := (1.0 + G) * c1.A
	a2p := (1.0 + G) * c2.A

	C1p := math.Hypot(a1p, c1.B)
	C2p := math.Hypot(a2p, c2.B)

	hp := func(x, y float64) float64 {
		if x == 0 && y == 0 {
			return 0.0
		}
		h := math.Atan2(y, x)
		if h < 0 {
			h += 2.0 * math.Pi
		}
		return h
	}

	h1p := hp(a1p, c1.B)
	h2p := hp(a2p, c2.B)

	dLp := c2.L - c1.L
	dCp := C2p - C1p

	var dhp float64
	if C1p*C2p != 0 {
		dh := h2p - h1p
		if dh > math.Pi {
			dh -= 2.0 * math.Pi
		}
		if dh < -math.Pi {
			dh += 2.0 * math.Pi
		}
		dhp = dh
	}

	dHp := 2.0 * math.Sqrt(C1p*C2p) * math.Sin(dhp*0.5)

	Lbarp := (c1.L + c2.L) * 0.5
	Cbarp := (C1p + C2p) * 0.5

	var hbarp float64
	if C1p*C2p == 0 {
		hbarp = h1p + h2p
	} else if dh := math.Abs(h1p - h2p); dh > math.Pi {
		if h1p+h2p < 2.0*math.Pi {
			hbarp = (h1p + h2p + 2.0*math.Pi) * 0.5
		} else {
			hbarp = (h1p + h2p - 2.0*math.Pi) * 0.5
		}
	} else {
		hbarp = (h1p + h2p) * 0.5
	}

	T := 1.0 -
		0.17*math.Cos(hbarp-math.Pi/6.0) +
		0.24*math.Cos(2.0*hbarp) +
		0.32*math.Cos(3.0*hbarp+math.Pi/30.0) -
		0.20*math.Cos(4.0*hbarp-63.0*math.Pi/180.0)

	Sl := 1.0 + (0.015*math.Pow(Lbarp-50.0, 2.0))/math.Sqrt(20.0+math.Pow(Lbarp-50.0, 2.0))
	Sc := 1.0 + 0.045*Cbarp
	Sh := 1.0 + 0.015*Cbarp*T

	deltaTheta := 30.0 * math.Pi / 180.0 *
		math.Exp(-math.Pow((hbarp*180.0/math.Pi-275.0)/25.0, 2.0))

	Rc := 2.0 * math.Sqrt(math.Pow(Cbarp, 7.0)/(math.Pow(Cbarp, 7.0)+math.Pow(25.0, 7.0)))
	Rt := -math.Sin(2.0*deltaTheta) * Rc

	return math.Sqrt(
		math.Pow(dLp/Sl, 2.0) +
			math.Pow(dCp/Sc, 2.0) +
			math.Pow(dHp/Sh, 2.0) +
			Rt*(dCp/Sc)*(dHp/Sh))
}

// --- ComputeContrast -------------------------------------------------------

// ComputeContrast is the Go equivalent of far2l's function of the same name.
// It returns the possibly adjusted foreground and the level it settled on.
//
// A pass that succeeds can still hand back the colour it was given: the walk
// stops as soon as the target is reached, which on the very first iteration
// means nothing moved but the value went through L*a*b* and back. far2l has
// the same behaviour and simply compares the 8-bit results, so callers should
// do that rather than treat "a pass ran" as "the colour changed".
func ComputeContrast(fg, bg rgbF) (newFg rgbF, level ContrastLevel) {
	labFg := rgbToLAB(fg)
	labBg := rgbToLAB(bg)

	dL := math.Abs(labFg.L - labBg.L)
	dE := deltaE2000(labFg, labBg)
	ratio := wcagComputeContrast(fg, bg)

	goodLab := dL >= 40.0 || dE >= 50.0
	goodWcag := ratio >= 7.0
	if goodLab && goodWcag {
		return fg, ContrastGood
	}

	// First pass: walk L* away from the background until ΔE2000 reaches 30.
	// The direction is re-evaluated every step, so a foreground that starts on
	// the wrong side crosses over rather than saturating.
	if adjusted, ok := walkLightness(labFg, labBg, 30.0, false); ok {
		return adjusted, ContrastGood
	}

	// Second pass: settle for ΔE2000 20, this time choosing the direction once
	// from whether the background is light or dark.
	if adjusted, ok := walkLightness(rgbToLAB(fg), labBg, 20.0, true); ok {
		return adjusted, ContrastGood
	}

	return fg, ContrastBad
}

// walkLightness nudges labFg.L towards the target ΔE2000 in at most 50 steps.
// When fixedDirection is set the side to move towards is decided once from the
// background lightness, matching far2l's second pass.
func walkLightness(labFg, labBg labF, target float64, fixedDirection bool) (rgbF, bool) {
	for i := 0; i < 50; i++ {
		dE := deltaE2000(labFg, labBg)
		if dE >= target {
			return labToRGB(labFg), true
		}

		step := math.Min(math.Max((target-dE)*0.25, 0.5), 3.0)

		darken := labBg.L > labFg.L
		if fixedDirection {
			darken = labBg.L > 50
		}
		if darken {
			labFg.L -= step
		} else {
			labFg.L += step
		}
		labFg.L = math.Min(math.Max(labFg.L, 0.0), 100.0)
	}
	return rgbF{}, false
}
