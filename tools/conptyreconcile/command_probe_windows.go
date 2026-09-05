//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

type commandProbeMeasurement struct {
	Width            int    `json:"width"`
	Height           int    `json:"height"`
	Command          string `json:"command"`
	RawBytes         int    `json:"raw_bytes"`
	RawCRLF          int    `json:"raw_crlf"`
	CUPBeforeCRLF    int    `json:"cup_before_crlf"`
	RenderedLines    int    `json:"rendered_lines"`
	RenderedCrossRow int    `json:"rendered_cross_row"`
	MarkerCount      int    `json:"marker_count"`
	ChildExitCode    uint32 `json:"child_exit_code"`
	ChildExited      bool   `json:"child_exited"`
	HostExited       bool   `json:"host_exited"`
	HandlesClosed    bool   `json:"handles_closed"`
	ResizeOffsets    []int  `json:"resize_offsets,omitempty"`
	RawSHA256        string `json:"raw_sha256"`
	ExpectedMarker   string `json:"expected_marker"`
	MarkerFound      bool   `json:"marker_found"`
}

type commandProbeReport struct {
	Mode         string                    `json:"mode"`
	Host         pinnedHostIdentity        `json:"host"`
	Root         string                    `json:"root"`
	Measurements []commandProbeMeasurement `json:"measurements"`
	CompletedAt  time.Time                 `json:"completed_at"`
}

// runNativeCommandProbe is diagnostic evidence for the real-command branch.
// It never turns CUP/CRLF patterns into history boundaries; those counts only
// quantify how often the pinned host's cursor paint exposes the limitation.
func runNativeCommandProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-command.json")
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
	// System32 is a stable, locally available tree and avoids making the
	// measurement depend on an untracked fixture or a user-specific path.
	root := `C:\Windows\System32`
	report := commandProbeReport{Mode: "pinned-conpty-command", Host: identity, Root: root}
	for _, dimensions := range [][2]int{{80, 25}, {20, 10}} {
		marker := fmt.Sprintf("__PINNED_CONPTY_PROBE_DIR_%dx%d_END__", dimensions[0], dimensions[1])
		command := fmt.Sprintf(`cmd.exe /d /q /c "set DIRCMD= & dir /s /b %s & echo %s & exit /b 0"`, root, marker)
		session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, dimensions[0], dimensions[1], true, nil, command, []string{marker})
		if runErr != nil {
			return fmt.Errorf("command probe %dx%d: %w", dimensions[0], dimensions[1], runErr)
		}
		measurement := measureCommandOutput(session, dimensions[0], dimensions[1], command, marker)
		report.Measurements = append(report.Measurements, measurement)
		artifactDir := reportPath + ".sessions"
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return err
		}
		if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, fmt.Sprintf("%dx%d.raw", dimensions[0], dimensions[1])), session.RawOutput, session.RawSHA256); err != nil {
			return err
		}
	}
	report.CompletedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	fmt.Printf("native command probe complete: %s\n", reportPath)
	return nil
}

var cupPattern = regexp.MustCompile("\x1b\\[[0-9]+;[0-9]+H\r\n")

func measureCommandOutput(session nativeProbeSession, width, height int, command, marker string) commandProbeMeasurement {
	raw := session.RawOutput
	sha := sha256.Sum256(raw)
	measurement := commandProbeMeasurement{
		Width: width, Height: height, Command: command, RawBytes: len(raw),
		RawCRLF: bytes.Count(raw, []byte("\r\n")), CUPBeforeCRLF: len(cupPattern.FindAll(raw, -1)),
		RenderedLines: len(session.RenderedLines), ExpectedMarker: marker,
		MarkerCount: bytes.Count(raw, []byte(marker)), ChildExitCode: session.ExitCode,
		ChildExited: session.ChildExited, HostExited: session.HostExited,
		HandlesClosed: session.HandlesClosed, ResizeOffsets: append([]int(nil), session.ResizeOffsets...),
		RawSHA256: hex.EncodeToString(sha[:]), MarkerFound: bytes.Contains(raw, []byte(marker)),
	}
	for _, line := range session.RenderedLines {
		if line.CrossRow {
			measurement.RenderedCrossRow++
		}
	}
	return measurement
}
