//go:build !windows && !freebsd

package editor

import (
	"github.com/unxed/f4/internal/gui"
	"github.com/unxed/f4/internal/terminal"
	"os/exec"
	"testing"
)

func TestConfigureExternalEditorProcessUsesControllingTTY(t *testing.T) {
	oldRunningGUI := gui.Running
	gui.Running = false
	t.Cleanup(func() { gui.Running = oldRunningGUI })

	cmd := exec.Command("true")
	ConfigureExternalEditorProcess(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("external editor process has no platform attributes")
	}
	if !cmd.SysProcAttr.Setsid || !cmd.SysProcAttr.Setctty {
		t.Fatalf("external editor process attributes = %+v, want a new session with its TTY", cmd.SysProcAttr)
	}
}

func TestConfigureExternalEditorProcessLeavesGUIEditorAlone(t *testing.T) {
	oldRunningGUI := gui.Running
	gui.Running = true
	t.Cleanup(func() { gui.Running = oldRunningGUI })

	cmd := exec.Command("true")
	ConfigureExternalEditorProcess(cmd)
	if cmd.SysProcAttr != nil {
		t.Fatalf("GUI editor process attributes = %+v, want none", cmd.SysProcAttr)
	}
}

func TestConfigureExternalEditorProcessCanOpenDevTTY(t *testing.T) {
	oldRunningGUI := gui.Running
	gui.Running = false
	t.Cleanup(func() { gui.Running = oldRunningGUI })

	pty, err := terminal.NewPTY()
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	defer pty.Close()

	pty.Cmd = exec.Command("sh", "-c", "test -t 0 && test -c /dev/tty")
	pty.Cmd.Stdin = pty.Slave
	pty.Cmd.Stdout = pty.Slave
	pty.Cmd.Stderr = pty.Slave
	ConfigureExternalEditorProcess(pty.Cmd)
	if err := pty.Cmd.Run(); err != nil {
		t.Fatalf("editor process could not use its controlling terminal: %v", err)
	}
}
