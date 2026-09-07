//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type clearProbeReport struct {
	Mode            string             `json:"mode"`
	Host            pinnedHostIdentity `json:"host"`
	Command         string             `json:"command"`
	RawBytes        int                `json:"raw_bytes"`
	RawSHA256       string             `json:"raw_sha256"`
	ClearScrollback int                `json:"clear_scrollback_count"`
	BeginInHistory  int                `json:"begin_in_history"`
	EndInHistory    int                `json:"end_in_history"`
	ChildExited     bool               `json:"child_exited"`
	HostExited      bool               `json:"host_exited"`
	HandlesClosed   bool               `json:"handles_closed"`
	CompletedAt     time.Time          `json:"completed_at"`
}

// runNativeClearProbe validates the source-documented semantic clear event:
// ESC[3J must remove committed history, while output after Clear-Host remains.
func runNativeClearProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-clear.json")
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
	begin := "__PINNED_CONPTY_PROBE_CLEAR_BEGIN__"
	end := "__PINNED_CONPTY_PROBE_CLEAR_END__"
	command := fmt.Sprintf(`powershell.exe -NoLogo -NoProfile -NonInteractive -Command "$OutputEncoding=[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); Write-Output '%s'; Clear-Host; Write-Output '%s'"`, begin, end)
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 80, 25, false, nil, command, []string{begin, end})
	if runErr != nil && session.RawOutput == nil {
		return fmt.Errorf("clear probe session: %w", runErr)
	}
	raw := session.RawOutput
	history := parseRenderedHistory(raw).Lines()
	beginCount, endCount := 0, 0
	for _, line := range history {
		if bytes.Equal(line.Bytes, []byte(begin)) {
			beginCount++
		}
		if bytes.Equal(line.Bytes, []byte(end)) {
			endCount++
		}
	}
	report := clearProbeReport{
		Mode: "pinned-conpty-clear", Host: identity, Command: command,
		RawBytes: len(raw), RawSHA256: session.RawSHA256,
		ClearScrollback: bytes.Count(raw, []byte("\x1b[3J")),
		BeginInHistory:  beginCount, EndInHistory: endCount,
		ChildExited: session.ChildExited, HostExited: session.HostExited,
		HandlesClosed: session.HandlesClosed, CompletedAt: time.Now().UTC(),
	}
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "80x25.raw"), raw, session.RawSHA256); err != nil {
		return err
	}
	if runErr != nil {
		return runErr
	}
	if report.ClearScrollback != 1 || report.BeginInHistory != 0 || report.EndInHistory != 1 {
		return fmt.Errorf("clear probe assertion failed: ESC[3J=%d begin=%d end=%d", report.ClearScrollback, report.BeginInHistory, report.EndInHistory)
	}
	fmt.Printf("native clear probe complete: %s esc3j=%d begin=%d end=%d\n", reportPath, report.ClearScrollback, report.BeginInHistory, report.EndInHistory)
	return nil
}
