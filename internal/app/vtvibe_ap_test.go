package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/internal/vtvibe/ap"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestAIDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("patcher"))
		case "/bad":
			w.WriteHeader(http.StatusTeapot)
		case "/large":
			_, _ = w.Write([]byte(strings.Repeat("x", vtvibeAPMaxDownload+1)))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	got, err := aiDownload(context.Background(), server.URL+"/ok")
	if err != nil || string(got) != "patcher" {
		t.Fatalf("successful download = %q, %v; want patcher, nil", got, err)
	}

	if _, err := aiDownload(context.Background(), server.URL+"/bad"); err == nil {
		t.Fatal("non-200 response returned nil error")
	}

	got, err = aiDownload(context.Background(), server.URL+"/large")
	if err != nil || len(got) != vtvibeAPMaxDownload {
		t.Fatalf("limited download length = %d, %v; want %d, nil", len(got), err, vtvibeAPMaxDownload)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := aiDownload(canceled, server.URL+"/ok"); err == nil {
		t.Fatal("canceled download returned nil error")
	}
	if _, err := aiDownload(context.Background(), "://invalid-url"); err == nil {
		t.Fatal("invalid URL returned nil error")
	}
}

func TestAIPatchTargetDir(t *testing.T) {
	if root, ok := aiPatchTargetDir(nil); ok || root != "" {
		t.Fatalf("nil panels frame = %q, %v; want empty, false", root, ok)
	}
	if root, ok := aiPatchTargetDir(&panel.PanelsFrame{}); ok || root != "" {
		t.Fatalf("empty panels frame = %q, %v; want empty, false", root, ok)
	}

	pf := &panel.PanelsFrame{}
	pf.Panels[0] = &panel.FileSystemPanel{Vfs: &aiVFSWrapper{AIVFS: vtvibe.NewVFS(vtvibe.NewSession())}}
	pf.Panels[1] = &panel.FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir())}
	root, ok := aiPatchTargetDir(pf)
	if !ok || root == "" {
		t.Fatalf("OS panel target = %q, %v; want a local directory", root, ok)
	}
}

func TestAILastAnswerPath(t *testing.T) {
	if got := aiLastAnswerPath(&vtvibe.Session{}); got != "" {
		t.Fatalf("empty session path = %q; want empty", got)
	}
	if got := aiLastAnswerPath(vtvibe.NewSession()); got != "/chat/0001-model.md" {
		t.Fatalf("new session path = %q; want /chat/0001-model.md", got)
	}
}

func writeVtvibeINI(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(config.GetF4ConfigDir(), vtvibeIniName)
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatalf("write vtvibe.ini: %v", err)
	}
	return path
}

func TestAIPatchExitCode(t *testing.T) {
	cases := []struct {
		status ap.Status
		want   int
	}{
		{ap.StatusSuccess, 0},
		{ap.StatusPartial, 2},
		{ap.StatusFailed, 1},
		{ap.Status("unknown"), 1},
	}
	for _, c := range cases {
		if got := aiPatchExitCode(c.status); got != c.want {
			t.Errorf("aiPatchExitCode(%s) = %d, want %d", c.status, got, c.want)
		}
	}
}

