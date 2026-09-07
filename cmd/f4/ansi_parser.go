package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/unxed/vtui"
)

type ParserState int

var DefaultTermAttr = vtui.SetIndexBoth(0, 7, 0) // Light Gray on Black (standard ANSI indices)

const (
	StateGround ParserState = iota
	StateEsc
	StateEscIntermediate
	StateCSI
	StateOSC
	StateAPC
	StateDCS
	StateDCSPass
)

// AnsiParser converts a stream of bytes into ScreenBuf operations.
type AnsiParser struct {
	State        ParserState
	Params       []string
	CurParam     strings.Builder
	Intermediate string
	Attr         uint64
	term         *TerminalView
	pty          PtyBackend
	// replyTo resolves where answers to device queries (DA, DSR, cursor
	// position, XTSMGRAPHICS, OSC 52) must go. One parser serves every shell
	// shown in the panel, so the shell that asked is not necessarily the one
	// whose PTY was wired in at construction. Nil falls back to the wired-in
	// PTY.
	replyTo            func() PtyBackend
	runeBuf            []byte
	lastRune           rune
	pendingWindowsSync []byte
	seenWindowsSync    []byte
	osc52WG            sync.WaitGroup

	// DCS state: the final byte of the introducer and the string that follows
	// it, kept apart from CurParam because the introducer parameters live there.
	dcsFinal    byte
	dcsBody     []byte
	dcsOverflow bool

	backgroundSyncMu       sync.Mutex
	backgroundSyncExpected [][]byte
	backgroundSyncBuffer   []byte
	privateSyncPending     int
	privateSyncSuffix      []byte
}

func (p *AnsiParser) waitOSC52() {
	p.osc52WG.Wait()
}

const privateSyncCompletionOSC = "\x1b]133;F4SYNC\x07"
const managedCommandStartOSC = "\x1b]133;C"

func NewAnsiParser(t *TerminalView, p PtyBackend) *AnsiParser {
	return &AnsiParser{
		term: t,
		pty:  p,
		Attr: DefaultTermAttr,
	}
}

// replyPty is the PTY that answers to device queries belong in: whichever
// shell is currently driving the terminal, or the wired-in one.
func (p *AnsiParser) replyPty() PtyBackend {
	if p.replyTo != nil {
		if target := p.replyTo(); target != nil {
			return target
		}
	}
	return p.pty
}

const maxPendingWindowsSync = 64 * 1024

var (
	windowsSyncStart = []byte(`cd /d "`)
	windowsSyncEnd   = []byte(`" & rem f4_sync`)
	windowsSyncSep   = []byte(`" & `)
)

func cloneBytes(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}
	return append([]byte(nil), data...)
}

func isPartialPrefix(data, prefix []byte) bool {
	return len(data) > 0 && len(data) < len(prefix) && bytes.Equal(data, prefix[:len(data)])
}

func longestSuffixPrefix(data, prefix []byte) int {
	max := len(prefix) - 1
	if max > len(data) {
		max = len(data)
	}
	for n := max; n > 0; n-- {
		if bytes.Equal(data[len(data)-n:], prefix[:n]) {
			return n
		}
	}
	return 0
}

// exciseWindowsSync removes the command used to synchronize the PTY's current
// directory. The PTY can split a long command at any byte boundary, so a
// command that has begun but not finished is kept until the next Process call
// instead of being rendered as terminal output.
//
// A few bytes that merely *could* be the beginning of that command are a
// different matter, and they are not held back. They cannot be: a chunk that
// ends on the c of CSI c looks exactly like the start of cd, and holding that
// byte back means never answering the query — the child is waiting for the
// answer and so sends nothing that would release it. It waits until its own
// timeout and concludes the terminal has no sixel, which is how this surfaced.
//
// So those bytes go to the screen and are only remembered, in seenWindowsSync,
// so that the next chunk can still match the command across the split. When it
// does match, the excision writes a carriage return and an erase line, which
// wipes the fragment that reached the screen along with the rest of the
// command. The cost is that the fragment can be visible for one frame.
func (p *AnsiParser) exciseWindowsSync(data []byte) []byte {
	if len(p.pendingWindowsSync) > 0 {
		combined := make([]byte, 0, len(p.pendingWindowsSync)+len(data))
		combined = append(combined, p.pendingWindowsSync...)
		combined = append(combined, data...)
		data = combined
		p.pendingWindowsSync = nil
	}

	// Bytes already sent to the screen last time, prepended for matching and
	// skipped on the way out so that nothing is printed twice.
	skip := len(p.seenWindowsSync)
	if skip > 0 {
		combined := make([]byte, 0, skip+len(data))
		combined = append(combined, p.seenWindowsSync...)
		combined = append(combined, data...)
		data = combined
		p.seenWindowsSync = nil
	}

	var visible []byte
	emit := func(b []byte) {
		if skip > 0 {
			if len(b) <= skip {
				skip -= len(b)
				return
			}
			b = b[skip:]
			skip = 0
		}
		visible = append(visible, b...)
	}

	for len(data) > 0 {
		startIdx := bytes.Index(data, windowsSyncStart)
		if startIdx == -1 {
			emit(data)
			p.seenWindowsSync = cloneBytes(data[len(data)-longestSuffixPrefix(data, windowsSyncStart):])
			return visible
		}

		emit(data[:startIdx])
		command := data[startIdx:]
		endIdx := bytes.Index(command, windowsSyncEnd)
		separatorIdx := bytes.Index(command, windowsSyncSep)

		if endIdx >= 0 && (separatorIdx < 0 || endIdx <= separatorIdx) {
			tokenEnd := startIdx + endIdx + len(windowsSyncEnd)
			if tokenEnd == len(data) {
				if len(command) > maxPendingWindowsSync {
					emit(command)
					return visible
				}
				p.pendingWindowsSync = cloneBytes(command)
				return visible
			}

			end := tokenEnd
			switch data[end] {
			case '\r':
				if end+1 == len(data) {
					p.pendingWindowsSync = cloneBytes(command)
					return visible
				}
				end++
				if data[end] == '\n' {
					end++
				}
			case '\n':
				end++
			}

			vtui.DebugLog("ANSI_PARSER: Excising background Windows CD sync")
			// The erase takes the whole line, so whatever part of the
			// command already reached the screen goes with it.
			skip = 0
			visible = append(visible, []byte("\r\x1b[2K")...)
			data = data[end:]
			continue
		}

		if separatorIdx < 0 {
			if len(command) > maxPendingWindowsSync {
				emit(command)
				return visible
			}
			p.pendingWindowsSync = cloneBytes(command)
			return visible
		}

		separatorEnd := startIdx + separatorIdx + len(windowsSyncSep)
		remainder := data[separatorEnd:]
		if len(remainder) == 0 || isPartialPrefix(remainder, []byte("rem f4_sync")) {
			if len(command) > maxPendingWindowsSync {
				emit(command)
				return visible
			}
			p.pendingWindowsSync = cloneBytes(command)
			return visible
		}

		// This is another command after a quoted cd, not f4's marker. Keep
		// the command itself visible while removing only the technical cd.
		data = data[separatorEnd:]
	}
	return visible
}

