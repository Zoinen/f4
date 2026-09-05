package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCastConversion(t *testing.T) {
	recordContent := `{"time":0.001,"dir":"down","msg":{"op":"hello","seq":1,"version":1}}
{"time":0.003,"dir":"up","msg":{"op":"welcome","replyTo":1,"version":1}}
`
	var out bytes.Buffer
	err := convertToCast(strings.NewReader(recordContent), &out, "Demo Cast")
	if err != nil {
		t.Fatalf("Cast conversion failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("Expected 2 cast lines (header + 1 event), got %d", len(lines))
	}
	if !strings.Contains(lines[0], `"version":2`) {
		t.Errorf("Header missing asciicast version 2: %s", lines[0])
	}
	if !strings.Contains(lines[1], "welcome") {
		t.Errorf("Event line missing welcome payload: %s", lines[1])
	}
}
