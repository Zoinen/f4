package main

import (
	"github.com/unxed/f4/textlayout"
	"github.com/unxed/vtui"
)

// editorGrapheme is a byte-addressable grapheme cluster. The editor keeps
// byte offsets internally, but user-visible cursor operations must stop at
// these boundaries rather than between the code points that form one glyph.
type editorGrapheme struct {
	text       string
	start, end int
	width      int
}

func editorGraphemes(data []byte) []editorGrapheme {
	visualClusters := textlayout.VisualClusters(string(data))
	clusters := make([]editorGrapheme, 0, len(visualClusters))
	for _, cluster := range visualClusters {
		clusters = append(clusters, editorGrapheme{
			text:  cluster.Text,
			start: cluster.Start,
			end:   cluster.End,
			width: cluster.Width,
		})
	}
	return clusters
}

// lineUsesVisualBidi limits the full visual engine to lines that actually
// contain RTL text. zoin-bot keeps the common long LTR path bounded by the
// existing small-window grapheme helpers.
func (ev *EditorView) lineUsesVisualBidi() bool {
	if vtui.DefaultBidiMode != vtui.BidiFull {
		return false
	}
	if ev.bidiCacheValid && ev.bidiCacheSession == ev.editSession && ev.bidiCacheLine == ev.CursorLine {
		return ev.bidiCacheValue
	}
	lineLen := ev.getLineLength(ev.CursorLine)
	if lineLen <= 0 {
		ev.bidiCacheSession = ev.editSession
		ev.bidiCacheLine = ev.CursorLine
		ev.bidiCacheValue = false
		ev.bidiCacheValid = true
		return false
	}
	lineStart := ev.li.GetLineOffset(ev.CursorLine)
	data, err := ev.pt.GetRange(lineStart, lineLen)
	value := err == nil && vtui.HasRTL(string(data))
	ev.bidiCacheSession = ev.editSession
	ev.bidiCacheLine = ev.CursorLine
	ev.bidiCacheValue = value
	ev.bidiCacheValid = true
	return value
}

func nextEditorGraphemeBoundary(data []byte, pos int) int {
	if pos < 0 {
		pos = 0
	}
	if pos >= len(data) {
		return len(data)
	}
	clusters := editorGraphemes(data[pos:])
	if len(clusters) == 0 {
		return pos + 1
	}
	return pos + clusters[0].end
}

// nextGraphemeBoundaryInLine reads only a small look-ahead window for the
// common case. If the window ends exactly at a cluster boundary, it grows
// until that boundary is proven or the line ends, so long lines do not turn a
// single right-arrow/Delete into a full-line allocation.
func (ev *EditorView) nextGraphemeBoundaryInLine(lineStart, lineLen, pos int) int {
	if pos < 0 {
		pos = 0
	}
	if pos >= lineLen {
		return lineLen
	}

	remaining := lineLen - pos
	take := 64
	if take > remaining {
		take = remaining
	}
	for {
		data, err := ev.pt.GetRange(lineStart+pos, take)
		if err != nil || len(data) == 0 {
			return pos + 1
		}
		next := nextEditorGraphemeBoundary(data, 0)
		if next < len(data) || take >= remaining {
			return pos + next
		}
		take *= 2
		if take > remaining {
			take = remaining
		}
	}
}

func (ev *EditorView) previousGraphemeBoundaryInLine(lineStart, pos int) int {
	if pos <= 0 {
		return 0
	}

	take := 64
	if take > pos {
		take = pos
	}
	for {
		start := pos - take
		data, err := ev.pt.GetRange(lineStart+start, take)
		if err != nil || len(data) == 0 {
			return pos - 1
		}
		clusters := editorGraphemes(data)
		if len(clusters) == 0 {
			return pos - 1
		}
		if clusters[0].start > 0 || start == 0 {
			last := clusters[len(clusters)-1]
			if split := textlayout.TrailingModifierStart(last.text); split >= 0 {
				return start + last.start + split
			}
			return start + last.start
		}
		take *= 2
		if take > pos {
			take = pos
		}
	}
}
