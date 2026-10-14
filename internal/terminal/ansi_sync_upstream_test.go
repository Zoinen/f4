package terminal

import (
	"bytes"
	"strings"
	"testing"
)

// zsh's line editor echoes the line f4 typed to follow the panel a character
// or two at a time, and redraws it where it wraps; the echo never sat in one
// chunk and stayed on the console (#1886). The chunks are the ones zsh 5.9
// wrote for " cd '/tmp' && true f4_sync" and for a path that wraps at 100
// columns.
func TestAnsiParser_UnixSyncEchoInZshPieces(t *testing.T) {
	short := []string{" \x08 cd '/t", "m", "p' && tru", "e", " f", "4", "_s", "y", "nc",
		"\x1b[?2004l\r\r\n",
		"\x1b[1m\x1b[7m#\x1b[27m\x1b[1m\x1b[0m       \r \r\r\x1b[0m\x1b[27m\x1b[24m\x1b[Jroot@host /tmp # \x1b[K", "\x1b[?2004h"}
	long := []string{" \x08 cd '/tmp", "/cl", "aud", "e-0/scratchpad/h1", "88", "6 \r\x1b[K/", "\r/d1", "/d", "2'", " ", "&", "&", " t", "ru", "e", " ",
		"f", "4_", "sy", "nc", "\x1b[?2004l\r\r\n", "\x1b[0m\x1b[27m\x1b[24m\x1b[Jroot@host d2 # \x1b[K\x1b[?2004h"}
	bash := []string{" cd '/tmp' && true f4_sync\r\n\x1b[?2004l\r\x1b[?2004hroot@host:/tmp# "}
	for name, chunks := range map[string][]string{"short": short, "wrapped": long, "bash in one piece": bash} {
		t.Run(name, func(t *testing.T) {
			tv := NewTerminalView(100, 24)
			parser := NewAnsiParser(tv, nil)
			parser.Process([]byte("root@host ~ # "))
			parser.ExpectUnixSyncEcho()
			for _, c := range chunks {
				parser.Process([]byte(c))
			}
			log := string(tv.GetAllLogBytes())
			if strings.Contains(log, "f4_sync") || strings.Contains(log, "cd '") || strings.Contains(log, "true") {
				t.Fatalf("the typed line leaked: %q", log)
			}
			if !strings.Contains(log, "root@host") {
				t.Fatalf("the prompt is gone: %q", log)
			}
			if parser.unixEchoArms.Load() != 0 || parser.unixEchoHeld != nil {
				t.Fatalf("echo still awaited (%d) or held (%q)", parser.unixEchoArms.Load(), parser.unixEchoHeld)
			}
		})
	}
}

// Nothing is held unless f4 typed a sync line: what the user types, even a
// " cd '" of their own, is on the screen as it comes; and a line that does
// not end in the marker is let through when it ends.
func TestAnsiParser_UnixSyncEchoHoldsOnlyItsOwnLine(t *testing.T) {
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, nil)
	for _, c := range []string{" ", "\x08 cd '", "/x", "'"} {
		if got := parser.ProcessFiltered([]byte(c)); len(got) == 0 {
			t.Fatalf("user input %q was held back", c)
		}
	}

	parser.ExpectUnixSyncEcho()
	var shown []byte
	for _, c := range []string{"\r\n$ ", " cd '/y'", " && ls", "\r\n", "a b\r\n"} {
		shown = append(shown, parser.ProcessFiltered([]byte(c))...)
	}
	if !bytes.Contains(shown, []byte(" cd '/y' && ls\r\n")) || !bytes.Contains(shown, []byte("a b")) {
		t.Fatalf("a line without the marker was not let through: %q", shown)
	}
}
