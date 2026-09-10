package viewer

import (
	bytes "bytes"
	vfs "github.com/unxed/f4/vfs"
	testing "testing"
)

func TestViewerUnwrappedSeekYieldsAndResumes(t *testing.T) {
	viewer, file := constructionTestViewer(t, bytes.Repeat([]byte("a"), 8*1024*1024), 80, 24, false)
	file.profile = vfs.ReadAccessDirectLocal
	for step := 0; step < 150; step++ {
		before := len(file.reads)
		offset, ready := viewer.semanticResolveTextWindowOffset(7 * 1024 * 1024)
		// Each slice scans 64 KiB and can miss at most twice in the 256 KiB
		// source cache. The old unlimited adapter read 27.8 MB in one call.
		if len(file.reads)-before > 2 {
			t.Fatal("seek exceeded source-read budget")
		}
		if step == 0 && ready {
			t.Fatal("seek did not yield")
		}
		if ready {
			if offset != 0 {
				t.Fatalf("wrong line start: %d", offset)
			}
			return
		}
	}
	t.Fatal("seek did not resume to BOF")
}
