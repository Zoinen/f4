package vtui

import (
	"math"
	"strings"
	"testing"
)

// TestValidateColors_FlagsLightYellowOnLightGray encodes the concrete
// regression named in f4#363: hotkey/highlight text rendered as a light
// yellow on a light gray background, as happened in far2l's default dark
// scheme. Pure WCAG contrast already catches this without any perceptual
// color-difference math: the pair sits at roughly 1.7:1, far below the
// 4.5:1 normal-text threshold.
func TestValidateColors_FlagsLightYellowOnLightGray(t *testing.T) {
	pairs := []ColorPair{
		{Name: "Dialog.HighlightText", FG: 0xFFFF00, BG: 0xC0C0C0},
	}
	errs := ValidateColors(pairs)
	if len(errs) == 0 {
		t.Fatal("expected the light-yellow-on-light-gray hotkey pair to be flagged, got no errors")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "insufficient contrast") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an insufficient-contrast error, got: %v", errs)
	}
}

// TestValidateColors_GoodSchemePasses makes sure the validator does not flag
// everything: plain, readable, unsaturated combinations must come back
// clean under the default rules.
func TestValidateColors_GoodSchemePasses(t *testing.T) {
	pairs := []ColorPair{
		{Name: "Dialog.Text", FG: 0x000000, BG: 0xC0C0C0}, // black on light gray
		{Name: "Panel.Text", FG: 0xD3D7CF, BG: 0x2E3436},  // light gray on dark gray
		{Name: "Editor.Text", FG: 0xFFFFFF, BG: 0x000080}, // white on navy
	}
	if errs := ValidateColors(pairs); len(errs) != 0 {
		t.Errorf("expected a well-behaved scheme to pass, got: %v", errs)
	}
}

// TestValidateColors_FlagsHarshSaturatedClash covers the axis pure luminance
// contrast misses entirely: two highly saturated colors of very different
// hue, "eye-gouging" even though their WCAG contrast ratio is comfortably
// above the readability threshold.
func TestValidateColors_FlagsHarshSaturatedClash(t *testing.T) {
	pairs := []ColorPair{
		// Saturated yellow on saturated blue: WCAG contrast is a healthy
		// ~8:1 (well above 4.5), so this pair must be flagged for its harsh
		// clash, not for insufficient contrast.
		{Name: "Test.HarshPair", FG: 0xFFFF00, BG: 0x0000FF},
	}
	errs := ValidateColors(pairs)
	if len(errs) == 0 {
		t.Fatal("expected the saturated yellow-on-blue pair to be flagged as a harsh clash")
	}
	for _, e := range errs {
		if strings.Contains(e.Error(), "insufficient contrast") {
			t.Errorf("pair has fine WCAG contrast, should not be flagged for that: %v", e)
		}
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "harsh color clash") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a harsh-color-clash error, got: %v", errs)
	}
}

// TestValidateColors_AllowsCyanOnNavyPanelText is the false-positive
// regression named in f4#363: bright cyan foreground on a navy background
// (f4's own default Panel.Text, Norton-Commander/far2l-style) is a
// comfortable, high-contrast, thoroughly ordinary terminal color choice.
// Both colors are "saturated" (C*=50 and C*=94) and their hues sit 110°
// apart, so a flat chroma+hue-delta check flags it — but the background's
// L*≈18 is well under HarshMinLightness, which is exactly what exempts it:
// this reads as light text on a dark panel, not two vivid fields clashing.
func TestValidateColors_AllowsCyanOnNavyPanelText(t *testing.T) {
	pairs := []ColorPair{
		{Name: "Panel.Text", FG: 0x00FFFF, BG: 0x0000A0},
	}
	if errs := ValidateColors(pairs); len(errs) != 0 {
		t.Errorf("expected cyan-on-navy panel text to pass, got: %v", errs)
	}
}

