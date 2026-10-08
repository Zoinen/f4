package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type windowsExecutionVFS struct{ vfs.VFS }

func (*windowsExecutionVFS) Stat(context.Context, string) (vfs.VFSItem, error) {
	// The Windows FISH+ helper synthesizes 0755 for writable files,
	// including ordinary documents and videos.
	return vfs.VFSItem{Name: "runme.cmd", IsExecutable: true}, nil
}
func (*windowsExecutionVFS) OpenPty(int, int) (any, error) {
	return nil, fmt.Errorf("test must reuse the existing remote PTY")
}
func (*windowsExecutionVFS) PtyChangeDirCommand(string) []byte { return nil }
func (*windowsExecutionVFS) PtyInitSequence() []byte           { return nil }
func (*windowsExecutionVFS) PtyInterrupt() []byte              { return []byte{3} }
func (*windowsExecutionVFS) PtyRunCommand(dir, command string) []byte {
	return []byte(fmt.Sprintf("cd /d \"C:\\Users\\xs\" & %s\r", command))
}
func (*windowsExecutionVFS) CommandRunnerInfo() vfs.CommandRunnerInfo {
	return vfs.CommandRunnerInfo{Dialect: vfs.CommandDialectCmd, MaxParallel: 1}
}

func TestRemoteWindowsEnterUsesPeerShellAndVisibleOutput(t *testing.T) {
	pf := newExecutionTestFrame(t)
	pf.ResizeConsole(80, 25)
	pf.ShowPanels = true
	pf.ShellPromptReady = true
	remote := &windowsExecutionVFS{VFS: vfs.NewNullVFS(0)}
	fsp := pf.GetActivePanel()
	fsp.Vfs = remote
	pty := &mockPty{}
	pf.RemotePtys = map[vfs.VFS]terminal.PtyBackend{remote: pty}
	name := "runme.cmd"
	ActionExecute(pf, remote, "/c/Users/xs", name, "/c/Users/xs/"+name)
	deadline := time.After(2 * time.Second)
	for pty.String() == "" {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("Enter did not dispatch the remote file")
		}
	}
	wire := pty.String()
	if !strings.HasPrefix(wire, `cd /d "C:\Users\xs" & `) || strings.Contains(wire, "./") || strings.Contains(wire, "printf") {
		t.Errorf("Windows peer received a non-Windows launch: %q", wire)
	}
	if pf.TermView.Muted {
		t.Error("remote launch muted all output while waiting for a POSIX marker")
	}
	if history := pf.CmdLine.Edit.History[0]; strings.HasPrefix(history, "./") {
		t.Errorf("remote Windows history uses a POSIX path: %q", history)
	}
	pf.Parser.Process([]byte("remote launch error\r\n"))
	var screen strings.Builder
	for _, row := range pf.TermView.GetBuffer() {
		screen.WriteString(terminal.CellsText(row))
	}
	if !strings.Contains(screen.String(), "remote launch error") {
		t.Error("remote command errors are invisible")
	}
	pf.Parser.Process([]byte("\x1b]133;D\x1b\\"))
	testutil.DrainUITasks()
	if pf.Executing || !pf.ShowPanels {
		t.Error("remote Windows prompt did not finish execution and restore panels")
	}
}

func TestRemoteWindowsCtrlCWhileTerminalVisible(t *testing.T) {
	pf := newExecutionTestFrame(t)
	pf.ResizeConsole(80, 25)
	remote := &windowsExecutionVFS{VFS: vfs.NewNullVFS(0)}
	pf.GetActivePanel().Vfs = remote
	pty := &mockPty{}
	pf.RemotePtys = map[vfs.VFS]terminal.PtyBackend{remote: pty}
	pf.BeginPromptDrivenExecution()
	ctrlC := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_C, ControlKeyState: vtinput.LeftCtrlPressed}
	if !pf.InterceptPluginKey(ctrlC) || pty.String() != "\x03" {
		t.Fatalf("Ctrl+C did not reach the visible remote shell before global hotkeys: %q", pty.String())
	}
}

type remoteVideoVFS struct{ vfs.VFS }

type posixExecutionVFS struct{ windowsExecutionVFS }

func (*posixExecutionVFS) CommandRunnerInfo() vfs.CommandRunnerInfo {
	return vfs.CommandRunnerInfo{Dialect: vfs.CommandDialectPOSIX, MaxParallel: 1}
}
func (*posixExecutionVFS) PtyRunCommand(dir, command string) []byte {
	return []byte(" cd '" + dir + "' && " + command + "\r")
}

func TestRemotePOSIXEnterKeepsManagedCompletion(t *testing.T) {
	pf := newExecutionTestFrame(t)
	pf.ResizeConsole(80, 25)
	pf.ShowPanels = true
	remote := &posixExecutionVFS{windowsExecutionVFS{VFS: vfs.NewNullVFS(0)}}
	pf.GetActivePanel().Vfs = remote
	pty := &mockPty{}
	pf.RemotePtys = map[vfs.VFS]terminal.PtyBackend{remote: pty}
	ActionExecute(pf, remote, "/home/xs", "runme.sh", "/home/xs/runme.sh")
	deadline := time.After(2 * time.Second)
	for pty.String() == "" {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("remote POSIX file did not launch")
		}
	}
	wire := pty.String()
	if !strings.Contains(wire, "133;C") || !strings.Contains(wire, "133;D") {
		t.Fatalf("remote POSIX launch lost its completion markers: %q", wire)
	}
	pf.Parser.Process([]byte("\x1b]133;C\x07shell output\r\n\x1b]133;D\x07"))
	testutil.DrainUITasks()
	if pf.Executing || !pf.ShowPanels {
		t.Fatal("remote POSIX completion did not restore panels")
	}
}

func (v *remoteVideoVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	item, err := v.VFS.Stat(ctx, path)
	item.IsExecutable = true // synthetic Windows permissions must not launch a movie as a command
	return item, err
}

func TestRemoteVideoEnterOpensLocalCopy(t *testing.T) {
	if _, _, supported := panel.AssociatedFileCommand("clip.mp4"); !supported {
		t.Skip("desktop associations unavailable")
	}
	pf := newExecutionTestFrame(t)
	pf.ResizeConsole(80, 25)
	pf.ShowPanels = true
	dir := t.TempDir()
	name := "IMG_20260527_185727.mp4"
	sourcePath := filepath.Join(dir, name)
	payload := []byte("remote video bytes")
	if err := os.WriteFile(sourcePath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	remote := &remoteVideoVFS{VFS: vfs.NewOSVFS(dir)}
	pf.GetActivePanel().Vfs = remote
	launched := make(chan string, 1)
	pf.ExternalUIRunner = func(command string, args []string, workingDir string) error {
		launched <- args[len(args)-1]
		return nil
	}
	ActionExecute(pf, remote, dir, name, sourcePath)
	deadline := time.After(2 * time.Second)
	var localPath string
	for localPath == "" {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case localPath = <-launched:
		case <-deadline:
			t.Fatal("remote movie did not open through the local desktop association")
		}
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(localPath)) })
	if localPath == sourcePath || filepath.Ext(localPath) != ".mp4" {
		t.Fatalf("desktop received remote path or lost media extension: %q", localPath)
	}
	data, err := os.ReadFile(localPath)
	if err != nil || string(data) != string(payload) {
		t.Fatalf("local movie = %q, error %v", data, err)
	}
	if !pf.ShowPanels || pf.Executing {
		t.Fatal("opening a remote movie activated shell execution")
	}
}
