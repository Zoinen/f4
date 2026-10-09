package editor

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtinput"
)

func TestEditorBufferHasNULCoverageBatch23(t *testing.T) {
	cases := []struct {
		name string
		pt   *piecetable.PieceTable
		want bool
	}{
		{"nil", nil, false},
		{"empty", piecetable.New(nil), false},
		{"text", piecetable.New([]byte("plain text")), false},
		{"binary", piecetable.New([]byte{'a', 0, 'b'}), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := editorBufferHasNUL(tc.pt); got != tc.want {
				t.Fatalf("editorBufferHasNUL = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestEditorAddCursorClickCoverageBatch23(t *testing.T) {
	valid := &vtinput.InputEvent{KeyDown: true, ControlKeyState: vtinput.LeftAltPressed}
	if !editorAddCursorClick(valid) {
		t.Fatal("Alt click was not recognized")
	}
	cases := []*vtinput.InputEvent{
		{ControlKeyState: vtinput.LeftAltPressed},
		{KeyDown: true, ControlKeyState: vtinput.LeftAltPressed, MouseEventFlags: vtinput.MouseMoved},
		{KeyDown: true, ControlKeyState: vtinput.LeftAltPressed, MouseEventFlags: vtinput.DoubleClick},
		{KeyDown: true, ControlKeyState: vtinput.LeftAltPressed | vtinput.ShiftPressed},
		{KeyDown: true, ControlKeyState: vtinput.LeftAltPressed | vtinput.LeftCtrlPressed},
	}
	for _, event := range cases {
		if editorAddCursorClick(event) {
			t.Errorf("event %#v was incorrectly recognized as an extra-caret click", event)
		}
	}
}

func TestEditorBlockMouseSelectionCoverageBatch23(t *testing.T) {
	if !editorBlockMouseSelection(&vtinput.InputEvent{ControlKeyState: vtinput.LeftAltPressed | vtinput.ShiftPressed}) {
		t.Fatal("Alt+Shift was not recognized as block selection")
	}
	for _, mods := range []vtinput.ControlKeyState{0, vtinput.LeftAltPressed, vtinput.ShiftPressed, vtinput.LeftCtrlPressed | vtinput.ShiftPressed} {
		if editorBlockMouseSelection(&vtinput.InputEvent{ControlKeyState: mods}) {
			t.Errorf("modifier state %#x was incorrectly recognized as block selection", mods)
		}
	}
}

func TestTrailingLineTerminatorCoverageBatch23(t *testing.T) {
	cases := [][]byte{nil, []byte("plain"), []byte("line\n"), []byte("line\r\n")}
	wants := [][]byte{nil, nil, []byte("\n"), []byte("\r\n")}
	for i, data := range cases {
		if got := trailingLineTerminator(data); !bytes.Equal(got, wants[i]) {
			t.Errorf("trailingLineTerminator(%q) = %q, want %q", data, got, wants[i])
		}
	}
}

func TestParseHexPatternCoverageBatch23(t *testing.T) {
	if got, want := mustParseHexPattern(t, "a 0f ??"), "(?s)\\x0a\\x0f."; got != want {
		t.Fatalf("parseHexPatternToRegex = %q, want %q", got, want)
	}
	if _, err := parseHexPatternToRegex("0g"); err == nil {
		t.Fatal("invalid hex pattern was accepted")
	}
}

func mustParseHexPattern(t *testing.T, pattern string) string {
	t.Helper()
	got, err := parseHexPatternToRegex(pattern)
	if err != nil {
		t.Fatalf("parseHexPatternToRegex(%q): %v", pattern, err)
	}
	return got
}

func TestParseHexReplacementCoverageBatch23(t *testing.T) {
	got, err := parseHexReplacement("a ff 10")
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{0x0a, 0xff, 0x10}; !bytes.Equal(got, want) {
		t.Fatalf("parseHexReplacement = %x, want %x", got, want)
	}
	if _, err := parseHexReplacement("zz"); err == nil {
		t.Fatal("invalid hex replacement was accepted")
	}
}

func TestAlternateDataStreamCoverageBatch23(t *testing.T) {
	paths := []struct {
		path string
		want bool
	}{
		{"https://host/file", false},
		{"plain-file", false},
	}
	if runtime.GOOS == "windows" {
		paths = append(paths, struct {
			path string
			want bool
		}{`C:\dir\file:stream`, true})
	}
	for _, tc := range paths {
		if got := isAlternateDataStream(tc.path); got != tc.want {
			t.Errorf("isAlternateDataStream(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestEditorVetoActionKeyCoverageBatch23(t *testing.T) {
	plain := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}
	ev := &EditorView{Indexing: true}
	if !ev.VetoActionKey(plain) {
		t.Fatal("Escape did not veto global actions while indexing")
	}
	if ev.VetoActionKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: false, VirtualKeyCode: vtinput.VK_ESCAPE}) {
		t.Fatal("key-up event unexpectedly vetoed a global action")
	}
	ev.Indexing = false
	ev.acEnabled = true
	ev.acMatches = []string{"suggestion"}
	if !ev.VetoActionKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB}) {
		t.Fatal("autocomplete Tab was not vetoed")
	}
}

func TestEditorBufferHasNULIgnoresBytesAfterProbeBatch23(t *testing.T) {
	data := bytes.Repeat([]byte{'x'}, 16*1024)
	data = append(data, 0)
	if editorBufferHasNUL(piecetable.New(data)) {
		t.Fatal("NUL outside the constructor probe was reported")
	}
}

func TestParseHexPatternAcceptsEmptyPatternBatch23(t *testing.T) {
	if got, want := mustParseHexPattern(t, ""), "(?s)"; got != want {
		t.Fatalf("empty hex pattern = %q, want %q", got, want)
	}
}