// TestValidateColors_AllowsRealClassicScheme runs the saturated FG/BG pairs
// actually shipped in f4's "Classic" style (SetDefaultF4Palette plus
// internal/theme/styles/classic.ini's overrides) through the harsh-clash
// check. Classic is f4's original built-in palette — bright accents over a
// navy panel throughout — and every one of these pairs both clears the
// chroma+hue-delta gates (so would be flagged without HarshMinLightness)
// and has a background around L*≈18, comfortably under the 31.5 floor.
// Panel.Title.Selected is the tightest real-world margin found for that
// floor: its background L* is ≈31, only ~0.8 under the cutoff.
//
// Like TestValidateColors_AllowsRealDefaultScheme, this only exercises the
// harsh-clash side (MinContrastRatio disabled): Panel.FastFindNoMatch sits a
// little under 4.5:1 WCAG contrast by design (it is a muted attention color
// over the panel background, not body text), which is an independent,
// unrelated, already-correct check that this fix does not touch.
func TestValidateColors_AllowsRealClassicScheme(t *testing.T) {
	rules := DefaultColorRules
	rules.MinContrastRatio = 0
	pairs := []ColorPair{
		{Name: "Panel.Text", FG: 0x00FFFF, BG: 0x0000A0},
		{Name: "Panel.Text.Selected", FG: 0xFFFF00, BG: 0x0000A0},
		{Name: "Panel.Title.Column", FG: 0xFFFF00, BG: 0x0000A0},
		{Name: "Panel.Title.Selected", FG: 0x00FFFF, BG: 0x3030C0},
		{Name: "Panel.Tabs.Accent", FG: 0xFFFF00, BG: 0x0000A0},
		{Name: "Panel.Tabs.Attention", FG: 0xFF8700, BG: 0x0000A0},
		{Name: "Panel.FastFindNoMatch", FG: 0xD75F5F, BG: 0x0000A0},
	}
	if errs := ValidateColorsWithRules(pairs, rules); len(errs) != 0 {
		t.Errorf("expected f4's shipped Classic theme to pass the harsh-clash check clean, got: %v", errs)
	}
}

// TestValidateColors_FlagsNearIsoluminantSaturatedClash is a second genuine
// harsh-clash regression, alongside TestValidateColors_FlagsHarshSaturatedClash,
// found while exercising f4's shipped "Default Dark" theme end-to-end
// (f4#363): its Menu.Highlight.Selected/HMenu.Highlight.Selected pairs put a
// saturated red directly on a saturated olive green, both around L*≈50 —
// genuinely near-isoluminant, unlike every false positive above — and at
// 1.7:1 WCAG contrast, poor enough that this would fail even with
// MinContrastRatio enabled. Both HarshMinLightness (neither color is dark)
// and plain contrast agree this one is a real problem, not a false
// positive of the chroma/hue check.
func TestValidateColors_FlagsNearIsoluminantSaturatedClash(t *testing.T) {
	pairs := []ColorPair{
		{Name: "Menu.Highlight.Selected", FG: 0xCC0000, BG: 0x4E9A06},
	}
	rules := DefaultColorRules
	rules.MinContrastRatio = 0 // isolate the clash check from the (also failing) contrast one
	errs := ValidateColorsWithRules(pairs, rules)
	if len(errs) == 0 {
		t.Fatal("expected the near-isoluminant red-on-olive pair to be flagged as a harsh clash")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Error(), "harsh color clash") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a harsh-color-clash error, got: %v", errs)
	}
}

// TestValidateColors_AllowsRealDefaultScheme runs every FG/BG pair actually
// shipped in f4's "Default Dark" theme (internal/theme/styles/default_dark.ini,
// itself ported from far2l) through the harsh-clash check. A validator that
// cannot run clean against a real, currently-shipped default scheme is not
// useful, so this is a broad regression net alongside the narrower
// cyan-on-navy case above.
//
// This only exercises the harsh-clash side (MinContrastRatio disabled): a
// few of this theme's decorative pairs (scrollbars, cursor highlights) sit
// below 4.5:1 WCAG contrast by design, which is an independent, unrelated,
// already-correct check that this fix does not touch.
func TestValidateColors_AllowsRealDefaultScheme(t *testing.T) {
	rules := DefaultColorRules
	rules.MinContrastRatio = 0
	pairs := []ColorPair{
		{Name: "Panel.Box", FG: 0x555753, BG: 0x2E3436},
		{Name: "Panel.Cursor", FG: 0x2E3436, BG: 0x06989A},
		{Name: "Panel.Cursor.Inactive", FG: 0xD3D7CF, BG: 0x555753},
		{Name: "Panel.Cursor.Inactive.Selected", FG: 0xFCE94F, BG: 0x555753},
		{Name: "Panel.Cursor.Selected", FG: 0xFCE94F, BG: 0x06989A},
		{Name: "Panel.Text", FG: 0x34E2E2, BG: 0x2E3436},
		{Name: "Panel.Text.Highlight", FG: 0xD3D7CF, BG: 0x3465A4},
		{Name: "Panel.Text.Info", FG: 0xFCE94F, BG: 0x2E3436},
		{Name: "Panel.Text.Selected", FG: 0xFCE94F, BG: 0x3F474A},
		{Name: "Dialog.Box", FG: 0x555753, BG: 0xD3D7CF},
		{Name: "Dialog.Combo.Box", FG: 0xEEEEEC, BG: 0x06989A},
		{Name: "Dialog.Combo.Highlight", FG: 0xFCE94F, BG: 0x06989A},
		{Name: "Dialog.Edit.Selected", FG: 0xFCE94F, BG: 0x4E9A06},
		{Name: "WarnDialog.Box", FG: 0xD3D7CF, BG: 0xCC0000},
		{Name: "WarnDialog.Box.Title.Highlight", FG: 0xFCE94F, BG: 0xCC0000},
		{Name: "Editor.Text", FG: 0x34E2E2, BG: 0x3465A4},
		{Name: "Editor.WrapMark", FG: 0xFCE94F, BG: 0x3465A4},
		{Name: "Editor.Occurrence", FG: 0x2E3436, BG: 0xFCE94F},
		{Name: "Editor.Syntax.String", FG: 0x8AE234, BG: 0x3465A4},
		{Name: "Viewer.Text.Selected", FG: 0x2E3436, BG: 0xFCE94F},
		{Name: "Keybar.Text", FG: 0x2E3436, BG: 0x06989A},
		{Name: "Help.Box", FG: 0x2E3436, BG: 0x06989A},
	}
	if errs := ValidateColorsWithRules(pairs, rules); len(errs) != 0 {
		t.Errorf("expected f4's shipped Default Dark theme to pass the harsh-clash check clean, got: %v", errs)
	}
}