func (p *AnsiParser) Process(data []byte) {
	if p == nil || len(data) == 0 {
		return
	}

	data = p.filterPrivateSync(data)
	if len(data) == 0 {
		return
	}

	data = p.filterExpectedBackgroundSyncEcho(data)
	if len(data) == 0 {
		return
	}

	// Heuristics: Hide background sync commands.
	// We MUST use bytes.* functions to avoid string() casts which destroy
	// incomplete UTF-8 sequences (e.g. when ConPTY splits a wide character chunk).
	if bytes.HasPrefix(data, []byte(" set +H; cd ")) {
		idx := bytes.Index(data, []byte("}\r\n"))
		if idx != -1 {
			data = data[idx+3:]
		}
	}

	if bytes.HasPrefix(data, []byte(" set +H; { trap")) {
		idx := bytes.Index(data, []byte("}\r\n"))
		if idx != -1 {
			data = data[idx+3:]
		}
	}

	if bytes.HasPrefix(data, []byte(" { trap")) {
		idx := bytes.Index(data, []byte("}\r\n"))
		if idx != -1 {
			data = data[idx+3:]
		}
	}

	for {
		startIdx := bytes.Index(data, []byte(" cd '"))
		if startIdx == -1 {
			break
		}
		endIdx := -1
		markerLen := 0
		for _, marker := range [][]byte{
			[]byte("' && true f4_sync"),
			[]byte("' # f4_sync"),
			[]byte("'; : f4_sync"),
		} {
			candidate := bytes.Index(data[startIdx:], marker)
			if candidate != -1 && (endIdx == -1 || candidate < endIdx) {
				endIdx = candidate
				markerLen = len(marker)
			}
		}
		if endIdx != -1 {
			actualEnd := startIdx + endIdx + markerLen
			if actualEnd < len(data) && data[actualEnd] == '\r' {
				actualEnd++
			}
			if actualEnd < len(data) && data[actualEnd] == '\n' {
				actualEnd++
			}
			vtui.DebugLog("ANSI_PARSER: Excising background Unix CD sync")
			newData := make([]byte, 0, startIdx+5+len(data)-actualEnd)
			newData = append(newData, data[:startIdx]...)
			newData = append(newData, []byte("\r\x1b[2K")...)
			newData = append(newData, data[actualEnd:]...)
			data = newData
			continue
		}
		break
	}

	data = p.exciseWindowsSync(data)

	if len(data) == 0 {
		return
	}

	for _, b := range data {
		// vtui.DebugLog("PARSER: Byte 0x%02X State %v", b, p.State)
		switch p.State {
		case StateGround:
			if b == 0x1b {
				p.State = StateEsc
				p.runeBuf = p.runeBuf[:0]
			} else if b < 0x80 {
				r := rune(b)
				p.term.PutChar(r, p.Attr)
				p.lastRune = r
				p.runeBuf = p.runeBuf[:0]
			} else {
				p.runeBuf = append(p.runeBuf, b)
				if utf8.FullRune(p.runeBuf) {
					r, _ := utf8.DecodeRune(p.runeBuf)
					p.term.PutChar(r, p.Attr)
					p.lastRune = r
					p.runeBuf = p.runeBuf[:0]
				} else if len(p.runeBuf) >= 4 {
					// Invalid sequence or too long, flush as is
					p.term.PutChar(rune(p.runeBuf[0]), p.Attr)
					p.runeBuf = p.runeBuf[1:]
				}
			}
		case StateEsc:
			if b == '[' {
				p.State = StateCSI
				p.Params = nil
				p.CurParam.Reset()
				p.Intermediate = ""
			} else if b == ']' {
				p.State = StateOSC
				p.Params = nil
				p.CurParam.Reset()
			} else if b == 'P' {
				p.State = StateDCS
				p.Params = nil
				p.CurParam.Reset()
				p.Intermediate = ""
				p.dcsFinal = 0
			} else if b == '_' {
				p.CurParam.Reset()
				p.State = StateAPC
			} else if b == '\\' {
				// String Terminator (ST)
				p.State = StateGround
			} else if b >= 0x20 && b <= 0x2F {
				// Промежуточные байты (например, '(' в ESC ( B)
				p.State = StateEscIntermediate
			} else {
				p.handleEsc(b)
				p.State = StateGround
			}
		case StateEscIntermediate:
			if b >= 0x20 && b <= 0x2F {
				// Продолжаем собирать промежуточные байты
			} else {
				// b — финальный байт (0x30-0x7E), завершаем последовательность
				p.State = StateGround
			}
		case StateCSI:
			if b >= 0x30 && b <= 0x39 { // '0'-'9'
				p.CurParam.WriteByte(b)
			} else if b == ';' {
				p.Params = append(p.Params, p.CurParam.String())
				p.CurParam.Reset()
			} else if b >= 0x3C && b <= 0x3F { // < = > ?
				p.CurParam.WriteByte(b)
			} else if b >= 0x20 && b <= 0x2F {
				// Intermediate bytes
				p.Intermediate += string(b)
			} else if b >= 0x40 && b <= 0x7E {
				p.Params = append(p.Params, p.CurParam.String())
				p.handleCSI(b)
				p.State = StateGround
			}
		case StateOSC:
			switch b {
			case 0x07: // BEL
				p.handleOSC()
				p.State = StateGround
			case 0x1b: // ESC
				p.handleOSC()
				p.State = StateEsc
			default:
				p.CurParam.WriteByte(b)
			}
		case StateAPC:
			switch b {
			case 0x07: // BEL
				p.handleAPC()
				p.State = StateGround
			case 0x1b: // ESC
				p.handleAPC()
				p.State = StateEsc
			default:
				p.CurParam.WriteByte(b)
			}
		case StateDCS:
			// The introducer of a device control string is shaped
			// like a CSI: parameters, then intermediates, then one
			// final byte that says what the string is.
			if b >= 0x30 && b <= 0x39 {
				p.CurParam.WriteByte(b)
			} else if b == ';' {
				p.Params = append(p.Params, p.CurParam.String())
				p.CurParam.Reset()
			} else if b >= 0x3C && b <= 0x3F {
				p.CurParam.WriteByte(b)
			} else if b >= 0x20 && b <= 0x2F {
				p.Intermediate += string(b)
			} else if b >= 0x40 && b <= 0x7E {
				p.Params = append(p.Params, p.CurParam.String())
				p.CurParam.Reset()
				p.dcsFinal = b
				p.dcsBody = p.dcsBody[:0]
				p.dcsOverflow = false
				p.State = StateDCSPass
			} else if b == 0x1b {
				p.State = StateEsc
			}
		case StateDCSPass:
			switch b {
			case 0x1b: // ESC, expected to be followed by a backslash
				p.handleDCS()
				p.State = StateEsc
			case 0x07: // BEL, which some emitters use instead of ST
				p.handleDCS()
				p.State = StateGround
			default:
				if len(p.dcsBody) < maxDCSBody {
					p.dcsBody = append(p.dcsBody, b)
				} else {
					// Keep consuming to the terminator, so a
					// runaway string cannot spill onto the
					// screen, but stop growing the buffer.
					p.dcsOverflow = true
				}
			}
		}
	}
	p.term.FlushLog()
}

