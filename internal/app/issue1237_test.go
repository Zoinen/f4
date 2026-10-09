package app

import (
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

// TestIssue1237GrabberFreezesTheScreenAsSeen is the regression test for issue
// #1237: Alt+Ins over the folder/command/editor history dialogs dropped their
// bottom-border hint and third column and shifted the text by a column. Those
// dialogs finish painting in FrameManager.OnRender, which runs after every
// frame's Show, and only while the dialog is the top frame. The grabber used
// to snapshot during its own first Show, i.e. the frames beneath it repainted
// without that layer, so the frozen picture was not the one the user saw.
func TestIssue1237GrabberFreezesTheScreenAsSeen(t *testing.T) {
	scr := setupGrabberScreen(t)
	attr := vtui.SetRGBBoth(0, 0xFFFFFF, 0x000000)

	OpenGrabber()
	g, ok := vtui.FrameManager.GetTopFrame().(*GrabberFrame)
	if !ok {
		t.Fatalf("top frame is %T, want *GrabberFrame", vtui.FrameManager.GetTopFrame())
	}

	// The next render pass: the frames under the grabber repaint without the
	// overlay layer, then the grabber is shown on top of them.
	scr.FillRect(0, 0, testGrabberW-1, testGrabberH-1, ' ', attr)
	g.Show(scr)

	if got := testutil.Rune(g.snap[0][0].Char); got != 'h' {
		t.Errorf("frozen row 0 starts with %q, want the text 'h' shown when Alt+Ins was pressed", got)
	}
	if got := testutil.Rune(scr.GetCell(0, 0).Char); got != 'h' {
		t.Errorf("screen row 0 starts with %q while grabbing, want 'h'", got)
	}
}
