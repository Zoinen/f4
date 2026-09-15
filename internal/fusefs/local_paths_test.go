package fusefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestMenuMountPathsRequireSameSessionAndContainment(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "name.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	source := vfs.NewOSVFS(root)
	m := &Mount{MountPoint: root, RootPath: "/remote", ReadOnly: true, done: make(chan struct{}), bridge: &bridge{v: source}}
	if paths, ok := m.localPaths(source, []string{"/remote/name.txt"}); !ok || len(paths) != 1 || paths[0] != filepath.Join(root, "name.txt") {
		t.Fatalf("paths=%v ok=%v", paths, ok)
	}
	for _, paths := range [][]string{{"/remote/../outside"}, {"/remote/missing"}, {"/remote/name.txt", "/other/name.txt"}} {
		if got, ok := m.localPaths(source, paths); ok {
			t.Fatalf("accepted %v as %v", paths, got)
		}
	}
	if _, ok := m.localPaths(vfs.NewOSVFS(root), []string{"/remote/name.txt"}); ok {
		t.Fatal("different provider instance accepted")
	}
	close(m.done)
	if _, ok := m.localPaths(source, []string{"/remote/name.txt"}); ok {
		t.Fatal("disappeared mount accepted")
	}
}

func TestMenuMountPathsRejectClosedProvider(t *testing.T) {
	source := vfs.NewOSVFS(t.TempDir())
	m := &Mount{bridge: &bridge{v: source, closed: true}, done: make(chan struct{})}
	if _, ok := m.localPaths(source, []string{"file"}); ok {
		t.Fatal("closed provider accepted")
	}
}
