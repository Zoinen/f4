package panel

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

func commandPayloadFiles(t *testing.T) []string {
	t.Helper()
	if err := InitializeProcessEnvironmentRuntime(); err != nil {
		t.Fatal(err)
	}
	paths, err := filepath.Glob(filepath.Join(processEnvironmentRuntimeDir, "command-posix-*"))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestLocalCommandPayloadPrivateAndRemoved(t *testing.T) {
	before := commandPayloadFiles(t)
	cmd := "printf '%s' " + ShellSingleQuote(strings.Repeat("it's private\n", 100))
	arg, cleanup, err := prepareLocalCommandEvaluation(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	if len(arg) > 256 || strings.Contains(arg, "private") {
		t.Fatalf("interactive argument is not a bounded, value-free handoff: %q", arg)
	}
	files := commandPayloadFiles(t)
	if len(files) != len(before)+1 {
		t.Fatalf("payload files: before %d, after %d", len(before), len(files))
	}
	found := false
	for _, path := range files {
		if !strings.Contains(arg, ShellSingleQuote(path)) {
			continue
		}
		found = true
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("payload permissions: %v", info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "eval "+ShellSingleQuote(cmd) {
			t.Fatalf("payload did not preserve exact command: %v", err)
		}
	}
	if !found {
		t.Fatal("handoff did not reference its private payload file")
	}
	cleanup()
	cleanup() // failure/shutdown can follow a completion callback
	if got := commandPayloadFiles(t); len(got) != len(before) {
		t.Fatalf("cleanup left %d payload files", len(got)-len(before))
	}
}

type commandWriteFailurePTY struct {
	*mockPty
	err error
}

func (p *commandWriteFailurePTY) Write(b []byte) (int, error) {
	if p.err == nil {
		return len(b) - 1, nil
	}
	return 0, p.err
}

func TestPanelsFrameLongCommandDispatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local POSIX command handoff")
	}
	for _, tt := range []struct {
		name string
		fail bool
		err  error
	}{
		{name: "completion"},
		{name: "close"},
		{name: "write error", fail: true, err: io.ErrClosedPipe},
		{name: "short write", fail: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			before := commandPayloadFiles(t)
			pf := setupMockPanelsFrame(t)
			t.Cleanup(pf.Close)
			pty := pf.Pty.(*mockPty)
			if tt.fail {
				pf.Pty = &commandWriteFailurePTY{mockPty: pty, err: tt.err}
			}
			fs := pf.GetActivePanel().Vfs
			pf.LastPtyPath, pf.LastPtyVFS = fs.GetPath(), fs
			cmd := "printf '%s' '" + strings.Repeat("long payload ", 200) + "'"
			pf.CmdLine.Edit.SetText(cmd)
			pressKey(pf, &vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN,
			})
			if tt.fail {
				if pf.Executing || !pf.ShowPanels || pf.TermView.Muted {
					t.Fatal("failed dispatch left the terminal busy, hidden, or muted")
				}
				if pf.CmdLine.Edit.GetText() != cmd {
					t.Fatal("failed dispatch discarded the command")
				}
			} else {
				wire := pty.String()
				if len(wire) >= 1024 || strings.Contains(wire, "long payload") {
					t.Fatalf("long user text leaked into the interactive line (%d bytes)", len(wire))
				}
				if len(commandPayloadFiles(t)) != len(before)+1 || !pf.Executing {
					t.Fatal("successful dispatch did not retain its payload until completion")
				}
				if tt.name == "close" {
					pf.Close()
				} else {
					pf.ReturnToPanels = false
					pf.applyTerminalBusyChange(false, false)
				}
			}
			if pf.commandPayloadCleanup != nil || len(commandPayloadFiles(t)) != len(before) {
				t.Fatal("command lifecycle left a private payload behind")
			}
		})
	}
}

func TestLocalCommandPayloadCreationFailure(t *testing.T) {
	commandPayloadFiles(t)
	oldDir := processEnvironmentRuntimeDir
	processEnvironmentRuntimeDir = filepath.Join(t.TempDir(), "not-a-directory")
	t.Cleanup(func() { processEnvironmentRuntimeDir = oldDir })
	if err := os.WriteFile(processEnvironmentRuntimeDir, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	_, cleanup, err := prepareLocalCommandEvaluation(strings.Repeat("x", 1024))
	if err == nil || cleanup != nil || !errors.As(err, new(*os.PathError)) {
		t.Fatalf("expected preparation failure without payload ownership, got %v", err)
	}
}
