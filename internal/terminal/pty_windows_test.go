//go:build windows

package terminal

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtui"
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

// drainFrameTasks runs queued UI tasks while an integration test waits for a
// ConPTY shell to publish its first prompt.
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

// TestConPTYPackageKeepsLongLinesWhole downloads the ConPTY package the way
// f4 does on first use, runs real programs in it, and checks what issue #885
// is about: a line longer than the window reaches the terminal as one run of
// text, lands in the log as one line, and stays one line through a reflow.
//
// It needs the network, so it runs only when F4_NATIVE_CONPTY_PACKAGE=1,
// which the Windows CI cells set. The two programs are separate subtests
// because they write through different paths: PowerShell is a console
// application of its own, while echo is cmd.exe's builtin, which is how most
// output reaches f4's shell.
func TestConPTYPackageKeepsLongLinesWhole(t *testing.T) {
	if os.Getenv("F4_NATIVE_CONPTY_PACKAGE") != "1" {
		t.Skip("set F4_NATIVE_CONPTY_PACKAGE=1 to download and run the ConPTY package")
	}
	if vtui.IsWine() {
		t.Skip("Wine has no ConPTY")
	}
	pkg, ok := conPTYPackageFor(runtime.GOARCH)
	if !ok {
		t.Skipf("no ConPTY package for %s", runtime.GOARCH)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// Not t.TempDir(): conpty.dll stays loaded in this process until it
	// exits, and Windows does not delete an image file that is mapped, so
	// the automatic cleanup fails the test after the test itself has passed.
	// Whatever can be removed is; the rest is left in the temp directory.
	root, err := os.MkdirTemp("", "f4-conpty-package-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Logf("left behind, still loaded: %v", err)
		}
	})
	dir, err := installConPTYPackage(ctx, http.DefaultClient, root, pkg)
	if err != nil {
		t.Fatalf("install ConPTY package: %v", err)
	}
	api, err := loadConPTYPackage(dir)
	if err != nil {
		t.Fatalf("load ConPTY package: %v", err)
	}

	const width = 80
	for _, tc := range []struct {
		name, line, command string
	}{
		{"powershell", strings.Repeat("A", 300), `powershell.exe -NoLogo -NoProfile -NonInteractive -Command "Write-Output ('A' * 300)"`},
		{"cmd-echo", strings.Repeat("B", 300), `cmd.exe /d /c echo ` + strings.Repeat("B", 300)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := runInConPTYPackage(t, api, tc.command)
			t.Logf("%d bytes from the pseudoconsole: %q", len(raw), clipForLog(raw))
			if !bytes.Contains(raw, []byte(tc.line)) {
				t.Fatalf("the %d-character line did not arrive as one run of text", len(tc.line))
			}

			tv := NewTerminalView(width, 25)
			tv.SetReflow(true)
			parser := NewAnsiParser(tv, nil)
			parser.Process(raw)
			assertOneLogLine(t, tv, tc.line, "at width 80")
			for _, w := range []int{33, 120, 80} {
				tv.Resize(w, 25)
				assertOneLogLine(t, tv, tc.line, fmt.Sprintf("after reflow to %d", w))
			}
		})
	}
}

func assertOneLogLine(t *testing.T, tv *TerminalView, line, when string) {
	t.Helper()
	log := string(tv.GetAllLogBytes())
	count := 0
	for _, l := range strings.Split(log, "\n") {
		if strings.TrimRight(l, "\r") == line {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("%s: the line is in the log %d times as a whole line; log %q", when, count, log)
	}
}

func clipForLog(b []byte) []byte {
	if len(b) > 2000 {
		return b[:2000]
	}
	return b
}

// runInConPTYPackage runs one command in a pseudoconsole of the package and
// returns everything the pseudoconsole wrote until it closed.
func runInConPTYPackage(t *testing.T, api *conPTYAPI, command string) []byte {
	t.Helper()
	p, err := newPTYWithAPI(api)
	if err != nil {
		t.Fatalf("create pseudoconsole: %v", err)
	}
	defer p.Close()
	if !p.PreservesLogicalLines() {
		t.Fatal("a pseudoconsole of the package does not report logical lines")
	}
	out := make(chan []byte, 1)
	go func() {
		var all bytes.Buffer
		buf := make([]byte, 32768)
		for {
			n, err := p.Read(buf)
			all.Write(buf[:n])
			if err != nil {
				out <- all.Bytes()
				return
			}
		}
	}()
	if err := p.Run(command); err != nil {
		t.Fatalf("run %q: %v", command, err)
	}
	// The exit watcher closes the pseudoconsole when the program ends, and
	// the read loop then sees the end of the stream.
	select {
	case raw := <-out:
		return raw
	case <-time.After(90 * time.Second):
		t.Fatalf("%q did not finish", command)
		return nil
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
// `exit` inside a batch file ends cmd.exe itself (only `exit /b` ends just
// the batch), and ConPTY keeps its output pipe open after the client is gone
// until the pseudoconsole is closed. f4 used to sit on that pipe forever: no
// prompt could arrive, the panels stayed hidden, Ctrl+C and Ctrl+Break had
// no process to reach (#409). The shell's exit must now end the command,
// bring the panels back and start a fresh shell that reaches its prompt.
