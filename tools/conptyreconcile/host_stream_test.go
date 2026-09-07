package main

import (
	"bytes"
	"testing"
)

func TestHostRenderStreamUsesOnlyHostCRLF(t *testing.T) {
	input := []byte("a\nb\r\n\x1b[8;25;80tC\r\n")
	var stream hostRenderStream
	for i := range input {
		stream.Feed(input[i : i+1])
	}
	lines := stream.Lines()
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !bytes.Equal(lines[0].Bytes, []byte("a\nb")) {
		t.Fatalf("first line = %q", lines[0].Bytes)
	}
	if !bytes.Equal(lines[1].Bytes, []byte("C")) {
		t.Fatalf("second line = %q", lines[1].Bytes)
	}
	frames := stream.Frames()
	if len(frames) != 1 || frames[0].Width != 80 || frames[0].Height != 25 {
		t.Fatalf("frames = %#v", frames)
	}
}

func TestParseResizeFrame(t *testing.T) {
	w, h, n, ok := parseResizeFrame([]byte("\x1b[8;40;121tmore"))
	if !ok || w != 121 || h != 40 || n != len("\x1b[8;40;121t") {
		t.Fatalf("got %d,%d,%d,%v", w, h, n, ok)
	}
}

func TestHostRenderStreamRetainsBracketedBytes(t *testing.T) {
	input := []byte("live: one\r\n\x1b[?25l\x1b[Hlive: one\r\n\x1b[?25h live: two\r\n")
	var stream hostRenderStream
	for i := range input {
		stream.Feed(input[i : i+1])
	}
	lines := stream.Lines()
	if len(lines) != 3 {
		t.Fatalf("got %d history lines, want 3", len(lines))
	}
	if !bytes.Equal(lines[0].Bytes, []byte("live: one")) || !bytes.Equal(lines[1].Bytes, []byte("\x1b[Hlive: one")) || !bytes.Equal(lines[2].Bytes, []byte(" live: two")) {
		t.Fatalf("history lines = %#v", lines)
	}
	frames := stream.RepaintFrames()
	if len(frames) != 1 || frames[0].Start != len("live: one\r\n") {
		t.Fatalf("repaint frames = %#v", frames)
	}
}

func TestHostRenderStreamHandlesSplitRepaintMarkers(t *testing.T) {
	var stream hostRenderStream
	stream.Feed([]byte("before\r\n\x1b[?25"))
	stream.Feed([]byte("lhidden\r\n\x1b[?25"))
	stream.Feed([]byte("hafter\r\n"))
	lines := stream.Lines()
	if len(lines) != 3 || !bytes.Equal(lines[0].Bytes, []byte("before")) || !bytes.Equal(lines[1].Bytes, []byte("hidden")) || !bytes.Equal(lines[2].Bytes, []byte("after")) {
		t.Fatalf("history lines = %#v", lines)
	}
	if len(stream.RepaintFrames()) != 1 {
		t.Fatalf("repaint frames = %#v", stream.RepaintFrames())
	}
}
