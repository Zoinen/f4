package panel

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtinput"
)

func TestHostConsoleReplyIsReturnedToPTYInsteadOfProcessedAsInput(t *testing.T) {
	pty := &mockPty{}
	pf := &PanelsFrame{
		Pty:               pty,
		ShellMode:         terminal.ShellModeHost,
		HostConsoleActive: true,
	}
	pf.noteHostConsoleQueries(pty, []byte("\x1b[6n"))

	const reply = "\x1b[7;12R"
	for _, ch := range reply {
		if !pf.consumeHostConsoleReply(&vtinput.InputEvent{
			Type:        vtinput.KeyEventType,
			KeyDown:     true,
			Char:        ch,
			InputSource: "ConPTY",
		}) {
			t.Fatalf("reply byte %q was not consumed", ch)
		}
	}
	if got := pty.String(); got != reply {
		t.Fatalf("PTY received %q, want %q", got, reply)
	}

	if !pf.consumeHostConsoleReply(&vtinput.InputEvent{
		Type:        vtinput.KeyEventType,
		InputSource: "ConPTY",
		KeyDown:     false,
	}) {
		t.Fatal("reply key-up was not swallowed")
	}
}

func TestHostConsoleReplyQueryCanCrossOutputReads(t *testing.T) {
	pty := &mockPty{}
	pf := &PanelsFrame{
		Pty:               pty,
		ShellMode:         terminal.ShellModeHost,
		HostConsoleActive: true,
	}
	pf.noteHostConsoleQueries(pty, []byte("\x1b["))
	pf.noteHostConsoleQueries(pty, []byte("6n"))

	for _, ch := range "\x1b[1;1R" {
		if !pf.consumeHostConsoleReply(&vtinput.InputEvent{
			Type:        vtinput.KeyEventType,
			KeyDown:     true,
			Char:        ch,
			InputSource: "ConPTY",
		}) {
			t.Fatalf("split-query reply byte %q was not consumed", ch)
		}
	}
	if got := pty.String(); got != "\x1b[1;1R" {
		t.Fatalf("PTY received %q, want %q", got, "\x1b[1;1R")
	}
}

func TestHostConsoleReplyNeedsOutstandingQuery(t *testing.T) {
	pf := &PanelsFrame{
		ShellMode:         terminal.ShellModeHost,
		HostConsoleActive: true,
	}
	if pf.consumeHostConsoleReply(&vtinput.InputEvent{
		Type:        vtinput.KeyEventType,
		KeyDown:     true,
		Char:        0x1b,
		InputSource: "ConPTY",
	}) {
		t.Fatal("plain Escape was mistaken for a terminal reply")
	}
}

func newHostReplyFrame() (*PanelsFrame, *mockPty) {
	pty := &mockPty{}
	return &PanelsFrame{
		Pty:               pty,
		ShellMode:         terminal.ShellModeHost,
		HostConsoleActive: true,
	}, pty
}

func hostReplyKey(ch rune, down bool) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: down, Char: ch, InputSource: "ConPTY"}
}

// typeHostReply delivers s the way Windows hands the host terminal's answer
// to f4: a KEY_EVENT down and up for every character. It returns the
// characters f4 let through as typed input.
func typeHostReply(pf *PanelsFrame, s string) string {
	var leaked strings.Builder
	for _, ch := range s {
		if !pf.consumeHostConsoleReply(hostReplyKey(ch, true)) {
			leaked.WriteRune(ch)
		}
		pf.consumeHostConsoleReply(hostReplyKey(ch, false))
	}
	return leaked.String()
}

func hostReplyPending(pf *PanelsFrame) int {
	pf.hostConsoleMu.Lock()
	defer pf.hostConsoleMu.Unlock()
	return len(pf.hostConsoleReplyState.pending)
}

// farQueryVT is how Far Manager asks the terminal something
// (far/console.cpp, query_vt): the query between two DA requests. Far reads
// its input until it has the DA answer twice and takes what lies between the
// two as the answer, so it waits for as long as the second one is missing.
func farQueryVT(query string) string { return "\x1b[0c" + query + "\x1b[0c" }

