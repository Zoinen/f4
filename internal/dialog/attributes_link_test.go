package dialog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// f4#1828: a symbolic link is not pointed at nothing without asking.

func TestLinkTargetExistsRelativeToTheLink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)
	link := filepath.Join(dir, "link")
	if !linkTargetExists(t.Context(), v, link, "real.txt") || !linkTargetExists(t.Context(), v, link, filepath.Join(dir, "real.txt")) {
		t.Fatal("an existing target was taken for missing")
	}
	if linkTargetExists(t.Context(), v, link, "missing.txt") {
		t.Fatal("a missing target was taken for existing")
	}
}

func TestConfirmDanglingLinkAsksOnlyForAMissingTarget(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)
	link := filepath.Join(dir, "link")

	ran := 0
	confirmDanglingLink(v, link, "real.txt", false, func() { ran++ })
	confirmDanglingLink(v, link, "missing.txt", true, func() { ran++ }) // a junction: its creation decides
	if ran != 2 {
		t.Fatalf("proceeded %d times, want 2 without asking", ran)
	}

	before := vtui.FrameManager.GetTopFrame()
	confirmDanglingLink(v, link, "missing.txt", false, func() { ran++ })
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || vtui.FrameManager.GetTopFrame() == before || ran != 2 {
		t.Fatalf("a missing target did not ask first (ran %d)", ran)
	}
	dlg.OnResult(1) // Cancel
	if ran != 2 {
		t.Fatal("Cancel saved the link")
	}
	confirmDanglingLink(v, link, "missing.txt", false, func() { ran++ })
	vtui.FrameManager.GetTopFrame().(*vtui.Window).OnResult(0) // Save
	if ran != 3 {
		t.Fatal("Save did not save the link")
	}
}
