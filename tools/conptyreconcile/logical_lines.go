package main

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

// logicalLine is the unit received from the host.  A line ends only when the
// stream contains an explicit LF; terminal rows are never consulted.
type logicalLine struct {
	Bytes      []byte `json:"bytes"`
	Terminator []byte `json:"terminator"`
}

type logicalLineStream struct {
	pending []byte
	lines   []logicalLine
}

func (s *logicalLineStream) Feed(data []byte) {
	for len(data) != 0 {
		at := bytes.IndexByte(data, '\n')
		if at < 0 {
			s.pending = append(s.pending, data...)
			return
		}
		s.pending = append(s.pending, data[:at]...)
		line := logicalLine{Bytes: append([]byte(nil), s.pending...), Terminator: []byte{'\n'}}
		if len(line.Bytes) != 0 && line.Bytes[len(line.Bytes)-1] == '\r' {
			line.Bytes = line.Bytes[:len(line.Bytes)-1]
			line.Terminator = []byte{'\r', '\n'}
		}
		s.lines = append(s.lines, line)
		s.pending = s.pending[:0]
		data = data[at+1:]
	}
}

func (s *logicalLineStream) Lines() []logicalLine {
	result := make([]logicalLine, len(s.lines))
	for i, line := range s.lines {
		result[i] = logicalLine{Bytes: append([]byte(nil), line.Bytes...), Terminator: append([]byte(nil), line.Terminator...)}
	}
	return result
}

func (s *logicalLineStream) Bytes() []byte {
	var result []byte
	for _, line := range s.lines {
		result = append(result, line.Bytes...)
		result = append(result, line.Terminator...)
	}
	return result
}

// reflowLogicalLines produces display rows from complete logical lines. It
// never changes, joins, or infers the stored lines; width is only a rendering
// concern.
func reflowLogicalLines(lines []logicalLine, width int) [][]byte {
	if width < 1 {
		return nil
	}
	rows := make([][]byte, 0, len(lines))
	for _, line := range lines {
		if len(line.Bytes) == 0 {
			rows = append(rows, []byte{})
			continue
		}
		var row []byte
		columns := 0
		start := 0
		for start < len(line.Bytes) {
			r, size := utf8.DecodeRune(line.Bytes[start:])
			if r == utf8.RuneError && size == 1 {
				size = 1
			}
			w := displayWidth(r)
			if columns > 0 && columns+w > width {
				rows = append(rows, row)
				row = nil
				columns = 0
			}
			row = append(row, line.Bytes[start:start+size]...)
			start += size
			columns += w
			if columns >= width {
				rows = append(rows, row)
				row = nil
				columns = 0
			}
		}
		if len(row) != 0 {
			rows = append(rows, row)
		}
	}
	return rows
}

func displayWidth(r rune) int {
	if r == '\u200d' || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if r < 0x20 || r == 0x7f {
		return 0
	}
	if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) || r >= 0x1f000 {
		return 2
	}
	return 1
}
