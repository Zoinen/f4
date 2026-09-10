package theme

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cursorColorTestPalette restores the palette and the value handed to vtui, so
// one test cannot leave a cursor color behind for the next one.
func cursorColorTestPalette(t *testing.T) {
	t.Helper()
	old := vtui.CursorColor
	t.Cleanup(func() { vtui.CursorColor = old })
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
}

func TestCursorColor_PublishedToVtui(t *testing.T) {
	cursorColorTestPalette(t)

	FinishColors()
	if vtui.CursorColor != 0xFFFFFF {
		t.Errorf("default cursor color handed to vtui is #%06X, want #FFFFFF", vtui.CursorColor)
	}
}

func TestCursorColor_ComesFromFarcolorsIni(t *testing.T) {
	cursorColorTestPalette(t)

	InitColors(ini.Parse(strings.NewReader(`[farcolors]
Terminal.Cursor = foreground:#ff00ff
`)))
	if vtui.CursorColor != 0xFF00FF {
		t.Errorf("cursor color handed to vtui is #%06X, want #FF00FF", vtui.CursorColor)
	}
}

// The caret travels across every background in the palette, so the pass that
// corrects a foreground against its own background has nothing to work with
// here and must leave the slot alone.
func TestCursorColor_SurvivesContrastCorrection(t *testing.T) {
	cursorColorTestPalette(t)

	oldCfg := config.App
	config.App.EnforceColorCorrection = true
	t.Cleanup(func() { config.App = oldCfg })

	// Foreground and background deliberately identical: any correction pass
	// that looked at this pair would move the foreground.
	InitColors(ini.Parse(strings.NewReader(`[farcolors]
Terminal.Cursor = foreground:#808080 | background:#808080
`)))
	if vtui.CursorColor != 0x808080 {
		t.Errorf("cursor color was corrected to #%06X, want the authored #808080", vtui.CursorColor)
	}
}

func TestCursorColor_ExportedAsAScheme(t *testing.T) {
	cursorColorTestPalette(t)

	InitColors(ini.Parse(strings.NewReader(`[farcolors]
Terminal.Cursor = foreground:#00ff00
`)))

	path := filepath.Join(t.TempDir(), "exported.ini")
	if err := ExportColors(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := ini.Load(path).GetString("farcolors", "Terminal.Cursor", ""); got == "" {
		t.Fatalf("exported scheme has no Terminal.Cursor key:\n%s", data)
	}
}

// The seam that actually matters: a finished palette has to reach the terminal
// as OSC 12, without anyone calling the sequence by hand.
func TestCursorColor_ReachesTheTerminalStream(t *testing.T) {
	cursorColorTestPalette(t)

	oldManage := vtui.ManageCursorStyle
	vtui.ManageCursorStyle = true
	t.Cleanup(func() { vtui.ManageCursorStyle = oldManage })

	scr := vtui.NewScreenBuf()
	renderer, ok := scr.Renderer.(*vtui.AnsiRenderer)
	if !ok {
		t.Skipf("default renderer is %T, not the ANSI one", scr.Renderer)
	}
	var out strings.Builder
	scr.Writer = &out

	InitColors(ini.Parse(strings.NewReader(`[farcolors]
Terminal.Cursor = foreground:#00ffff
`)))
	renderer.Flush()

	if want := "\x1b]12;#00FFFF\x07"; !strings.Contains(out.String(), want) {
		t.Errorf("the frame does not carry %q, got %q", want, out.String())
	}
}
