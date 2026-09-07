package main

import (
	"bytes"
	"testing"
)

func TestPrintableStreamOnlyRemovesControls(t *testing.T) {
	input := []byte("a\x1b[2Kb\x1b]0;title\x07\r\nc")
	if got, want := printableStream(input), []byte("ab\r\nc"); !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