const hostDA1Reply = "\x1b[?61;4;6;7;14;21;22;23;24;28;32;42c"

func expectHostRepliesReachChild(t *testing.T, pf *PanelsFrame, pty *mockPty, replies string) {
	t.Helper()
	if leaked := typeHostReply(pf, replies); leaked != "" {
		t.Fatalf("reply bytes %q were taken for typed input", leaked)
	}
	if got := pty.String(); got != replies {
		t.Fatalf("the child received %q, want %q", got, replies)
	}
	if pending := hostReplyPending(pf); pending != 0 {
		t.Fatalf("%d queries still outstanding after every answer arrived", pending)
	}
}

// Far's first query, before it draws anything, asks whether the terminal
// does synchronized output (DECRQM 2026). The DECRQM answer was not one f4
// knew: it knocked the second DA answer out of step, that DA went to f4 as
// typed keys, and Far in the host console sat on a blank screen waiting for
// it forever (#1376).
func TestHostConsoleFarDECRQMQueryAnswersAllReachFar(t *testing.T) {
	pf, pty := newHostReplyFrame()
	pf.noteHostConsoleQueries(pty, []byte(farQueryVT("\x1b[?2026$p")))
	expectHostRepliesReachChild(t, pf, pty, hostDA1Reply+"\x1b[?2026;2$y"+hostDA1Reply)
}

// Far asks for the whole 256-colour palette in one OSC 4, which the terminal
// answers colour by colour. The query is 2 KiB and crosses output reads.
func TestHostConsoleFarPaletteQueryAnswersAllReachFar(t *testing.T) {
	pf, pty := newHostReplyFrame()
	var query, answers strings.Builder
	query.WriteString("\x1b]4")
	for i := 0; i < 256; i++ {
		fmt.Fprintf(&query, ";%d;?", i)
		fmt.Fprintf(&answers, "\x1b]4;%d;rgb:%04x/%04x/%04x\x1b\\", i, i*257, i*257, i*257)
	}
	query.WriteString("\x1b\\")
	output := farQueryVT(query.String())
	for len(output) > 0 {
		n := min(700, len(output))
		pf.noteHostConsoleQueries(pty, []byte(output[:n]))
		output = output[n:]
	}
	if pending := hostReplyPending(pf); pending != 258 {
		t.Fatalf("recorded %d outstanding answers, want 258 (two DA and 256 colours)", pending)
	}
	expectHostRepliesReachChild(t, pf, pty, hostDA1Reply+answers.String()+hostDA1Reply)
}

// A terminal that does not know a query says nothing -- which is why Far
// wraps its queries in DA. The DA answer that follows shows the query went
// unanswered; it must still reach Far, and the silent query must not stay
// outstanding.
func TestHostConsoleUnansweredQueryDoesNotHoldBackTheNextAnswer(t *testing.T) {
	for _, query := range []string{"\x1b[?2026$p", "\x1b]4;0;?;1;?\x1b\\"} {
		pf, pty := newHostReplyFrame()
		pf.noteHostConsoleQueries(pty, []byte(farQueryVT(query)))
		expectHostRepliesReachChild(t, pf, pty, hostDA1Reply+hostDA1Reply)
	}
}

// Esc pressed while a query is outstanding looks like the start of its
// answer and is held; the key after it shows it was typed, and the Esc goes
// to the child. The query stays outstanding and its answer still arrives.
func TestHostConsoleEscTypedDuringQueryReachesTheChild(t *testing.T) {
	pf, pty := newHostReplyFrame()
	pf.noteHostConsoleQueries(pty, []byte("\x1b[6n"))
	if !pf.consumeHostConsoleReply(hostReplyKey(0x1b, true)) {
		t.Fatal("Esc during an outstanding query was not held as a possible answer")
	}
	if pf.consumeHostConsoleReply(hostReplyKey('j', true)) {
		t.Fatal("a typed key after Esc was taken for part of an answer")
	}
	if got := pty.String(); got != "\x1b" {
		t.Fatalf("the child received %q, want the held Esc", got)
	}
	pty.Reset()
	expectHostRepliesReachChild(t, pf, pty, "\x1b[3;1R")
}

