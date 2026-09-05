//go:build windows

package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

const (
	probeBundleURL   = "https://github.com/microsoft/terminal/releases/download/v1.12.10982.0/Microsoft.WindowsTerminal_Win11_1.12.10983.0_8wekyb3d8bbwe.msixbundle"
	probePackageName = "CascadiaPackage_1.12.10983.0_x64.msix"
)

type nativeProbeReport struct {
	Mode               string               `json:"mode"`
	Host               pinnedHostIdentity   `json:"host"`
	BundleURL          string               `json:"bundle_url"`
	Package            string               `json:"package"`
	GOOS               string               `json:"goos"`
	GOARCH             string               `json:"goarch"`
	WorkingDir         string               `json:"working_dir"`
	Executable         string               `json:"probe_executable"`
	Environment        map[string]string    `json:"environment"`
	ExpectedInput      []byte               `json:"expected_input"`
	ResizeDuringOutput bool                 `json:"resize_during_output"`
	Sessions           []nativeProbeSession `json:"sessions"`
	CompletedAt        time.Time            `json:"completed_at"`
}

func runNativeProbe(hostPath, reportPath string, resizeDuringOutput bool) error {
	if reportPath == "" {
		reportPath = filepath.Join(filepath.Dir(os.Args[0]), "pinned-conpty-probe.json")
	}
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
		return fmt.Errorf("create native probe report directory: %w", err)
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
		return fmt.Errorf("locate probe executable: %w", err)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return err
	}
	report := nativeProbeReport{
		Mode: "pinned-conpty-probe", Host: identity, BundleURL: probeBundleURL,
		Package: probePackageName, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		WorkingDir: workingDir, Executable: executable, Environment: probeEnvironment(), ExpectedInput: []byte(probeWorkload()),
		ResizeDuringOutput: resizeDuringOutput,
	}
	dimensionsList := [][2]int{{80, 25}, {1, 1}, {121, 40}}
	if !resizeDuringOutput {
		// The control run isolates the same 80x25 workload used by the normal
		// probe. Static 1x1 output can legitimately block a terminal child on
		// this pinned host because there is no resize/reflow escape hatch.
		dimensionsList = [][2]int{{80, 80}}
	}
	for _, dimensions := range dimensionsList {
		session, runErr := runNativeProbeSession(resolved, executable, dimensions[0], dimensions[1], resizeDuringOutput)
		report.Sessions = append(report.Sessions, session)
		artifactDir := reportPath + ".sessions"
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return fmt.Errorf("create native probe artifact directory: %w", err)
		}
		artifact := filepath.Join(artifactDir, fmt.Sprintf("%dx%d.raw", dimensions[0], dimensions[1]))
		if err := writeAndVerifyRawArtifact(artifact, session.RawOutput, session.RawSHA256); err != nil {
			return fmt.Errorf("write native probe raw output: %w", err)
		}
		if runErr != nil {
			_ = writeJSON(reportPath, report)
			return fmt.Errorf("native probe %dx%d: %w", dimensions[0], dimensions[1], runErr)
		}
		if resizeDuringOutput {
			continue
		}
		controlWorkload := []byte(controlProbeWorkload())
		// Keep the semantic-control phase in a wide, otherwise static host so
		// tab stops and OSC 8 are observed without unrelated row wrapping.
		controlWidth, controlHeight := 512, 25
		controlCommand := fmt.Sprintf(`"%s" -emit-control -emit-probe-width %d`, executable, controlWidth)
		control, controlErr := runNativeProbeSessionWithWorkload(resolved, executable, controlWidth, controlHeight, resizeDuringOutput, controlWorkload, controlCommand, controlExpectedMarkers())
		report.Sessions = append(report.Sessions, control)
		controlArtifact := filepath.Join(artifactDir, fmt.Sprintf("%dx%d-control.raw", controlWidth, controlHeight))
		if err := writeAndVerifyRawArtifact(controlArtifact, control.RawOutput, control.RawSHA256); err != nil {
			return fmt.Errorf("write native control probe raw output: %w", err)
		}
		if controlErr != nil {
			_ = writeJSON(reportPath, report)
			return fmt.Errorf("native control probe %dx%d: %w", dimensions[0], dimensions[1], controlErr)
		}
		alternateWorkload := []byte(alternateProbeWorkload(dimensions[0]))
		alternateCommand := fmt.Sprintf(`"%s" -emit-alternate -emit-probe-width %d`, executable, dimensions[0])
		alternate, alternateErr := runNativeProbeSessionWithWorkload(resolved, executable, dimensions[0], dimensions[1], resizeDuringOutput, alternateWorkload, alternateCommand, alternateExpectedMarkers())
		report.Sessions = append(report.Sessions, alternate)
		alternateArtifact := filepath.Join(artifactDir, fmt.Sprintf("%dx%d-alternate.raw", dimensions[0], dimensions[1]))
		if err := writeAndVerifyRawArtifact(alternateArtifact, alternate.RawOutput, alternate.RawSHA256); err != nil {
			return fmt.Errorf("write native alternate probe raw output: %w", err)
		}
		if alternateErr != nil {
			_ = writeJSON(reportPath, report)
			return fmt.Errorf("native alternate probe %dx%d: %w", dimensions[0], dimensions[1], alternateErr)
		}
	}
	report.CompletedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	fmt.Printf("native OpenConsole probe complete: %s\n", reportPath)
	return nil
}

func runNativePartialProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-partial.json")
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
	workload := partialProbeWorkload(80)
	command := fmt.Sprintf(`"%s" -emit-partial -emit-probe-width 80`, executable)
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 80, 25, true, workload, command, nil)
	report := nativeProbeReport{Mode: "pinned-conpty-partial", Host: identity, ExpectedInput: workload, ResizeDuringOutput: true, Sessions: []nativeProbeSession{session}, CompletedAt: time.Now().UTC()}
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
	return runErr
}

func writeAndVerifyRawArtifact(path string, data []byte, expectedSHA string) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	got, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(got, data) {
		return fmt.Errorf("artifact bytes differ from captured raw output: got %d bytes want %d", len(got), len(data))
	}
	hash := sha256.Sum256(got)
	if hex.EncodeToString(hash[:]) != expectedSHA {
		return fmt.Errorf("artifact SHA-256 differs from report: got %s want %s", hex.EncodeToString(hash[:]), expectedSHA)
	}
	return nil
}

func probeEnvironment() map[string]string {
	result := make(map[string]string)
	for _, name := range []string{"WT_SESSION", "WT_PROFILE_ID", "TERM", "TERM_PROGRAM", "WSLENV", "ConEmuANSI", "PROMPT", "CHCP"} {
		if value, ok := os.LookupEnv(name); ok {
			result[name] = value
		}
	}
	return result
}

func runNativeProbeSession(hostPath, executable string, width, height int, resizeDuringOutput bool) (session nativeProbeSession, runErr error) {
	workload := []byte(probeWorkloadForWidth(width))
	return runNativeProbeSessionWithWorkload(hostPath, executable, width, height, resizeDuringOutput, workload,
		fmt.Sprintf(`"%s" -emit-probe -emit-probe-width %d`, executable, width), probeExpectedMarkers())
}

func runNativeProbeSessionWithWorkload(hostPath, executable string, width, height int, resizeDuringOutput bool, workload []byte, childCommand string, markers []string) (session nativeProbeSession, runErr error) {
	return runNativeProbeSessionWithWorkloadQuirk(hostPath, executable, width, height, resizeDuringOutput, workload, childCommand, markers, true)
}

