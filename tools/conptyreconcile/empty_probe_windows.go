//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type emptyProbeReport struct {
	Mode          string             `json:"mode"`
	Host          pinnedHostIdentity `json:"host"`
	Session       nativeProbeSession `json:"session"`
	RawBytes      int                `json:"raw_bytes"`
	RawSHA256     string             `json:"raw_sha256"`
	RenderedLines int                `json:"rendered_lines"`
	Empty         bool               `json:"empty"`
	ChildExited   bool               `json:"child_exited"`
	HostExited    bool               `json:"host_exited"`
	HandlesClosed bool               `json:"handles_closed"`
	CompletedAt   time.Time          `json:"completed_at"`
}

// runNativeEmptyProbe starts a child that produces no output. It checks the
// host's quick-return path rather than treating startup controls as a frame.
func runNativeEmptyProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-empty.json")
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
	command := `cmd.exe /d /q /c exit /b 0`
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 80, 25, false, nil, command, nil)
	report := emptyProbeReport{Mode: "pinned-conpty-empty", Host: identity, Session: session, RawBytes: len(session.RawOutput), RawSHA256: session.RawSHA256, ChildExited: session.ChildExited, HostExited: session.HostExited, HandlesClosed: session.HandlesClosed, CompletedAt: time.Now().UTC()}
	report.RenderedLines = len(parseRenderedHistory(session.RawOutput).Lines())
	// Startup controls are out-of-band and do not constitute an empty frame.
	report.Empty = report.RenderedLines == 0
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "80x25.raw"), session.RawOutput, session.RawSHA256); err != nil {
		return err
	}
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	if runErr != nil {
		return fmt.Errorf("empty probe session: %w", runErr)
	}
	if !report.Empty {
		return fmt.Errorf("empty child rendered %d logical lines", report.RenderedLines)
	}
	fmt.Printf("native empty probe complete: %s raw_bytes=%d rendered_lines=%d child=%t host=%t handles=%t\n", reportPath, report.RawBytes, report.RenderedLines, report.ChildExited, report.HostExited, report.HandlesClosed)
	return nil
}
