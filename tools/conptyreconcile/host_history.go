package main

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"
)

// renderedHistory is a consumer for the renderer stream. It applies only
// explicit VT operations to the line currently being accumulated; it never
// infers a boundary from rows, width, or repeated content.
type renderedHistoryLine struct {
	Bytes      []byte `json:"bytes"`
	Terminator []byte `json:"terminator"`
	CrossRow   bool   `json:"cross_row,omitempty"`
}

type renderedHistory struct {
	lines          []renderedHistoryLine
	line           []rune
	column         int
	row            int
	lineStartRow   int
	crossRow       bool
	width          int
	wrapPending    bool
	cupPendingCRLF bool
}

func parseRenderedHistory(data []byte) renderedHistory {
	return parseRenderedHistoryAtWidth(data, 0)
}

// parseRenderedHistoryAtWidth applies the pinned-host cursor rule when the
// caller knows the fixed host width. An absolute CUP in the last column is the
// source-defined cursor-paint transition; the following text continues the
// already accumulated logical line instead of creating a boundary. Width 0
// keeps the conservative generic parser used by diagnostics without a host
// dimension.
func parseRenderedHistoryAtWidth(data []byte, width int) renderedHistory {
	var h renderedHistory
	h.width = width
	for i := 0; i < len(data); {
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			if h.cupPendingCRLF {
				h.cupPendingCRLF = false
				i += 2
				continue
			}
			h.commit([]byte{'\r', '\n'})
			i += 2
			continue
		}
		switch data[i] {
		case '\r':
			h.column = 0
			i++
			continue
		case '\n':
			h.row++
			h.crossRow = true
			i++
			continue
		case 0x1b:
			consumed := h.consumeEscape(data[i:])
			if consumed > 0 {
				i += consumed
				continue
			}
			i++
			continue
		}
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			r = rune(data[i])
		}
		h.put(r)
		i += size
	}
	return h
}

func (h *renderedHistory) commit(term []byte) {
	encoded := make([]byte, 0, len(h.line)*2)
	for _, r := range h.line {
		encoded = append(encoded, string(r)...)
	}
	h.lines = append(h.lines, renderedHistoryLine{
		Bytes: encoded, Terminator: append([]byte(nil), term...), CrossRow: h.crossRow,
	})
	h.line = nil
	h.column = 0
	h.row++
	h.lineStartRow = h.row
	h.crossRow = false
	h.wrapPending = false
}

func (h *renderedHistory) put(r rune) {
	if h.wrapPending {
		h.column = len(h.line)
		h.wrapPending = false
		h.crossRow = false
	}
	for len(h.line) < h.column {
		h.line = append(h.line, ' ')
	}
	if h.column == len(h.line) {
		h.line = append(h.line, r)
	} else {
		h.line[h.column] = r
	}
	h.column++
}

func (h *renderedHistory) consumeEscape(data []byte) int {
	if len(data) < 2 {
		return 0
	}
	if data[1] == ']' {
		for i := 2; i < len(data); i++ {
			if data[i] == 0x07 {
				return i + 1
			}
			if data[i] == 0x1b && i+1 < len(data) && data[i+1] == '\\' {
				return i + 2
			}
		}
		return 0
	}
	if data[1] == '[' {
		for i := 2; i < len(data); i++ {
			if data[i] < 0x40 || data[i] > 0x7e {
				continue
			}
			params := string(data[2:i])
			h.consumeCSI(params, data[i])
			return i + 1
		}
		return 0
	}
	if data[1] >= 0x40 && data[1] <= 0x7e {
		if data[1] == 'D' || data[1] == 'E' || data[1] == 'M' {
			h.row++
			h.crossRow = true
		}
		return 2
	}
	return 2
}

func (h *renderedHistory) consumeCSI(params string, final byte) {
	private := bytes.HasPrefix([]byte(params), []byte("?"))
	params = string(bytes.TrimLeft([]byte(params), "?"))
	values := make([]int, 0, 2)
	for _, field := range splitCSI(params) {
		if field == "" {
			values = append(values, 0)
			continue
		}
		n, err := strconv.Atoi(field)
		if err != nil {
			return
		}
		values = append(values, n)
	}
	one := func() int {
		if len(values) == 0 || values[0] == 0 {
			return 1
		}
		return values[0]
	}
	switch final {
	case 'm', 'h', 'l', 'r', 's', 'u':
		_ = private
	case 'K':
		if len(h.line) > h.column {
			h.line = h.line[:h.column]
		}
	case 'J':
		// ESC[3J is the pinned host's explicit clear-scrollback event. It is
		// semantic history mutation, not repaint noise, so discard committed
		// history and the in-progress line exactly when the host requests it.
		if len(values) > 0 && values[0] == 3 {
			h.lines = nil
			h.line = nil
			h.column = 0
			h.crossRow = false
			h.wrapPending = false
			h.cupPendingCRLF = false
		}
	case 'C':
		h.column += one()
	case 'D':
		h.column -= one()
		if h.column < 0 {
			h.column = 0
		}
	case 'G', '`':
		h.column = one() - 1
		if h.column < 0 {
			h.column = 0
		}
	case 'H', 'f':
		row, col := 1, 1
		if len(values) > 0 && values[0] != 0 {
			row = values[0]
		}
		if len(values) > 1 && values[1] != 0 {
			col = values[1]
		}
		if h.width > 0 && col == h.width && len(h.line) != 0 {
			// The pinned XtermEngine uses this absolute last-column CUP while
			// repairing deferred wrap state. It is not a logical boundary; the
			// next printable bytes belong to the line already being accumulated.
			h.row = row - 1
			h.column = len(h.line)
			h.wrapPending = true
			h.cupPendingCRLF = true
			return
		}
		if row-1 != h.row {
			h.crossRow = true
		}
		h.row, h.column = row-1, col-1
	}
}

func splitCSI(params string) []string {
	if params == "" {
		return nil
	}
	return strings.Split(params, ";")
}

func (h renderedHistory) Lines() []renderedHistoryLine {
	result := make([]renderedHistoryLine, len(h.lines))
	for i, line := range h.lines {
		result[i] = renderedHistoryLine{Bytes: append([]byte(nil), line.Bytes...), Terminator: append([]byte(nil), line.Terminator...), CrossRow: line.CrossRow}
	}
	return result
}
