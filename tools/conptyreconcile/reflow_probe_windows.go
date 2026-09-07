//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type reflowCheckpoint struct {
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	StoredLines      int    `json:"stored_lines"`
	DisplayRows      int    `json:"display_rows"`
	VisibleRows      int    `json:"visible_rows"`
	ScreenSHA256     string `json:"screen_sha256"`
	CursorRow        int    `json:"cursor_row"`
	CursorColumn     int    `json:"cursor_column"`
	StoredHistorySHA string `json:"stored_history_sha256"`
	LayoutStatus     string `json:"layout_status"`
}

type nativeReflowReport struct {
	Mode          string             `json:"mode"`
	Host          pinnedHostIdentity `json:"host"`
	ExpectedInput []byte             `json:"expected_input"`
	Session       nativeProbeSession `json:"session"`
	Checkpoints   []reflowCheckpoint `json:"checkpoints"`
	InitialA      []payloadAssertion `json:"initial_a"`
	CompletedAt   time.Time          `json:"completed_at"`
}

func runNativeReflowProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-reflow.json")
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
	workload := reflowProbeWorkload(80)
	command := fmt.Sprintf(`"%s" -emit-reflow -emit-probe-width 80`, executable)
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 80, 80, false, workload, command, []string{probeBeginMarker, probeEndMarker})
	if runErr != nil {
		return fmt.Errorf("native reflow source session: %w", runErr)
	}
	lines := authoredReflowLines(workload)
	checkpoints := makeReflowCheckpoints(lines)
	report := nativeReflowReport{
		Mode: "pinned-conpty-reflow", Host: identity, ExpectedInput: workload,
		Session: session, Checkpoints: checkpoints,
		InitialA:    assertStaticPayload(workload, session.RawOutput, probeBeginMarker, probeEndMarker),
		CompletedAt: time.Now().UTC(),
	}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "80x80.raw"), session.RawOutput, session.RawSHA256); err != nil {
		return err
	}
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	for _, assertion := range report.InitialA {
		if assertion.Status == "failed" {
			return fmt.Errorf("native reflow source history failed: %s", assertion.Name)
		}
	}
	fmt.Printf("native consumer reflow probe complete: %s\n", reportPath)
	return nil
}

func authoredReflowLines(workload []byte) []logicalLine {
	var stream logicalLineStream
	stream.Feed(stripCursorVisibilityWrapper(workload))
	return stream.Lines()
}

func makeReflowCheckpoints(lines []logicalLine) []reflowCheckpoint {
	stored := joinLogicalLineBytes(lines)
	hash := sha256.Sum256(stored)
	historySHA := hex.EncodeToString(hash[:])
	checkpoints := make([]reflowCheckpoint, 0, 9)
	for _, dimensions := range [][2]int{{1, 1}, {79, 24}, {80, 25}, {81, 26}, {121, 40}, {20, 10}, {121, 10}, {1, 25}, {80, 25}} {
		rows := reflowLogicalLines(lines, dimensions[0])
		screen := screenRows(rows, dimensions[0], dimensions[1])
		cursorRow, cursorColumn := cursorPosition(lines, dimensions[0])
		status := "passed"
		if !rowsRoundTrip(rows, stored) || len(screen) != dimensions[1] || cursorRow < 0 || cursorRow > len(rows) || cursorColumn < 0 || cursorColumn > dimensions[0] {
			status = "failed"
		}
		checkpoints = append(checkpoints, reflowCheckpoint{
			Width: dimensions[0], Height: dimensions[1], StoredLines: len(lines),
			DisplayRows: len(rows), VisibleRows: minInt(len(rows), dimensions[1]),
			StoredHistorySHA: historySHA, LayoutStatus: status,
			ScreenSHA256: rowsSHA256(screen), CursorRow: cursorRow, CursorColumn: cursorColumn,
		})
	}
	return checkpoints
}

func joinLogicalLineBytes(lines []logicalLine) []byte {
	var result []byte
	for _, line := range lines {
		result = append(result, line.Bytes...)
		result = append(result, line.Terminator...)
	}
	return result
}

func rowsRoundTrip(rows [][]byte, stored []byte) bool {
	var joined []byte
	for _, row := range rows {
		joined = append(joined, row...)
	}
	withoutTerminators := bytes.ReplaceAll(stored, []byte("\r\n"), nil)
	withoutTerminators = bytes.ReplaceAll(withoutTerminators, []byte("\n"), nil)
	return bytes.Equal(joined, withoutTerminators)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
