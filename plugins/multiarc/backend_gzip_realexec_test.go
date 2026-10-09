package multiarc

import (
	"context"
	"path/filepath"
	"testing"
)

// Saving the one member of a lone .gz from the editor recompresses it in
// place.
func TestGzipRealReplaceMember(t *testing.T) {
	requireRealTool(t, "gzip")
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"app.log": "old line\n"})
	runReal(t, dir, "gzip", "-f", "app.log")
	arc := filepath.Join(dir, "app.log.gz")
	t.Cleanup(closeSharedMultiArcTempDirs)
	v := openReal(t, arc)

	writeMember(t, v, "/app.log", "new line\n")
	if got := readMember(t, openReal(t, arc), "/app.log"); got != "new line\n" {
		t.Fatalf("app.log = %q after replacing it", got)
	}
	if err := v.Remove(context.Background(), "/app.log"); err == nil {
		t.Error("deleting the only member of a .gz should be refused")
	}
	if _, err := v.Create(context.Background(), "/other.log"); err == nil {
		t.Error("a second member in a .gz should be refused")
	}
	assertNoScratchLeft(t, arc)
}
