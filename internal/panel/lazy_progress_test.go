package panel

import (
	"testing"

	"github.com/unxed/vtui"
)

// The progress window of a task that only has a status line must not draw an
// empty bar: ScreenObject.Show would switch a SetVisible(false) back on at
// every draw, so the bar itself skips drawing until it has a percentage
// (f4#1411, reported again on fb112e4).
func TestLazyProgressBarDrawsOnlyOnceShown(t *testing.T) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(20, 3)
	bar := &lazyProgressBar{ProgressBar: vtui.NewProgressBar(0, 1, 10)}

	bar.Show(scr)
	bar.DisplayObject(scr)
	for x := 0; x < 10; x++ {
		if c := scr.GetCell(x, 1); c.Char == '░' || c.Char == '█' {
			t.Fatalf("a hidden bar drew %q at column %d", rune(c.Char), x) // #nosec G115 -- a glyph
		}
	}

	bar.shown = true
	bar.SetPercent(50)
	bar.Show(scr)
	if c := scr.GetCell(0, 1); c.Char != '█' {
		t.Fatalf("a shown bar at 50%% has %q in its first cell, want a filled block", rune(c.Char)) // #nosec G115 -- a glyph
	}
	if c := scr.GetCell(9, 1); c.Char != '░' {
		t.Fatalf("a shown bar at 50%% has %q in its last cell, want the empty shade", rune(c.Char)) // #nosec G115 -- a glyph
	}
}