// expectPrivateSyncCompletion starts a protocol-delimited suppression window
// for a shell cwd update. Interactive zsh redraws its input through ZLE, so an
// exact byte match of the echoed command is inherently timing-dependent. The
// private command prints privateSyncCompletionOSC after it has run; everything
// before that marker is technical echo/redraw traffic, while bytes following
// it are the real prompt and must be rendered normally.
func (p *AnsiParser) expectPrivateSyncCompletion() {
	if p == nil {
		return
	}
	p.backgroundSyncMu.Lock()
	p.privateSyncPending++
	p.backgroundSyncMu.Unlock()
}

func (p *AnsiParser) cancelPrivateSyncCompletion() {
	if p == nil {
		return
	}
	p.backgroundSyncMu.Lock()
	if p.privateSyncPending > 0 {
		p.privateSyncPending--
	}
	if p.privateSyncPending == 0 {
		p.privateSyncSuffix = nil
	}
	p.backgroundSyncMu.Unlock()
}

func (p *AnsiParser) filterPrivateSync(data []byte) []byte {
	p.backgroundSyncMu.Lock()
	defer p.backgroundSyncMu.Unlock()

	if p.privateSyncPending == 0 {
		if len(p.privateSyncSuffix) == 0 {
			return data
		}
		result := append(p.privateSyncSuffix, data...)
		p.privateSyncSuffix = nil
		return result
	}

	buffer := append(p.privateSyncSuffix, data...)
	p.privateSyncSuffix = nil
	marker := []byte(privateSyncCompletionOSC)
	for p.privateSyncPending > 0 {
		if index := bytes.Index(buffer, marker); index >= 0 {
			buffer = buffer[index+len(marker):]
			p.privateSyncPending--
			continue
		}

		// A foreground command marker means the private update did not reach its
		// completion marker. Fail open instead of ever hiding user output.
		if bytes.Contains(buffer, []byte(managedCommandStartOSC)) {
			p.privateSyncPending = 0
			return buffer
		}

		// Retain a possible split prefix of either protocol marker. OSC 133 C is
		// the fail-open boundary for a foreground command, and it may be divided
		// at any byte by the PTY read loop. It currently shares a prefix with the
		// private marker, but tracking both explicitly keeps this safety property
		// independent of either marker's spelling.
		keep := backgroundSyncPrefixSuffixLen(buffer, marker)
		if foregroundKeep := backgroundSyncPrefixSuffixLen(buffer,
			[]byte(managedCommandStartOSC)); foregroundKeep > keep {
			keep = foregroundKeep
		}
		if keep > 0 {
			p.privateSyncSuffix = append(p.privateSyncSuffix, buffer[len(buffer)-keep:]...)
		}
		return nil
	}

	return buffer
}