// Esc pressed a moment before the answer arrives: the answer's own ESC shows
// the held one was typed. The typed Esc goes to the child first, and the
// answer after it is still recognised.
func TestHostConsoleEscTypedJustBeforeAnswer(t *testing.T) {
	pf, pty := newHostReplyFrame()
	pf.noteHostConsoleQueries(pty, []byte(farQueryVT("\x1b[?2026$p")))
	replies := hostDA1Reply + "\x1b[?2026;2$y" + hostDA1Reply
	if leaked := typeHostReply(pf, "\x1b"+replies); leaked != "" {
		t.Fatalf("reply bytes %q were taken for typed input", leaked)
	}
	if got := pty.String(); got != "\x1b"+replies {
		t.Fatalf("the child received %q, want the typed Esc and then the answers", got)
	}
	if pending := hostReplyPending(pf); pending != 0 {
		t.Fatalf("%d queries still outstanding after every answer arrived", pending)
	}
}

// Queries nobody answers must not turn every later Esc into a held key.
func TestHostConsoleUnansweredQueriesExpire(t *testing.T) {
	now := time.Unix(1000, 0)
	oldNow := hostConsoleNow
	hostConsoleNow = func() time.Time { return now }
	t.Cleanup(func() { hostConsoleNow = oldNow })

	pf, pty := newHostReplyFrame()
	pf.noteHostConsoleQueries(pty, []byte("\x1b[6n"))
	now = now.Add(hostConsoleReplyWindow + time.Second)
	if pf.consumeHostConsoleReply(hostReplyKey(0x1b, true)) {
		t.Fatal("Esc long after an unanswered query was held as its answer")
	}
	if pending := hostReplyPending(pf); pending != 0 {
		t.Fatalf("%d expired queries still outstanding", pending)
	}
}

func TestHostConsoleQueryAt(t *testing.T) {
	cases := []struct {
		data       string
		kinds      []hostConsoleReplyKind
		length     int
		incomplete bool
	}{
		{"\x1b[6n", []hostConsoleReplyKind{hostConsoleReplyCPR}, 4, false},
		{"\x1b[5n", []hostConsoleReplyKind{hostConsoleReplyDSR}, 4, false},
		{"\x1b[0c", []hostConsoleReplyKind{hostConsoleReplyDA}, 4, false},
		{"\x1b[>c", []hostConsoleReplyKind{hostConsoleReplyDA}, 4, false},
		{"\x1b[?2026$p", []hostConsoleReplyKind{hostConsoleReplyDECRQM}, 9, false},
		{"\x1b[4$p", []hostConsoleReplyKind{hostConsoleReplyDECRQM}, 5, false},
		{"\x1b]4;1;?\x07", []hostConsoleReplyKind{hostConsoleReplyOSC}, 8, false},
		{"\x1b]10;?;?\x1b\\", []hostConsoleReplyKind{hostConsoleReplyOSC, hostConsoleReplyOSC}, 10, false},
		{"\x1b[=c", nil, 4, false},                 // DA3 is answered with DCS
		{"\x1b[?2004h", nil, 8, false},             // a mode set, not a query
		{"\x1b]4;1;rgb:0/0/0\x07", nil, 16, false}, // a palette set
		{"\x1b]0;title\x07", nil, 10, false},
		{"\x1b]4;1;?\x1b[0m", nil, 7, false}, // ESC cancels the string
		{"\x1b", nil, 0, true},
		{"\x1b[?2026", nil, 0, true},
		{"\x1b]4;1;?", nil, 0, true},
		{"\x1b]4;1;?\x1b", nil, 0, true},
		{"\x1bM", nil, 0, false},
	}
	for _, c := range cases {
		kinds, length, incomplete := hostConsoleQueryAt([]byte(c.data))
		if !slices.Equal(kinds, c.kinds) || length != c.length || incomplete != c.incomplete {
			t.Errorf("hostConsoleQueryAt(%q) = %v, %d, %v; want %v, %d, %v",
				c.data, kinds, length, incomplete, c.kinds, c.length, c.incomplete)
		}
	}
}
