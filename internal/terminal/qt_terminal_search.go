package terminal

import (
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/textsearch"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

type terminalHistorySearch struct {
	query        string
	width        int
	activeOffset int
	lineStart    int
	byteOffset   int
	byteEnd      int
	match        extui.TerminalSearchMatch
}

func (s terminalHistorySearch) visibleMatch(layout terminalSemanticLayout, width int) *extui.TerminalSearchMatch {
	if layout.altScreen || s.width != width || s.activeOffset != layout.activeOffset ||
		s.match.Sequence == 0 || s.match.StartRow < layout.contentStart || s.match.EndRow >= layout.totalRows {
		return nil
	}
	return &s.match
}

func (tv *TerminalView) HandleHistorySearchAction(anchor vtui.Frame, action map[string]any) bool {
	name := semantic.String(action["action"])
	if name != "terminal.search" && name != "terminal.searchAgain" {
		return false
	}
	if semantic.String(action["target"]) != vtui.SemanticID(tv) {
		return false
	}

	tv.mu.Lock()
	query, altScreen := tv.semanticSearch.query, tv.UseAltScreen
	tv.mu.Unlock()
	if altScreen {
		return true
	}
	showNotFound := func() {
		vtui.ShowMessageOn(
			anchor,
			i18n.Msg("Search.Title"),
			i18n.Msg("Search.NotFound"),
			[]string{i18n.Msg("vtui.Ok")},
		)
	}
	if name == "terminal.searchAgain" && query != "" {
		if !tv.findHistoryMatch(query, semantic.Bool(action["reverse"]), true) {
			showNotFound()
		}
		return true
	}

	vtui.InputBoxOn(
		anchor,
		i18n.Msg("Search.Title"),
		i18n.Msg("Search.Prompt"),
		query,
		func(query string) {
			if query != "" && !tv.findHistoryMatch(query, true, false) {
				vtui.FrameManager.PostTask(showNotFound)
			}
		},
	)
	return true
}

type terminalSearchLine struct {
	text   []byte
	starts []int
	first  int
}

// Preserve soft wraps, including a logical line split across terminal stores.
func (tv *TerminalView) historySearchLine(layout terminalSemanticLayout, row int) terminalSearchLine {
	first := row
	for first > layout.contentStart {
		_, hardBreak := tv.semanticRowTextRangeUnsafe(layout, first-1, 0, -1)
		if hardBreak {
			break
		}
		first--
	}

	line := terminalSearchLine{first: first, starts: []int{}, text: []byte{}}
	for row := first; row < layout.totalRows; row++ {
		text, hardBreak := tv.semanticRowTextRangeUnsafe(layout, row, 0, -1)
		line.starts = append(line.starts, len(line.text))
		if hardBreak || row == layout.totalRows-1 {
			text = strings.TrimRight(text, " ")
		}
		line.text = append(line.text, text...)
		if hardBreak {
			break
		}
	}
	return line
}

func (l terminalSearchLine) cellAt(offset int, endExclusive bool) (row, column int) {
	index := len(l.starts) - 1
	for index > 0 && (l.starts[index] > offset || (endExclusive && l.starts[index] == offset)) {
		index--
	}
	for _, r := range string(l.text[l.starts[index]:offset]) {
		column += max(1, runewidth.RuneWidth(r))
	}
	return l.first + index, column
}

func (tv *TerminalView) findHistoryMatch(query string, reverse, repeat bool) bool {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	layout := tv.semanticLayoutUnsafe()
	if layout.altScreen || query == "" {
		return false
	}

	previous := tv.semanticSearch
	if previous.query != query {
		tv.semanticSearch = terminalHistorySearch{query: query}
		tv.semanticSearch.match.Sequence = previous.match.Sequence
	}
	if layout.contentStart >= layout.totalRows {
		return false
	}

	viewportRows := tv.semanticViewportRows
	if viewportRows <= 0 {
		viewportRows = max(1, tv.Height)
	}
	start := min(layout.totalRows-1, tv.semanticScrollTop+viewportRows-1)
	start = max(start, layout.contentStart)
	repeat = repeat && previous.query == query && previous.visibleMatch(layout, tv.Width) != nil
	if repeat {
		start = previous.lineStart
	}
	row := start
	wrapped := false
	for {
		line := tv.historySearchLine(layout, row)
		data := line.text
		base := 0
		limit := len(data)
		if repeat && !wrapped && line.first == previous.lineStart {
			if reverse {
				limit = min(limit, previous.byteOffset)
			} else {
				base = min(limit, previous.byteEnd)
			}
		}
		searchOffset := 0
		if reverse {
			searchOffset = limit - base
		}
		offset, length, _ := textsearch.FindMatch(
			data[base:limit],
			query,
			false,
			reverse,
			false,
			false,
			false,
			searchOffset,
		)
		if offset >= 0 && length > 0 {
			first, left := line.cellAt(base+offset, false)
			last, right := line.cellAt(base+offset+length, true)
			tv.semanticSearch = terminalHistorySearch{
				query: query, width: tv.Width, activeOffset: layout.activeOffset,
				lineStart: line.first, byteOffset: base + offset, byteEnd: base + offset + length,
				match: extui.TerminalSearchMatch{
					Sequence: previous.match.Sequence + 1,
					StartRow: first, StartColumn: left, EndRow: last, EndColumn: right,
				},
			}
			maxTop := max(0, layout.totalRows-viewportRows)
			top := min(max(tv.semanticScrollTop, layout.contentStart), maxTop)
			if first < top || last >= top+viewportRows {
				top = max(min(layout.contentStart, maxTop), min(first-viewportRows/2, maxTop))
			}
			tv.semanticScrollTop = top
			tv.semanticFollowTail = false
			return true
		}

		if wrapped && (reverse && line.first <= start || !reverse && line.first+len(line.starts) > start) {
			return false
		}
		row = line.first + len(line.starts)
		if reverse {
			row = line.first - 1
		}
		if row < layout.contentStart || row >= layout.totalRows {
			if wrapped {
				return false
			}
			wrapped = true
			row = layout.contentStart
			if reverse {
				row = layout.totalRows - 1
			}
		}
	}
}
