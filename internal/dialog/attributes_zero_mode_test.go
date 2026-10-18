//go:build !windows

package dialog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// Unchecking the last permission of a file in the attributes dialog makes
// its mode 000 (f4#1981: -r-------- stayed -r--------).
func TestApplyUnixAttributes_LastPermissionRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, nil, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	v := vfs.NewOSVFS(dir)
	item, err := v.Stat(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	edit := unixAttributesEdit{mode: 0, keepMode: 0}
	if _, err := setUnixAttributesForTargets(context.Background(), v, []AttributesTarget{{Path: path, Item: item}}, edit, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0 {
		t.Fatalf("mode after removing the last permission = %o, want 000", perm)
	}
}
