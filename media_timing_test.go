package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestMediaTimingEmitsCorrelatedJSON(t *testing.T) {
	var output bytes.Buffer
	oldEnabled := mediaTimingEnabled.Load()
	mediaTimingOutput.Lock()
	oldWriter := mediaTimingOutput.writer
	mediaTimingOutput.writer = &output
	mediaTimingOutput.Unlock()
	mediaTimingEnabled.Store(true)
	t.Cleanup(func() {
		mediaTimingEnabled.Store(oldEnabled)
		mediaTimingOutput.Lock()
		mediaTimingOutput.writer = oldWriter
		mediaTimingOutput.Unlock()
	})

	ctx := mediaTimingWithCorrelation(
		context.Background(), "qt-17", "source-9")
	mediaTimingEmitAt(ctx, "broker.range.end", "go.media", 12345,
		"bytes", 4096, "cache", "exact")

	line := strings.TrimSpace(output.String())
	if !strings.HasPrefix(line, mediaTimingLogPrefix) {
		t.Fatalf("media timing line has wrong prefix: %q", line)
	}
	var record map[string]any
	if err := json.Unmarshal(
		[]byte(strings.TrimPrefix(line, mediaTimingLogPrefix)), &record); err != nil {
		t.Fatalf("decode media timing JSON: %v", err)
	}
	for key, want := range map[string]any{
		"schema":      mediaTimingSchema,
		"event":       "go.broker.range.end",
		"requestId":   "qt-17",
		"traceId":     "source-9",
		"thread":      "go.media",
		"monotonicNs": float64(12345),
		"bytes":       float64(4096),
		"cache":       "exact",
	} {
		if got := record[key]; got != want {
			t.Fatalf("%s = %#v, want %#v (record %#v)", key, got, want, record)
		}
	}
}

func TestMediaTimingDetachedContextKeepsCorrelation(t *testing.T) {
	mediaTimingEnabled.Store(true)
	t.Cleanup(func() { mediaTimingEnabled.Store(false) })
	parent, cancel := context.WithCancel(mediaTimingWithCorrelation(
		context.Background(), "qt-1", "provider-2"))
	detached := mediaTimingDetachedContext(parent)
	cancel()
	if detached.Err() != nil {
		t.Fatalf("detached context inherited cancellation: %v", detached.Err())
	}
	correlation, ok := detached.Value(mediaTimingContextKey{}).(mediaTimingCorrelation)
	if !ok || correlation.requestID != "qt-1" || correlation.traceID != "provider-2" {
		t.Fatalf("detached correlation = %#v", correlation)
	}
}
