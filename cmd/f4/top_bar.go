package main

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/rivo/uniseg"
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
			leftStr = truncateTopBarMiddle(leftStr, width-rightW-1)
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

// truncateTopBarMiddle keeps both ends of a title when the right-hand status
// fields leave too little room for it. In particular, a full path keeps its
// filename instead of losing the useful part to a right-side ellipsis.
// runewidth's grapheme-aware helpers keep wide and combining characters from
// splitting across terminal cells.
func truncateTopBarMiddle(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}
	const marker = "..."
	markerWidth := runewidth.StringWidth(marker)
	if maxWidth <= markerWidth {
		return runewidth.Truncate(s, maxWidth, "")
	}

	remaining := maxWidth - markerWidth
	leftWidth := remaining / 2
	rightWidth := remaining - leftWidth
	return runewidth.Truncate(s, leftWidth, "") + marker + truncateTopBarSuffix(s, rightWidth)
}

func truncateTopBarSuffix(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxWidth {
		return s
	}

	clusters := make([]string, 0, len([]rune(s)))
	graphemes := uniseg.NewGraphemes(s)
	for graphemes.Next() {
		clusters = append(clusters, graphemes.Str())
	}

	width := 0
	start := len(clusters)
	for start > 0 {
		clusterWidth := runewidth.StringWidth(clusters[start-1])
		if width+clusterWidth > maxWidth {
			break
		}
		start--
		width += clusterWidth
	}
	return strings.Join(clusters[start:], "")
}