// expectBackgroundSyncEcho registers a private command whose terminal echo
// must not reach scrollback. Darwin's PTY commonly splits zsh's echo across
// several reads, so Process cannot reliably recognize it one chunk at a time.
func (p *AnsiParser) expectBackgroundSyncEcho(command string) {
	if p == nil || command == "" {
		return
	}

	p.backgroundSyncMu.Lock()
	p.backgroundSyncExpected = append(p.backgroundSyncExpected, []byte(command))
	p.backgroundSyncMu.Unlock()
}

func (p *AnsiParser) filterExpectedBackgroundSyncEcho(data []byte) []byte {
	p.backgroundSyncMu.Lock()
	defer p.backgroundSyncMu.Unlock()

	if len(p.backgroundSyncExpected) == 0 {
		// Defensive fail-open: a previous recovery must never leave bytes
		// stranded after its expectation has already been discarded.
		if len(p.backgroundSyncBuffer) != 0 {
			buffered := append(p.backgroundSyncBuffer, data...)
			p.backgroundSyncBuffer = nil
			return buffered
		}
		return data
	}

	p.backgroundSyncBuffer = append(p.backgroundSyncBuffer, data...)
	filtered := make([]byte, 0, len(data)+5)
	for len(p.backgroundSyncExpected) > 0 {
		expected := p.backgroundSyncExpected[0]
		startIdx := bytes.Index(p.backgroundSyncBuffer, expected)
		if startIdx >= 0 {
			// Everything before the exact command is ordinary prompt/output and
			// can be rendered immediately. Keep only the candidate echo while we
			// wait for its terminating newline.
			filtered = append(filtered, p.backgroundSyncBuffer[:startIdx]...)
			p.backgroundSyncBuffer = p.backgroundSyncBuffer[startIdx:]

			tail := p.backgroundSyncBuffer[len(expected):]
			if lineEnd := bytes.IndexByte(tail, '\n'); lineEnd >= 0 {
				actualEnd := len(expected) + lineEnd + 1
				filtered = append(filtered, []byte("\r\x1b[2K")...)
				p.backgroundSyncBuffer = p.backgroundSyncBuffer[actualEnd:]
				p.backgroundSyncExpected = p.backgroundSyncExpected[1:]
				vtui.DebugLog("ANSI_PARSER: Excising streamed background Unix CD sync")
				continue
			}

			// Once a foreground-command OSC appears, the background echo either
			// was not emitted or was transformed beyond recognition. Do not hold
			// the command's control sequence (and all subsequent stdout) hostage.
			// Likewise, cap only the bytes *after* an exact match rather than
			// buffering an arbitrary 64 KiB of unrelated terminal traffic.
			if backgroundSyncHasManagedCommandOSC(tail) || len(tail) > 4096 {
				filtered = append(filtered, p.backgroundSyncBuffer...)
				p.backgroundSyncBuffer = nil
				p.backgroundSyncExpected = nil
				return filtered
			}
			return filtered
		}

		// A managed-command OSC without the registered echo proves that the shell
		// suppressed or rewrote it. It must reach TerminalView immediately or the
		// busy state can never settle and restore the panels.
		if backgroundSyncHasManagedCommandOSC(p.backgroundSyncBuffer) {
			filtered = append(filtered, p.backgroundSyncBuffer...)
			p.backgroundSyncBuffer = nil
			p.backgroundSyncExpected = nil
			return filtered
		}

		// No full match exists yet. Only a suffix that is also a prefix of the
		// expected command can possibly complete on a later PTY read; render all
		// other bytes now instead of accumulating arbitrary stdout.
		keep := backgroundSyncPrefixSuffixLen(p.backgroundSyncBuffer, expected)
		// A completed line with no viable trailing command prefix means the echo
		// was suppressed or rewritten, so fail open and discard the expectation.
		// If a viable prefix follows that completed line, however, flush the old
		// output while retaining just that suffix. This covers a PTY read that
		// contains an old prompt/line plus the first fragment of the real echo.
		if bytes.IndexByte(p.backgroundSyncBuffer, '\n') >= 0 && keep == 0 {
			filtered = append(filtered, p.backgroundSyncBuffer...)
			p.backgroundSyncBuffer = nil
			p.backgroundSyncExpected = nil
			return filtered
		}
		flushLen := len(p.backgroundSyncBuffer) - keep
		filtered = append(filtered, p.backgroundSyncBuffer[:flushLen]...)
		p.backgroundSyncBuffer = p.backgroundSyncBuffer[flushLen:]
		return filtered
	}

	filtered = append(filtered, p.backgroundSyncBuffer...)
	p.backgroundSyncBuffer = nil
	return filtered
}

