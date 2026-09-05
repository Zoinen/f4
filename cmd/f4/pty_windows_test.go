//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

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

func TestActionExecuteBatchDoesNotReturnPanelsEarly(t *testing.T) {
	if !conPTYAvailable() {
		t.Skip("ConPTY unavailable")
	}

	oldSpawn := spawnLocalShellPTY
	oldConfig := AppConfig
	defer func() {
		spawnLocalShellPTY = oldSpawn
		AppConfig = oldConfig
	}()
	spawnLocalShellPTY = true
	AppConfig.ConsoleMode = "own"
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)

	deadline := time.Now().Add(5 * time.Second)
	for pf.getActivePTY() == nil {
		if time.Now().After(deadline) {
			t.Fatal("local ConPTY did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Publishing the PTY is not the same as having received cmd.exe's first
	// prompt. Starting the command in between those events races the prompt
	// driven completion guard: the startup prompt may be delivered after the
	// command has armed ignoreNextPrompt. That ordering is covered by the
	// state-machine tests; this integration test focuses on batch completion.
	promptDeadline := time.Now().Add(5 * time.Second)
	for !pf.shellPromptReady {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
		}
		if time.Now().After(promptDeadline) {
			t.Fatal("local ConPTY startup prompt did not arrive")
		}
		time.Sleep(10 * time.Millisecond)
	}

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
	drainTasks := func() {
		for {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				task()
			default:
				return
			}
		}
	}
	for pf.showPanels {
		drainTasks()
		if time.Since(start) > 5*time.Second {
			t.Fatal("actionExecute did not hide panels")
		}
		time.Sleep(10 * time.Millisecond)
	}
	panelsReturned := time.Duration(0)
	completionDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(completionDeadline) {
		drainTasks()
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
