package panel

import (
	"fmt"
	"github.com/unxed/f4/vfs"
	"testing"
)

func BenchmarkDirectorySizeRestore(b *testing.B) {
	for _, count := range []int{20, 128, 10000} {
		for _, cached := range []bool{false, true} {
			b.Run(fmt.Sprintf("rows=%d/cached=%v", count, cached), func(b *testing.B) {
				fs := vfs.NewOSVFS(b.TempDir())
				fp := &FileSystemPanel{Vfs: fs}
				entries := make([]*FileEntry, count)
				if cached {
					key, _ := directorySizeCacheKey(fs, fs.GetPath())
					fp.sizeCache = map[directorySizeKey]map[string]directorySize{key: make(map[string]directorySize, count)}
				}
				for i := range entries {
					e := &FileEntry{VFSItem: vfs.VFSItem{Name: fmt.Sprintf("folder-%05d", i), IsDir: true}}
					entries[i] = e
					if cached {
						key, _ := directorySizeCacheKey(fs, fs.GetPath())
						fp.sizeCache[key][e.Name] = directorySize{bytes: int64(i)}
					}
				}
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					for _, e := range entries {
						e.SizeCalculated = false
						e.SizeApproximate = false
					}
					fp.restoreDirectorySizesFor(fs, fs.GetPath(), entries, false)
				}
			})
		}
	}
}
