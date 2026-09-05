package main

import (
	"bytes"
	"fmt"
	"strings"
)

// payloadAssertion is deliberately explicit.  A native transport success is
// not a history success: every assertion below is either passed, failed, or
// deferred to the VT-history consumer.  Deferred assertions make the gate
// fail closed instead of silently treating a partial check as complete.
type payloadAssertion struct {
	Name          string `json:"name"`
	Status        string `json:"status"`
	ExpectedCount int    `json:"expected_count,omitempty"`
	ObservedCount int    `json:"observed_count,omitempty"`
	Detail        string `json:"detail,omitempty"`
}

func assertStaticPayload(expected, raw []byte, markers ...string) []payloadAssertion {
	if len(markers) == 0 {
		markers = []string{probeBeginMarker, probeEndMarker}
	}
	var expectedStream logicalLineStream
	expectedStream.Feed(stripCursorVisibilityWrapper(expected))
	lines := expectedStream.Lines()
	rendered := parseRenderedHistory(raw).Lines()
	segment, segmentOK := renderedMarkerSegment(rendered, markers[0], markers[len(markers)-1])
	result := make([]payloadAssertion, 0, len(lines)+4)
	if !segmentOK {
		result = append(result, payloadAssertion{
			Name: "history-order", Status: "failed", ExpectedCount: len(lines),
			Detail: "begin/end markers do not delimit a rendered logical sequence",
		})
	} else if len(segment) != len(lines) {
		result = append(result, payloadAssertion{
			Name: "history-order", Status: "failed", ExpectedCount: len(lines),
			ObservedCount: len(segment), Detail: "rendered record count differs",
		})
	} else {
		result = append(result, payloadAssertion{
			Name: "history-order", Status: "passed", ExpectedCount: len(lines),
			ObservedCount: len(segment), Detail: "rendered records match in order",
		})
	}

	lineFrequency := make(map[string]int)
	for _, line := range lines {
		if !bytes.Contains(line.Bytes, []byte{0x1b}) && !bytes.Contains(line.Bytes, []byte{'\t'}) {
			needle := append(append([]byte(nil), line.Bytes...), line.Terminator...)
			lineFrequency[string(needle)]++
		}
	}
	for index, line := range lines {
		name := fmt.Sprintf("line[%d]", index)
		if colon := bytes.IndexByte(line.Bytes, ':'); colon > 0 {
			name = string(line.Bytes[:colon])
		}
		if bytes.Contains(line.Bytes, []byte{0x1b}) || bytes.Contains(line.Bytes, []byte{'\t'}) {
			result = append(result, payloadAssertion{Name: name, Status: "deferred", Detail: "line contains VT controls; rendered control history is checked separately"})
			continue
		}
		needle := append(append([]byte(nil), line.Bytes...), line.Terminator...)
		observed := countRenderedLine(segment, line.Bytes, line.Terminator)
		if bytes.HasPrefix(line.Bytes, []byte("spaces-nine:")) && observed == 0 {
			// The source-defined ECH threshold permits only this exact trimmed
			// alternative; no width or row heuristic is used.
			trimmedBytes := bytes.TrimRight(line.Bytes, " ")
			observed = countRenderedLine(segment, trimmedBytes, line.Terminator)
		}
		expectedCount := lineFrequency[string(needle)]
		status, detail := "passed", "exact rendered logical line found in order"
		if observed != expectedCount {
			status, detail = "failed", "rendered logical line count differs"
		}
		positionMatch := segmentOK && index < len(segment) && bytes.Equal(segment[index].Bytes, line.Bytes) && bytes.Equal(segment[index].Terminator, line.Terminator)
		if bytes.HasPrefix(line.Bytes, []byte("spaces-nine:")) && segmentOK && index < len(segment) {
			positionMatch = bytes.Equal(segment[index].Bytes, bytes.TrimRight(line.Bytes, " ")) && bytes.Equal(segment[index].Terminator, line.Terminator)
		}
		if !positionMatch {
			status, detail = "failed", "rendered records are out of order or differ"
		}
		if segmentOK && index < len(segment) && segment[index].CrossRow {
			status, detail = "deferred", "rendered record crossed an explicit host row move"
		}
		result = append(result, payloadAssertion{Name: name, Status: status, ExpectedCount: expectedCount, ObservedCount: observed, Detail: detail})
	}
	var markerBytes []byte
	for _, line := range segment {
		markerBytes = append(markerBytes, line.Bytes...)
		markerBytes = append(markerBytes, line.Terminator...)
	}
	withoutNewlines := strings.NewReplacer("\r", "", "\n", "").Replace(string(markerBytes))
	for _, marker := range markers {
		observed := strings.Count(withoutNewlines, marker)
		status, detail := "passed", "marker count is exact in rendered sequence"
		if observed != 1 {
			status, detail = "failed", "marker count is not exactly one"
		}
		result = append(result, payloadAssertion{Name: marker, Status: status, ExpectedCount: 1, ObservedCount: observed, Detail: detail})
	}
	return result
}

