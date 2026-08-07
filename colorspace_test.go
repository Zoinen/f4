package main

import (
	"math"
	"testing"
)

// Values taken from far2l's own algorithm. If the port drifts, these move.
func TestComputeContrast_MatchesFar2l(t *testing.T) {
	cases := []struct {
		name   string
		fg, bg uint32
		want   uint32
	}{
		// Keybar and horizontal menu in default dark: dark grey on teal. far2l
		// leaves this alone; the old WCAG-only approximation turned it white.
		{"keybar label", 0x2E3436, 0x06989A, 0x2E3436},
		// Panel text in far2l's stock palette.
		{"panel text", 0x34E2E2, 0x3465A4, 0x34E2E2},
		// Yellow highlight on a light grey dialog: WCAG says 1.2:1, far2l keeps it.
		{"dialog highlight", 0xFCE94F, 0xD3D7CF, 0xFBE94F},
		// Genuinely too close: the inactive prompt gets lifted.
		{"inactive prompt", 0x555753, 0x2E3436, 0x848682},
		// A dim scrollbar over a dark background.
		{"dim scrollbar", 0x5A5A5A, 0x232323, 0x7A7A7A},
		// Nothing to do at either extreme.
		{"white on black", 0xFFFFFF, 0x000000, 0xFFFFFF},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CorrectContrast(tc.fg, tc.bg); got != tc.want {
				t.Errorf("#%06x on #%06x -> #%06x, want #%06x", tc.fg, tc.bg, got, tc.want)
			}
		})
	}
}

// Packing a colour to 8 bits and unpacking it must be lossless, otherwise
// every pair the algorithm leaves alone would drift by a unit on each call.
func TestRGBRoundTripIsLossless(t *testing.T) {
	for c := uint32(0); c < 256; c++ {
		packed := c<<16 | c<<8 | c
		if got := toRGB24(toRGBF(packed)); got != packed {
			t.Fatalf("#%06x round-tripped to #%06x", packed, got)
		}
	}
}

// Correcting an already-corrected pair must not keep pushing it.
func TestComputeContrast_Idempotent(t *testing.T) {
	fg, bg := uint32(0x555753), uint32(0x2E3436)
	once := CorrectContrast(fg, bg)
	twice := CorrectContrast(once, bg)

	for shift := 0; shift < 24; shift += 8 {
		diff := int((twice>>shift)&0xFF) - int((once>>shift)&0xFF)
		if diff > 1 || diff < -1 {
			t.Fatalf("second pass moved #%06x to #%06x", once, twice)
		}
	}
}

// Spot-check the colour space conversions against published values so a typo
// in the matrices surfaces here rather than as a vague colour shift.
func TestRGBToLAB_KnownValues(t *testing.T) {
	cases := []struct {
		rgb     uint32
		L, A, B float64
	}{
		{0xFFFFFF, 100.0, 0.0, 0.0},
		{0x000000, 0.0, 0.0, 0.0},
		{0xFF0000, 53.24, 80.09, 67.20},
		{0x0000FF, 32.30, 79.19, -107.86},
	}
	for _, tc := range cases {
		got := rgbToLAB(toRGBF(tc.rgb))
		if math.Abs(got.L-tc.L) > 0.1 || math.Abs(got.A-tc.A) > 0.1 || math.Abs(got.B-tc.B) > 0.1 {
			t.Errorf("#%06x -> L*a*b* (%.2f, %.2f, %.2f), want (%.2f, %.2f, %.2f)",
				tc.rgb, got.L, got.A, got.B, tc.L, tc.A, tc.B)
		}
	}
}

func TestLABtoRGB_RoundTrip(t *testing.T) {
	for _, c := range []uint32{0x000000, 0xFFFFFF, 0x2E3436, 0x34E2E2, 0xFCE94F, 0xCC0000} {
		back := toRGB24(labToRGB(rgbToLAB(toRGBF(c))))
		for shift := 0; shift < 24; shift += 8 {
			diff := int((back>>shift)&0xFF) - int((c>>shift)&0xFF)
			if diff > 1 || diff < -1 {
				t.Errorf("#%06x round-tripped to #%06x", c, back)
				break
			}
		}
	}
}

func TestDeltaE2000_KnownPairs(t *testing.T) {
	// Identical colours are zero apart; black and white are the far extreme.
	if d := deltaE2000(rgbToLAB(toRGBF(0x808080)), rgbToLAB(toRGBF(0x808080))); d != 0 {
		t.Errorf("deltaE2000 of a colour with itself = %.4f, want 0", d)
	}
	if d := deltaE2000(rgbToLAB(toRGBF(0x000000)), rgbToLAB(toRGBF(0xFFFFFF))); math.Abs(d-100.0) > 0.1 {
		t.Errorf("deltaE2000 black/white = %.2f, want ~100", d)
	}
}
