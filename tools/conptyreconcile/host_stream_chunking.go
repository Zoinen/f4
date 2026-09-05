package main

import (
	"bytes"
	"fmt"
)

type chunkingAssertion struct {
	Mode   string `json:"mode"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// verifyHostStreamChunking checks parser state at the boundaries that matter
// to this protocol: one-byte reads, fixed short reads, and a deterministic
// pseudo-random schedule.  It compares the explicit host CRLF lines and the
// diagnostic resize sequences; it never derives a line from a grid row.
func verifyHostStreamChunking(data []byte, seed uint64) ([]chunkingAssertion, error) {
	baseline := parseHostRenderStream(data, 0)
	var logicalBaseline logicalLineStream
	logicalBaseline.Feed(data)
	checks := []struct {
		name string
		feed func(*hostRenderStream)
	}{
		{name: "one-byte", feed: func(stream *hostRenderStream) {
			for i := range data {
				stream.Feed(data[i : i+1])
			}
		}},
		{name: "fixed-7", feed: func(stream *hostRenderStream) {
			for offset := 0; offset < len(data); {
				end := offset + 7
				if end > len(data) {
					end = len(data)
				}
				stream.Feed(data[offset:end])
				offset = end
			}
		}},
		{name: "prng", feed: func(stream *hostRenderStream) {
			state := seed | 1
			for offset := 0; offset < len(data); {
				state = state*6364136223846793005 + 1
				size := int((state>>32)%31) + 1
				end := offset + size
				if end > len(data) {
					end = len(data)
				}
				stream.Feed(data[offset:end])
				offset = end
			}
		}},
	}
	result := make([]chunkingAssertion, 0, len(checks))
	for _, check := range checks {
		got := hostRenderStream{}
		check.feed(&got)
		if !sameHostRenderStream(baseline, got) {
			result = append(result, chunkingAssertion{
				Mode: check.name, Status: "failed",
				Detail: fmt.Sprintf("chunk schedule changed explicit lines or frames (baseline lines=%d frames=%d, got lines=%d frames=%d)",
					len(baseline.Lines()), len(baseline.Frames()), len(got.Lines()), len(got.Frames())),
			})
			continue
		}
		result = append(result, chunkingAssertion{Mode: check.name, Status: "passed"})
	}
	for _, assertion := range result {
		if assertion.Status != "passed" {
			return result, fmt.Errorf("host stream chunking assertion %s failed: %s", assertion.Mode, assertion.Detail)
		}
	}
	logicalChecks := []struct {
		name string
		feed func(*logicalLineStream)
	}{
		{name: "logical-one-byte", feed: func(stream *logicalLineStream) {
			for i := range data {
				stream.Feed(data[i : i+1])
			}
		}},
		{name: "logical-fixed-7", feed: func(stream *logicalLineStream) {
			for offset := 0; offset < len(data); {
				end := offset + 7
				if end > len(data) {
					end = len(data)
				}
				stream.Feed(data[offset:end])
				offset = end
			}
		}},
		{name: "logical-prng", feed: func(stream *logicalLineStream) {
			state := seed | 1
			for offset := 0; offset < len(data); {
				state = state*6364136223846793005 + 1
				size := int((state>>32)%31) + 1
				end := offset + size
				if end > len(data) {
					end = len(data)
				}
				stream.Feed(data[offset:end])
				offset = end
			}
		}},
	}
	for _, check := range logicalChecks {
		got := logicalLineStream{}
		check.feed(&got)
		left, right := logicalBaseline.Lines(), got.Lines()
		status := "passed"
		detail := "logical lines are invariant under chunking"
		if len(left) != len(right) {
			status = "failed"
			detail = fmt.Sprintf("logical line count changed (baseline=%d got=%d)", len(left), len(right))
		} else {
			for i := range left {
				if !bytes.Equal(left[i].Bytes, right[i].Bytes) || !bytes.Equal(left[i].Terminator, right[i].Terminator) {
					status = "failed"
					detail = fmt.Sprintf("logical line %d changed under chunking", i)
					break
				}
			}
		}
		result = append(result, chunkingAssertion{Mode: check.name, Status: status, Detail: detail})
	}
	for _, assertion := range result[3:] {
		if assertion.Status != "passed" {
			return result, fmt.Errorf("logical stream chunking assertion %s failed: %s", assertion.Mode, assertion.Detail)
		}
	}
	return result, nil
}

func parseHostRenderStream(data []byte, _ int) hostRenderStream {
	stream := hostRenderStream{}
	stream.Feed(data)
	return stream
}

func sameHostRenderStream(left, right hostRenderStream) bool {
	leftLines, rightLines := left.Lines(), right.Lines()
	if len(leftLines) != len(rightLines) {
		return false
	}
	for i := range leftLines {
		if !bytes.Equal(leftLines[i].Bytes, rightLines[i].Bytes) || !bytes.Equal(leftLines[i].Terminator, rightLines[i].Terminator) {
			return false
		}
	}
	leftFrames, rightFrames := left.Frames(), right.Frames()
	if len(leftFrames) != len(rightFrames) {
		return false
	}
	for i := range leftFrames {
		if leftFrames[i] != rightFrames[i] {
			return false
		}
	}
	leftRepaints, rightRepaints := left.RepaintFrames(), right.RepaintFrames()
	if len(leftRepaints) != len(rightRepaints) {
		return false
	}
	for i := range leftRepaints {
		if leftRepaints[i] != rightRepaints[i] {
			return false
		}
	}
	return true
}
