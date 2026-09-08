//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func init() {
	// The Go test executable is placed in a temporary build directory, while
	// CI installs the same ConPTY pair into this package directory. Production
	// binaries do not use this hook: they resolve the pair next to f4.exe.
	conPTYBundleDirectoryOverride = os.Getwd
}

func TestConPTYAvailable_DoesNotPanic(t *testing.T) {
	avail := conPTYAvailable()
	if !avail {
		pty, err := NewPTY()
		if err == nil {
			if pty != nil {
				_ = pty.Close() // Cleanup is secondary to the unexpected allocation success.
			}
			t.Fatal("NewPTY succeeded when conPTYAvailable() reported false")
		}
	}
}

func TestBundledConPTYIsSelected(t *testing.T) {
	if vtui.IsWine() {
		t.Skip("Wine does not run the native ConPTY bundle")
	}
	api, err := bundledConPTY()
	if err != nil {
		t.Skipf("optional ConPTY bundle not installed: %v", err)
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(api.path, "bundled:") {
		t.Fatalf("expected bundled ConPTY, got path %q", api.path)
	}
	dllPath := strings.TrimPrefix(api.path, "bundled:")
	if got := filepath.Clean(filepath.Dir(dllPath)); got != filepath.Clean(dir) {
		t.Fatalf("ConPTY loaded from %q, want the test-installed pair in %q", got, dir)
	}
}

// startLocalConPTY brings up a PanelsFrame on a real ConPTY and waits until
// cmd.exe has printed its first prompt. Starting a command before that races
// the prompt-driven completion guard: the startup prompt may be delivered
// after the command has armed ignoreNextPrompt. That ordering is covered by
// the state-machine tests; the integration tests here focus on completion.
//
// The caller closes the frame with `defer pf.Close()`, not t.Cleanup: the
// shell is `cd`ed into t.TempDir() by the test, and cleanups run in reverse
// order, so a Close registered before TempDir would run after the RemoveAll
// and leave cmd.exe holding the directory it is asked to delete.
func startLocalConPTY(t *testing.T) *PanelsFrame {
	t.Helper()
	if !conPTYAvailable() {
		t.Skip("ConPTY unavailable")
	}

	oldNewLocalPTY := newLocalPTY
	newLocalPTY = func() (PtyBackend, error) { return NewPTY() }
	oldSpawn := spawnLocalShellPTY
	oldConfig := AppConfig
	t.Cleanup(func() {
		newLocalPTY = oldNewLocalPTY
		spawnLocalShellPTY = oldSpawn
		AppConfig = oldConfig
	})
	spawnLocalShellPTY = true
	AppConfig.ConsoleMode = "own"
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	pf.ResizeConsole(80, 25)
	waitForLocalConPTYPrompt(t, pf, nil)
	return pf
}

// waitForLocalConPTYPrompt waits until a local shell other than previous is
// published and has printed its first prompt.
func waitForLocalConPTYPrompt(t *testing.T, pf *PanelsFrame, previous PtyBackend) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for pf.getActivePTY() == nil || pf.getActivePTY() == previous {
		drainFrameTasks()
		if time.Now().After(deadline) {
			t.Fatal("local ConPTY did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	promptDeadline := time.Now().Add(5 * time.Second)
	for !pf.shellPromptReady {
		drainFrameTasks()
		if time.Now().After(promptDeadline) {
			t.Fatal("local ConPTY startup prompt did not arrive")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func drainFrameTasks() {
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			return
		}
	}
}

func TestActionExecuteBatchDoesNotReturnPanelsEarly(t *testing.T) {
	pf := startLocalConPTY(t)
	defer pf.Close()

	dir := t.TempDir()
	finished := filepath.Join(dir, "finished.marker")
	script := filepath.Join(dir, "f4-batch-probe.cmd")
	// ECHO stays on deliberately: with it cmd prints the prompt (and the
	// prompt mark f4 injects) in front of every batch line, which is what
	// made the panels return while the batch was still running (#409).
	// timeout spawns no child process, so the child check cannot help.
	content := "echo started>started.marker\r\ntimeout /t 3 /nobreak >nul\r\necho finished>finished.marker\r\ntimeout /t 2 /nobreak >nul\r\n"
	if err := os.WriteFile(script, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	actionExecute(pf, vfs.NewOSVFS(dir), dir, filepath.Base(script), script)
	start := time.Now()
	for pf.showPanels {
		drainFrameTasks()
		if time.Since(start) > 5*time.Second {
			t.Fatal("actionExecute did not hide panels")
		}
		time.Sleep(10 * time.Millisecond)
	}
	panelsReturned := time.Duration(0)
	completionDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(completionDeadline) {
		drainFrameTasks()
		if pf.showPanels {
			panelsReturned = time.Since(start)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if panelsReturned == 0 {
		if _, err := os.Stat(finished); err == nil {
			t.Fatal("batch finished but panels did not return")
		}
		t.Fatal("panels did not return after batch completion")
	}
	if _, err := os.Stat(finished); err != nil {
		t.Fatalf("panels returned after %v before batch finished: %v", panelsReturned, err)
	}
	t.Logf("panels returned after batch completion in %v", panelsReturned)
}

// `exit` inside a batch file ends cmd.exe itself (only `exit /b` ends just
// the batch), and ConPTY keeps its output pipe open after the client is gone
// until the pseudoconsole is closed. f4 used to sit on that pipe forever: no
// prompt could arrive, the panels stayed hidden, Ctrl+C and Ctrl+Break had
// no process to reach (#409). The shell's exit must now end the command,
// bring the panels back and start a fresh shell that reaches its prompt.
func TestActionExecuteBatchExitRestartsShell(t *testing.T) {
	pf := startLocalConPTY(t)
	defer pf.Close()
	oldPTY := pf.getActivePTY()

	dir := t.TempDir()
	after := filepath.Join(dir, "after.marker")
	script := filepath.Join(dir, "f4-batch-exit.cmd")
	content := "echo started>started.marker\r\nexit\r\necho after>after.marker\r\n"
	if err := os.WriteFile(script, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	actionExecute(pf, vfs.NewOSVFS(dir), dir, filepath.Base(script), script)
	start := time.Now()
	for pf.showPanels {
		drainFrameTasks()
		if time.Since(start) > 5*time.Second {
			t.Fatal("actionExecute did not hide panels")
		}
		time.Sleep(10 * time.Millisecond)
	}
	deadline := time.Now().Add(15 * time.Second)
	for !pf.showPanels {
		drainFrameTasks()
		if time.Now().After(deadline) {
			t.Fatalf("panels did not return after the batch exited the shell (executing=%v)", pf.executing)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pf.executing {
		t.Fatal("execution still marked running after the shell exited")
	}
	if _, err := os.Stat(after); err == nil {
		t.Fatal("the line after exit ran: exit did not end the shell, the test proves nothing")
	}

	waitForLocalConPTYPrompt(t, pf, oldPTY)
	if pf.getActivePTY() == oldPTY {
		t.Fatal("the dead shell was not replaced")
	}
	if pf.isPtyBusy() {
		t.Fatal("the fresh shell reports busy at its prompt")
	}
	t.Logf("panels returned and the shell was replaced in %v", time.Since(start))
}

func TestSystemConPTYAvailableForPortableBuild(t *testing.T) {
	if vtui.IsWine() {
		t.Skip("ConPTY unavailable under Wine")
	}
	api, err := systemConPTY()
	if err != nil {
		t.Fatal(err)
	}
	if api.path != "system:kernel32.dll" {
		t.Fatalf("unexpected portable API: %s", api.path)
	}
}
