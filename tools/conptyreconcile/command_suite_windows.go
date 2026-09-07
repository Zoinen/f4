//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type commandSuiteCase struct {
	Name     string   `json:"name"`
	Command  string   `json:"command"`
	Expected []string `json:"expected"`
	Observed []string `json:"observed"`
	Exact    bool     `json:"exact"`
	ExitCode uint32   `json:"exit_code"`
	Child    bool     `json:"child_exited"`
	Host     bool     `json:"host_exited"`
	Handles  bool     `json:"handles_closed"`
	RawSHA   string   `json:"raw_sha256"`
	Error    string   `json:"error,omitempty"`
}

type commandSuiteReport struct {
	Mode        string             `json:"mode"`
	Host        pinnedHostIdentity `json:"host"`
	Width       int                `json:"width"`
	Cases       []commandSuiteCase `json:"cases"`
	CompletedAt time.Time          `json:"completed_at"`
}

func runNativeCommandSuite(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-command-suite.json")
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
	fixture := filepath.Join(os.TempDir(), fmt.Sprintf("pinned-conpty-suite-%d.txt", time.Now().UnixNano()))
	if err := os.WriteFile(fixture, []byte("suite-type-value\r\n"), 0o600); err != nil {
		return err
	}
	defer os.Remove(fixture)
	begin := "__PINNED_CONPTY_PROBE_SUITE_BEGIN__"
	end := "__PINNED_CONPTY_PROBE_SUITE_END__"
	typePath := fixture
	cases := []struct {
		name     string
		command  string
		expected []string
	}{
		{"echo", fmt.Sprintf(`cmd.exe /d /q /c "chcp 65001 >nul & echo %s & echo suite-echo-value & echo %s & exit /b 0"`, begin, end), []string{begin, "suite-echo-value", end}},
		{"type", fmt.Sprintf(`cmd.exe /d /q /c "chcp 65001 >nul & echo %s & type %s & echo %s & exit /b 0"`, begin, typePath, end), []string{begin, "suite-type-value", end}},
		{"findstr", fmt.Sprintf(`cmd.exe /d /q /c "chcp 65001 >nul & echo %s & echo suite-findstr-value | findstr suite-findstr-value & echo %s & exit /b 0"`, begin, end), []string{begin, "suite-findstr-value", end}},
		{"powershell", fmt.Sprintf(`powershell.exe -NoLogo -NoProfile -NonInteractive -Command "$OutputEncoding=[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); Write-Output '%s'; Write-Output 'suite-powershell-value'; Write-Output '%s'"`, begin, end), []string{begin, "suite-powershell-value", end}},
	}
	report := commandSuiteReport{Mode: "pinned-conpty-command-suite", Host: identity, Width: 512}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	for _, item := range cases {
		session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 512, 25, false, nil, item.command, []string{begin, end})
		observed := []string{}
		rendered := parseRenderedHistoryAtWidth(session.RawOutput, 512).Lines()
		if segment, ok := renderedMarkerSegmentTrimmed(rendered, begin, end); ok {
			for _, line := range segment {
				observed = append(observed, string(bytes.TrimRight(line.Bytes, " ")))
			}
		}
		caseReport := commandSuiteCase{Name: item.name, Command: item.command, Expected: item.expected, Observed: observed, Exact: equalStrings(observed, item.expected), ExitCode: session.ExitCode, Child: session.ChildExited, Host: session.HostExited, Handles: session.HandlesClosed, RawSHA: session.RawSHA256}
		if runErr != nil {
			caseReport.Error = runErr.Error()
		}
		report.Cases = append(report.Cases, caseReport)
		if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, item.name+"-512x25.raw"), session.RawOutput, session.RawSHA256); err != nil {
			return err
		}
	}
	report.CompletedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	for _, item := range report.Cases {
		if !item.Exact || item.ExitCode != 0 || !item.Child || !item.Host || !item.Handles {
			return fmt.Errorf("command suite case %s failed exact=%t exit=%d child=%t host=%t handles=%t", item.Name, item.Exact, item.ExitCode, item.Child, item.Host, item.Handles)
		}
	}
	fmt.Printf("native command suite complete: %s cases=%d\n", reportPath, len(report.Cases))
	return nil
}

func renderedMarkerSegmentTrimmed(lines []renderedHistoryLine, begin, end string) ([]renderedHistoryLine, bool) {
	start := -1
	for index, line := range lines {
		if bytes.Equal(bytes.TrimRight(line.Bytes, " "), []byte(begin)) {
			start = index
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	for index := start + 1; index < len(lines); index++ {
		if bytes.Equal(bytes.TrimRight(lines[index].Bytes, " "), []byte(end)) {
			return append([]renderedHistoryLine(nil), lines[start:index+1]...), true
		}
	}
	return nil, false
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