func TestAIRunPatcherAppliesAndDryRuns(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	aiUndoTestStack(t)

	root := t.TempDir()
	target := filepath.Join(root, "a.txt")
	if err := os.WriteFile(target, []byte("line1\nline2\n"), 0600); err != nil {
		t.Fatalf("setup: %v", err)
	}
	patch := &vtvibe.Patch{
		ID: "aa000001",
		Text: "aa000001 AP 3.2\n\naa000001 FILE\na.txt\n\n" +
			"aa000001 REPLACE\naa000001 snippet\nline1\naa000001 content\nLINE1\n\n" +
			"aa000001 REPLACE\naa000001 snippet\nline2\naa000001 content\nLINE2\n",
	}

	pf := &panel.PanelsFrame{}
	waitForResult := func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			top := vtui.FrameManager.GetTopFrame()
			// aiRunPatcher's progress dialog closes itself with Close(),
			// which only sets its exit code (vtui's real event loop is
			// what actually pops a Done frame, right after running the
			// task/event that closed it - see frameManager.Step). This
			// harness drains vtui.FrameManager.TaskChan directly (pumpFor)
			// without going through that loop, so the closed progress
			// dialog from THIS call, or a previous one right above it,
			// can still be sitting on top, already Done, once its own
			// result dialog (pushed on top of it) has been popped below.
			// Skip any such leftover instead of mistaking it for a fresh
			// result.
			for top != nil && top.IsDone() {
				vtui.FrameManager.RemoveFrame(top)
				top = vtui.FrameManager.GetTopFrame()
			}
			// A lone, not-yet-done top frame is not necessarily the result:
			// it is just as often aiRunPatcher's progress dialog, still
			// running its worker. RunProgressTaskAfter closes the progress
			// dialog and pushes the result dialog in the very same UI task
			// (frame.go's `if dialogShown { dlg.Close() }; onComplete(err)`),
			// so a genuine result never sits alone - it always lands on top
			// of the now-Done progress dialog it replaced. Treating a lone
			// frame as the result races the worker: on a slow poll tick the
			// progress dialog can still be the only frame around (its own
			// worker goroutine simply hasn't finished os.WriteFile/ap.Apply
			// yet), and force-closing it here returns before that write ever
			// runs, so the very next os.ReadFile in this test sees the old
			// content. Wait for the two-deep shape instead: closed progress
			// underneath, fresh result on top.
			activeFrames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
			if top != nil && len(activeFrames) >= 2 {
				top.Close()
				vtui.FrameManager.RemoveFrame(top)
				return
			}
			pumpFor(20 * time.Millisecond)
		}
		t.Fatal("aiRunPatcher never reported a result")
	}

	aiRunPatcher(pf, patch, root, true, nil)
	waitForResult()
	if got, err := os.ReadFile(target); err != nil || string(got) != "line1\nline2\n" {
		t.Fatalf("dry run changed the file: %q, %v", got, err)
	}
	if aiTopUndo() != nil {
		t.Fatal("a dry run recorded a transaction to undo")
	}

	// Only the second REPLACE is checked (the review screen's Options.Only):
	// the first one must stay unapplied.
	aiRunPatcher(pf, patch, root, false, map[ap.ModKey]bool{{FilePath: "a.txt", ModIdx: 1}: true})
	waitForResult()
	if got, err := os.ReadFile(target); err != nil || string(got) != "line1\nLINE2\n" {
		t.Fatalf("applied file = %q, %v; want only the checked modification to have landed", got, err)
	}
	// The real run is on the undo stack (Ctrl+Z in the AI panel).
	u := aiTopUndo()
	if u == nil {
		t.Fatal("a real run did not record its transaction")
	}
	if err := u.Revert(); err != nil {
		t.Fatalf("Revert: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "line1\nline2\n" {
		t.Fatalf("reverted file = %q, %v; want the original", got, err)
	}
}

func TestAIWriteContextFile_WritesThroughSessionVFS(t *testing.T) {
	name := "coverage-vtvibe-ap.md"
	want := []byte("patch specification")
	if err := aiWriteContextFile(name, want); err != nil {
		t.Fatalf("aiWriteContextFile: %v", err)
	}

	r, err := vtvibe.NewVFS(aiSession()).Open(context.Background(), "/ctx/"+name)
	if err != nil {
		t.Fatalf("open attached context: %v", err)
	}
	defer func() { _ = r.Close() }()
	got := make([]byte, len(want))
	if n, err := r.ReadAt(context.Background(), got, 0); err != nil || n != len(want) || string(got) != string(want) {
		t.Fatalf("attached context = %q, n=%d, err=%v; want %q", got, n, err, want)
	}
}
