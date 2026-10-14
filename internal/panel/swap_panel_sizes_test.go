package panel

import (
	"testing"

	"github.com/unxed/f4/internal/appcmd"
	"github.com/unxed/f4/internal/config"
)

// f4#1901: Ctrl+U swaps the panels together with their sizes, as in far2l:
// the wider panel stays wider on its new side, and the heights go along.
func TestSwapPanelsKeepsEachPanelsSize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	old := config.App
	t.Cleanup(func() { config.App = old })
	config.App.WidthDecrement = 10 // left 30 columns, right 50
	config.App.LeftHeightDecrement = 3
	config.App.RightHeightDecrement = 0

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	left, right := pf.Panels[0], pf.Panels[1]
	width := func(p Panel) int { x1, _, x2, _ := p.GetPosition(); return x2 - x1 + 1 }
	height := func(p Panel) int { _, y1, _, y2 := p.GetPosition(); return y2 - y1 + 1 }
	lw, rw, lh, rh := width(left), width(right), height(left), height(right)
	if lw >= rw || lh >= rh {
		t.Fatalf("before: left %dx%d, right %dx%d; want the left one narrower and lower", lw, lh, rw, rh)
	}

	pf.HandleCommand(appcmd.CmSwapPanels, nil)

	if pf.Panels[0] != right || pf.Panels[1] != left {
		t.Fatal("the panels were not swapped")
	}
	if width(left) != lw || width(right) != rw {
		t.Errorf("widths after Ctrl+U: old left %d (was %d), old right %d (was %d)", width(left), lw, width(right), rw)
	}
	if height(left) != lh || height(right) != rh {
		t.Errorf("heights after Ctrl+U: old left %d (was %d), old right %d (was %d)", height(left), lh, height(right), rh)
	}
	if config.App.WidthDecrement != -10 || config.App.LeftHeightDecrement != 0 || config.App.RightHeightDecrement != 3 {
		t.Errorf("saved sizes = %d, %d, %d; want -10, 0, 3", config.App.WidthDecrement, config.App.LeftHeightDecrement, config.App.RightHeightDecrement)
	}
}
