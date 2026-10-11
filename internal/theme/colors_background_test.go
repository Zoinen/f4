package theme

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// An element the theme puts on the text's background follows the background
// the text is actually drawn on; one the theme sets apart stays as it is, and
// nothing changes while the text is on the theme's own background (#1232).
func TestOnTextBackground(t *testing.T) {
	SetDefaultF4Palette()
	savedText, savedBar := vtui.Palette[ColEditorText], vtui.Palette[ColEditorScrollbar]
	savedCorrection := config.App.EnforceColorCorrection
	t.Cleanup(func() {
		vtui.Palette[ColEditorText], vtui.Palette[ColEditorScrollbar] = savedText, savedBar
		config.App.EnforceColorCorrection = savedCorrection
	})
	config.App.EnforceColorCorrection = false

	vtui.Palette[ColEditorText] = vtui.SetRGBBoth(0, 0x34E2E2, 0x3465A4)
	vtui.Palette[ColEditorScrollbar] = vtui.SetRGBBoth(0, 0x34E2E2, 0x3465A4)
	colorerText := vtui.SetRGBBoth(0, 0xC0C0C0, 0x000000)

	if got := OnTextBackground(ColEditorScrollbar, ColEditorText, vtui.Palette[ColEditorText]); got != vtui.Palette[ColEditorScrollbar] {
		t.Errorf("on the theme's own text background the bar became %#x", got)
	}
	if fg, bg := GetColorRGBBoth(OnTextBackground(ColEditorScrollbar, ColEditorText, colorerText)); fg != 0x34E2E2 || bg != 0x000000 {
		t.Errorf("bar is #%06x on #%06x, want its own #34e2e2 on the text's #000000", fg, bg)
	}

	vtui.Palette[ColEditorScrollbar] = vtui.SetRGBBoth(0, 0x34E2E2, 0x06989A)
	if got := OnTextBackground(ColEditorScrollbar, ColEditorText, colorerText); got != vtui.Palette[ColEditorScrollbar] {
		t.Errorf("a bar the theme sets apart from the text became %#x", got)
	}
}

// Moved onto a background it was never corrected for, a foreground is
// corrected for the new pair when contrast correction is on.
func TestOnBackgroundOf_CorrectsContrastForTheNewBackground(t *testing.T) {
	saved := config.App.EnforceColorCorrection
	t.Cleanup(func() { config.App.EnforceColorCorrection = saved })
	attr := vtui.SetRGBBoth(0, 0x000000, 0x3465A4)
	onBlack := vtui.SetRGBBoth(0, 0xC0C0C0, 0x000000)

	config.App.EnforceColorCorrection = false
	if fg, bg := GetColorRGBBoth(OnBackgroundOf(attr, onBlack)); fg != 0x000000 || bg != 0x000000 {
		t.Errorf("without correction got #%06x on #%06x, want #000000 on #000000", fg, bg)
	}
	config.App.EnforceColorCorrection = true
	if fg, bg := GetColorRGBBoth(OnBackgroundOf(attr, onBlack)); fg == 0x000000 || bg != 0x000000 {
		t.Errorf("with correction got #%06x on #%06x, want a corrected foreground on #000000", fg, bg)
	}
}
