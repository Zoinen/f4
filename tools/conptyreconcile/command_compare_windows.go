//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type commandLineMismatch struct {
	Index            int    `json:"index"`
	Expected         string `json:"expected"`
	Observed         string `json:"observed"`
	ExpectedHex      string `json:"expected_hex,omitempty"`
	ObservedHex      string `json:"observed_hex,omitempty"`
	ObservedCrossRow bool   `json:"observed_cross_row,omitempty"`
}

type commandCompareReport struct {
	Mode                    string                `json:"mode"`
	Host                    pinnedHostIdentity    `json:"host"`
	SessionWidth            int                   `json:"session_width"`
	SessionHeight           int                   `json:"session_height"`
	Command                 string                `json:"command"`
	RedirectCommand         string                `json:"redirect_command"`
	RedirectedBytes         int                   `json:"redirected_bytes"`
	RedirectedSHA256        string                `json:"redirected_sha256"`
	HostRawBytes            int                   `json:"host_raw_bytes"`
	HostRawSHA256           string                `json:"host_raw_sha256"`
	ExpectedLines           int                   `json:"expected_lines"`
	ObservedLines           int                   `json:"observed_lines"`
	MismatchCount           int                   `json:"mismatch_count"`
	NormalizedMismatchCount int                   `json:"normalized_mismatch_count"`
	ContentMismatchCount    int                   `json:"content_mismatch_count"`
	TrailingPaddingOnly     int                   `json:"trailing_padding_only"`
	CrossRowMismatch        int                   `json:"cross_row_mismatch"`
	CUPBeforeCRLF           int                   `json:"cup_before_crlf"`
	LCSLength               int                   `json:"lcs_length"`
	LCSInsertions           int                   `json:"lcs_insertions"`
	LCSDeletions            int                   `json:"lcs_deletions"`
	LCSReplacements         int                   `json:"lcs_replacements"`
	Mismatches              []commandLineMismatch `json:"mismatches,omitempty"`
	FirstMismatchContext    []commandLineMismatch `json:"first_mismatch_context,omitempty"`
	ConsumerResizes         []consumerResizeCheck `json:"consumer_resizes,omitempty"`
	ChildExitCode           uint32                `json:"child_exit_code"`
	ChildExited             bool                  `json:"child_exited"`
	HostExited              bool                  `json:"host_exited"`
	HandlesClosed           bool                  `json:"handles_closed"`
	CompletedAt             time.Time             `json:"completed_at"`
}

// runNativeCommandCompare executes the same recursive dir command through a
// pinned ConPTY and directly to a file. The file is an independent byte-level
// ground truth for child line boundaries; host rendering is compared only
// after stripping the renderer's out-of-band controls via RenderedLines.
func runNativeCommandCompare(hostPath, reportPath string) error {
	return runNativeCommandCompareAtWidth(hostPath, reportPath, 80)
}

