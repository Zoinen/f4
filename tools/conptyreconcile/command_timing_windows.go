//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type commandTimingSession struct {
	Height       int     `json:"height"`
	ElapsedMS    float64 `json:"elapsed_ms"`
	HostRawBytes int     `json:"host_raw_bytes"`
	ChildExited  bool    `json:"child_exited"`
	ExitCode     uint32  `json:"child_exit_code"`
}

type commandTimingReport struct {
	Mode          string                 `json:"mode"`
	Host          pinnedHostIdentity     `json:"host"`
	SessionWidth  int                    `json:"session_width"`
	Command       string                 `json:"command"`
	RedirectedMS  float64                `json:"redirected_ms"`
	RedirectedLen int                    `json:"redirected_bytes"`
	Sessions      []commandTimingSession `json:"sessions"`
	CompletedAt   time.Time              `json:"completed_at"`
}

// runNativeCommandTiming measures what the pinned host costs (idea 1.1 of
// unxed/f4#1681): the same recursive dir once redirected to a file and once
// through the host at several session heights (idea 1.2: the host forces a
// paint for every scrolled-out row once the buffer height is full). It only
// measures; nothing is compared and no gate depends on it.
func runNativeCommandTiming(hostPath, reportPath string, width int, heights []int) error {
	if width < 1 || len(heights) == 0 {
		return fmt.Errorf("command timing needs a positive width and at least one height")
	}
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-command-timing.json")
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
	root := `C:\Windows\System32`
	begin := "__PINNED_CONPTY_PROBE_DIR_TIMING_BEGIN__"
	end := "__PINNED_CONPTY_PROBE_DIR_TIMING_END__"
	command := fmt.Sprintf(`cmd.exe /d /q /c "chcp 65001 >nul & echo %s & set DIRCMD= & dir /s /b %s & echo %s & exit /b 0"`, begin, root, end)
	report := commandTimingReport{Mode: "pinned-conpty-command-timing", Host: identity, SessionWidth: width, Command: command}

	started := time.Now()
	redirected, err := runRedirectedDir(root, reportPath+".redirected.raw")
	if err != nil {
		return err
	}
	report.RedirectedMS = float64(time.Since(started).Microseconds()) / 1000
	report.RedirectedLen = len(redirected)

	for _, height := range heights {
		started = time.Now()
		session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, width, height, false, nil, command, []string{begin, end})
		elapsed := time.Since(started)
		if runErr != nil {
			return fmt.Errorf("pinned command timing session at height %d: %w", height, runErr)
		}
		report.Sessions = append(report.Sessions, commandTimingSession{
			Height: height, ElapsedMS: float64(elapsed.Microseconds()) / 1000, HostRawBytes: len(session.RawOutput),
			ChildExited: session.ChildExited, ExitCode: session.ExitCode,
		})
		fmt.Printf("native command timing: width=%d height=%d elapsed_ms=%.1f raw_bytes=%d\n", width, height, float64(elapsed.Microseconds())/1000, len(session.RawOutput))
	}
	report.CompletedAt = time.Now().UTC()
	fmt.Printf("native command timing: redirected_ms=%.1f redirected_bytes=%d\n", report.RedirectedMS, report.RedirectedLen)
	return writeJSON(reportPath, report)
}
