//go:build windows

package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

type lifecycleCaseReport struct {
	Name           string `json:"name"`
	CloseOrder     string `json:"close_order"`
	ExpectedWait   string `json:"expected_wait"`
	ObservedWait   string `json:"observed_wait"`
	ChildExited    bool   `json:"child_exited"`
	HostExited     bool   `json:"host_exited"`
	HandlesClosed  bool   `json:"handles_closed"`
	PromptObserved bool   `json:"prompt_observed,omitempty"`
	OutputBytes    int    `json:"output_bytes,omitempty"`
	TailClean      bool   `json:"tail_clean,omitempty"`
}

type lifecycleProbeReport struct {
	Mode        string                `json:"mode"`
	Host        pinnedHostIdentity    `json:"host"`
	Cases       []lifecycleCaseReport `json:"cases"`
	CompletedAt time.Time             `json:"completed_at"`
}

// runNativeLifecycleProbe exercises normal EOF, bounded cancellation, and an
// output-pipe break. It intentionally uses the low-level pinned handles so the
// close order is observable and no reader goroutine can hide a leak.
func runNativeLifecycleProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-lifecycle.json")
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
	cases := []struct {
		name, command, order string
		wait                 time.Duration
		breakOutput          bool
	}{
		{"startup-eof", `cmd.exe /d /q /c exit /b 0`, "host-first", 10 * time.Second, false},
		{"empty-eof", `cmd.exe /d /q /c exit /b 0`, "pipes-first", 10 * time.Second, false},
		{"cancel-timeout", `powershell.exe -NoLogo -NoProfile -NonInteractive -Command "Start-Sleep -Seconds 5"`, "host-first", 250 * time.Millisecond, false},
		{"broken-pipe", `powershell.exe -NoLogo -NoProfile -NonInteractive -Command "Start-Sleep -Seconds 5"`, "pipes-first", 250 * time.Millisecond, true},
	}
	report := lifecycleProbeReport{Mode: "pinned-conpty-lifecycle", Host: identity}
	promptCase, promptErr := runInteractivePromptCase(resolved, `cmd.exe /d /q /k "prompt __PINNED_CONPTY_PROBE_PROMPT__$G"`)
	if promptErr != nil {
		return promptErr
	}
	report.Cases = append(report.Cases, promptCase)
	tailCase, tailErr := runEOFWithTailCase(resolved, executable)
	if tailErr != nil {
		return tailErr
	}
	report.Cases = append(report.Cases, tailCase)
	for _, item := range cases {
		caseReport, caseErr := runManualLifecycleCase(resolved, executable, item.name, item.command, item.order, item.wait, item.breakOutput)
		if caseErr != nil {
			return caseErr
		}
		report.Cases = append(report.Cases, caseReport)
	}
	report.CompletedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	for _, item := range report.Cases {
		if item.Name == "first-prompt" {
			if item.ObservedWait != "timeout" || !item.PromptObserved || !item.ChildExited || !item.HostExited || !item.HandlesClosed {
				return fmt.Errorf("lifecycle prompt case failed: %+v", item)
			}
			continue
		}
		if item.Name == "eof-tail" {
			if item.ObservedWait != "exit" || !item.TailClean || !item.ChildExited || !item.HostExited || !item.HandlesClosed {
				return fmt.Errorf("lifecycle EOF-tail case failed: %+v", item)
			}
			continue
		}
		if item.ExpectedWait == "exit" && (item.ObservedWait != "exit" || !item.ChildExited || !item.HostExited || !item.HandlesClosed) {
			return fmt.Errorf("lifecycle EOF case failed: %+v", item)
		}
		if item.ExpectedWait == "timeout" && (item.ObservedWait != "timeout" || !item.ChildExited || !item.HostExited || !item.HandlesClosed) {
			return fmt.Errorf("lifecycle timeout case failed: %+v", item)
		}
	}
	fmt.Printf("native lifecycle probe complete: %s cases=%d\n", reportPath, len(report.Cases))
	return nil
}

func runEOFWithTailCase(hostPath, executable string) (lifecycleCaseReport, error) {
	result := lifecycleCaseReport{Name: "eof-tail", CloseOrder: "host-first", ExpectedWait: "exit"}
	begin, end := "__PINNED_CONPTY_PROBE_EOF_BEGIN__", "__PINNED_CONPTY_PROBE_EOF_END__"
	command := fmt.Sprintf(`cmd.exe /d /q /c "echo %s & echo %s & exit /b 0"`, begin, end)
	workload := []byte(begin + "\r\n" + end + "\r\n")
	session, err := runNativeProbeSessionWithWorkload(hostPath, executable, 512, 25, false, workload, command, []string{begin, end})
	if err != nil {
		return result, err
	}
	result.ObservedWait = "exit"
	result.ChildExited = session.ChildExited
	result.HostExited = session.HostExited
	result.HandlesClosed = session.HandlesClosed
	result.OutputBytes = len(session.RawOutput)
	position := bytes.LastIndex(session.RawOutput, []byte(end))
	if position >= 0 {
		tail := bytes.TrimSpace(printableStream(session.RawOutput[position+len(end):]))
		result.TailClean = len(tail) == 0
	}
	return result, nil
}

