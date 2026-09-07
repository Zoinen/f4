package main

import (
	"bytes"
	"testing"
)

func TestRenderedHistoryAppliesInlineHostVT(t *testing.T) {
	raw := []byte("old\x1b[2K\x1b[1Grewritten\r\ntabs:\x1b[3CX\x1b[7CY\r\n")
	lines := parseRenderedHistory(raw).Lines()
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if !bytes.Equal(lines[0].Bytes, []byte("rewritten")) {
		t.Fatalf("rewrite = %q", lines[0].Bytes)
	}
	if !bytes.Equal(lines[1].Bytes, []byte("tabs:   X       Y")) {
		t.Fatalf("tabs = %q", lines[1].Bytes)
	}
	if lines[0].CrossRow || lines[1].CrossRow {
		t.Fatal("inline controls unexpectedly marked cross-row")
	}
}

func TestRenderedHistoryMarksCrossRowCUP(t *testing.T) {
	lines := parseRenderedHistory([]byte("one\r\n\x1b[8;1Htwo\r\n")).Lines()
	if len(lines) != 2 || !lines[1].CrossRow {
		t.Fatalf("lines = %#v, want second line cross-row", lines)
	}
}

func TestRenderedHistoryIgnoresSGRAndOSC(t *testing.T) {
	lines := parseRenderedHistory([]byte("\x1b[31mred\x1b[m\r\n\x1b]0;pinned-conpty-probe\x07\r\n")).Lines()
	if len(lines) != 2 || !bytes.Equal(lines[0].Bytes, []byte("red")) || len(lines[1].Bytes) != 0 {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestRenderedHistoryClearsScrollbackOnCSI3J(t *testing.T) {
	lines := parseRenderedHistory([]byte("before\r\n\x1b[3Jafter\r\n")).Lines()
	if len(lines) != 1 || !bytes.Equal(lines[0].Bytes, []byte("after")) {
		t.Fatalf("history after clear = %#v", lines)
	}
}

func TestRenderedHistorySkipsOSC8ST(t *testing.T) {
	raw := []byte("\x1b]8;id=123;https://example.test\x1b\\link\x1b]8;;\x1b\\\r\n")
	lines := parseRenderedHistory(raw).Lines()
	if len(lines) != 1 || !bytes.Equal(lines[0].Bytes, []byte("link")) {
		t.Fatalf("OSC 8 history = %#v", lines)
	}
}
