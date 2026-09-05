package main

import (
	"bytes"
	"testing"
)

func TestLogicalLineStreamUsesOnlyExplicitLF(t *testing.T) {
	want := []byte("ascii\r\nlong: " + string(bytes.Repeat([]byte{'C'}, 257)) + "\nrepeat\nrepeat\n")
	var stream logicalLineStream
	for i := 0; i < len(want); i++ {
		stream.Feed(want[i : i+1])
	}
	lines := stream.Lines()
	if len(lines) != 4 {
		t.Fatalf("got %d complete lines, want 4", len(lines))
	}
	if got := stream.Bytes(); !bytes.Equal(got, want) {
		t.Fatalf("round trip changed bytes: got %d want %d", len(got), len(want))
	}
	if got := len(lines[1].Bytes); got != 263 {
		t.Fatalf("long logical line length = %d, want 263", got)
	}
	if !bytes.Equal(lines[0].Terminator, []byte{'\r', '\n'}) || !bytes.Equal(lines[1].Terminator, []byte{'\n'}) {
		t.Fatal("line terminators were not preserved")
	}
}

func TestReflowDoesNotChangeLogicalLines(t *testing.T) {
	line := logicalLine{Bytes: bytes.Repeat([]byte{'C'}, 257), Terminator: []byte{'\n'}}
	for _, width := range []int{1, 79, 80, 81, 161} {
		rows := reflowLogicalLines([]logicalLine{line}, width)
		var joined []byte
		for _, row := range rows {
			joined = append(joined, row...)
		}
		if !bytes.Equal(joined, line.Bytes) {
			t.Fatalf("width %d changed logical bytes", width)
		}
	}
}

func TestProbePayloadCoversRequiredLineClasses(t *testing.T) {
	var stream logicalLineStream
	// Cursor visibility wrappers are out-of-band renderer controls and do not
	// terminate a logical record.  The authored payload itself is what this
	// round-trip test measures.
	authored := stripCursorVisibilityWrapper([]byte(probeWorkload()))
	stream.Feed(authored)
	lines := stream.Lines()
	if len(lines) < 17 {
		t.Fatalf("probe has only %d explicit lines", len(lines))
	}
	joined := stream.Bytes()
	if !bytes.Equal(joined, authored) {
		t.Fatal("probe logical splitter changed authored bytes")
	}
	if bytes.Count(joined, []byte("repeat: SAME\r\n")) != 3 {
		t.Fatal("probe must contain exactly three identical records")
	}
	for _, prefix := range []string{"exact-n-minus-1: ", "exact-n: ", "exact-n-plus-1: ", "exact-2n-plus-1: ", "repeat-char: ", "alternating: ", "spaces:       ", "empty:", "unicode: ", "long: "} {
		if !bytes.Contains(joined, []byte(prefix)) {
			t.Fatalf("probe missing required line %q", prefix)
		}
	}
	for _, want := range []struct {
		prefix string
		length int
	}{
		{"exact-n-minus-1: ", 79},
		{"exact-n: ", 80},
		{"exact-n-plus-1: ", 81},
		{"exact-2n-plus-1: ", 161},
	} {
		found := false
		for _, line := range lines {
			if bytes.HasPrefix(line.Bytes, []byte(want.prefix)) {
				if len(line.Bytes) != want.length {
					t.Fatalf("%s length=%d, want %d", want.prefix, len(line.Bytes), want.length)
				}
				found = true
			}
		}
		if !found {
			t.Fatalf("line %q not found", want.prefix)
		}
	}
}
