package fileops

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// A copy takes the source's creation time along where ReadBirthTime can find
// it (Linux), and changes nothing where it cannot (f4#2035).
func TestWithSourceBirthTimeFollowsReadBirthTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	osv := vfs.NewOSVFS(dir)
	want, known := vfs.ReadBirthTime(path)

	var item vfs.VFSItem
	withSourceBirthTime(&item, osv, path, osv)
	if item.SetBTime != known {
		t.Fatalf("SetBTime = %v, want %v (birth time known: %v)", item.SetBTime, known, known)
	}
	if known && !item.BTime.Equal(want) {
		t.Errorf("BTime = %v, want %v", item.BTime, want)
	}
}

// Where the stat already carries a birth time (Windows, macOS) the copy keeps
// its existing behaviour and does not look the source up again.
func TestWithSourceBirthTimeKeepsStatBirthTime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	osv := vfs.NewOSVFS(dir)
	stamp := time.Unix(1_000_000_000, 0)
	item := vfs.VFSItem{BTime: stamp, KnownMetadata: vfs.MetadataBTime}
	withSourceBirthTime(&item, osv, path, osv)
	if item.SetBTime || !item.BTime.Equal(stamp) {
		t.Errorf("item changed: SetBTime=%v BTime=%v", item.SetBTime, item.BTime)
	}
}
