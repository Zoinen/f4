package vtui

import (
	"strings"
	"testing"
)

func TestHelpResizeKeepsCaptureOverLinks(t *testing.T) {
	fm := gestureManager(t)
	engine := NewHelpEngine(&mockHelpVFS{})
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "~" + strings.Repeat("Link", 16) + "~next@"
	}
	engine.AddTopic(&HelpTopic{Name: "resize", Lines: lines})
	help := NewHelpView(engine, "resize")
	fm.Push(help)
	help.Show(fm.scr)
	selected := help.selectedIdx
	x2, y2 := help.X2, help.Y2
	fm.dispatchEvent(gestureEvent(x2, y2, true, 1, false), false)
	for step := 1; step <= 5; step++ {
		x, y := x2-12-step, y2-step
		fm.dispatchEvent(gestureEvent(x, y, true, 1, true), false)
		if help.X2 != x || help.Y2 != y {
			t.Fatalf("held shrink step %d over help link ignored: %d/%d, want %d/%d",
				step, help.X2, help.Y2, x, y)
		}
		help.Show(fm.scr)
	}
	if help.selectedIdx != selected {
		t.Fatal("resize movement selected a help link")
	}
	// The declared minimum clamps the size, but reversing stays responsive.
	fm.dispatchEvent(gestureEvent(help.X1+2, help.Y1+2, true, 1, true), false)
	if help.X2-help.X1+1 != 20 || help.Y2-help.Y1+1 != 5 {
		t.Fatal("help did not shrink to its usable minimum")
	}
	fm.dispatchEvent(gestureEvent(help.X1+30, help.Y1+10, true, 1, true), false)
	if help.X2-help.X1 != 30 || help.Y2-help.Y1 != 10 {
		t.Fatal("help failed to expand while the resize button remained held")
	}
	fm.dispatchEvent(gestureEvent(help.X2-1, help.Y2-1, false, 0, false), false)
	if help.IsMouseCaptured() || fm.capturedFrame != nil {
		t.Fatal("help resize capture survived release")
	}
}
