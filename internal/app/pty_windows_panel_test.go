//go:build windows

package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"testing"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// startLocalConPTY brings up a panel.PanelsFrame on a real ConPTY and waits until
// cmd.exe has printed its first prompt. Starting a command before that races
// the prompt-driven completion guard: the startup prompt may be delivered
// after the command has armed ignoreNextPrompt. That ordering is covered by
// the state-machine tests; the integration tests here focus on completion.
//
// The caller closes the frame with `defer pf.Close()`, not t.Cleanup: the
// shell is `cd`ed into t.TempDir() by the test, and cleanups run in reverse
// order, so a Close registered before TempDir would run after the RemoveAll
// and leave cmd.exe holding the directory it is asked to delete.
// waitForLocalConPTYPrompt waits until a local shell other than previous is
// published and has printed its first prompt.
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

func startLocalConPTY(t *testing.T) *panel.PanelsFrame {
	t.Helper()
	if !terminal.ConPTYAvailable() {
		t.Skip("ConPTY unavailable")
	}

	oldSpawn := panel.SpawnLocalShellPTY
	oldConfig := config.App
	t.Cleanup(func() {
		panel.SpawnLocalShellPTY = oldSpawn
		config.App = oldConfig
	})
	panel.SpawnLocalShellPTY = true
	config.App.ConsoleMode = "own"
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := panel.NewPanelsFrame()
	pf.ResizeConsole(80, 25)
	waitForLocalConPTYPrompt(t, pf, nil)
	return pf
}

