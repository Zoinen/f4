package panel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type directorySizeLookupVFS struct {
	*vfs.OSVFS
	panel        *FileSystemPanel
	inspect      bool
	absCalls     int
	absUnderLock bool
}

func (fs *directorySizeLookupVFS) Abs(path string) (string, error) {
	if fs.inspect {
		fs.absCalls++
		if fs.panel.sizeCacheMu.TryLock() {
			fs.panel.sizeCacheMu.Unlock()
		} else {
			fs.absUnderLock = true
		}
	}
	return fs.OSVFS.Abs(path)
}

func TestDirectorySizeRestoreResolvesParentOnceOutsideCacheLock(t *testing.T) {
	root := t.TempDir()
	entries := make([]*FileEntry, 20)
	for i := range entries {
		name := fmt.Sprintf("folder-%d", i)
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
		entries[i] = &FileEntry{VFSItem: vfs.VFSItem{Name: name, IsDir: true}}
	}
	fs := &directorySizeLookupVFS{OSVFS: vfs.NewOSVFS(root)}
	fp := &FileSystemPanel{Vfs: fs}
	fs.panel = fp
	if _, err := fp.CalculateDirectorySize(context.Background(), fs, root, nil); err != nil {
		t.Fatal(err)
	}
	fs.inspect = true
	fp.restoreDirectorySizesFor(fs, root, entries, false)
	if fs.absCalls > 1 || fs.absUnderLock {
		t.Fatalf("restoring 20 children made %d Abs calls; backend call under cache lock=%v", fs.absCalls, fs.absUnderLock)
	}
	for _, e := range entries {
		if !e.SizeCalculated || !e.SizeApproximate {
			t.Fatalf("lost cached child %q", e.Name)
		}
	}
}

