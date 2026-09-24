package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// TestIssue1184DocumentEnterFallsThroughToExecute is the panel-level
// regression test for issue #1184. An office document is a ZIP container, so
// the archive provider recognizes it by content; ordinary Enter must still
// leave it to the extension association (Execute), and only the explicit
// Ctrl+PgDn gesture may browse it as an archive.
func TestIssue1184DocumentEnterFallsThroughToExecute(t *testing.T) {
	vfs.RegisterProvider(&archive.ArchiveProvider{})
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	root := t.TempDir()
	zipPath := filepath.Join(root, "container.zip")
	createTestZipForNav(t, zipPath)
	container, err := os.ReadFile(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	docxPath := filepath.Join(root, "report.docx")
	if err := os.WriteFile(docxPath, container, 0600); err != nil { // #nosec G703 -- docxPath is inside the private test temp directory.
		t.Fatal(err)
	}

	fp := NewFileSystemPanel(0, 0, 80, 25, vfs.NewOSVFS(root))
	t.Cleanup(func() {
		fp.cancelProviderOpen()
		if fp.Vfs != nil {
			_ = fp.Vfs.Close()
		}
		if fp.CancelLoad != nil {
			fp.CancelLoad()
		}
		fp.StopLoadingAnimation()
	})
	waitForLoad(t, fp)
	fp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "report.docx"}}}
	fp.SetCursorIndex(0)

	enter := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}
	if fp.ProcessKey(enter) {
		t.Fatal("ordinary Enter on a document row was consumed instead of falling through to Execute")
	}
	if _, ok := fp.Vfs.(*vfs.OSVFS); !ok {
		t.Fatalf("ordinary Enter changed VFS to %T", fp.Vfs)
	}
	if fp.ProviderOpenTask != nil {
		t.Fatal("ordinary Enter started a provider open on a document")
	}

	pf := &PanelsFrame{ActiveIdx: 0, ShowPanels: true, CmdLine: cmdline.NewCommandLine(">")}
	pf.Panels[0] = fp
	called := 0
	oldExecute := Execute
	Execute = func(*PanelsFrame, vfs.VFS, string, string, string) { called++ }
	t.Cleanup(func() { Execute = oldExecute })
	if !pf.ProcessKey(enter) {
		t.Fatal("PanelsFrame did not handle ordinary Enter")
	}
	if called != 1 {
		t.Fatalf("ordinary Enter executed the document %d times, want once", called)
	}

	if !fp.EnterSelectedFromAction() {
		t.Fatal("Ctrl+PgDn action did not start entry into the document container")
	}
	waitForLoad(t, fp)
	if _, ok := fp.Vfs.(*archive.ArchiveVFS); !ok {
		t.Fatalf("Ctrl+PgDn left VFS as %T, want archive VFS", fp.Vfs)
	}
}
