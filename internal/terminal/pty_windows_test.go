//go:build windows

package terminal

import (
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func init() {
	// The Go test executable is placed in a temporary build directory, while
	// CI installs the same ConPTY pair into this package directory. Production
	// binaries do not use this hook: they resolve the pair next to f4.exe.
	conPTYBundleDirectoryOverride = os.Getwd
}

func TestConPTYAvailable_DoesNotPanic(t *testing.T) {
	avail := ConPTYAvailable()
	if !avail {
		Pty, err := NewPTY()
		if err == nil {
			if Pty != nil {
				_ = Pty.Close() // Cleanup is secondary to the unexpected allocation success.
			}
			t.Fatal("NewPTY succeeded when ConPTYAvailable() reported false")
		}
	}
}

func TestBundledConPTYIsSelected(t *testing.T) {
	if vtui.IsWine() {
		t.Skip("Wine does not run the native ConPTY bundle")
	}
	api, err := bundledConPTY()
	if err != nil {
		t.Fatalf("bundled ConPTY: %v", err)
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

// `exit` inside a batch file ends cmd.exe itself (only `exit /b` ends just
// the batch), and ConPTY keeps its output pipe open after the client is gone
// until the pseudoconsole is closed. f4 used to sit on that pipe forever: no
// prompt could arrive, the panels stayed hidden, Ctrl+C and Ctrl+Break had
// no process to reach (#409). The shell's exit must now end the command,
// bring the panels back and start a fresh shell that reaches its prompt.
