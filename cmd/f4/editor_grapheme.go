package main

import (
	"github.com/unxed/f4/textlayout"
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

// snapEditorOffsetToClusterBoundary prevents a mouse hit inside a multi-code
// point glyph from becoming a selection anchor in the middle of that glyph.
// The visual caret map normally returns boundaries already, but its fallback
// for an incomplete/capped fragment can expose a code-point boundary. Keep the
// editor's byte-addressable model while making selection starts atomic.
func snapEditorOffsetToClusterBoundary(data []byte, pos int) int {
	if pos <= 0 {
		return 0
	}
	if pos >= len(data) {
		return len(data)
	}
	for _, cluster := range textlayout.VisualClusters(string(data)) {
		if pos > cluster.Start && pos < cluster.End {
			return cluster.Start
		}
	}
	return pos
}

func (ev *EditorView) snapMouseOffsetToClusterBoundary(offset int) int {
	if ev == nil || ev.pt == nil || ev.li == nil || offset < 0 {
		return offset
	}
	line := ev.li.GetLineAtOffset(offset)
	lineStart := ev.li.GetLineOffset(line)
	lineLen := ev.getLineLength(line)
	if offset < lineStart || offset > lineStart+lineLen {
		return offset
	}
	data, err := ev.pt.GetRange(lineStart, lineLen)
	if err != nil {
		return offset
	}
	return lineStart + snapEditorOffsetToClusterBoundary(data, offset-lineStart)
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
			return start + clusters[len(clusters)-1].start
		}
		take *= 2
		if take > pos {
			take = pos
		}
	}
}

// previousDeletionBoundaryInLine preserves the editor's established
// backspace behavior for a trailing Indic virama or Thaana mark. Cursor
// movement and Shift+Left must use the complete grapheme cluster, however:
// splitting a modifier there makes a reverse selection start in the middle of
// the visible symbol (for example, Shift+Left on the final म् selected only
// the virama). Keep the deletion-only compatibility rule separate.
func (ev *EditorView) previousDeletionBoundaryInLine(lineStart, pos int) int {
	boundary := ev.previousGraphemeBoundaryInLine(lineStart, pos)
	if boundary >= pos {
		return boundary
	}
	data, err := ev.pt.GetRange(lineStart+boundary, pos-boundary)
	if err != nil || len(data) == 0 {
		return boundary
	}
	if split := textlayout.TrailingModifierStart(string(data)); split >= 0 {
		return boundary + split
	}
	return boundary
}