// TestValidateColorsWithRules_ThresholdsAreHonored spot-checks that both
// checks can be independently disabled and that the contrast threshold is
// configurable, the way LayoutRules' fields work for the layout validator.
func TestValidateColorsWithRules_ThresholdsAreHonored(t *testing.T) {
	pairs := []ColorPair{{Name: "p", FG: 0xFFFF00, BG: 0xC0C0C0}}

	// Disabling the contrast check entirely must silence the regression case.
	rules := DefaultColorRules
	rules.MinContrastRatio = 0
	if errs := ValidateColorsWithRules(pairs, rules); len(errs) != 0 {
		t.Errorf("MinContrastRatio = 0 should disable the contrast check, got: %v", errs)
	}

	// A lax enough ratio must let a mediocre pair through.
	rules = DefaultColorRules
	rules.MinContrastRatio = 1.0
	if errs := ValidateColorsWithRules(pairs, rules); len(errs) != 0 {
		t.Errorf("a 1.0:1 minimum should accept every pair, got: %v", errs)
	}
}

// TestValidateColorsWithRules_MinLightnessExemptsClash spot-checks
// HarshMinLightness directly, using the tightest real margin found for it:
// Classic's Panel.Title.Selected (background L*≈31) must pass under
// DefaultColorRules, and the existing yellow-on-blue clash (darker color
// L*≈32) must keep flagging, one point higher. Disabling the exemption (0)
// or raising the floor past that darker color's own L* must flag or exempt
// each one respectively, showing the field genuinely gates on lightness
// rather than on anything specific to one color pair.
func TestValidateColorsWithRules_MinLightnessExemptsClash(t *testing.T) {
	titleSelected := []ColorPair{{Name: "Panel.Title.Selected", FG: 0x00FFFF, BG: 0x3030C0}}
	yellowOnBlue := []ColorPair{{Name: "Test.HarshPair", FG: 0xFFFF00, BG: 0x0000FF}}

	if errs := ValidateColorsWithRules(titleSelected, DefaultColorRules); len(errs) != 0 {
		t.Errorf("Panel.Title.Selected should pass under DefaultColorRules, got: %v", errs)
	}
	if errs := ValidateColorsWithRules(yellowOnBlue, DefaultColorRules); len(errs) == 0 {
		t.Error("yellow-on-blue should still be flagged under DefaultColorRules")
	}

	// Disabling the exemption (0) must flag Panel.Title.Selected too: it
	// clears the same chroma and hue-delta gates as the genuine clash, it
	// is only the lightness floor that tells them apart.
	rules := DefaultColorRules
	rules.HarshMinLightness = 0
	if errs := ValidateColorsWithRules(titleSelected, rules); len(errs) == 0 {
		t.Error("HarshMinLightness = 0 should disable the exemption and flag Panel.Title.Selected")
	}

	// A floor above yellow-on-blue's own darker color (L*≈32) must exempt
	// it too, showing the field genuinely gates on lightness rather than on
	// anything specific to Panel.Title.Selected.
	rules = DefaultColorRules
	rules.HarshMinLightness = 40
	if errs := ValidateColorsWithRules(yellowOnBlue, rules); len(errs) != 0 {
		t.Errorf("a 40-point floor should exempt yellow-on-blue (its darker color is L*≈32), got: %v", errs)
	}
}

