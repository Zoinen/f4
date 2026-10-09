package theme

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtui"
)

// A far2l theme file authored before a color slot existed simply has no row
// for it. Such a slot must not keep leaking whatever the palette happened to
// hold before the theme was applied — it should default to sharing its
// parent element's background instead (f4#1232: this is what made an
// imported theme's Viewer.Scrollbar/Viewer.Arrows/Editor.Scrollbar show a
// stray unrelated color instead of blending in).
func TestApplyColorIni_MissingSlotInheritsParentBackground(t *testing.T) {
	saved := append([]uint64(nil), vtui.Palette...)
	t.Cleanup(func() { vtui.Palette = saved })

	// A minimal theme that only ever heard of Viewer.Text and Editor.Text,
	// not the newer Viewer.Scrollbar / Viewer.Arrows / Editor.Scrollbar.
	iniPath := filepath.Join(t.TempDir(), "old-imported-theme.ini")
	body := "[farcolors]\n" +
		"Viewer.Text = foreground:#ffffff | background:#123456\n" +
		"Editor.Text = foreground:#ffffff | background:#654321\n"
	if err := os.WriteFile(iniPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	// Seed the palette with an unrelated stock color first, the way a base
	// style is applied before a user theme is layered on top, so a bug that
	// simply leaves the slot untouched would be caught.
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	vtui.Palette[ColViewerScrollbar] = vtui.SetRGBBoth(0, 0xFFFF00, 0x0000A0)
	vtui.Palette[ColViewerArrows] = vtui.SetRGBBoth(0, 0xFFFF00, 0x0000A0)
	vtui.Palette[ColEditorScrollbar] = vtui.SetRGBBoth(0, 0x808080, 0x0000A0)
	vtui.Palette[ColEditorWrapMark] = vtui.SetRGBBoth(0, 0xFFFF00, 0x0000A0)

	InitColors(ini.Load(iniPath))

	_, viewerBg := GetColorRGBBoth(vtui.Palette[ColViewerText])
	_, editorBg := GetColorRGBBoth(vtui.Palette[ColEditorText])

	for name, index := range map[string]int{
		"Viewer.Scrollbar": ColViewerScrollbar,
		"Viewer.Arrows":    ColViewerArrows,
	} {
		if _, bg := GetColorRGBBoth(vtui.Palette[index]); bg != viewerBg {
			t.Errorf("%s = #%06x, want the viewer text background #%06x", name, bg, viewerBg)
		}
	}
	if _, bg := GetColorRGBBoth(vtui.Palette[ColEditorScrollbar]); bg != editorBg {
		t.Errorf("Editor.Scrollbar = #%06x, want the editor text background #%06x", bg, editorBg)
	}
	// The "»" that ends a wrapped row is drawn on the editor's text, so it
	// must not show the default's blue box either (f4#1232, reported again on
	// 1c2e8c8).
	if _, bg := GetColorRGBBoth(vtui.Palette[ColEditorWrapMark]); bg != editorBg {
		t.Errorf("Editor.WrapMark = #%06x, want the editor text background #%06x", bg, editorBg)
	}
}

// A theme that DOES set the slot must keep working exactly as before: the
// inheritance fallback only fills gaps, it never overrides an explicit value.
func TestApplyColorIni_ExplicitSlotIsNotOverriddenByInheritance(t *testing.T) {
	saved := append([]uint64(nil), vtui.Palette...)
	t.Cleanup(func() { vtui.Palette = saved })

	iniPath := filepath.Join(t.TempDir(), "explicit-theme.ini")
	body := "[farcolors]\n" +
		"Viewer.Text = foreground:#ffffff | background:#123456\n" +
		"Viewer.Scrollbar = foreground:#808080 | background:#abcdef\n"
	if err := os.WriteFile(iniPath, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	InitColors(ini.Load(iniPath))

	if _, bg := GetColorRGBBoth(vtui.Palette[ColViewerScrollbar]); bg != 0xABCDEF {
		t.Errorf("Viewer.Scrollbar = #%06x, want the theme's own #abcdef, inheritance must not override it", bg)
	}
}

// The colour of selected text in the editor (f4#234) was always that of a
// selection in a dialog's edit line. A theme written before the slot existed
// has no row for it and must look as before; a theme with the row sets its own.
func TestApplyColorIni_EditorSelectedTextFollowsDialogEditSelectedUntilSet(t *testing.T) {
	saved := append([]uint64(nil), vtui.Palette...)
	t.Cleanup(func() { vtui.Palette = saved })

	write := func(body string) *ini.File {
		path := filepath.Join(t.TempDir(), "theme.ini")
		if err := os.WriteFile(path, []byte("[farcolors]\n"+body), 0o600); err != nil {
			t.Fatal(err)
		}
		return ini.Load(path)
	}

	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	vtui.Palette[ColEditorSelectedText] = vtui.SetRGBBoth(0, 0x010203, 0x040506)
	InitColors(write("Dialog.Edit.Selected = foreground:#ffffff | background:#000080\n"))
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColEditorSelectedText]); fg != 0xffffff || bg != 0x000080 {
		t.Errorf("without a row the editor selection = #%06x on #%06x, want that of Dialog.Edit.Selected #ffffff on #000080", fg, bg)
	}

	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	InitColors(write("Dialog.Edit.Selected = foreground:#ffffff | background:#000080\n" +
		"Editor.Text.Selected = foreground:#000000 | background:#ffff00\n"))
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColEditorSelectedText]); fg != 0x000000 || bg != 0xffff00 {
		t.Errorf("with a row the editor selection = #%06x on #%06x, want #000000 on #ffff00", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[vtui.ColDialogEditSelected]); fg != 0xffffff || bg != 0x000080 {
		t.Errorf("Dialog.Edit.Selected changed to #%06x on #%06x with the editor row", fg, bg)
	}
}

