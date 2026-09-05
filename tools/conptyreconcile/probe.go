package main

import (
	"os"
	"strings"
	"time"
)

const (
	probeBeginMarker     = "__PINNED_CONPTY_PROBE_BEGIN__"
	probeEndMarker       = "__PINNED_CONPTY_PROBE_END__"
	controlBeginMarker   = "__PINNED_CONPTY_PROBE_CONTROL_BEGIN__"
	controlEndMarker     = "__PINNED_CONPTY_PROBE_CONTROL_END__"
	alternateBeginMarker = "__PINNED_CONPTY_PROBE_ALT_BEGIN__"
	alternateEndMarker   = "__PINNED_CONPTY_PROBE_ALT_END__"
	scrollBeginMarker    = "__PINNED_CONPTY_PROBE_SCROLL_BEGIN__"
	scrollEndMarker      = "__PINNED_CONPTY_PROBE_SCROLL_END__"
	edgeBeginMarker      = "__PINNED_CONPTY_PROBE_EDGE_BEGIN__"
	edgeEndMarker        = "__PINNED_CONPTY_PROBE_EDGE_END__"
	quirkBeginMarker     = "__PINNED_CONPTY_PROBE_QUIRK_BEGIN__"
	quirkEndMarker       = "__PINNED_CONPTY_PROBE_QUIRK_END__"
)

// probeWorkload exercises host operations any terminal must handle. Logical
// records are delimited by the explicit CRLF bytes authored here; no display
// row is used as a line boundary.
func probeWorkload() string { return probeWorkloadForWidth(80) }

func partialProbeWorkload(width int) []byte {
	if width < 1 {
		width = 80
	}
	return []byte("partial: " + strings.Repeat("P", width+40) + "\r\n")
}

func probeWorkloadForWidth(width int) string {
	if width < 1 {
		width = 80
	}
	var b strings.Builder
	// Keep the cursor hidden while the long-line payload is rendered. The
	// pinned host then does not invoke PaintCursor, isolating line wrapping
	// from the cursor-side state reset documented in PINNED_HOST_FACTS.md:11.
	b.WriteString("\x1b[?25l")
	b.WriteString(probeBeginMarker)
	b.WriteString("\r\n")
	// Keep the principal long-line proof at the top of the buffer, before
	// cursor/rewrite and alternate-screen operations can trigger repaint.
	b.WriteString("long: ")
	b.WriteString(strings.Repeat("C", 257))
	b.WriteString("\r\n")
	b.WriteString("ascii: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\r\n")
	for _, item := range []struct {
		name  string
		count int
		value byte
	}{
		{"exact-n-minus-1", width - 1, 'N'},
		{"exact-n", width, 'N'},
		{"exact-n-plus-1", width + 1, 'N'},
		{"exact-2n-plus-1", 2*width + 1, 'N'},
	} {
		b.WriteString(item.name)
		b.WriteString(": ")
		prefix := len(item.name) + 2
		b.WriteString(strings.Repeat(string(item.value), maxInt(0, item.count-prefix)))
		b.WriteString("\r\n")
	}
	b.WriteString("width-edge: ")
	b.WriteString(strings.Repeat("B", width))
	b.WriteString("\r\n")
	b.WriteString("repeat-char: ")
	b.WriteString(strings.Repeat("R", 97))
	b.WriteString("\r\n")
	b.WriteString("alternating: ")
	for i := 0; i < 129; i++ {
		if i%2 == 0 {
			b.WriteByte('0')
		} else {
			b.WriteByte('1')
		}
	}
	b.WriteString("\r\n")
	b.WriteString("spaces:       \r\n")
	b.WriteString("spaces-eight:        \r\n")
	b.WriteString("spaces-nine:         \r\n")
	b.WriteString("empty:\r\n")
	b.WriteString("unicode: 漢字 e\u0301 ☕️ 😀 👩‍💻 אבג العربية\r\n")
	// Repeated identical records catch accidental line coalescing, loss, or
	// deduplication in the terminal history path.
	b.WriteString("repeat: SAME\r\n")
	b.WriteString("repeat: SAME\r\n")
	b.WriteString("repeat: SAME\r\n")
	b.WriteString(probeEndMarker)
	b.WriteString("\r\n")
	b.WriteString("\x1b[?25h")
	return b.String()
}

func controlProbeWorkload() string {
	var b strings.Builder
	// Keep cursor painting out of this semantic-control phase. The control
	// assertions target the rendered effect of SGR/erase/CUP/tabs/title; the
	// cursor-side wrap interaction is covered separately by the long-line
	// visible-cursor diagnostic.
	b.WriteString("\x1b[?25l")
	b.WriteString("control-warmup\r\n")
	b.WriteString(controlBeginMarker)
	b.WriteString("\r\n")
	b.WriteString("\x1b[31mred\x1b[0m\r\n")
	b.WriteString("\x1b[1mbold\x1b[22m\r\n")
	b.WriteString("\x1b[4munder\x1b[24m\r\n")
	b.WriteString("\x1b[7mreverse\x1b[27m\r\n")
	b.WriteString("old\x1b[2K\x1b[1Grewritten\r\n")
	b.WriteString("cursor: one\x1b[1Gtwo\r\n")
	// A second explicit newline gives the host a fresh row after its tab-stop
	// cursor reposition, so the tab expansion can be checked as a complete
	// rendered line without inferring a boundary from the CUP itself.
	b.WriteString("tabs:\tX\tY\r\n\r\n")
	b.WriteString("\x1b]8;;https://example.test\x1b\\link\x1b]8;;\x1b\\\r\n")
	b.WriteString("\x1b]0;pinned-conpty-probe\x07\r\n")
	b.WriteString(controlEndMarker)
	b.WriteString("\r\n")
	b.WriteString("\x1b[?25h")
	// Keep the initial screen erase outside the marked control payload: it can
	// itself trigger an absolute repaint before the marker-bearing phase ends.
	b.WriteString("\x1b[2J\x1b[H")
	return b.String()
}