// TestPaletteColorPairs_SkipsUnsetSlotsAndNamesByIndex exercises the
// adapter between a raw vtui palette (as used by Palette/SetRGBBoth) and the
// pair-based validator API.
func TestPaletteColorPairs_SkipsUnsetSlotsAndNamesByIndex(t *testing.T) {
	palette := make([]uint64, 3)
	palette[0] = 0 // unset: must be skipped
	palette[1] = SetRGBBoth(0, 0x123456, 0x654321)
	palette[2] = SetIndexBoth(0, 1, 2) // resolved via ThemePalette

	names := []string{"Slot0", "Slot1"} // shorter than the palette: index 2 falls back

	pairs := PaletteColorPairs(palette, names)
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs (slot 0 skipped), got %d: %+v", len(pairs), pairs)
	}

	if pairs[0].Name != "Slot1" || pairs[0].FG != 0x123456 || pairs[0].BG != 0x654321 {
		t.Errorf("unexpected RGB pair: %+v", pairs[0])
	}

	wantFG, wantBG := ThemePalette[1], ThemePalette[2]
	if pairs[1].Name != "palette[2]" || pairs[1].FG != wantFG || pairs[1].BG != wantBG {
		t.Errorf("unexpected indexed pair: %+v (want fg=#%06x bg=#%06x)", pairs[1], wantFG, wantBG)
	}
}

// TestAssertColors_ReportsThroughErrorfInterface makes sure the test helper
// mirrors AssertLayout: it must call Errorf exactly when there are errors.
func TestAssertColors_ReportsThroughErrorfInterface(t *testing.T) {
	var calls int
	fake := &fakeT{errorf: func(string, ...any) { calls++ }}

	AssertColors(fake, []ColorPair{{Name: "ok", FG: 0x000000, BG: 0xFFFFFF}})
	if calls != 0 {
		t.Errorf("a passing pair must not call Errorf, got %d calls", calls)
	}

	AssertColors(fake, []ColorPair{{Name: "bad", FG: 0xFFFF00, BG: 0xC0C0C0}})
	if calls != 1 {
		t.Errorf("a failing pair must call Errorf once, got %d calls", calls)
	}
}

type fakeT struct {
	errorf func(string, ...any)
}

func (f *fakeT) Errorf(format string, args ...any) { f.errorf(format, args...) }

// --- Color math sanity checks -------------------------------------------

func TestContrastRatioRGB_KnownValues(t *testing.T) {
	if got := contrastRatioRGB(0xFFFFFF, 0x000000); math.Abs(got-21.0) > 0.05 {
		t.Errorf("white/black contrast = %.3f, want ~21", got)
	}
	if got := contrastRatioRGB(0x808080, 0x808080); math.Abs(got-1.0) > 1e-9 {
		t.Errorf("identical color contrast = %.3f, want 1", got)
	}
}

func TestRgbToLab_KnownValues(t *testing.T) {
	cases := []struct {
		rgb     uint32
		l, a, b float64
	}{
		{0xFFFFFF, 100.0, 0.0, 0.0},
		{0x000000, 0.0, 0.0, 0.0},
		{0xFF0000, 53.24, 80.09, 67.20},
	}
	for _, tc := range cases {
		l, a, b := rgbToLab(tc.rgb)
		if math.Abs(l-tc.l) > 0.1 || math.Abs(a-tc.a) > 0.1 || math.Abs(b-tc.b) > 0.1 {
			t.Errorf("#%06x -> Lab(%.2f, %.2f, %.2f), want (%.2f, %.2f, %.2f)", tc.rgb, l, a, b, tc.l, tc.a, tc.b)
		}
	}
}

func TestHueDeltaDeg_WrapsAroundCorrectly(t *testing.T) {
	// Two points near the +a axis and -a axis are 180 degrees apart either
	// way you measure, never reported as 0 or as more than 180.
	if got := hueDeltaDeg(10, 1, -10, -1); math.Abs(got-180.0) > 1.0 {
		t.Errorf("opposite hues = %.1f degrees, want ~180", got)
	}
	if got := hueDeltaDeg(10, 0, 10, 0); got != 0 {
		t.Errorf("identical hues = %.1f degrees, want 0", got)
	}
}
