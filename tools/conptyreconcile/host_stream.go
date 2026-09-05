package main

import (
	"bytes"
	"strconv"
)

// hostFrame records an XTWINOPS sequence observed in renderer output. PTY
// resize boundaries are not inferred from this sequence: the native harness
// records its own ResizePseudoConsole output offset instead.
type hostFrame struct {
	Offset int `json:"offset"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type repaintFrame struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// hostRenderStream consumes the pinned host's renderer stream. Logical lines
// end only at host-emitted CRLF. Child LF bytes and terminal row geometry are
// never consulted.
type hostRenderStream struct {
	raw           []byte
	scan          int
	pending       []byte
	lines         []logicalLine
	frames        []hostFrame
	repaintFrames []repaintFrame
	repaintOpen   bool
	repaintStart  int
}

func (s *hostRenderStream) Feed(data []byte) {
	s.raw = append(s.raw, data...)
	for s.scan < len(s.raw) {
		open := []byte("\x1b[?25l")
		if bytes.HasPrefix(s.raw[s.scan:], open) {
			s.repaintStart = s.scan
			s.scan += len(open)
			s.repaintOpen = true
			continue
		}
		if isTokenPrefix(s.raw[s.scan:], open) {
			return
		}
		close := []byte("\x1b[?25h")
		if bytes.HasPrefix(s.raw[s.scan:], close) {
			s.scan += len(close)
			if s.repaintOpen {
				s.repaintFrames = append(s.repaintFrames, repaintFrame{Start: s.repaintStart, End: s.scan})
				s.repaintOpen = false
			}
			continue
		}
		if isTokenPrefix(s.raw[s.scan:], close) {
			return
		}
		if width, height, size, ok := parseResizeFrame(s.raw[s.scan:]); ok {
			s.frames = append(s.frames, hostFrame{Offset: s.scan, Width: width, Height: height})
			s.scan += size
			continue
		}
		if potentialResizePrefix(s.raw[s.scan:]) {
			// Keep an incomplete CSI frame for the next chunk.
			break
		}
		if s.raw[s.scan] == '\r' && s.scan+1 < len(s.raw) && s.raw[s.scan+1] == '\n' {
			s.lines = append(s.lines, logicalLine{
				Bytes:      append([]byte(nil), s.pending...),
				Terminator: []byte{'\r', '\n'},
			})
			s.pending = s.pending[:0]
			s.scan += 2
			continue
		}
		if s.raw[s.scan] == '\r' && s.scan+1 == len(s.raw) {
			// CRLF may be split across reads.
			break
		}
		s.pending = append(s.pending, s.raw[s.scan])
		s.scan++
	}
}

func isTokenPrefix(data, token []byte) bool {
	return len(data) < len(token) && bytes.Equal(data, token[:len(data)])
}

func potentialResizePrefix(data []byte) bool {
	prefix := []byte("\x1b[8;")
	if len(data) >= len(prefix) {
		return bytes.HasPrefix(data, prefix)
	}
	return bytes.Equal(data, prefix[:len(data)])
}

func (s *hostRenderStream) Lines() []logicalLine {
	result := make([]logicalLine, len(s.lines))
	for i, line := range s.lines {
		result[i] = logicalLine{Bytes: append([]byte(nil), line.Bytes...), Terminator: append([]byte(nil), line.Terminator...)}
	}
	return result
}

func (s *hostRenderStream) Frames() []hostFrame {
	return append([]hostFrame(nil), s.frames...)
}

func (s *hostRenderStream) RepaintFrames() []repaintFrame {
	return append([]repaintFrame(nil), s.repaintFrames...)
}

func parseResizeFrame(data []byte) (width, height, size int, ok bool) {
	const prefix = "\x1b[8;"
	if !bytes.HasPrefix(data, []byte(prefix)) {
		return 0, 0, 0, false
	}
	separator := bytes.IndexByte(data[len(prefix):], ';')
	if separator <= 0 {
		return 0, 0, 0, false
	}
	separator += len(prefix)
	end := bytes.IndexByte(data[separator+1:], 't')
	if end <= 0 {
		return 0, 0, 0, false
	}
	end += separator + 1
	height, errH := strconv.Atoi(string(data[len(prefix):separator]))
	width, errW := strconv.Atoi(string(data[separator+1 : end]))
	if errH != nil || errW != nil || width < 1 || height < 1 {
		return 0, 0, 0, false
	}
	return width, height, end + 1, true
}
