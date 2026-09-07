package main

import (
	"strings"
	"testing"
)

// syncEnv is a terminal with a parser and a fake pty, with the cursor on the
// top row so that the tests can read what was printed off row zero. A fresh
// terminal starts at the bottom (rule 2 in TERMINAL.md).
func syncEnv(t *testing.T) (*TerminalView, *AnsiParser, *mockPty) {
	t.Helper()
	tv := NewTerminalView(80, 24)
	pty := &mockPty{}
	tv.pty = pty
	tv.SetCursor(0, 0)
	return tv, NewAnsiParser(tv, pty), pty
}

func syncRow(tv *TerminalView, row int) string {
	var sb strings.Builder
	for _, c := range tv.Lines[row] {
		sb.WriteRune(testRune(c.Char))
	}
	return strings.TrimRight(sb.String(), " ")
}

// A chunk that ends on the final byte of a query has to be answered there and
// then. The child sent CSI c and is now waiting: nothing else is coming that
// could release a byte held back for later.
func TestAnsiQueryAtChunkEndIsAnswered(t *testing.T) {
	for _, query := range []string{"\x1b[c", "\x1b[0c", "\x1b[?1;1S", "\x1b[16t"} {
		_, p, pty := syncEnv(t)
		p.Process([]byte(query))
		if pty.String() == "" {
			t.Errorf("%q went unanswered", query)
		}
	}
}

// The same, one byte at a time, which is how ConPTY fragments things.
func TestAnsiQuerySplitByteByByteIsAnswered(t *testing.T) {
	_, p, pty := syncEnv(t)
	for _, b := range []byte("\x1b[c") {
		p.Process([]byte{b})
	}
	if got := pty.String(); got != "\x1b[?62;4c" {
		t.Errorf("device attributes: got %q", got)
	}
}

// No byte of ordinary output may be held back either: a prompt that ends on a
// c would lose it until the next thing the child printed.
func TestAnsiTrailingTextIsNotWithheld(t *testing.T) {
	for _, text := range []string{"abc", "cd", "cd /d", "c"} {
		tv, p, _ := syncEnv(t)
		p.Process([]byte(text))
		if got := syncRow(tv, 0); got != text {
			t.Errorf("printed %q, screen shows %q", text, got)
		}
	}
}

// The command that keeps the panel and the shell in step must still be hidden,
// however the PTY chops it up.
func TestWindowsSyncExcisedAtEveryChunkBoundary(t *testing.T) {
	const stream = "ok\r\ncd /d \"C:\\\\tmp\" & rem f4_sync\r\ndone"
	for split := 0; split <= len(stream); split++ {
		tv, p, _ := syncEnv(t)
		p.Process([]byte(stream[:split]))
		p.Process([]byte(stream[split:]))

		var screen strings.Builder
		for row := 0; row < 4; row++ {
			screen.WriteString(syncRow(tv, row))
			screen.WriteByte('\n')
		}
		got := screen.String()
		if strings.Contains(got, "f4_sync") || strings.Contains(got, "cd /d") {
			t.Fatalf("split at %d leaked the technical command:\n%s", split, got)
		}
		if !strings.Contains(got, "ok") || !strings.Contains(got, "done") {
			t.Fatalf("split at %d lost real output:\n%s", split, got)
		}
	}
}

// A cd the user typed is not the technical one and stays on the screen.
func TestWindowsSyncLeavesRealCommandsAlone(t *testing.T) {
	tv, p, _ := syncEnv(t)
	p.Process([]byte("cd /d \"C:\\\\tmp\" & dir\r\n"))
	if got := syncRow(tv, 0); !strings.Contains(got, "dir") {
		t.Errorf("the user's command must survive: %q", got)
	}
}
