package main

import (
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	applyTranscriptMaxLines = 10_000
	applyTranscriptMaxBytes = 4 << 20
)

type applyTranscript struct {
	mu      sync.RWMutex
	lines   []string
	bytes   int
	omitted bool
}

func newApplyTranscript() *applyTranscript { return &applyTranscript{} }

func (t *applyTranscript) Add(line string) {
	if t == nil {
		return
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	line = strings.ToValidUTF8(line, "\uFFFD")
	lineOmitted := false
	if len(line) > applyTranscriptMaxBytes {
		lineOmitted = true
		start := len(line) - applyTranscriptMaxBytes
		for start < len(line) && !utf8.RuneStart(line[start]) {
			start++
		}
		line = line[start:]
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	if lineOmitted {
		t.omitted = true
	}
	t.lines = append(t.lines, line)
	t.bytes += len(line)
	for len(t.lines) > applyTranscriptMaxLines || (t.bytes > applyTranscriptMaxBytes && len(t.lines) > 1) {
		t.bytes -= len(t.lines[0])
		t.lines = t.lines[1:]
		t.omitted = true
	}
}

func (t *applyTranscript) Snapshot() []string {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	extra := 0
	if t.omitted {
		extra = 1
	}
	lines := make([]string, 0, len(t.lines)+extra)
	if t.omitted {
		lines = append(lines, Msg("ApplyCommand.OutputOmitted"))
	}
	lines = append(lines, t.lines...)
	return lines
}
