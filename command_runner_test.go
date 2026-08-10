package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCommandEnvironmentWithOverwritesReservedValue(t *testing.T) {
	got := commandEnvironmentWith([]string{"PATH=/bin", "f4_apply_literal_percent_8c1e=wrong"}, applyCommandLiteralPercentEnv, "%")
	count := 0
	for _, entry := range got {
		if strings.EqualFold(strings.SplitN(entry, "=", 2)[0], applyCommandLiteralPercentEnv) {
			count++
			if entry != applyCommandLiteralPercentEnv+"=%" {
				t.Fatalf("reserved entry = %q", entry)
			}
		}
	}
	if count != 1 {
		t.Fatalf("reserved entry count = %d in %#v", count, got)
	}
}

func TestCommandLineWriterNormalizesInvalidOutputToUTF8(t *testing.T) {
	var got string
	w := newCommandLineWriter(func(line string) { got = line })
	if _, err := w.Write([]byte{'x', 0xff, '\n'}); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("output is not valid UTF-8: %q", got)
	}
}

func TestCommandLineWriterBoundsUnterminatedOutput(t *testing.T) {
	var chunks []string
	w := newCommandLineWriter(func(line string) { chunks = append(chunks, line) })
	payload := bytes.Repeat([]byte{'x'}, commandOutputChunkBytes*2+17)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if len(w.pending) > commandOutputChunkBytes {
		t.Fatalf("pending output = %d bytes", len(w.pending))
	}
	w.Flush()
	total := 0
	for _, chunk := range chunks {
		if len(chunk) > commandOutputChunkBytes {
			t.Fatalf("callback chunk = %d bytes", len(chunk))
		}
		total += len(chunk)
	}
	if total != len(payload) {
		t.Fatalf("streamed bytes = %d, want %d", total, len(payload))
	}
}

func TestCommandLineWriterDoesNotSplitUTF8RuneAtChunkBoundary(t *testing.T) {
	var chunks []string
	w := newCommandLineWriter(func(line string) { chunks = append(chunks, line) })
	payload := append(bytes.Repeat([]byte{'x'}, commandOutputChunkBytes-1), []byte("яz")...)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if got := strings.Join(chunks, ""); got != string(payload) {
		t.Fatalf("joined chunks lost UTF-8 data at boundary: %q", got[len(got)-8:])
	}
}

func TestCommandLineWriterStreamsCompleteAndPartialLines(t *testing.T) {
	var got []string
	w := newCommandLineWriter(func(line string) { got = append(got, line) })
	if n, err := w.Write([]byte("one\r\ntw")); err != nil || n != 7 {
		t.Fatalf("first Write = %d, %v", n, err)
	}
	if n, err := w.Write([]byte("o\nthree\r")); err != nil || n != 8 {
		t.Fatalf("second Write = %d, %v", n, err)
	}
	w.Flush()
	if want := []string{"one", "two", "three"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lines = %#v, want %#v", got, want)
	}
	// Flush is idempotent and must not duplicate the last unterminated line.
	w.Flush()
	if len(got) != 3 {
		t.Fatalf("second Flush duplicated output: %#v", got)
	}
}