func waitForLocalConPTYPrompt(t *testing.T, pf *panel.PanelsFrame, previous terminal.PtyBackend) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for pf.GetActivePTY() == nil || pf.GetActivePTY() == previous {
		drainFrameTasks()
		if time.Now().After(deadline) {
			t.Fatal("local ConPTY did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	promptDeadline := time.Now().Add(5 * time.Second)
	for !pf.ShellPromptReady {
		drainFrameTasks()
		if time.Now().After(promptDeadline) {
			t.Fatal("local ConPTY startup prompt did not arrive")
		}
		time.Sleep(10 * time.Millisecond)
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

	ActionExecute(pf, vfs.NewOSVFS(dir), dir, filepath.Base(script), script)
	start := time.Now()
	for pf.ShowPanels {
		drainFrameTasks()
		if time.Since(start) > 5*time.Second {
			t.Fatal("ActionExecute did not hide panels")
		}
		time.Sleep(10 * time.Millisecond)
	}
	panelsReturned := time.Duration(0)
	completionDeadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(completionDeadline) {
		drainFrameTasks()
		if pf.ShowPanels {
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

func TestActionExecuteBatchExitRestartsShell(t *testing.T) {
	pf := startLocalConPTY(t)
	defer pf.Close()
	oldPTY := pf.GetActivePTY()

	dir := t.TempDir()
	after := filepath.Join(dir, "after.marker")
	script := filepath.Join(dir, "f4-batch-exit.cmd")
	content := "echo started>started.marker\r\nexit\r\necho after>after.marker\r\n"
	if err := os.WriteFile(script, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	ActionExecute(pf, vfs.NewOSVFS(dir), dir, filepath.Base(script), script)
	start := time.Now()
	for pf.ShowPanels {
		drainFrameTasks()
		if time.Since(start) > 5*time.Second {
			t.Fatal("ActionExecute did not hide panels")
		}
		time.Sleep(10 * time.Millisecond)
	}
	deadline := time.Now().Add(15 * time.Second)
	for !pf.ShowPanels {
		drainFrameTasks()
		if time.Now().After(deadline) {
			t.Fatalf("panels did not return after the batch exited the shell (executing=%v)", pf.Executing)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pf.Executing {
		t.Fatal("execution still marked running after the shell exited")
	}
	if _, err := os.Stat(after); err == nil {
		t.Fatal("the line after exit ran: exit did not end the shell, the test proves nothing")
	}

	waitForLocalConPTYPrompt(t, pf, oldPTY)
	if pf.GetActivePTY() == oldPTY {
		t.Fatal("the dead shell was not replaced")
	}
	if pf.IsPtyBusy() {
		t.Fatal("the fresh shell reports busy at its prompt")
	}
	t.Logf("panels returned and the shell was replaced in %v", time.Since(start))
}

func TestLocalConPTYHistoryKeepsStartupBeforeCommands(t *testing.T) {
	for _, inherit := range []bool{false, true} {
		name := "fresh-history"
		if inherit {
			name = "inherited-history"
		}
		t.Run(name, func(t *testing.T) { checkLocalConPTYWorkspaceHistory(t, inherit) })
	}
}

func checkLocalConPTYWorkspaceHistory(t *testing.T, inherit bool) {
	t.Helper()
	pf := startLocalConPTY(t)
	defer pf.Close()
	run := func(frame *panel.PanelsFrame, output string) {
		t.Helper()
		frame.CmdLine.Edit.SetText("echo " + output)
		frame.ProcessKey(&vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN,
		})
		deadline := time.Now().Add(8 * time.Second)
		for {
			drainFrameTasks()
			if !frame.Executing && strings.Contains(string(frame.TermView.GetAllLogBytes()), "\n"+output+"\n") {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("command did not finish: executing=%v log=%q", frame.Executing, frame.TermView.GetAllLogBytes())
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	run(pf, "F4_FIRST_OUTPUT")
	pf.ResizeConsole(100, 45)
	run(pf, "F4_SECOND_OUTPUT")
	text := string(pf.TermView.GetAllLogBytes())
	if strings.LastIndex(text, "Microsoft Windows") > strings.Index(text, "F4_FIRST_OUTPUT") {
		t.Fatalf("startup banner appeared after a command: %q", text)
	}
	config.App.InheritTerminalHistory = inherit
	clone := pf.Clone()
	defer clone.Close()
	waitForLocalConPTYPrompt(t, clone, nil)
	if clone.GetActivePTY() == pf.GetActivePTY() {
		t.Fatal("new workspace shares the source shell")
	}
	for _, output := range []string{"\nF4_FIRST_OUTPUT\n", "\nF4_SECOND_OUTPUT\n"} {
		want := 0
		if inherit {
			want = 1
		}
		if text := string(clone.TermView.GetAllLogBytes()); strings.Count(text, output) != want {
			t.Fatalf("history inheritance=%v: wanted %d copies of %q, got %q", inherit, want, output, text)
		}
	}
	run(clone, "F4_CLONED_OUTPUT")
	text = string(clone.TermView.GetAllLogBytes())
	if strings.Count(text, "\nF4_CLONED_OUTPUT\n") != 1 {
		t.Fatalf("new workspace lost or duplicated its command output: %q", text)
	}
	run(pf, "F4_SOURCE_LATER_OUTPUT")
	if got := string(clone.TermView.GetAllLogBytes()); got != text {
		t.Fatalf("source shell output changed the new workspace history: %q", got)
	}
	sourceText := string(pf.TermView.GetAllLogBytes())
	for _, output := range []string{"\nF4_FIRST_OUTPUT\n", "\nF4_SECOND_OUTPUT\n", "\nF4_SOURCE_LATER_OUTPUT\n"} {
		if strings.Count(sourceText, output) != 1 {
			t.Fatalf("source workspace lost or duplicated command output %q: %q", output, sourceText)
		}
	}
	if strings.Contains(sourceText, "F4_CLONED_OUTPUT") {
		t.Fatalf("new shell output leaked into the source workspace: %q", sourceText)
	}
	if strings.LastIndex(text, "Microsoft Windows") > strings.LastIndex(text, "F4_CLONED_OUTPUT") {
		t.Fatalf("new shell banner appeared after its command: %q", text)
	}
	clone.TermView.SetVisible(true)
	clone.TermView.SetFocus(true)
	model := clone.TermView.SemanticModelWithBottomOverlay(nil, 1)
	if model.CursorVisible {
		t.Fatal("idle shell cursor is visible beside the separate command input")
	}
	prompt := strings.TrimSpace(clone.TermView.PromptSnapshot().Text)
	for _, row := range model.WindowRows {
		var line strings.Builder
		for _, run := range row.Runs {
			line.WriteString(run.Text)
		}
		if prompt != "" && strings.TrimSpace(line.String()) == prompt {
			t.Fatalf("standalone shell prompt leaked into native history: %q", line.String())
		}
	}
}