// runInteractivePromptCase proves that a real interactive child reaches its
// first prompt before the bounded cancellation path closes the session. The
// reader exits as soon as the explicit prompt marker is observed; cleanup then
// closes the output handle and verifies that the reader cannot leak.
func runInteractivePromptCase(hostPath, command string) (lifecycleCaseReport, error) {
	result := lifecycleCaseReport{Name: "first-prompt", CloseOrder: "host-first", ExpectedWait: "timeout"}
	pty, err := createPinnedPseudoConsole(hostPath, 512, 25)
	if err != nil {
		return result, err
	}
	hostPID, _, err := verifyPinnedHostProcess(pty.hostProcess, hostPath)
	if err != nil {
		pty.close()
		pty.closePipes()
		return result, err
	}
	if err := attachPinnedClient(pty, command); err != nil {
		pty.close()
		pty.closePipes()
		return result, err
	}
	childPID := pty.childPID
	type readResult struct {
		data []byte
		err  error
	}
	readReady := make(chan readResult, 1)
	const promptMarker = "__PINNED_CONPTY_PROBE_PROMPT__>"
	go func() {
		var all bytes.Buffer
		buffer := make([]byte, 32*1024)
		for {
			var read uint32
			err := windows.ReadFile(pty.output, buffer, &read, nil)
			if read > 0 {
				_, _ = all.Write(buffer[:read])
				if bytes.Contains(all.Bytes(), []byte(promptMarker)) {
					readReady <- readResult{data: all.Bytes()}
					return
				}
			}
			if err != nil {
				readReady <- readResult{data: all.Bytes(), err: err}
				return
			}
		}
	}()
	var read readResult
	select {
	case read = <-readReady:
		result.OutputBytes = len(read.data)
		result.PromptObserved = bytes.Contains(read.data, []byte(promptMarker))
	case <-time.After(5 * time.Second):
		_ = windows.CloseHandle(pty.output)
		pty.output = 0
		read = <-readReady
		result.OutputBytes = len(read.data)
	}
	event, waitErr := windows.WaitForSingleObject(pty.childProcess, 250)
	if waitErr != nil {
		terminatePinnedClient(pty)
		pty.close()
		pty.closePipes()
		return result, waitErr
	}
	if event == uint32(windows.WAIT_TIMEOUT) {
		result.ObservedWait = "timeout"
		terminatePinnedClient(pty)
	} else if event == windows.WAIT_OBJECT_0 {
		result.ObservedWait = "exit"
		_ = windows.CloseHandle(pty.childProcess)
		pty.childProcess = 0
	} else {
		result.ObservedWait = fmt.Sprintf("wait-%d", event)
		terminatePinnedClient(pty)
	}
	pty.close()
	pty.closePipes()
	result.ChildExited, _ = processExited(childPID)
	result.HostExited, _ = processExited(hostPID)
	result.HandlesClosed = pty.signal == 0 && pty.ptyReference == 0 && pty.input == 0 && pty.output == 0 && pty.childProcess == 0 && pty.hostProcess == 0
	return result, nil
}

func runManualLifecycleCase(hostPath, executable, name, command, order string, wait time.Duration, breakOutput bool) (lifecycleCaseReport, error) {
	result := lifecycleCaseReport{Name: name, CloseOrder: order}
	pty, err := createPinnedPseudoConsole(hostPath, 512, 25)
	if err != nil {
		return result, err
	}
	hostPID, _, err := verifyPinnedHostProcess(pty.hostProcess, hostPath)
	if err != nil {
		pty.close()
		pty.closePipes()
		return result, err
	}
	if err := attachPinnedClient(pty, command); err != nil {
		pty.close()
		pty.closePipes()
		return result, err
	}
	childPID := pty.childPID
	event, waitErr := windows.WaitForSingleObject(pty.childProcess, uint32(wait/time.Millisecond))
	if waitErr != nil {
		terminatePinnedClient(pty)
		pty.close()
		pty.closePipes()
		return result, waitErr
	}
	if event == windows.WAIT_OBJECT_0 {
		result.ExpectedWait = "exit"
		result.ObservedWait = "exit"
		_ = windows.CloseHandle(pty.childProcess)
		pty.childProcess = 0
	} else if event == uint32(windows.WAIT_TIMEOUT) {
		result.ExpectedWait = "timeout"
		result.ObservedWait = "timeout"
		if breakOutput && pty.output != 0 {
			_ = windows.CloseHandle(pty.output)
			pty.output = 0
		}
		terminatePinnedClient(pty)
	} else {
		result.ExpectedWait = "timeout"
		result.ObservedWait = fmt.Sprintf("wait-%d", event)
		terminatePinnedClient(pty)
	}
	if order == "pipes-first" {
		pty.closePipes()
		pty.close()
	} else {
		pty.close()
		pty.closePipes()
	}
	result.ChildExited, _ = processExited(childPID)
	result.HostExited, _ = processExited(hostPID)
	result.HandlesClosed = pty.signal == 0 && pty.ptyReference == 0 && pty.input == 0 && pty.output == 0 && pty.childProcess == 0 && pty.hostProcess == 0
	return result, nil
}
