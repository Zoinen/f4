package main

import (
	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtui"
)

// TopBar is a generic top status bar used by Editor and Viewer.
type TopBar struct {
	vtui.Bar
	GetLeft  func() string
	GetRight func() string
	ColorIdx int

	// GetAttr, when set, chooses the colour of the bar. A zero answer means
	// the palette decides, so a frame only has to say something when it has
	// something to say. It is how the picture viewer shows that the file on
	// screen is selected.
	GetAttr func() uint64
}

func NewTopBar(getLeft, getRight func() string) *TopBar {
	return &TopBar{GetLeft: getLeft, GetRight: getRight, ColorIdx: ColViewerStatus}
}

func (tb *TopBar) Show(scr *vtui.ScreenBuf) {
	tb.Bar.Show(scr)
	if !tb.IsVisible() {
		return
	}
	attr := vtui.Palette[tb.ColorIdx]
	if tb.GetAttr != nil {
		if a := tb.GetAttr(); a != 0 {
			attr = a
		}
	}
	tb.DrawBackground(scr, attr)

	leftStr := ""
	if tb.GetLeft != nil {
		leftStr = tb.GetLeft()
	}
	rightStr := ""
	if tb.GetRight != nil {
		rightStr = tb.GetRight()
	}

	width := tb.X2 - tb.X1 + 1
	leftW := runewidth.StringWidth(leftStr)
	rightW := runewidth.StringWidth(rightStr)

	if leftW+rightW > width {
		if width > rightW+1 {
			leftStr = runewidth.Truncate(leftStr, width-rightW-1, "…")
		} else {
			leftStr = ""
			rightStr = runewidth.Truncate(rightStr, width, "…")
		}
	}

	if leftStr != "" {
		scr.Write(tb.X1, tb.Y1, vtui.StringToCharInfo(leftStr, attr))
	}
	if rightStr != "" {
		scr.Write(tb.X2-runewidth.StringWidth(rightStr)+1, tb.Y1, vtui.StringToCharInfo(rightStr, attr))
	}
}
