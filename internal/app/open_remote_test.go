package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// notLocalVFS is a file system that is not *vfs.OSVFS, like an archive or a
// remote host, but reads real files so that the test has something to copy.
type notLocalVFS struct{ *vfs.OSVFS }

// Enter on a file that is not on a local disk (inside an archive, f4#1999)
// copies it to a temporary directory and opens the copy with the associated
// program instead of refusing; the copy is removed when f4 exits.
func TestEnterOnNonLocalFileOpensATemporaryCopy(t *testing.T) {
	if _, _, supported := panel.AssociatedFileCommand("test"); !supported {
		t.Skipf("associated-file launch is unsupported on %s", runtime.GOOS)
	}
	discardStaleUITasks()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	launches := make(chan recordedExternalUICall, 1)
	pf.ExternalUIRunner = func(command string, args []string, dir string) error {
		launches <- recordedExternalUICall{command: command, args: append([]string(nil), args...), dir: dir}
		return nil
	}

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "report.pdf"), []byte("%PDF-1.4 test"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := notLocalVFS{vfs.NewOSVFS(src)}
	path := v.Join(src, "report.pdf")

	ActionExecute(pf, v, src, "report.pdf", path)

	var launch recordedExternalUICall
	deadline := time.After(5 * time.Second)
	for launch.command == "" {
		select {
		case launch = <-launches:
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the associated program was not started")
		}
	}

	copyDir := launch.dir
	copyPath := filepath.Join(copyDir, "report.pdf")
	wantCommand, wantArgs, _ := panel.AssociatedFileCommand(copyPath)
	if launch.command != wantCommand || len(launch.args) != len(wantArgs) || launch.args[len(wantArgs)-1] != copyPath {
		t.Fatalf("launched %s %q in %s, want %s %q", launch.command, launch.args, copyDir, wantCommand, wantArgs)
	}
	if filepath.Clean(copyDir) == filepath.Clean(src) {
		t.Fatal("the program got the original path, not a copy")
	}
	data, err := os.ReadFile(copyPath)
	if err != nil || string(data) != "%PDF-1.4 test" {
		t.Fatalf("copy: %q, %v", data, err)
	}

	removeOpenedCopies()
	if _, err := os.Stat(copyDir); !os.IsNotExist(err) {
		t.Fatalf("the copy is still there after exit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(src, "report.pdf")); err != nil {
		t.Fatalf("the original is gone: %v", err)
	}
}