func renderedMarkerSegment(lines []renderedHistoryLine, begin, end string) ([]renderedHistoryLine, bool) {
	start := -1
	for index, line := range lines {
		if bytes.Equal(line.Bytes, []byte(begin)) {
			start = index
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	for index := start + 1; index < len(lines); index++ {
		if bytes.Equal(lines[index].Bytes, []byte(end)) {
			return append([]renderedHistoryLine(nil), lines[start:index+1]...), true
		}
	}
	return nil, false
}

func countRenderedLine(lines []renderedHistoryLine, bytesValue, term []byte) int {
	count := 0
	for _, line := range lines {
		if bytes.Equal(line.Bytes, bytesValue) && bytes.Equal(line.Terminator, term) {
			count++
		}
	}
	return count
}

// assertAlternatePayload treats text written while the alternate buffer is
// active as deliberately non-history. The handoff markers are still required
// exactly once, while both alternate records must be absent from the live
// logical history. No row shape or content-based deduplication is used.
func assertAlternatePayload(raw []byte, markers ...string) []payloadAssertion {
	stream := parseHostRenderStream(raw, 0)
	var history []byte
	for _, line := range stream.Lines() {
		history = append(history, line.Bytes...)
		history = append(history, line.Terminator...)
	}
	printable := printableStream(history)
	withoutNewlines := strings.NewReplacer("\r", "", "\n", "").Replace(string(printable))
	result := make([]payloadAssertion, 0, len(markers)+2)
	for _, marker := range markers {
		observed := strings.Count(withoutNewlines, marker)
		status := "passed"
		detail := "alternate handoff marker count is exact"
		if observed != 1 {
			status = "failed"
			detail = "alternate handoff marker count is not exactly one"
		}
		result = append(result, payloadAssertion{Name: marker, Status: status, ExpectedCount: 1, ObservedCount: observed, Detail: detail})
	}
	for _, record := range []string{"alternate-end", "alt-screen"} {
		observed := strings.Count(string(printable), record+"\r\n")
		status := "passed"
		detail := "alternate-buffer record is absent from primary history"
		if observed != 0 {
			status = "failed"
			detail = "alternate-buffer record leaked into primary history"
		}
		result = append(result, payloadAssertion{Name: record, Status: status, ExpectedCount: 0, ObservedCount: observed, Detail: detail})
	}
	return result
}

func stripCursorVisibilityWrapper(expected []byte) []byte {
	const hide = "\x1b[?25l"
	const show = "\x1b[?25h"
	if bytes.HasPrefix(expected, []byte(hide)) {
		expected = expected[len(hide):]
	}
	if bytes.HasSuffix(expected, []byte(show)) {
		expected = expected[:len(expected)-len(show)]
	}
	return expected
}

func assertControlPayload(raw []byte, markers ...string) []payloadAssertion {
	history := parseRenderedHistory(raw).Lines()
	result := make([]payloadAssertion, 0, 10)
	want := []struct {
		name string
		line []byte
	}{
		{"control-warmup", []byte("control-warmup")},
		{controlBeginMarker, []byte(controlBeginMarker)},
		{"red", []byte("red")},
		{"bold", []byte("bold")},
		{"under", []byte("under")},
		{"reverse", []byte("reverse")},
		{"rewritten", []byte("rewritten")},
		{"cursor", []byte("twosor: one")},
		{"tabs", append([]byte("tabs:"), append(bytes.Repeat([]byte{' '}, 3), append([]byte("X"), append(bytes.Repeat([]byte{' '}, 7), 'Y')...)...)...)},
		{"link", []byte("link")},
		{controlEndMarker, []byte(controlEndMarker)},
	}
	for _, item := range want {
		observed := 0
		crossRow := false
		for _, line := range history {
			if bytes.Equal(line.Bytes, item.line) {
				observed++
				crossRow = crossRow || line.CrossRow
			}
		}
		status := "passed"
		detail := "rendered logical line matches exact expected bytes"
		if observed != 1 {
			status = "failed"
			detail = "rendered logical line count differs"
		}
		if item.name == "tabs" && observed == 0 {
			for _, line := range history {
				if line.CrossRow {
					status = "deferred"
					detail = "tab line crossed an absolute host repaint row"
					break
				}
			}
		}
		if item.name == "link" && observed == 0 && bytes.Contains(raw, []byte("link")) {
			status = "deferred"
			detail = "hyperlink text crossed an absolute host row move; OSC 8 bytes are checked separately"
		}
		result = append(result, payloadAssertion{Name: item.name, Status: status, ExpectedCount: 1, ObservedCount: observed, Detail: detail})
		_ = crossRow
	}
	start, end := bytes.Index(raw, []byte(controlBeginMarker)), bytes.Index(raw, []byte(controlEndMarker))
	payload := raw
	if start >= 0 && end > start {
		payload = raw[start:end]
	}
	for _, item := range []struct {
		name string
		seq  []byte
	}{
		{"sgr-red", []byte("\x1b[31m")},
		{"sgr-default", []byte("\x1b[m")},
		{"sgr-bold", []byte("\x1b[1m")},
		{"sgr-bold-off", []byte("\x1b[22m")},
		{"sgr-underlined", []byte("\x1b[4m")},
		{"sgr-underlined-off", []byte("\x1b[24m")},
		{"sgr-reverse", []byte("\x1b[7m")},
		{"sgr-reverse-off", []byte("\x1b[27m")},
	} {
		observed := bytes.Count(payload, item.seq)
		status := "passed"
		detail := "host renderer emitted the source-defined SGR sequence"
		if observed != 1 {
			status = "failed"
			detail = "host renderer SGR sequence count differs"
		}
		result = append(result, payloadAssertion{Name: item.name, Status: status, ExpectedCount: 1, ObservedCount: observed, Detail: detail})
	}
	linkStart := bytes.Contains(payload, []byte("\x1b]8;id="))
	linkEnd := bytes.Contains(payload, []byte("\x1b]8;;\x1b\\"))
	linkStatus, linkDetail := "passed", "host renderer emitted OSC 8 with a process-local id and ST terminator"
	if !linkStart || !linkEnd {
		linkStatus, linkDetail = "failed", "host renderer OSC 8 sequence or ST terminator is absent"
	}
	result = append(result, payloadAssertion{Name: "osc8-st", Status: linkStatus, ExpectedCount: 2, ObservedCount: boolCount(linkStart) + boolCount(linkEnd), Detail: linkDetail})
	titleSequence := []byte("\x1b]0;pinned-conpty-probe\x07")
	// OSC title is emitted after the marked payload and is intentionally
	// out-of-band; count it in the complete host stream, not the payload slice.
	titleCount := bytes.Count(raw, titleSequence)
	titleStatus, titleDetail := "passed", "host emitted the expected out-of-band title sequence"
	if titleCount != 1 {
		titleStatus, titleDetail = "failed", "host title sequence count differs"
	}
	result = append(result, payloadAssertion{Name: "title-osc", Status: titleStatus, ExpectedCount: 1, ObservedCount: titleCount, Detail: titleDetail})
	_ = markers
	return result
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func assertionFailures(assertions []payloadAssertion) []string {
	var failures []string
	for _, assertion := range assertions {
		if assertion.Status != "passed" {
			failures = append(failures, fmt.Sprintf("%s=%s: %s", assertion.Name, assertion.Status, assertion.Detail))
		}
	}
	return failures
}
