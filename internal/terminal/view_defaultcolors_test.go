package terminal

import (
	"testing"

	"github.com/unxed/vtui"
)

func TestHostDefaultColorsKeepsExplicitColours(t *testing.T) {
	def := hostDefaultColors(DefaultTermAttr)
	if def&vtui.ForegroundDefault == 0 || def&vtui.BackgroundDefault == 0 {
		t.Fatalf("default colours not turned into terminal defaults: %#x", def)
	}
	red := vtui.SetIndexBoth(0, 1, 4)
	if got := hostDefaultColors(red); got != red {
		t.Errorf("explicit index colours changed: %#x -> %#x", red, got)
	}
	rgb := vtui.SetRGBBoth(0, 0x102030, 0x000000)
	if got := hostDefaultColors(rgb); got != rgb {
		t.Errorf("explicit RGB colours (black among them) changed: %#x -> %#x", rgb, got)
	}
	// Only the side left at its default is turned over.
	half := hostDefaultColors(vtui.SetIndexBack(DefaultTermAttr, 2))
	if half&vtui.ForegroundDefault == 0 || half&vtui.BackgroundDefault != 0 {
		t.Errorf("half-default attr = %#x", half)
	}
}

func TestShowDrawsDefaultColoursOnlyWhenAsked(t *testing.T) {
	tv := NewTerminalView(10, 3)
	tv.SetPosition(0, 0, 9, 2)
	tv.PutChar('x', DefaultTermAttr)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(10, 3)

	tv.Show(scr)
	if got := scr.GetCell(0, 0).Attributes; got&(vtui.ForegroundDefault|vtui.BackgroundDefault) != 0 {
		t.Errorf("default colours drawn as terminal defaults without being asked: %#x", got)
	}

	tv.DefaultColors = true
	tv.Show(scr)
	for _, at := range [][2]int{{0, 0}, {5, 2}} {
		got := scr.GetCell(at[0], at[1]).Attributes
		if got&vtui.ForegroundDefault == 0 || got&vtui.BackgroundDefault == 0 {
			t.Errorf("cell %v = %#x, want the terminal's default colours", at, got)
		}
	}
}
