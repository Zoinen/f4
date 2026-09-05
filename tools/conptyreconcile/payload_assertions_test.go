package main

import (
	"bytes"
	"testing"
)

func TestAssertStaticPayloadChecksExactCounts(t *testing.T) {
	expected := []byte("__PINNED_CONPTY_PROBE_BEGIN__\r\nrepeat: SAME\r\nrepeat: SAME\r\n__PINNED_CONPTY_PROBE_END__\r\n")
	raw := append([]byte("\x1b[2J"), expected...)
	assertions := assertStaticPayload(expected, raw)
	if failures := assertionFailures(assertions); len(failures) != 0 {
		t.Fatalf("unexpected failures: %v", failures)
	}
	// The two identical lines are each separate records; the checker must not
	// collapse them or treat a single occurrence as proof for both.
	if got := bytes.Count(raw, []byte("repeat: SAME\r\n")); got != 2 {
		t.Fatalf("test fixture has %d repeated records, want 2", got)
	}
}

func TestAssertStaticPayloadDefersControlLine(t *testing.T) {
	expected := []byte("plain: ok\r\nrewritten: old\x1b[2K\x1b[1Gnew\r\n")
	assertions := assertStaticPayload(expected, expected)
	var deferred bool
	for _, assertion := range assertions {
		if assertion.Name == "rewritten" && assertion.Status == "deferred" {
			deferred = true
		}
	}
	if !deferred {
		t.Fatalf("control line was not explicitly deferred: %#v", assertions)
	}
}