func scrollProbeWorkload() []byte {
	var b strings.Builder
	b.WriteString("\x1b[?25l")
	b.WriteString(scrollBeginMarker)
	b.WriteString("\r\n")
	for i := 0; i < 4; i++ {
		b.WriteString("before-")
		b.WriteString(string(rune('0' + i)))
		b.WriteString("\r\n")
	}
	b.WriteString("eviction-boundary: ")
	b.WriteString(strings.Repeat("E", 257))
	b.WriteString("\r\n")
	for i := 0; i < 32; i++ {
		b.WriteString("scroll-line-")
		b.WriteString(string(rune('A' + i%26)))
		b.WriteString(" ")
		b.WriteString(strings.Repeat(string(rune('a'+i%26)), 23))
		b.WriteString("\r\n")
	}
	b.WriteString(scrollEndMarker)
	b.WriteString("\r\n\x1b[?25h")
	return []byte(b.String())
}

func emitScrollWorkload() error {
	_, err := os.Stdout.Write(scrollProbeWorkload())
	return err
}

func emitSemanticWorkload(kind string) error {
	_, err := os.Stdout.Write([]byte(semanticProbeWorkload(kind, "__PINNED_CONPTY_PROBE_SEMANTIC_BEGIN__", "__PINNED_CONPTY_PROBE_SEMANTIC_END__")))
	return err
}

func edgeProbeWorkload() []byte {
	var b strings.Builder
	b.WriteString("\x1b[?25l")
	b.WriteString(edgeBeginMarker)
	b.WriteString("\r\n")
	b.WriteString("spaces-eight-top:        \r\n")
	b.WriteString("spaces-nine-top:         \r\n")
	for i := 0; i < 22; i++ {
		b.WriteString("edge-filler-")
		b.WriteString(string(rune('A' + i)))
		b.WriteString("\r\n")
	}
	b.WriteString("spaces-eight-bottom:        \r\n")
	b.WriteString("spaces-nine-bottom:         \r\n")
	b.WriteString("blink: ")
	b.WriteString("\x1b[?12h")
	b.WriteString("visible")
	b.WriteString("\x1b[?12l\r\n")
	b.WriteString("\x1b[?7l")
	b.WriteString("nowrap: ")
	b.WriteString(strings.Repeat("W", 257))
	b.WriteString("\r\n\x1b[?7h")
	b.WriteString(edgeEndMarker)
	b.WriteString("\r\n\x1b[?25h")
	return []byte(b.String())
}

func emitEdgeWorkload() error {
	input := edgeProbeWorkload()
	// Let the host paint the two top-of-viewport threshold records before the
	// later filler scrolls them out. Without this synchronization a single
	// child write can leave those rows unpainted, which cannot test the
	// eight-vs-nine ECH threshold at all.
	cut := strings.Index(string(input), "edge-filler-A")
	if cut < 0 {
		cut = len(input)
	}
	if _, err := os.Stdout.Write(input[:cut]); err != nil {
		return err
	}
	time.Sleep(100 * time.Millisecond)
	_, err := os.Stdout.Write(input[cut:])
	return err
}

func quirkProbeWorkload() []byte {
	var b strings.Builder
	b.WriteString(quirkBeginMarker)
	b.WriteString("\r\n")
	for i := 0; i < 120; i++ {
		b.WriteString("quirk-line-")
		b.WriteString(string(rune('A' + i%26)))
		b.WriteString(" ")
		b.WriteString(strings.Repeat("Q", 96))
		b.WriteString("\r\n")
	}
	b.WriteString(quirkEndMarker)
	b.WriteString("\r\n")
	return []byte(b.String())
}

// alternateProbeWorkload is a separate phase because leaving the alternate
// buffer causes the host to repaint the primary buffer.  Keeping that repaint
// out of the primary payload makes the exact-line assertion about history,
// rather than about a frame that the source explicitly describes as redraw.
func alternateProbeWorkload(width int) string {
	if width < 1 {
		width = 80
	}
	var b strings.Builder
	b.WriteString("\x1b[?1049halt-screen\r\n")
	b.WriteString("alternate-end\r\n\x1b[?1049l\r\n")
	// Emit the stable handoff markers after the alternate buffer is restored;
	// the repaint caused by leaving it must not swallow new history markers.
	b.WriteString(alternateBeginMarker)
	b.WriteString("\r\n")
	b.WriteString(alternateEndMarker)
	b.WriteString("\r\n")
	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func probeExpectedMarkers() []string {
	// Alternate-screen contents are intentionally not required here: ConPTY
	// restores the primary screen and may legitimately omit text written while
	// the alternate buffer was active. The outer markers survive repaint and
	// are the stable handoff contract for this probe.
	return []string{probeBeginMarker, probeEndMarker}
}

func controlExpectedMarkers() []string {
	return []string{controlBeginMarker, controlEndMarker}
}

func alternateExpectedMarkers() []string {
	return []string{alternateBeginMarker, alternateEndMarker}
}

func probeOutputContainsMarker(output []byte, marker string) bool {
	return strings.Contains(string(output), marker)
}