func TestDirectorySizeCacheNavigation(t *testing.T) {
	for _, syncLoad := range []bool{false, true} {
		t.Run(map[bool]string{false: "streamed", true: "synchronous"}[syncLoad], func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			oldConfig := config.App
			defer func() { config.App = oldConfig }()
			config.App.SyncPanelLoad = syncLoad
			root := t.TempDir()
			child := filepath.Join(root, "parent", "child")
			if err := os.MkdirAll(filepath.Join(child, "empty"), 0700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(child, "file")
			if err := os.WriteFile(file, []byte("1234"), 0600); err != nil {
				t.Fatal(err)
			}
			fs := vfs.NewOSVFS(root)
			fp := NewFileSystemPanel(0, 0, 80, 20, fs)
			waitForLoad(t, fp)
			stats, err := fp.CalculateDirectorySize(context.Background(), fs, filepath.Join(root, "parent"), nil)
			if err != nil || stats.Bytes != 4 {
				t.Fatalf("scan: %+v, %v", stats, err)
			}
			check := func(name string, want int64) {
				t.Helper()
				for _, e := range fp.Entries {
					if e.Name == name {
						if e.Size != want || !e.SizeCalculated || !strings.HasPrefix(entrySizeText(e), "~") {
							t.Fatalf("cached entry: %+v, text %q", e, entrySizeText(e))
						}
						if got := semanticFileSize(e); !strings.HasPrefix(got, "~") || len(got) > 11 {
							t.Fatalf("Qt cached size: %q", got)
						}
						return
					}
				}
				t.Fatalf("missing %s", name)
			}
			for _, rel := range []string{"parent", "parent/child", "parent", "parent/child"} {
				if err := fs.SetPath(filepath.Join(root, rel)); err != nil {
					t.Fatal(err)
				}
				fp.ReadDirectory()
				waitForLoad(t, fp)
				if strings.HasSuffix(rel, "child") {
					check("empty", 0)
				} else {
					check("child", 4)
				}
			}
			// A fresh scan replaces cached totals, including empty folders.
			if err := os.WriteFile(file, []byte("1234567"), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := fp.CalculateDirectorySize(context.Background(), fs, child, nil); err != nil {
				t.Fatal(err)
			}
			if err := fs.SetPath(filepath.Dir(child)); err != nil {
				t.Fatal(err)
			}
			fp.ReadDirectory()
			waitForLoad(t, fp)
			check("child", 7)
			// Another backend at the same path must not inherit this scan.
			fp.Vfs = vfs.NewOSVFS(filepath.Dir(child))
			fp.ReadDirectory()
			waitForLoad(t, fp)
			for _, e := range fp.Entries {
				if e.SizeCalculated {
					t.Fatalf("size leaked to a different VFS: %+v", e)
				}
			}
		})
	}
}

func TestDirectorySizeCacheCancelledScan(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	fs := vfs.NewOSVFS(root)
	fp := NewFileSystemPanel(0, 0, 80, 20, fs)
	waitForLoad(t, fp)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := fp.CalculateDirectorySize(ctx, fs, root, func(string, vfs.OpStats) { cancel() })
	if err != context.Canceled {
		t.Fatalf("scan error: %v", err)
	}
	fp.ReadDirectory()
	waitForLoad(t, fp)
	for _, e := range fp.Entries {
		if e.SizeCalculated {
			t.Fatalf("cancelled scan published a size: %+v", e)
		}
	}
}

func TestQtCalculatedSizeApproximationMetadata(t *testing.T) {
	for _, tc := range []struct {
		name           string
		metadata, rows bool
	}{
		{"full", false, false}, {"deferred", true, false}, {"paged", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldMetadata := semantic.SetPanelCatalogMetadataEnabled(tc.metadata)
			oldRows := semantic.SetPanelCatalogRowsEnabled(tc.rows)
			t.Cleanup(func() {
				semantic.SetPanelCatalogRowsEnabled(oldRows)
				semantic.SetPanelCatalogMetadataEnabled(oldMetadata)
			})
			e := &FileEntry{VFSItem: vfs.VFSItem{Name: "child", IsDir: true, Size: 1234}, SizeCalculated: true, SizeApproximate: true}
			fp := &FileSystemPanel{Vfs: vfs.NewOSVFS(t.TempDir()), Table: vtui.NewTable(0, 0, 40, 10, nil), Entries: []*FileEntry{e}, SelectedItems: make(map[string]bool)}
			t.Cleanup(fp.unpublishSemanticMetadataSnapshot)
			readChunk := func(m extui.PanelModel) (map[string]any, bool) {
				if tc.rows {
					return BuildLivePanelCatalogMetadataChunk(m.ID, m.Path, m.CatalogRevision, m.MetadataRevision, 0, 1)
				}
				return BuildPanelCatalogMetadataChunk(m.ID, m.Path, m.CatalogRevision, m.MetadataRevision, 0, 1)
			}
			text := func(m extui.PanelModel) string {
				if !tc.metadata {
					return m.Entries[0].SizeText
				}
				chunk, ok := readChunk(m)
				if !ok {
					t.Fatal("current metadata rejected")
				}
				return semantic.String(semantic.AppMapSlice(chunk["entries"])[0]["sizeText"])
			}
			first := fp.SemanticPanelModel(nil, 0, true)
			if got := text(first); got != "~1 234" {
				t.Fatalf("approximate=%q", got)
			}
			e.SizeApproximate = false
			if tc.metadata && !tc.rows {
				if got := text(first); got != "~1 234" {
					t.Fatalf("snapshot mutated=%q", got)
				}
			}
			// The ordinary snapshot path must detect an unchanged-byte flag
			// transition itself; the live pager uses the explicit F3 commit.
			if tc.rows {
				fp.CommitSemanticMetadataMutation()
			}
			next := fp.SemanticPanelModel(nil, 0, true)
			if (tc.metadata && next.CatalogRevision != first.CatalogRevision) || next.MetadataRevision <= first.MetadataRevision {
				t.Fatalf("wrong revisions: before=(%d,%d) after=(%d,%d)", first.CatalogRevision, first.MetadataRevision, next.CatalogRevision, next.MetadataRevision)
			}
			if got := text(next); got != "1 234" {
				t.Fatalf("exact=%q", got)
			}
			if tc.metadata {
				if _, ok := readChunk(first); ok {
					t.Fatal("obsolete metadata accepted")
				}
			}
		})
	}
}

func TestQtPhasedMetadataPreservesCalculatedDirectorySize(t *testing.T) {
	for _, approximate := range []bool{false, true} {
		name := "exact"
		if approximate {
			name = "cached"
		}
		t.Run(name, func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			oldConfig := config.App
			config.App.SyncPanelLoad = false
			t.Cleanup(func() { config.App = oldConfig })
			load := newPhasedPanelLoad(
				[]vfs.VFSItem{{Name: "folder", IsDir: true}},
				[]vfs.VFSItem{{Name: "folder", IsDir: true, Size: 32, SizeKnown: true}}, true)
			fs := newPhasedPanelVFS("/catalog", map[string]*phasedPanelLoad{"/catalog": load})
			fp := NewFileSystemPanel(0, 0, 50, 15, fs)
			t.Cleanup(func() {
				select {
				case <-load.release:
				default:
					close(load.release)
				}
				if fp.CancelLoad != nil {
					fp.CancelLoad()
				}
				fp.StopLoadingAnimation()
			})
			waitForPanelCondition(t, "phased base", func() bool { return !fp.catalogProvisional && len(fp.Entries) == 2 })
			entry := fp.Entries[1]
			entry.Size, entry.SizeCalculated, entry.SizeApproximate = 1234, true, approximate
			close(load.release)
			waitForLoad(t, fp)
			if entry.Size != 1234 || !entry.SizeCalculated || entry.SizeApproximate != approximate {
				t.Fatalf("directory metadata replaced measured size: %+v", entry)
			}
		})
	}
}
