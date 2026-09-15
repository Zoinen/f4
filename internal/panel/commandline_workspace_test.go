package panel

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

func TestWorkspaceCommandWaitsForShellStartup(t *testing.T) {
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	fm := vtui.FrameManager
	fm.Push(pf)
	pf.ResizeConsole(80, 25)
	pf.SetCommandLineFocus(true)
	pf.CmdLine.Edit.SetText("python")
	oldSpawn, oldFactory := SpawnLocalShellPTY, newLocalPTY
	defer func() { SpawnLocalShellPTY, newLocalPTY = oldSpawn, oldFactory }()
	gate := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(gate) }) }
	defer release()
	backend := &mockPty{}
	SpawnLocalShellPTY = true
	newLocalPTY = func() (terminal.PtyBackend, error) {
		<-gate
		return backend, nil
	}
	if !pf.RunCommandInNewWorkspace() {
		t.Fatal("workspace launch was not handled")
	}
	clone := fm.GetTopFrame().(*PanelsFrame)
	defer clone.Close()
	if clone == pf || clone.CmdLine.Edit.GetText() != "python" || backend.String() != "" {
		t.Fatal("command did not wait in the new workspace")
	}
	release()
	deadline := time.After(2 * time.Second)
	for clone.CmdLine.Edit.GetText() != "" {
		select {
		case task := <-fm.TaskChan:
			task()
		case <-deadline:
			t.Fatalf("shell became ready but command was not submitted: %q", backend.String())
		}
	}
	if !strings.Contains(backend.String(), "python") {
		t.Fatalf("command not sent to cloned PTY: %q", backend.String())
	}
	before := backend.String()
	clone.submitPendingWorkspaceCommand()
	if backend.String() != before || pf.CmdLine.Edit.GetText() != "python" {
		t.Fatal("startup resubmitted the command or changed the source prompt")
	}
}
