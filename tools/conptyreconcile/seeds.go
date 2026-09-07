package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
)

type seedConsumerCheck struct {
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	HistorySHA256 string `json:"history_sha256"`
	ScreenSHA256  string `json:"screen_sha256"`
	SpilledPieces int    `json:"spilled_pieces"`
	Status        string `json:"status"`
}

// verifySeedConsumerChecks applies the consumer-only B/C operations to the
// logical records obtained from this native session. It never reconstructs a
// record from rows: rendered lines are already delimited by host CRLF and are
// stored whole before reflow/scrollback.
func verifySeedConsumerChecks(session nativeProbeSession, begin, end string) ([]seedConsumerCheck, error) {
	segment, ok := renderedMarkerSegment(session.RenderedLines, begin, end)
	if !ok {
		return nil, fmt.Errorf("seed markers do not delimit rendered history")
	}
	lines := make([]logicalLine, len(segment))
	for i, line := range segment {
		lines[i] = logicalLine{Bytes: append([]byte(nil), line.Bytes...), Terminator: append([]byte(nil), line.Terminator...)}
	}
	model := newConsumerScrollback(8)
	for _, line := range lines {
		model.Append(line)
	}
	baseline := model.historyBytes()
	checks := make([]seedConsumerCheck, 0, 5)
	for _, size := range [][2]int{{1, 1}, {79, 24}, {80, 25}, {121, 40}, {512, 25}} {
		before := rowsSHA256(model.visible(0, size[1], size[0]))
		_ = model.visible(1, size[1], size[0])
		_ = model.visible(0, size[1], size[0])
		after := rowsSHA256(model.visible(0, size[1], size[0]))
		status := "passed"
		if !bytes.Equal(model.historyBytes(), baseline) || before != after {
			status = "failed"
		}
		checks = append(checks, seedConsumerCheck{Width: size[0], Height: size[1], HistorySHA256: model.historySHA256(), ScreenSHA256: after, SpilledPieces: len(model.spilled.pieces), Status: status})
		if status != "passed" {
			return checks, fmt.Errorf("consumer history changed at %dx%d", size[0], size[1])
		}
	}
	if len(model.spilled.pieces) != maxInt(0, len(lines)-8) {
		return checks, fmt.Errorf("piece-table spill count=%d want=%d", len(model.spilled.pieces), maxInt(0, len(lines)-8))
	}
	return checks, nil
}

// seedWorkload is generated deterministically and contains no expected screen
// model. It is only the byte payload authored by the child process.
func seedWorkload(seed uint64, width int) []byte {
	if width < 1 {
		width = 80
	}
	rng := rand.New(rand.NewSource(int64(seed)))
	begin := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_BEGIN__", seed)
	end := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_END__", seed)
	var b strings.Builder
	// Keep cursor painting out of the deterministic seed so A/B/C assertions
	// measure stream and consumer behavior, not the host's documented visible
	// cursor wrap-state reset.
	b.WriteString("\x1b[?25l")
	b.WriteString(begin)
	b.WriteString("\r\n")
	b.WriteString(fmt.Sprintf("ascii: seed-%016x\r\n", seed))
	b.WriteString("edge: ")
	b.WriteString(strings.Repeat("X", maxInt(0, width-6)))
	b.WriteString("\r\n")
	b.WriteString("repeat: SAME\r\nrepeat: SAME\r\nrepeat: SAME\r\n")
	b.WriteString("unicode: 漢字 e\u0301 😀 👩‍💻 אבג العربية\r\n")
	b.WriteString("long: ")
	b.WriteString(strings.Repeat("C", width+177+rng.Intn(83)))
	b.WriteString("\r\n")
	b.WriteString(end)
	b.WriteString("\r\n")
	b.WriteString("\x1b[?25h")
	return []byte(b.String())
}