func backgroundSyncHasManagedCommandOSC(data []byte) bool {
	return bytes.Contains(data, []byte("\x1b]133;"))
}

// backgroundSyncPrefixSuffixLen returns the longest suffix of data that is a
// prefix of expected. Only those bytes need to survive until the next PTY read.
func backgroundSyncPrefixSuffixLen(data, expected []byte) int {
	max := len(data)
	if len(expected)-1 < max {
		max = len(expected) - 1
	}
	for n := max; n > 0; n-- {
		if bytes.Equal(data[len(data)-n:], expected[:n]) {
			return n
		}
	}
	return 0
}

// maxDCSBody caps the device control string we are willing to buffer. A sixel
// image of the largest raster we would accept fits comfortably inside it.
const maxDCSBody = 64 << 20

// handleDCS dispatches a finished device control string. Sixel is the only
// one we do anything with; the rest are swallowed, which is already better
// than the alternative, since an unhandled DCS used to spill its body onto
// the screen as text.
func (p *AnsiParser) handleDCS() {
	final := p.dcsFinal
	body := p.dcsBody
	overflow := p.dcsOverflow
	p.dcsFinal = 0
	p.dcsBody = p.dcsBody[:0]
	p.dcsOverflow = false

	if final != 'q' || p.Intermediate != "" || overflow {
		return
	}
	args := make([]int, len(p.Params))
	for i, s := range p.Params {
		s = strings.TrimLeft(s, "?<=>")
		args[i], _ = strconv.Atoi(s)
	}
	p.term.HandleSixelDCS(args, string(body), p.Attr)
}

func (p *AnsiParser) handleEsc(cmd byte) {
	// vtui.DebugLog("ANSI_PARSER: ESC %c", cmd)
	switch cmd {
	case '7':
		p.term.SaveCursor()
	case '8':
		p.term.RestoreCursor()
	case 'M': // Reverse Index
		p.term.ReverseIndex()
	case 'D': // Index
		p.term.Index()
	case 'E': // Next Line
		p.term.NextLine()
	case 'c': // RIS - Reset to Initial State
		p.term.ResetBuffer(p.term.Width, p.term.Height)
	case '=', '>':
		// Application/Numeric Keypad Mode - пока просто поглощаем
	}
}

