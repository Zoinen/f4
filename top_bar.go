package main

import (
	"github.com/unxed/vtui"
)

// TopBar is a generic top status bar used by Editor and Viewer.
type TopBar struct {
	vtui.Bar
	GetValue func() string

	// GetAttr, when set, chooses the colour of the bar. A zero answer means
	// the palette decides, so a frame only has to say something when it has
	// something to say. It is how the picture viewer shows that the file on
	// screen is selected.
	GetAttr func() uint64
}

func NewTopBar(cb func() string) *TopBar {
	return &TopBar{GetValue: cb}
}

func (tb *TopBar) Show(scr *vtui.ScreenBuf) {
	tb.Bar.Show(scr)
	if !tb.IsVisible() || tb.GetValue == nil {
		return
	}
	attr := vtui.Palette[ColViewerStatus]
	if tb.GetAttr != nil {
		if a := tb.GetAttr(); a != 0 {
			attr = a
		}
	}
	tb.DrawBackground(scr, attr)
	scr.Write(tb.X1, tb.Y1, vtui.StringToCharInfo(tb.GetValue(), attr))
}
