package panel

import (
	"bytes"
	"strconv"
	"time"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// hostConsoleReplyKind identifies a query that the child sent through the
// host console. Windows turns the host terminal's answer into KEY_EVENT
// records on f4's input handle, so those records need to be sent back to the
// child instead of being interpreted as command-line text.
//
// Every query a child may be waiting on has to be known here, not only the
// ones f4 cares about. The replies come back in the order the queries went
// out, and one reply f4 does not recognise used to knock the rest out of
// step: Far Manager asks with a query between two DA requests and reads until
// it has both DA answers (far/console.cpp, query_vt), so the DECRQM answer to
// its first query at startup cost it the second DA, and it waited on it
// forever before drawing anything (#1376).
type hostConsoleReplyKind byte

const (
	hostConsoleReplyUnknown hostConsoleReplyKind = iota // reply-shaped, but nothing asked for it
	hostConsoleReplyCPR                                 // ESC [ row ; col R
	hostConsoleReplyDSR                                 // ESC [ status n
	hostConsoleReplyDA                                  // ESC [ ? ... c, ESC [ > ... c
	hostConsoleReplyDECRQM                              // ESC [ ? mode ; value $ y
	hostConsoleReplyOSC                                 // ESC ] 4 ; index ; rgb:... ST, ESC ] 10 ; rgb:... ST
)

const (
	// hostConsoleCSIMax bounds a CSI query or reply; real ones are a few
	// dozen bytes.
	hostConsoleCSIMax = 64
	// hostConsoleOSCQueryMax bounds an OSC query. Far Manager asks for its
	// whole 256-colour palette in a single OSC 4 of about 2 KiB.
	hostConsoleOSCQueryMax = 8192
	// hostConsoleOSCReplyMax bounds one OSC reply: a single colour.
	hostConsoleOSCReplyMax = 256
	// hostConsoleMaxPending bounds the queries waiting for an answer.
	hostConsoleMaxPending = 4096
)

// hostConsoleReplyWindow is how long outstanding queries stay outstanding
// after they were sent or after the last byte of an answer arrived. A host
// terminal answers within milliseconds; a query still unanswered after this
// will not be answered, and an Esc typed after it is only a key.
var hostConsoleReplyWindow = 2 * time.Second

// hostConsoleNow is the clock the reply window is measured with; tests move it.
var hostConsoleNow = time.Now

type hostConsoleReplyState struct {
	pending  []hostConsoleReplyKind
	pty      terminal.PtyBackend
	tail     []byte    // an escape sequence the last output read ended inside
	buffer   []byte    // the reply received so far
	deadline time.Time // pending queries count as unanswered after this
	keyUps   int
}

// noteHostConsoleQueries records the protocol queries for which a host
// terminal can answer with keyboard-looking bytes. Keeping this list makes a
// plain Escape key safe: it is considered a reply candidate only while the
// child has an outstanding query.
func (pf *PanelsFrame) noteHostConsoleQueries(pty terminal.PtyBackend, data []byte) {
	if len(data) == 0 || pf.ShellMode != terminal.ShellModeHost {
		return
	}

	pf.hostConsoleMu.Lock()
	defer pf.hostConsoleMu.Unlock()
	if !pf.HostConsoleActive {
		return
	}

	state := &pf.hostConsoleReplyState
	combined := data
	if len(state.tail) > 0 {
		combined = append(append([]byte(nil), state.tail...), data...)
	}
	state.tail = state.tail[:0]
	noted := 0
	for i := 0; i < len(combined); {
		next := bytes.IndexByte(combined[i:], 0x1b)
		if next < 0 {
			break
		}
		i += next
		kinds, length, incomplete := hostConsoleQueryAt(combined[i:])
		if incomplete {
			// The sequence continues in the next read.
			state.tail = append(state.tail, combined[i:]...)
			break
		}
		state.pending = append(state.pending, kinds...)
		noted += len(kinds)
		if length == 0 {
			length = 1
		}
		i += length
	}
	if noted == 0 {
		return
	}
	if extra := len(state.pending) - hostConsoleMaxPending; extra > 0 {
		state.pending = state.pending[extra:]
	}
	state.pty = pty
	state.deadline = hostConsoleNow().Add(hostConsoleReplyWindow)
}

// hostConsoleQueryAt reads the escape sequence data starts with. It returns
// the replies the host terminal sends for it (none if it is not a query), its
// length (0 if data does not start a sequence it knows the end of), and
// whether data ends before the sequence does.
func hostConsoleQueryAt(data []byte) (kinds []hostConsoleReplyKind, length int, incomplete bool) {
	if len(data) < 2 {
		return nil, 0, true
	}
	switch data[1] {
	case '[':
		return hostConsoleCSIQueryAt(data)
	case ']':
		return hostConsoleOSCQueryAt(data)
	}
	return nil, 0, false
}

func hostConsoleCSIQueryAt(data []byte) ([]hostConsoleReplyKind, int, bool) {
	i := 2
	var private byte
	if i < len(data) && data[i] >= '<' && data[i] <= '?' {
		private = data[i]
		i++
	}
	paramsStart := i
	for i < len(data) && data[i] >= 0x30 && data[i] <= 0x3f {
		i++
	}
	params := string(data[paramsStart:i])
	intermediatesStart := i
	for i < len(data) && data[i] >= 0x20 && data[i] <= 0x2f {
		i++
	}
	intermediates := string(data[intermediatesStart:i])
	if i == len(data) {
		return nil, 0, i < hostConsoleCSIMax
	}
	final := data[i]
	if final < 0x40 || final > 0x7e {
		return nil, 0, false
	}
	length := i + 1

	var kind hostConsoleReplyKind
	switch {
	case final == 'n' && private == 0 && intermediates == "" && params == "6":
		kind = hostConsoleReplyCPR
	case final == 'n' && private == 0 && intermediates == "" && params == "5":
		kind = hostConsoleReplyDSR
	case final == 'c' && (private == 0 || private == '>') && intermediates == "" && (params == "" || params == "0"):
		kind = hostConsoleReplyDA
	case final == 'p' && (private == 0 || private == '?') && intermediates == "$" && isDecimal(params):
		kind = hostConsoleReplyDECRQM
	default:
		return nil, length, false
	}
	return []hostConsoleReplyKind{kind}, length, false
}

// hostConsoleOSCQueryAt recognises the colour queries: OSC 4 (palette) and
// OSC 10-19 (dynamic colours). The host terminal answers every "?" in them
// with a reply of its own.
func hostConsoleOSCQueryAt(data []byte) ([]hostConsoleReplyKind, int, bool) {
	end, length := -1, 0
	for i := 2; i < len(data) && end < 0; i++ {
		switch data[i] {
		case 0x07:
			end, length = i, i+1
		case 0x1b:
			if i+1 == len(data) {
				return nil, 0, len(data) < hostConsoleOSCQueryMax
			}
			if data[i+1] != '\\' {
				// ESC cancels the string; the next sequence starts here.
				return nil, i, false
			}
			end, length = i, i+2
		}
	}
	if end < 0 {
		return nil, 0, len(data) < hostConsoleOSCQueryMax
	}

	fields := bytes.Split(data[2:end], []byte(";"))
	code, err := strconv.Atoi(string(fields[0]))
	if err != nil || (code != 4 && (code < 10 || code > 19)) {
		return nil, length, false
	}
	var kinds []hostConsoleReplyKind
	for _, field := range fields[1:] {
		if string(field) == "?" {
			kinds = append(kinds, hostConsoleReplyOSC)
		}
	}
	return kinds, length, false
}

func isDecimal(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// consumeHostConsoleReply handles one KEY_EVENT byte produced by the host
// terminal. It returns true only for a complete, or currently matching,
// terminal reply; ordinary input follows ProcessKey's normal routing.
func (pf *PanelsFrame) consumeHostConsoleReply(e *vtinput.InputEvent) bool {
	if e == nil || e.Type != vtinput.KeyEventType || e.InputSource != "ConPTY" ||
		pf.ShellMode != terminal.ShellModeHost || !pf.IsHostConsoleActive() {
		return false
	}

	pf.hostConsoleMu.Lock()
	state := &pf.hostConsoleReplyState
	if !e.KeyDown {
		if len(state.buffer) > 0 {
			pf.hostConsoleMu.Unlock()
			return true
		}
		if state.keyUps > 0 {
			state.keyUps--
			pf.hostConsoleMu.Unlock()
			return true
		}
		pf.hostConsoleMu.Unlock()
		return false
	}

	now := hostConsoleNow()
	if len(state.buffer) == 0 {
		// Every reply starts with ESC; anything else is typing.
		if len(state.pending) == 0 || e.Char != 0x1b {
			pf.hostConsoleMu.Unlock()
			return false
		}
		if now.After(state.deadline) {
			unanswered := len(state.pending)
			state.pending = nil
			state.pty = nil
			pf.hostConsoleMu.Unlock()
			vtui.DebugLog("HOST_REPLY: %d queries went unanswered; Esc is a key", unanswered)
			return false
		}
	}
	if e.Char < 0 || e.Char > 0xff {
		return pf.abandonHostConsoleReplyLocked()
	}

	state.buffer = append(state.buffer, byte(e.Char))
	kind, status := hostConsoleReplyProgress(state.buffer)
	switch status {
	case hostConsoleReplyPartial:
		state.deadline = now.Add(hostConsoleReplyWindow)
		pf.hostConsoleMu.Unlock()
		return true
	case hostConsoleReplyBroken:
		state.buffer = state.buffer[:len(state.buffer)-1]
		if e.Char != 0x1b {
			return pf.abandonHostConsoleReplyLocked()
		}
		// An Esc typed just before the answer arrived: what was held goes
		// to the child as typed, and this ESC starts the answer.
		stray := state.buffer
		state.buffer = []byte{0x1b}
		state.deadline = now.Add(hostConsoleReplyWindow)
		pty := state.pty
		pf.hostConsoleMu.Unlock()
		if pty != nil {
			_, _ = pf.WritePTY(pty, stray)
		}
		return true
	}

	sequence := append([]byte(nil), state.buffer...)
	state.buffer = nil
	state.keyUps = 1
	state.deadline = now.Add(hostConsoleReplyWindow)
	matched, skipped := false, 0
	for i, pending := range state.pending {
		if pending == kind {
			// The host terminal answers in order: the queries before
			// this one were not answered and will not be.
			matched, skipped = true, i
			state.pending = state.pending[i+1:]
			break
		}
	}
	pty := state.pty
	if len(state.pending) == 0 {
		state.pty = nil
	}
	pf.hostConsoleMu.Unlock()

	if !matched {
		vtui.DebugLog("HOST_REPLY: %q answers no outstanding query; passed to the child", sequence)
	} else if skipped > 0 {
		vtui.DebugLog("HOST_REPLY: %d queries went unanswered before %q", skipped, sequence)
	}
	if pty != nil {
		_, _ = pf.WritePTY(pty, sequence)
	}
	return true
}

// abandonHostConsoleReplyLocked gives up on the reply being received: what
// looked like its start was typed, and it goes to the child as it was. The
// queries stay outstanding. It releases hostConsoleMu.
func (pf *PanelsFrame) abandonHostConsoleReplyLocked() bool {
	state := &pf.hostConsoleReplyState
	stray := state.buffer
	state.buffer = nil
	pty := state.pty
	pf.hostConsoleMu.Unlock()
	if len(stray) > 0 && pty != nil {
		_, _ = pf.WritePTY(pty, stray)
	}
	return false
}

type hostConsoleReplyStatus byte

const (
	hostConsoleReplyPartial hostConsoleReplyStatus = iota // more bytes to come
	hostConsoleReplyDone                                  // a complete reply
	hostConsoleReplyBroken                                // not a reply after all
)

// hostConsoleReplyProgress reads the bytes of a reply received so far.
func hostConsoleReplyProgress(sequence []byte) (hostConsoleReplyKind, hostConsoleReplyStatus) {
	if len(sequence) == 0 || sequence[0] != 0x1b {
		return hostConsoleReplyUnknown, hostConsoleReplyBroken
	}
	if len(sequence) == 1 {
		return hostConsoleReplyUnknown, hostConsoleReplyPartial
	}
	switch sequence[1] {
	case '[':
		return hostConsoleCSIReplyProgress(sequence)
	case ']':
		return hostConsoleOSCReplyProgress(sequence)
	}
	return hostConsoleReplyUnknown, hostConsoleReplyBroken
}

func hostConsoleCSIReplyProgress(sequence []byte) (hostConsoleReplyKind, hostConsoleReplyStatus) {
	if len(sequence) == 2 {
		return hostConsoleReplyUnknown, hostConsoleReplyPartial
	}
	last := sequence[len(sequence)-1]
	// Parameter and intermediate bytes may precede the final byte. No other
	// printable character belongs to a CSI reply, so an accidental user string
	// cannot keep the candidate alive indefinitely.
	if last >= 0x20 && last <= 0x3f {
		if len(sequence) >= hostConsoleCSIMax {
			return hostConsoleReplyUnknown, hostConsoleReplyBroken
		}
		return hostConsoleReplyUnknown, hostConsoleReplyPartial
	}
	if last < 0x40 || last > 0x7e {
		return hostConsoleReplyUnknown, hostConsoleReplyBroken
	}
	switch last {
	case 'c':
		return hostConsoleReplyDA, hostConsoleReplyDone
	case 'R':
		return hostConsoleReplyCPR, hostConsoleReplyDone
	case 'n':
		return hostConsoleReplyDSR, hostConsoleReplyDone
	case 'y':
		if sequence[len(sequence)-2] == '$' {
			return hostConsoleReplyDECRQM, hostConsoleReplyDone
		}
	}
	return hostConsoleReplyUnknown, hostConsoleReplyDone
}

func hostConsoleOSCReplyProgress(sequence []byte) (hostConsoleReplyKind, hostConsoleReplyStatus) {
	if len(sequence) == 2 {
		return hostConsoleReplyUnknown, hostConsoleReplyPartial
	}
	last, previous := sequence[len(sequence)-1], sequence[len(sequence)-2]
	if previous == 0x1b && len(sequence) > 3 {
		if last == '\\' {
			return hostConsoleReplyOSC, hostConsoleReplyDone
		}
		return hostConsoleReplyUnknown, hostConsoleReplyBroken
	}
	switch {
	case last == 0x07:
		return hostConsoleReplyOSC, hostConsoleReplyDone
	case last == 0x1b:
		return hostConsoleReplyUnknown, hostConsoleReplyPartial
	case last < 0x20 || last > 0x7e:
		return hostConsoleReplyUnknown, hostConsoleReplyBroken
	}
	if len(sequence) >= hostConsoleOSCReplyMax {
		return hostConsoleReplyUnknown, hostConsoleReplyBroken
	}
	return hostConsoleReplyUnknown, hostConsoleReplyPartial
}

func (pf *PanelsFrame) resetHostConsoleReplyState() {
	pf.hostConsoleReplyState = hostConsoleReplyState{}
}