func (p *AnsiParser) handleCSI(cmd byte) {
	// vtui.DebugLog("ANSI_PARSER: CSI %s %c (args: %v, intermediate: %q)", p.CurParam.String(), cmd, p.Params, p.Intermediate)
	args := make([]int, len(p.Params))
	// If there are no arguments, args will be an empty slice.
	// This is important for correct handling of default commands.
	for i, s := range p.Params {
		s = strings.TrimLeft(s, "?<=>")
		val, _ := strconv.Atoi(s)
		args[i] = val
	}

	switch cmd {
	case 'm':
		if len(args) == 0 {
			p.handleSGR(args, 0) // Default reset
		} else {
			for i := 0; i < len(args); {
				consumed := p.handleSGR(args, i)
				i += consumed
			}
		}
	case 'H', 'f':
		row, col := 1, 1
		if len(args) > 0 && args[0] != 0 {
			row = args[0]
		}
		if len(args) > 1 && args[1] != 0 {
			col = args[1]
		}
		p.term.SetCursor(col-1, row-1)
	case 'J':
		mode := 0
		if len(args) > 0 {
			mode = args[0]
		}
		p.term.EraseDisplay(mode, p.Attr)
	case 'K':
		mode := 0
		if len(args) > 0 {
			mode = args[0]
		}
		p.term.EraseLine(mode, p.Attr)
	case 'r': // DECSTBM - Set Top and Bottom Margins
		top, bottom := 1, p.term.Height
		if len(args) > 0 && args[0] != 0 {
			top = args[0]
		}
		if len(args) > 1 && args[1] != 0 {
			bottom = args[1]
		}
		p.term.ScrollTop = top - 1
		p.term.ScrollBottom = bottom - 1
		p.term.SetCursor(0, 0)
	case 'c':
		if p.replyPty() != nil {
			// VT220 with sixel graphics. Programs decide whether to
			// send pictures by looking for the 4 in this list, so a
			// terminal that draws them has to say so; the level had
			// to rise with it, because sixel does not exist below
			// VT220 and a 4 next to a 1 would just be a puzzle.
			p.replyPty().Write([]byte("\x1b[?62;4c"))
		}
	case 't':
		if len(args) > 0 && p.replyPty() != nil {
			cw, ch := p.term.CellSize()
			switch args[0] {
			case 18:
				resp := fmt.Sprintf("\x1b[8;%d;%dt", p.term.Height, p.term.Width)
				p.replyPty().Write([]byte(resp))
			case 14:
				resp := fmt.Sprintf("\x1b[4;%d;%dt", p.term.Height*ch, p.term.Width*cw)
				p.replyPty().Write([]byte(resp))
			case 16:
				resp := fmt.Sprintf("\x1b[6;%d;%dt", ch, cw)
				p.replyPty().Write([]byte(resp))
			}
		}
	case 'h', 'l': // DECSET / DECRST
		isSet := cmd == 'h'
		for _, s := range p.Params {
			s = strings.TrimLeft(s, "?")
			switch s {
			case "1":
				p.term.ApplicationCursorKeys = isSet
			case "25":
				// DECTCEM. ConPTY brackets every repaint frame in ?25l ... ?25h
				// (docs/TERMINAL_LEDGER.md P7); the reflow oracle keys on the
				// closing one to know a frame has ended.
				p.term.SetCursorVisible(isSet)
				if isSet && p.term.OnCursorShown != nil {
					p.term.OnCursorShown()
				}
			case "7":
				p.term.AutoWrap = isSet
			case "80": // DECSDM
				p.term.SetSixelDisplayMode(isSet)
			case "1000":
				if isSet {
					p.term.MouseTrackingMode = 1000
				} else {
					p.term.MouseTrackingMode = 0
				}
			case "1002":
				if isSet {
					p.term.MouseTrackingMode = 1002
				} else {
					p.term.MouseTrackingMode = 0
				}
			case "1003":
				if isSet {
					p.term.MouseTrackingMode = 1003
				} else {
					p.term.MouseTrackingMode = 0
				}
			case "1006":
				p.term.MouseSGRMode = isSet
			case "1049", "47":
				p.term.SetAltScreen(isSet)
				if isSet {
					p.term.EraseDisplay(2, p.Attr)
				}
			case "9001":
				p.term.Win32InputMode = isSet
			case "2004":
				p.term.BracketedPasteMode = isSet
			}
		}
	case 'L': // Insert blank lines
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.ScrollDown(p.term.CursorY, p.term.ScrollBottom, n)
	case 'M': // Delete lines
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.scrollUp(p.term.CursorY, p.term.ScrollBottom, n)
	case 'P': // Delete characters
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.DeleteCharacters(n, p.Attr)
	case '@': // Insert blank characters
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.InsertBlankCharacters(n, p.Attr)
	case 'S':
		if len(p.Params) > 0 && strings.HasPrefix(p.Params[0], "?") {
			p.handleGraphicsAttributes(args)
			return
		}
		// Scroll up (text moves up)
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.scrollUp(p.term.ScrollTop, p.term.ScrollBottom, n)
	case 'T': // Scroll down (text moves down)
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.ScrollDown(p.term.ScrollTop, p.term.ScrollBottom, n)
	case 'A':
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.SetCursor(p.term.CursorX, p.term.CursorY-n)
	case 'B':
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.SetCursor(p.term.CursorX, p.term.CursorY+n)
	case 'C':
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.SetCursor(p.term.CursorX+n, p.term.CursorY)
	case 'D':
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.SetCursor(p.term.CursorX-n, p.term.CursorY)
	case 'G', '`':
		col := 1
		if len(args) > 0 && args[0] != 0 {
			col = args[0]
		}
		p.term.SetCursor(col-1, p.term.CursorY)
	case 'd':
		row := 1
		if len(args) > 0 && args[0] != 0 {
			row = args[0]
		}
		p.term.SetCursor(p.term.CursorX, row-1)
	case 'n': // DSR - Device Status Report
		if len(args) > 0 {
			switch args[0] {
			case 5:
				if p.replyPty() != nil {
					p.replyPty().Write([]byte("\x1b[0n"))
				}
			case 6:
				if p.replyPty() != nil {
					resp := fmt.Sprintf("\x1b[%d;%dR", p.term.CursorY+1, p.term.CursorX+1)
					p.replyPty().Write([]byte(resp))
				}
			}
		}
	case 's':
		if len(p.Params) == 0 || (len(p.Params) == 1 && p.Params[0] == "") {
			p.term.SaveCursor()
		}
	case 'u':
		s0 := ""
		if len(p.Params) > 0 {
			s0 = p.Params[0]
		}
		if s0 == "" {
			p.term.RestoreCursor()
		} else if strings.HasPrefix(s0, "=") {
			flags, _ := strconv.Atoi(s0[1:])
			mode := 1
			if len(p.Params) > 1 {
				mode, _ = strconv.Atoi(p.Params[1])
			}
			switch mode {
			case 1:
				p.term.KittyFlags = flags
			case 2:
				p.term.KittyFlags |= flags
			case 3:
				p.term.KittyFlags &= ^flags
			}
		} else if strings.HasPrefix(s0, ">") {
			flags, _ := strconv.Atoi(s0[1:])
			if len(p.term.KittyFlagsStack) >= 32 {
				p.term.KittyFlagsStack = p.term.KittyFlagsStack[1:] // Limit stack size
			}
			p.term.KittyFlagsStack = append(p.term.KittyFlagsStack, p.term.KittyFlags)
			p.term.KittyFlags = flags
		} else if strings.HasPrefix(s0, "<") {
			count, _ := strconv.Atoi(s0[1:])
			if count <= 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				if len(p.term.KittyFlagsStack) == 0 {
					p.term.KittyFlags = 0
					break
				}
				last := len(p.term.KittyFlagsStack) - 1
				p.term.KittyFlags = p.term.KittyFlagsStack[last]
				p.term.KittyFlagsStack = p.term.KittyFlagsStack[:last]
			}
		} else if strings.HasPrefix(s0, "?") {
			if p.replyPty() != nil {
				resp := fmt.Sprintf("\x1b[?%du", p.term.KittyFlags)
				p.replyPty().Write([]byte(resp))
			}
		} else {
			p.term.RestoreCursor()
		}
	case 'b': // REP - Repeat last character
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.RepeatLastChar(n, p.lastRune, p.Attr)
	case 'X': // ECH - Erase Character
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.EraseCharacter(n, p.Attr)
	case 'p': // DECRQM
		if p.Intermediate == "$" {
			p.handleDECRQM(args)
		}
	}
}

