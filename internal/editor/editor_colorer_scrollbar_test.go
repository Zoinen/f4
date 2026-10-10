//go:build !lite

package editor

import (
	"testing"

	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// When Colorer draws the text on a background of its own, the scrollbar the
// theme puts on the text's background follows it rather than staying a
// stripe of the theme's background beside the text (#1232).
func TestEditor_ScrollbarFollowsColorerTextBackground(t *testing.T) {
	ev, _ := occurrenceEditor(t, "text\n")
	savedText, savedBar := vtui.Palette[theme.ColEditorText], vtui.Palette[theme.ColEditorScrollbar]
	t.Cleanup(func() {
		vtui.Palette[theme.ColEditorText], vtui.Palette[theme.ColEditorScrollbar] = savedText, savedBar
	})
	vtui.Palette[theme.ColEditorText] = vtui.SetRGBBoth(0, 0x34E2E2, 0x3465A4)
	vtui.Palette[theme.ColEditorScrollbar] = vtui.SetRGBBoth(0, 0x34E2E2, 0x3465A4)

	if _, bg := theme.GetColorRGBBoth(ev.ScrollBar.Attr()); bg != 0x3465A4 {
		t.Errorf("scrollbar on #%06x with the text on the theme's own background, want #3465a4", bg)
	}
	ev.Highlighter = &ColorerHighlighter{typeSettings: colorerTypeSettings{back: 0x101010, backSet: true}}
	if _, bg := theme.GetColorRGBBoth(ev.ScrollBar.Attr()); bg != 0x101010 {
		t.Errorf("scrollbar on #%06x with Colorer's text on #101010, want the text's background", bg)
	}
}
