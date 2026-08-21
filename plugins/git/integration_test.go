package gitplugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestSnapshotStillAtDirectoryRejectsStalePanelLocation(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	filesystem := vfs.NewOSVFS(root)

	current := vfs.PanelSnapshot{VFS: filesystem, Path: root}
	if !snapshotStillAtDirectory(current, root) {
		t.Fatal("current local panel did not match its observed directory")
	}

	if err := filesystem.SetPath(child); err != nil {
		t.Fatal(err)
	}
	current.Path = child
	if snapshotStillAtDirectory(current, root) {
		t.Fatal("stale worker directory matched a panel that had entered a child directory")
	}
	if !snapshotStillAtDirectory(current, child) {
		t.Fatal("current child directory did not match its observation")
	}

	if snapshotStillAtDirectory(vfs.PanelSnapshot{}, child) {
		t.Fatal("non-local snapshot matched an observed directory")
	}
}