func (p *AnsiParser) handleDECRQM(args []int) {
	if len(p.Params) == 0 || p.replyPty() == nil {
		return
	}

	// Ensure there is an actual mode number provided (not just an empty param or prefix)
	modeStr := strings.TrimLeft(p.Params[0], "?<=>")
	if modeStr == "" {
		return
	}

	mode := args[0]
	isDecPrivate := len(p.Params) > 0 && strings.HasPrefix(p.Params[0], "?")

	state := 0 // 0 = not recognized
	if isDecPrivate {
		switch mode {
		case 1:
			if p.term.ApplicationCursorKeys {
				state = 1
			} else {
				state = 2
			}
		case 47, 1049:
			if p.term.UseAltScreen {
				state = 1
			} else {
				state = 2
			}
		case 2004:
			if p.term.BracketedPasteMode {
				state = 1
			} else {
				state = 2
			}
		case 9001:
			if p.term.Win32InputMode {
				state = 1
			} else {
				state = 2
			}
		}
		resp := fmt.Sprintf("\x1b[?%d;%d$y", mode, state)
		p.replyPty().Write([]byte(resp))
	} else {
		resp := fmt.Sprintf("\x1b[%d;%d$y", mode, state)
		p.replyPty().Write([]byte(resp))
	}
}

func (p *AnsiParser) handleOSC() {
	s := p.CurParam.String()
	// vtui.DebugLog("ANSI_PARSER: OSC payload: %q", s)
	p.CurParam.Reset()
	if s == "" {
		return
	}

	parts := strings.SplitN(s, ";", 2)
	if len(parts) < 2 {
		return
	}

	cmd, err := strconv.Atoi(parts[0])
	if err != nil {
		return
	}

	if cmd == 0 || cmd == 2 {
		vtui.DebugLog("ANSI_OSC_TRACE: Received window title change: %q", parts[1])
		p.term.Title = parts[1]
		if p.term.OnTitleChange != nil {
			p.term.OnTitleChange(p.term.Title)
		}
		return
	}
	if cmd == 133 {
		// В последовательности OSC 133;C BEL, cmd это 133, а аргумент 'C' находится в parts[1]
		if len(parts) > 1 {
			p.term.HandleOSC133(parts[1])
		}
		return
	}

	if cmd == 52 {
		subparts := strings.SplitN(parts[1], ";", 2)
		if len(subparts) == 2 {
			if subparts[1] == "?" {
				if p.replyPty() != nil {
					p.osc52WG.Add(1)
					go func(subCmd string) {
						defer p.osc52WG.Done()
						allowed := false
						if vtui.GlobalClipboardAccessManager != nil {
							auth := vtui.GlobalClipboardAccessManager.Authorize("Terminal_OSC52_Read")
							if auth == 1 || auth == 2 {
								allowed = true
							}
						}
						if allowed {
							clip := vtui.GetClipboard()
							b64 := base64.StdEncoding.EncodeToString([]byte(clip))
							// Best effort: a dead pty is reported by the read side.
							_, _ = fmt.Fprintf(p.replyPty(), "\x1b]52;%s;%s\x07", subCmd, b64)
						}
					}(subparts[0])
				}
			} else {
				decoded, err := base64.StdEncoding.DecodeString(subparts[1])
				if err == nil {
					vtui.SetClipboard(string(decoded))
				}
			}
		}
		return
	}

	if cmd == 4 {
		subparts := strings.SplitN(parts[1], ";", 2)
		if len(subparts) < 2 {
			return
		}

		idx, _ := strconv.Atoi(subparts[0])
		if idx < 0 || idx >= 256 {
			return
		}

		colorStr := subparts[1]
		var rgbVal uint32
		parsed := false

		if strings.HasPrefix(colorStr, "#") && len(colorStr) >= 7 {
			v, err := strconv.ParseUint(colorStr[1:7], 16, 32)
			if err == nil {
				// #nosec G115 -- ParseUint with bitSize 32 rejects values outside uint32.
				rgbVal = uint32(v)
				parsed = true
			}
		} else if strings.HasPrefix(colorStr, "rgb:") {
			// format rgb:RR/GG/BB
			rgbParts := strings.Split(colorStr[4:], "/")
			if len(rgbParts) == 3 {
				r, _ := strconv.ParseUint(rgbParts[0], 16, 8)
				g, _ := strconv.ParseUint(rgbParts[1], 16, 8)
				b, _ := strconv.ParseUint(rgbParts[2], 16, 8)
				// #nosec G115 -- each component was parsed with bitSize 8, so the packed value is at most 0xFFFFFF.
				rgbVal = uint32((r << 16) | (g << 8) | b)
				parsed = true
			}
		}

		if parsed {
			p.term.Palette[idx] = rgbVal
		}
	}
}

func (p *AnsiParser) handleAPC() {
	s := p.CurParam.String()
	p.CurParam.Reset()
	// The kitty graphics protocol shares the APC channel with the far2l
	// extensions; the leading G is what tells them apart.
	if strings.HasPrefix(s, "G") {
		p.term.HandleKittyAPC(s[1:])
		return
	}
	if strings.HasPrefix(s, "far2l") {
		p.term.HandleFar2lAPC(s)
	}
}

