package main

import (
	"strings"

	"github.com/unxed/f4/textlayout"
	"github.com/unxed/vtui"
)

// viewerTextRow describes the part of a decoded viewer buffer that occupies
// one screen row. The byte counts are offsets in the decoded viewer buffer,
// not offsets in the original file.
type viewerTextRow struct {
	lineLen      int
	textLen      int
	visualWidth  int
	foundNewline bool
}

// layoutViewerTextRow keeps the viewer's wrapping and painting on the same
// grapheme boundaries. Counting runes here makes combining marks, Indic
// conjuncts, and Thaana marks consume a column of their own; the resulting row
// then writes past the viewer frame and can overwrite a dialog or scrollbar.
func layoutViewerTextRow(data []byte, width, tabSize int, wrap bool) viewerTextRow {
	if width < 1 {
		width = 1
	}
	if tabSize < 1 {
		tabSize = 8
	}

	contentEnd := len(data)
	foundNewline := false
	for i, b := range data {
		if b == '\n' {
			contentEnd = i
			foundNewline = true
			break
		}
	}
	textEnd := contentEnd
	if textEnd > 0 && data[textEnd-1] == '\r' {
		textEnd--
	}

	clusters := textlayout.VisualClusters(string(data[:textEnd]))
	consumed := 0
	visualWidth := 0
	for _, cluster := range clusters {
		clusterWidth := cluster.Width
		if cluster.Text == "\t" {
			clusterWidth = tabSize - (visualWidth % tabSize)
		}
		if clusterWidth <= 0 {
			clusterWidth = 1
		}
		if wrap && consumed > 0 && visualWidth+clusterWidth > width {
			break
		}
		consumed = cluster.End
		visualWidth += clusterWidth
		if wrap && visualWidth >= width {
			break
		}
	}

	if !wrap {
		consumed = textEnd
		visualWidth = 0
		for _, cluster := range clusters {
			clusterWidth := cluster.Width
			if cluster.Text == "\t" {
				clusterWidth = tabSize - (visualWidth % tabSize)
			}
			if clusterWidth <= 0 {
				clusterWidth = 1
			}
			visualWidth += clusterWidth
		}
	}

	row := viewerTextRow{
		lineLen:      consumed,
		textLen:      consumed,
		visualWidth:  visualWidth,
		foundNewline: foundNewline && consumed >= textEnd,
	}
	if row.foundNewline {
		row.lineLen = contentEnd + 1
		row.textLen = textEnd
	}
	if !foundNewline && !wrap {
		row.lineLen = len(data)
		row.textLen = textEnd
	}
	if row.lineLen == 0 && len(data) > 0 {
		// A lone carriage return or another non-rendered control still has to
		// advance the viewer, otherwise the wrapped row loop cannot progress.
		row.lineLen = 1
		row.textLen = 0
	}
	return row
}

// viewerTextCells renders one already-wrapped logical fragment in terminal
// order. Every cell retains the source cluster's byte offset so URL hover and
// Ctrl-click continue to work even when a right-to-left run is reordered.
func viewerTextCells(text string, attr uint64, tabSize, maxWidth int) ([]vtui.CharInfo, []int) {
	if tabSize < 1 {
		tabSize = 8
	}
	if maxWidth < 0 {
		maxWidth = 0
	}

	cells := make([]vtui.CharInfo, 0, len(text))
	offsets := make([]int, 0, len(text))
	visualCol := 0
	for _, cluster := range textlayout.VisualClustersInVisualOrder(text) {
		// Rendered width, not cluster.Width: a tab expands to the next stop
		// and everything else takes the width SanitizeCluster reports after
		// replacing control characters, so the layout width is never used here.
		var width int
		plainSpaces := false
		displayText, sanitizedWidth := vtui.SanitizeCluster(cluster.Text)
		if cluster.Text == "\t" {
			width = tabSize - (visualCol % tabSize)
			displayText = strings.Repeat(" ", width)
			plainSpaces = true
		} else {
			width = sanitizedWidth
			plainSpaces = displayText == " "
		}
		if width <= 0 {
			continue
		}
		if visualCol >= maxWidth {
			break
		}
		if visualCol+width > maxWidth {
			width = maxWidth - visualCol
			if width <= 0 {
				break
			}
			displayText = strings.Repeat(" ", width)
			plainSpaces = true
		}

		if plainSpaces {
			for i := 0; i < width; i++ {
				cells = append(cells, vtui.CharInfo{Char: ' ', Attributes: attr})
				offsets = append(offsets, cluster.Start)
			}
		} else {
			clusterCells := vtui.AppendCluster(nil, displayText, width, attr)
			cells = append(cells, clusterCells...)
			for i := 0; i < len(clusterCells); i++ {
				offsets = append(offsets, cluster.Start)
			}
		}
		visualCol += width
	}
	return cells, offsets
}

// applyViewerSearchAttr changes every cell whose source grapheme intersects
// the byte range of the current search result. Search offsets are byte
// offsets, while a rendered cell can represent a multi-byte grapheme (and a
// wide grapheme can occupy two cells), so matching cells through cluster
// boundaries keeps highlighting correct for UTF-8 and reordered bidi text.
func applyViewerSearchAttr(cells []vtui.CharInfo, text string, cellByteOffsets []int, matchStart, matchEnd int, attr uint64) {
	if matchStart < 0 {
		matchStart = 0
	}
	if matchEnd > len(text) {
		matchEnd = len(text)
	}
	if matchStart >= matchEnd {
		return
	}

	clusterEnds := make(map[int]int)
	for _, cluster := range textlayout.VisualClustersInVisualOrder(text) {
		if cluster.End > clusterEnds[cluster.Start] {
			clusterEnds[cluster.Start] = cluster.End
		}
	}
	for i, start := range cellByteOffsets {
		if i >= len(cells) {
			break
		}
		end, ok := clusterEnds[start]
		if ok && start < matchEnd && end > matchStart {
			cells[i].Attributes = attr
		}
	}
}