func runNativeCommandCompareAtWidth(hostPath, reportPath string, width int) error {
	if width < 1 {
		return fmt.Errorf("command comparison width must be positive, got %d", width)
	}
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", fmt.Sprintf("pinned-conpty-command-compare-%d.json", width))
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
	begin := "__PINNED_CONPTY_PROBE_DIR_COMPARE_BEGIN__"
	end := "__PINNED_CONPTY_PROBE_DIR_COMPARE_END__"
	command := fmt.Sprintf(`cmd.exe /d /q /c "chcp 65001 >nul & echo %s & set DIRCMD= & dir /s /b %s & echo %s & exit /b 0"`, begin, root, end)
	redirectCommand := fmt.Sprintf(`cmd.exe /d /q /c "chcp 65001 >nul & set DIRCMD= & dir /s /b %s"`, root)
	redirectPath := reportPath + ".redirected.raw"
	redirected, err := runRedirectedDir(root, redirectPath)
	if err != nil {
		return err
	}
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, width, 1000, false, nil, command, []string{begin, end})
	if runErr != nil {
		return fmt.Errorf("pinned command comparison session: %w", runErr)
	}
	expected := splitCommandLines(redirected)
	rendered := parseRenderedHistoryAtWidth(session.RawOutput, width).Lines()
	segment, ok := renderedMarkerSegment(rendered, begin, end)
	if !ok || len(segment) < 2 {
		return fmt.Errorf("pinned command comparison markers did not delimit rendered output")
	}
	observed := segment[1 : len(segment)-1]
	observedLogical := make([]logicalLine, 0, len(observed))
	for _, line := range observed {
		observedLogical = append(observedLogical, logicalLine{Bytes: append([]byte(nil), line.Bytes...), Terminator: append([]byte(nil), line.Terminator...)})
	}
	consumerResizes, resizeErr := verifyConsumerResizeDuringFeed(observedLogical, []int{1, 79, 80, 121, 20, width})
	if resizeErr != nil {
		return fmt.Errorf("consumer resize replay: %w", resizeErr)
	}
	report := commandCompareReport{
		Mode: "pinned-conpty-command-compare", Host: identity, SessionWidth: width, SessionHeight: 1000, Command: command,
		RedirectCommand: redirectCommand, RedirectedBytes: len(redirected),
		HostRawBytes: len(session.RawOutput), HostRawSHA256: session.RawSHA256,
		ExpectedLines: len(expected), ObservedLines: len(observed),
		CUPBeforeCRLF: len(cupPattern.FindAll(session.RawOutput, -1)),
		ChildExitCode: session.ExitCode, ChildExited: session.ChildExited,
		HostExited: session.HostExited, HandlesClosed: session.HandlesClosed,
		CompletedAt:     time.Now().UTC(),
		ConsumerResizes: consumerResizes,
	}
	redirectHash := sha256.Sum256(redirected)
	report.RedirectedSHA256 = fmt.Sprintf("%x", redirectHash[:])
	diffStats := lcsLineStats(expected, func() [][]byte {
		result := make([][]byte, len(observed))
		for index, line := range observed {
			result[index] = line.Bytes
		}
		return result
	}())
	report.LCSLength, report.LCSInsertions, report.LCSDeletions, report.LCSReplacements = diffStats.LCS, diffStats.Insertions, diffStats.Deletions, diffStats.Replacements
	for index := 0; index < len(expected) || index < len(observed); index++ {
		want, got := "", ""
		var cross bool
		if index < len(expected) {
			want = string(expected[index])
		}
		if index < len(observed) {
			got = string(observed[index].Bytes)
			cross = observed[index].CrossRow
		}
		if want != got {
			report.MismatchCount++
			if cross {
				report.CrossRowMismatch++
			}
			if strings.TrimRight(got, " ") == strings.TrimRight(want, " ") {
				report.TrailingPaddingOnly++
			} else {
				report.ContentMismatchCount++
			}
			if len(report.Mismatches) < 20 {
				report.Mismatches = append(report.Mismatches, commandLineMismatch{
					Index: index, Expected: want, Observed: got,
					ExpectedHex: hex.EncodeToString([]byte(want)), ObservedHex: hex.EncodeToString([]byte(got)),
					ObservedCrossRow: cross,
				})
			}
			if len(report.FirstMismatchContext) == 0 && strings.TrimRight(got, " ") != strings.TrimRight(want, " ") {
				for contextIndex := maxInt(0, index-2); contextIndex <= minCommandInt(len(expected), index+3); contextIndex++ {
					contextWant, contextGot := "", ""
					contextCross := false
					if contextIndex < len(expected) {
						contextWant = string(expected[contextIndex])
					}
					if contextIndex < len(observed) {
						contextGot = string(observed[contextIndex].Bytes)
						contextCross = observed[contextIndex].CrossRow
					}
					report.FirstMismatchContext = append(report.FirstMismatchContext, commandLineMismatch{
						Index: contextIndex, Expected: contextWant, Observed: contextGot,
						ExpectedHex: hex.EncodeToString([]byte(contextWant)), ObservedHex: hex.EncodeToString([]byte(contextGot)),
						ObservedCrossRow: contextCross,
					})
				}
			}
		}
		if want != strings.TrimRight(got, " ") {
			report.NormalizedMismatchCount++
		}
	}
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, fmt.Sprintf("%dx1000.raw", width)), session.RawOutput, session.RawSHA256); err != nil {
		return err
	}
	if report.MismatchCount != 0 {
		return fmt.Errorf("native command comparison found %d line mismatches (report %s)", report.MismatchCount, reportPath)
	}
	fmt.Printf("native command comparison complete: %s mismatches=%d expected_lines=%d observed_lines=%d cup_before_crlf=%d\n", reportPath, report.MismatchCount, report.ExpectedLines, report.ObservedLines, report.CUPBeforeCRLF)
	return nil
}

func minCommandInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func runRedirectedDir(root, path string) ([]byte, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("cmd.exe", "/d", "/q", "/c", fmt.Sprintf("chcp 65001 >nul & set DIRCMD= & dir /s /b %s", root))
	cmd.Stdout = file
	cmd.Stderr = file
	cmd.Env = append(os.Environ(), "DIRCMD=", "PAGER=", "GIT_PAGER=", "GIT_TERMINAL_PROMPT=0")
	err = cmd.Run()
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("redirected command failed: %w", err)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func splitCommandLines(data []byte) [][]byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.TrimSuffix(data, []byte("\n"))
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte("\n"))
	result := make([][]byte, len(parts))
	for index, part := range parts {
		result[index] = append([]byte(nil), part...)
	}
	return result
}