func runNativeProbeSessionWithWorkloadQuirk(hostPath, executable string, width, height int, resizeDuringOutput bool, workload []byte, childCommand string, markers []string, resizeQuirk bool) (session nativeProbeSession, runErr error) {
	return runNativeProbeSessionWithWorkloadQuirkDelay(hostPath, executable, width, height, resizeDuringOutput, workload, childCommand, markers, resizeQuirk, 0)
}

func runNativeProbeSessionWithWorkloadQuirkDelay(hostPath, executable string, width, height int, resizeDuringOutput bool, workload []byte, childCommand string, markers []string, resizeQuirk bool, firstResizeDelay time.Duration) (session nativeProbeSession, runErr error) {
	// resizeDuringOutput is retained for the inactive host-resize alternative
	// documented by B1-0. The gate and all acceptance paths pass false; only
	// the explicit diagnostic -probe command may exercise the old behavior.
	session.InitialWidth, session.InitialHeight = width, height
	session.ExpectedInput = append([]byte(nil), workload...)
	session.Command = childCommand
	session.StartedAt = time.Now().UTC()
	pty, err := createPinnedPseudoConsoleWithQuirk(hostPath, width, height, resizeQuirk)
	if err != nil {
		session.Error = err.Error()
		return session, err
	}
	hostPID, hostIdentity, err := verifyPinnedHostProcess(pty.hostProcess, hostPath)
	if err != nil {
		session.Error = err.Error()
		return session, err
	}
	session.HostPID = hostPID
	session.HostProcess = hostIdentity
	pty.hostPID = hostPID
	session.HostCommand = pty.hostCommandLine
	defer pty.close()
	defer pty.closePipes()
	recorder := newHostCaptureRecorder(0, width, height)
	recorder.append(streamInput, workload, "native-probe-child-payload")
	outputReady := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, readErr := readPinnedOutputRecorded(pty, recorder)
		outputReady <- struct {
			data []byte
			err  error
		}{data: data, err: readErr}
	}()
	if err := attachPinnedClient(pty, childCommand); err != nil {
		session.Error = err.Error()
		return session, err
	}
	session.ChildPID = pty.childPID
	resizeSchedule := [][2]int{{1, 1}, {width, height}, {121, 40}, {80, 25}}
	if !resizeDuringOutput {
		resizeSchedule = nil
	}
	for index, dimensions := range resizeSchedule {
		// The child deliberately writes in short chunks.  These bounded pauses
		// make at least one resize overlap active output without making timing a
		// completion heuristic.
		delay := time.Duration(2+index*3) * time.Millisecond
		if index == 0 && firstResizeDelay > 0 {
			delay = firstResizeDelay
		}
		time.Sleep(delay)
		resize := probeResize{At: time.Now().UTC(), Width: dimensions[0], Height: dimensions[1]}
		// Record the beginning at the instant the resize request is issued.  The
		// pinned host does not serialize a frame-end marker into the output pipe;
		// recording after WriteFile would move the checkpoint past any bytes that
		// raced with the request.
		recorder.resize(dimensions[0], dimensions[1])
		if err := resizePinnedPseudoConsole(pty, uint16(dimensions[0]), uint16(dimensions[1])); err != nil {
			resize.Error = err.Error()
			session.Resizes = append(session.Resizes, resize)
			session.Error = err.Error()
			terminatePinnedClient(pty)
			return session, err
		}
		session.Resizes = append(session.Resizes, resize)
	}
	exitCode, err := waitProbeClient(pty)
	if err != nil {
		session.Error = err.Error()
		terminatePinnedClient(pty)
		return session, err
	}
	session.ExitCode = exitCode
	if exitCode != 0 {
		err := fmt.Errorf("native probe child exited with code %d", exitCode)
		session.Error = err.Error()
		return session, err
	}
	if pty.input != 0 {
		_ = windows.CloseHandle(pty.input)
		pty.input = 0
	}
	settleErr := waitOutputQuiescent(recorder, 300*time.Millisecond, 10*time.Second)
	pty.close()
	session.ChildExited = true
	hostExited, hostExitErr := processExited(pty.hostPID)
	session.HostExited = hostExited
	if hostExitErr != nil {
		session.Error = hostExitErr.Error()
		return session, hostExitErr
	}
	result := <-outputReady
	if result.err != nil {
		session.Error = result.err.Error()
		return session, result.err
	}
	if settleErr != nil {
		session.Error = settleErr.Error()
		return session, settleErr
	}
	pty.closePipes()
	session.HandlesClosed = pty.signal == 0 && pty.ptyReference == 0 && pty.input == 0 && pty.output == 0 && pty.childProcess == 0
	session.FinishedAt = time.Now().UTC()
	session.RawOutput = result.data
	var logical hostRenderStream
	logical.Feed(result.data)
	session.LogicalLines = logical.Lines()
	session.RenderedLines = parseRenderedHistory(result.data).Lines()
	session.Frames = logical.Frames()
	session.RepaintFrames = logical.RepaintFrames()
	session.Chunking, err = verifyHostStreamChunking(result.data, uint64(width)*uint64(height)+uint64(len(workload)))
	if err != nil {
		session.Error = err.Error()
		return session, err
	}
	if !resizeDuringOutput {
		if len(markers) > 0 && markers[0] == alternateBeginMarker {
			session.Assertions = assertAlternatePayload(result.data, markers...)
		} else if len(markers) > 0 && markers[0] == controlBeginMarker {
			session.Assertions = assertControlPayload(result.data, markers...)
		} else {
			session.Assertions = assertStaticPayload(workload, result.data, markers...)
		}
		session.AssertionFailures = assertionFailures(session.Assertions)
	}
	snapshot := recorder.snapshot()
	session.Events = snapshot.Events
	for _, event := range snapshot.Events {
		if event.Kind == streamResize {
			session.ResizeOffsets = append(session.ResizeOffsets, event.OutputOffset)
		}
	}
	hash := sha256.Sum256(result.data)
	session.RawSHA256 = hex.EncodeToString(hash[:])
	previous := -1
	for _, marker := range markers {
		observed := result.data
		if bytes.Count(observed, []byte(marker)) == 0 {
			observed = printableStream(result.data)
			if bytes.Count(observed, []byte(marker)) == 0 {
				// Host repaint can insert CR/LF between adjacent marker bytes;
				// this stream is used only to locate the handoff marker, never
				// to construct logical lines.
				observed = bytes.ReplaceAll(observed, []byte{'\r'}, nil)
				observed = bytes.ReplaceAll(observed, []byte{'\n'}, nil)
			}
		}
		count := bytes.Count(observed, []byte(marker))
		if count != 1 {
			session.MarkerWarnings = append(session.MarkerWarnings, fmt.Sprintf("raw output contains marker %q %d times; logical history must reconcile repaint", marker, count))
		}
		position := bytes.Index(observed, []byte(marker))
		if position < 0 {
			err := fmt.Errorf("native output does not contain marker %q", marker)
			session.Error = err.Error()
			return session, err
		}
		if position <= previous {
			session.MarkerWarnings = append(session.MarkerWarnings, fmt.Sprintf("raw output marker %q is out of order; logical history must reconcile repaint", marker))
		}
		previous = position
		session.Markers = append(session.Markers, marker)
	}
	return session, nil
}

