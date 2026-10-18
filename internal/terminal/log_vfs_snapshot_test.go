package terminal

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

var logSnapshotSGR = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestTerminalLogANSISnapshotMapsViewportAndFreezesColours(t *testing.T) {
	for _, name := range []string{"piece", "piece wrap", "history", "history wrap", "active", "active wrap", "EOF", "alternate screen"} {
		t.Run(name, func(t *testing.T) {
			tv := NewTerminalView(6, 3)
			defer tv.Close()
			red := vtui.SetRGBBoth(DefaultTermAttr, 0xff0000, 0)
			blue := vtui.SetRGBBoth(DefaultTermAttr, 0x0000ff, 0x101010)
			cells := func(text string, attr uint64) []vtui.CharInfo {
				line := make([]vtui.CharInfo, 6)
				for i := range line {
					line[i] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
				}
				for i, r := range []rune(text) {
					line[i] = vtui.CharInfo{Char: uint64(r), Attributes: attr}
				}
				return line
			}
			tv.mu.Lock()
			history := []byte("éab界cd\nold\n")
			tv.Pt.Insert(0, history)
			tv.Li.Rebuild(tv.Pt)
			tv.Engine.InvalidateFrom(0)
			tv.styles = []StyleChange{{Offset: 0, Attr: red}, {Offset: len("éab"), Attr: blue}}
			tv.GridHistory = [][]vtui.CharInfo{cells("Gé", red), cells("尾", blue)}
			tv.GridHistoryWrap = []bool{true, false}
			tv.Lines = [][]vtui.CharInfo{cells("Live", blue), cells("続", red), cells("end", DefaultTermAttr)}
			tv.WrapFlags = []bool{true, false, false}
			tv.CursorY = 2
			pieceRows := tv.semanticPieceRowsUnsafe()
			wantPrefix := "éab界cd"
			switch name {
			case "piece":
				tv.semanticScrollTop = 0
			case "piece wrap":
				tv.semanticScrollTop, wantPrefix = 1, "d\nold"
			case "history":
				tv.semanticScrollTop, wantPrefix = pieceRows, "Gé尾"
			case "history wrap":
				tv.semanticScrollTop, wantPrefix = pieceRows+1, "尾\nLive"
			case "active":
				tv.semanticScrollTop, wantPrefix = pieceRows+2, "Live続"
			case "active wrap":
				tv.semanticScrollTop, wantPrefix = pieceRows+3, "続\nend"
			case "EOF":
				tv.Lines = [][]vtui.CharInfo{cells("Live", red), cells("", DefaultTermAttr), cells("", DefaultTermAttr)}
				tv.WrapFlags = []bool{false, false, false}
				tv.CursorY = 2
				tv.semanticScrollTop, wantPrefix = pieceRows+4, ""
			case "alternate screen":
				tv.UseAltScreen = true
				tv.semanticScrollTop, wantPrefix = 2, ""
			}
			tv.mu.Unlock()

			plain := tv.terminalLogSnapshot()
			v := NewTerminalLogVFS(tv, nil)
			offset, ok := v.InitialOffset()
			if !ok || offset < 0 || offset > int64(len(v.data)) {
				t.Fatalf("invalid ANSI initial offset %d (%v), size %d", offset, ok, len(v.data))
			}
			if got := logSnapshotSGR.ReplaceAll(v.data, nil); !bytes.Equal(got, plain.data) {
				t.Fatalf("ANSI text = %q, plain snapshot = %q", got, plain.data)
			}
			if got := logSnapshotSGR.ReplaceAll(v.data[:offset], nil); !bytes.Equal(got, plain.data[:plain.viewportOffset]) {
				t.Fatalf("ANSI viewport prefix = %q, want %q", got, plain.data[:plain.viewportOffset])
			}
			suffix := string(logSnapshotSGR.ReplaceAll(v.data[offset:], nil))
			if (wantPrefix == "" && suffix != "") || !strings.HasPrefix(suffix, wantPrefix) {
				t.Fatalf("viewport starts %q, want %q", suffix, wantPrefix)
			}
			if !bytes.Contains(v.data, []byte("\x1b[0;38;2;255;0;0;48;2;0;0;0m")) {
				t.Fatalf("frozen log lost red cell colours: %q", v.data)
			}
			frozen := bytes.Clone(v.data)
			raw, err := v.Open(context.Background(), "term://log")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = raw.Close() }()
			tv.PutChar('X', blue)
			if _, err := raw.(*terminalLogWrapper).RefreshSize(context.Background()); err != nil {
				t.Fatal(err)
			}
			got := make([]byte, raw.Size())
			if _, err := raw.ReadAt(context.Background(), got, 0); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, frozen) {
				t.Fatalf("output changed frozen log: %q", got)
			}
			if data, ok := v.immutableBytes(); !ok || !bytes.Equal(data, frozen) {
				t.Fatal("immutable snapshot bytes differ from the opened log")
			}
			if next, ok := v.InitialOffset(); !ok || next != offset {
				t.Fatal("output moved frozen viewport")
			}
		})
	}
}

func TestTerminalLogANSISnapshotCapturesTextStyleAndViewportAtomically(t *testing.T) {
	tv := NewTerminalView(8, 2)
	defer tv.Close()
	red := vtui.SetRGBBoth(DefaultTermAttr, 0xff0000, 0)
	blue := vtui.SetRGBBoth(DefaultTermAttr, 0x0000ff, 0)
	setState := func(second bool) {
		tv.mu.Lock()
		defer tv.mu.Unlock()
		char, attr, top := uint64('A'), red, 0
		if second {
			char, attr, top = 'B', blue, 1
		}
		for y := range tv.Lines {
			for x := range tv.Lines[y] {
				tv.Lines[y][x] = vtui.CharInfo{Char: ' ', Attributes: DefaultTermAttr}
			}
			tv.Lines[y][0] = vtui.CharInfo{Char: char, Attributes: attr}
			tv.Lines[y][1] = vtui.CharInfo{Char: uint64('0' + y), Attributes: attr}
		}
		tv.CursorY = 1
		tv.semanticScrollTop = top
	}
	setState(false)
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for second := false; ; second = !second {
			select {
			case <-stop:
				return
			default:
				setState(second)
			}
		}
	}()
	defer func() { close(stop); <-done }()
	for range 200 {
		v := NewTerminalLogVFS(tv, nil)
		offset, ok := v.InitialOffset()
		if !ok || offset < 0 || offset >= int64(len(v.data)) {
			t.Fatalf("invalid snapshot offset %d (%v)", offset, ok)
		}
		text := string(logSnapshotSGR.ReplaceAll(v.data, nil))
		suffix := string(logSnapshotSGR.ReplaceAll(v.data[offset:], nil))
		wantAttr, wantSuffix := red, "A0\nA1"
		switch text {
		case "A0\nA1":
		case "B0\nB1":
			wantAttr, wantSuffix = blue, "B1"
		default:
			t.Fatalf("torn snapshot text %q", text)
		}
		if suffix != wantSuffix || !bytes.Contains(v.data, []byte(ansiLogStyle(wantAttr, &tv.Palette))) {
			t.Fatalf("snapshot %q has mismatched style or viewport suffix %q", v.data, suffix)
		}
	}
}
