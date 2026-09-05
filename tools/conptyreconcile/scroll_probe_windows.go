//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type scrollCheckpoint struct {
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Offset        int    `json:"offset"`
	VisibleRows   int    `json:"visible_rows"`
	VisibleSHA256 string `json:"visible_sha256"`
	HistorySHA256 string `json:"history_sha256"`
	Status        string `json:"status"`
}

type nativeScrollReport struct {
	Mode             string             `json:"mode"`
	Host             pinnedHostIdentity `json:"host"`
	Session          nativeProbeSession `json:"session"`
	ExpectedLines    int                `json:"expected_lines"`
	ObservedLines    int                `json:"observed_lines"`
	SpilledPieces    int                `json:"spilled_pieces"`
	SpilledBytes     int                `json:"spilled_bytes"`
	HistorySHA256    string             `json:"history_sha256"`
	EvictionBoundary bool               `json:"eviction_boundary_preserved"`
	Checkpoints      []scrollCheckpoint `json:"checkpoints"`
	CompletedAt      time.Time          `json:"completed_at"`
}

func runNativeScrollProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-scroll.json")
	}
	resolved, err := ensureProbeHost(hostPath)
	if err != nil {
		return err
	}
	identity, err := verifyPinnedHost(resolved)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	width, height := 512, 8
	workload := scrollProbeWorkload()
	command := fmt.Sprintf(`"%s" -emit-scroll`, executable)
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, width, height, false, workload, command, []string{scrollBeginMarker, scrollEndMarker})
	report := nativeScrollReport{Mode: "pinned-conpty-scroll", Host: identity, Session: session}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "512x8.raw"), session.RawOutput, session.RawSHA256); err != nil {
		return err
	}
	if runErr != nil {
		report.CompletedAt = time.Now().UTC()
		_ = writeJSON(reportPath, report)
		return fmt.Errorf("native scroll source session: %w", runErr)
	}
	rendered := parseRenderedHistoryAtWidth(session.RawOutput, width).Lines()
	segment, ok := renderedMarkerSegment(rendered, scrollBeginMarker, scrollEndMarker)
	if !ok || len(segment) < 2 {
		return fmt.Errorf("scroll markers did not delimit rendered output")
	}
	expectedStream := logicalLineStream{}
	expectedStream.Feed(stripCursorVisibilityWrapper(workload))
	expected := expectedStream.Lines()
	if len(expected) < 2 {
		return fmt.Errorf("scroll workload has no marked lines")
	}
	expected = expected[1 : len(expected)-1]
	observed := make([]logicalLine, 0, len(segment)-2)
	for _, line := range segment[1 : len(segment)-1] {
		observed = append(observed, logicalLine{Bytes: append([]byte(nil), line.Bytes...), Terminator: append([]byte(nil), line.Terminator...)})
	}
	report.ExpectedLines, report.ObservedLines = len(expected), len(observed)
	if len(expected) != len(observed) {
		return writeScrollFailure(reportPath, report, fmt.Errorf("line count mismatch: got %d want %d", len(observed), len(expected)))
	}
	for i := range expected {
		if !bytes.Equal(expected[i].Bytes, observed[i].Bytes) {
			return writeScrollFailure(reportPath, report, fmt.Errorf("line %d differs", i))
		}
	}
	model := newConsumerScrollback(8)
	for _, line := range observed {
		model.Append(line)
	}
	report.SpilledPieces = len(model.spilled.pieces)
	report.SpilledBytes = len(model.spilled.Bytes())
	report.HistorySHA256 = model.historySHA256()
	for _, line := range model.spilled.pieces {
		if bytes.Contains(line, []byte("eviction-boundary: "+string(bytes.Repeat([]byte{'E'}, 257)))) {
			report.EvictionBoundary = true
		}
	}
	if !report.EvictionBoundary {
		return writeScrollFailure(reportPath, report, fmt.Errorf("eviction boundary line not preserved in piece table"))
	}
	canonical := model.historyBytes()
	for _, dimensions := range [][3]int{{512, 8, 0}, {80, 8, 0}, {20, 8, 4}, {121, 12, 9}, {7, 5, 0}, {80, 8, 6}, {20, 8, 0}} {
		w, h, offset := dimensions[0], dimensions[1], dimensions[2]
		rows := model.visible(offset, h, w)
		status := "passed"
		if !bytes.Equal(model.historyBytes(), canonical) {
			status = "failed"
		}
		report.Checkpoints = append(report.Checkpoints, scrollCheckpoint{Width: w, Height: h, Offset: offset, VisibleRows: len(rows), VisibleSHA256: rowsSHA256(rows), HistorySHA256: model.historySHA256(), Status: status})
		if status != "passed" {
			return writeScrollFailure(reportPath, report, fmt.Errorf("history changed at width=%d offset=%d", w, offset))
		}
	}
	report.CompletedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	fmt.Printf("native scrollback probe complete: %s lines=%d spilled=%d history_sha256=%s\n", reportPath, report.ObservedLines, report.SpilledPieces, report.HistorySHA256)
	return nil
}

func writeScrollFailure(path string, report nativeScrollReport, err error) error {
	report.CompletedAt = time.Now().UTC()
	_ = writeJSON(path, report)
	return err
}
