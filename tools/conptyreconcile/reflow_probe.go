package main

import "bytes"

// reflowProbeWorkload establishes the native history once. It deliberately
// contains no host-resize operation: B1-0 changes only the consumer's display
// width after this static pinned-host session has supplied whole lines.
func reflowProbeWorkload(width int) []byte {
	if width < 1 {
		width = 80
	}
	return []byte("\x1b[?25l" + probeBeginMarker + "\r\n" +
		"reflow-long: " + string(bytes.Repeat([]byte{'Q'}, width*3+17)) + "\r\n" +
		"reflow-exact: " + string(bytes.Repeat([]byte{'E'}, width-14)) + "\r\n" +
		probeEndMarker + "\r\n\x1b[?25h")
}