func (p *AnsiParser) handleSGR(args []int, i int) int {
	if len(args) == 0 {
		p.Attr = DefaultTermAttr
		return 1
	}

	n := args[i]
	switch {
	case n == 0:
		p.Attr = DefaultTermAttr
	case n == 1:
		p.Attr |= vtui.ForegroundIntensity
	case n == 2:
		p.Attr |= vtui.ForegroundDim
	case n == 4:
		p.Attr |= vtui.CommonLvbUnderscore
	case n == 5:
		// Blink - ignored in many TUIs or mapped to intensity
	case n == 7:
		p.Attr |= vtui.CommonLvbReverse
	case n == 9:
		p.Attr |= vtui.CommonLvbStrikeout
	case n == 22:
		p.Attr &= ^(vtui.ForegroundIntensity | vtui.ForegroundDim)
	case n == 24:
		p.Attr &= ^vtui.CommonLvbUnderscore
	case n == 27:
		p.Attr &= ^vtui.CommonLvbReverse
	case n == 29:
		p.Attr &= ^vtui.CommonLvbStrikeout

	case n >= 30 && n <= 37:
		p.Attr = vtui.SetIndexFore(p.Attr, uint8(n-30))
	case n == 38:
		if i+2 < len(args) {
			if args[i+1] == 5 { // 256 colors
				idx := args[i+2]
				if idx >= 0 && idx < 256 {
					p.Attr = vtui.SetIndexFore(p.Attr, uint8(idx))
				}
				return 3
			} else if args[i+1] == 2 && i+4 < len(args) { // TrueColor
				r, g, b := uint32(args[i+2]), uint32(args[i+3]), uint32(args[i+4])
				p.Attr = vtui.SetRGBFore(p.Attr, (r<<16)|(g<<8)|b)
				return 5
			}
		}
	case n == 39:
		p.Attr = vtui.SetIndexFore(p.Attr, vtui.GetIndexFore(DefaultTermAttr))

	case n >= 40 && n <= 47:
		p.Attr = vtui.SetIndexBack(p.Attr, uint8(n-40))
	case n == 48:
		if i+2 < len(args) {
			if args[i+1] == 5 { // 256 colors
				idx := args[i+2]
				if idx >= 0 && idx < 256 {
					p.Attr = vtui.SetIndexBack(p.Attr, uint8(idx))
				}
				return 3
			} else if args[i+1] == 2 && i+4 < len(args) { // TrueColor
				r, g, b := uint32(args[i+2]), uint32(args[i+3]), uint32(args[i+4])
				p.Attr = vtui.SetRGBBack(p.Attr, (r<<16)|(g<<8)|b)
				return 5
			}
		}
	case n == 49:
		p.Attr = vtui.SetIndexBack(p.Attr, vtui.GetIndexBack(DefaultTermAttr))

	case n >= 90 && n <= 97:
		p.Attr = vtui.SetIndexFore(p.Attr, uint8(n-90+8))
	case n >= 100 && n <= 107:
		p.Attr = vtui.SetIndexBack(p.Attr, uint8(n-100+8))
	}
	return 1
}

// sixelColorRegisters is how many colour registers we tell a client we have.
// It is the number Windows Terminal reports, and the number the VT340 had once
// its options were fitted. Reporting more would be honest — the decoder grows
// its palette on demand and has no real limit — but it would also invite an
// encoder to quantise to a palette of that size, which is slow and pointless:
// registers are redefinable at any point in the sequence and every band keeps
// the colour it was drawn with, so 256 registers already buy an unlimited
// number of colours in one picture. That is how full colour over sixel works
// in practice.
const sixelColorRegisters = 256

// handleGraphicsAttributes answers XTSMGRAPHICS, CSI ? Pi ; Pa ; Pv S. Sixel
// libraries use it to find out how many colours they may use and how large a
// picture may be, and some of them wait for the answer, so a terminal that
// claims sixel in its device attributes has to reply to this as well.
func (p *AnsiParser) handleGraphicsAttributes(args []int) {
	if p.replyPty() == nil {
		return
	}
	item, action := 0, 0
	if len(args) > 0 {
		item = args[0]
	}
	if len(args) > 1 {
		action = args[1]
	}

	// PTY replies are best effort: a dead pty is reported by the read side.
	switch item {
	case 1: // number of colour registers
		switch action {
		case 1, 2, 3, 4:
			_, _ = fmt.Fprintf(p.replyPty(), "\x1b[?1;0;%dS", sixelColorRegisters)
		default:
			p.replyPty().Write([]byte("\x1b[?1;2S"))
		}
	case 2: // sixel raster geometry
		switch action {
		case 1, 2, 3, 4:
			cw, ch := p.term.CellSize()
			w, h := p.term.Width*cw, p.term.Height*ch
			if w > sixelMaxSide {
				w = sixelMaxSide
			}
			if h > sixelMaxSide {
				h = sixelMaxSide
			}
			_, _ = fmt.Fprintf(p.replyPty(), "\x1b[?2;0;%d;%dS", w, h)
		default:
			p.replyPty().Write([]byte("\x1b[?2;2S"))
		}
	default:
		// ReGIS geometry, and anything we have never heard of.
		_, _ = fmt.Fprintf(p.replyPty(), "\x1b[?%d;1S", item)
	}
}