func waitProbeClient(pty *pinnedConPTY) (uint32, error) {
	if pty == nil || pty.childProcess == 0 {
		return 0, fmt.Errorf("native probe child process was not created")
	}
	// Recursive real-command probes can legitimately take longer than ten
	// seconds on a loaded Windows host. Keep a bounded watchdog, but leave
	// enough time for the pinned ConPTY to drain its output.
	const probeChildTimeout = 60 * 1000
	event, err := windows.WaitForSingleObject(pty.childProcess, probeChildTimeout)
	if err != nil {
		return 0, fmt.Errorf("WaitForSingleObject(native probe child): %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		var exitCode uint32
		_ = windows.GetExitCodeProcess(pty.childProcess, &exitCode)
		return 0, fmt.Errorf("native probe child did not finish within %d ms (wait=%d exit=%d)", probeChildTimeout, event, exitCode)
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(pty.childProcess, &exitCode); err != nil {
		return 0, fmt.Errorf("GetExitCodeProcess(native probe child): %w", err)
	}
	handle := pty.childProcess
	pty.childProcess = 0
	_ = windows.CloseHandle(handle)
	return exitCode, nil
}

func ensureProbeHost(hostPath string) (string, error) {
	if hostPath != "" {
		return filepath.Clean(hostPath), nil
	}
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	if base == "" {
		return "", fmt.Errorf("LOCALAPPDATA is unavailable; cannot choose native host cache")
	}
	root := filepath.Join(base, "pinned-conpty", "1.12.10983.0")
	host := filepath.Join(root, strings.TrimSuffix(probePackageName, ".msix"), "OpenConsole.exe")
	if _, err := verifyPinnedHost(host); err == nil {
		return host, nil
	}
	if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
		return "", fmt.Errorf("create native host cache: %w", err)
	}
	lock := root + ".lock"
	deadline := time.Now().Add(2 * time.Minute)
	for {
		err := os.Mkdir(lock, 0o755)
		if err == nil {
			break
		}
		if !os.IsExist(err) || time.Now().After(deadline) {
			return "", fmt.Errorf("acquire native host cache lock: %w", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	defer os.Remove(lock)
	if _, err := verifyPinnedHost(host); err == nil {
		return host, nil
	}
	bundle := root + ".msixbundle"
	if err := downloadProbeBundle(bundle); err != nil {
		return "", err
	}
	tempRoot, err := os.MkdirTemp(filepath.Dir(root), "native-conpty-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempRoot)
	if err := extractProbeZip(bundle, tempRoot); err != nil {
		return "", fmt.Errorf("extract MSIX bundle: %w", err)
	}
	packageFile, err := findProbeFile(tempRoot, probePackageName)
	if err != nil {
		return "", err
	}
	packageDir := filepath.Join(root, strings.TrimSuffix(probePackageName, ".msix"))
	if err := os.RemoveAll(packageDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		return "", err
	}
	if err := extractProbeZip(packageFile, packageDir); err != nil {
		return "", fmt.Errorf("extract x64 package: %w", err)
	}
	if _, err := verifyPinnedHost(host); err != nil {
		return "", fmt.Errorf("downloaded package failed pinned host verification: %w", err)
	}
	return host, nil
}

func downloadProbeBundle(destination string) error {
	part := destination + ".part"
	client := http.Client{Timeout: 5 * time.Minute}
	response, err := client.Get(probeBundleURL)
	if err != nil {
		return fmt.Errorf("download pinned MSIX bundle: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("download pinned MSIX bundle: HTTP %s", response.Status)
	}
	file, err := os.Create(part)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(file, response.Body)
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(part)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(part)
		return closeErr
	}
	_ = os.Remove(destination)
	if err := os.Rename(part, destination); err != nil {
		_ = os.Remove(part)
		return err
	}
	return nil
}

func extractProbeZip(source, destination string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer archive.Close()
	root, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	for _, entry := range archive.File {
		name := filepath.FromSlash(entry.Name)
		target := filepath.Join(root, name)
		relative, err := filepath.Rel(root, target)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return fmt.Errorf("zip entry escapes destination: %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		reader, err := entry.Open()
		if err != nil {
			return err
		}
		output, err := os.Create(target)
		if err == nil {
			_, err = io.Copy(output, reader)
		}
		_ = reader.Close()
		if output != nil {
			_ = output.Close()
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func findProbeFile(root, name string) (string, error) {
	var matches []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.EqualFold(info.Name(), name) {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("expected exactly one %s in bundle, found %d", name, len(matches))
	}
	return matches[0], nil
}