// The prompt of the command line (f4#234): the path follows CommandLine.Prefix
// and the user@host part keeps its green on Prefix's background, until a theme
// has rows for them.
func TestApplyColorIni_CommandLinePromptSlotsFollowPrefixUntilSet(t *testing.T) {
	saved := append([]uint64(nil), vtui.Palette...)
	t.Cleanup(func() { vtui.Palette = saved })

	load := func(body string) {
		path := filepath.Join(t.TempDir(), "theme.ini")
		if err := os.WriteFile(path, []byte("[farcolors]\n"+body), 0o600); err != nil {
			t.Fatal(err)
		}
		vtui.SetDefaultPalette()
		SetDefaultF4Palette()
		InitColors(ini.Load(path))
	}

	load("CommandLine.Prefix = foreground:#ffff00 | background:#000080\n")
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColCommandLinePath]); fg != 0xffff00 || bg != 0x000080 {
		t.Errorf("CommandLine.Path without a row = #%06x on #%06x, want that of Prefix #ffff00 on #000080", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColCommandLineUser]); fg != 0x8ae234 || bg != 0x000080 {
		t.Errorf("CommandLine.User without a row = #%06x on #%06x, want the green #8ae234 on Prefix's #000080", fg, bg)
	}

	load("CommandLine.Prefix = foreground:#ffff00 | background:#000080\n" +
		"CommandLine.Path = foreground:#00ffff | background:#000000\n" +
		"CommandLine.User = foreground:#ff00ff | background:#000000\n")
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColCommandLinePath]); fg != 0x00ffff || bg != 0x000000 {
		t.Errorf("CommandLine.Path with a row = #%06x on #%06x, want #00ffff on #000000", fg, bg)
	}
	if fg, bg := GetColorRGBBoth(vtui.Palette[ColCommandLineUser]); fg != 0xff00ff || bg != 0x000000 {
		t.Errorf("CommandLine.User with a row = #%06x on #%06x, want #ff00ff on #000000", fg, bg)
	}
}
