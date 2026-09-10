package panel

import (
	"context"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"time"
)

// Aggregate only when catalog, metadata or selection changes. Cursor updates
// reuse the totals; slow filesystem queries never run in a semantic exporter.
type nativePanelStatusCache struct {
	initialized                  bool
	catalog, metadata, selection int64
	files, directories           int
	selectedSize, totalSize      int64
	path, linkPath               string
	free                         uint64
	freeKnown                    bool
	target                       string
	querying                     bool
	queriedAt                    time.Time
}

func (fp *FileSystemPanel) enrichNativePanelStatus(model *extui.PanelModel) {
	if fp == nil || fp.Vfs == nil {
		return
	}
	model.UseSortGroups = fp.UseSortGroups
	cache := &fp.nativeStatus
	if !cache.initialized || cache.catalog != fp.catalogRevision || cache.metadata != fp.metadataRevision || cache.selection != fp.selectionRevision {
		cache.initialized = true
		cache.catalog, cache.metadata, cache.selection = fp.catalogRevision, fp.metadataRevision, fp.selectionRevision
		cache.files, cache.directories, cache.selectedSize, cache.totalSize = 0, 0, 0, 0
		for _, entry := range fp.Entries {
			if entry.Name == ".." {
				continue
			}
			if !entry.IsDir {
				cache.totalSize += max(int64(0), entry.Size)
			}
			if !entry.Selected {
				continue
			}
			if entry.IsDir {
				cache.directories++
			} else {
				cache.files++
			}
			if !entry.IsDir || entry.SizeCalculated {
				cache.selectedSize += max(int64(0), entry.Size)
			}
		}
	}
	model.SelectedFiles, model.SelectedDirectories = cache.files, cache.directories
	model.SelectedSize, model.TotalSize = cache.selectedSize, cache.totalSize
	path := fp.Vfs.GetPath()
	linkPath := ""
	if cursor := fp.GetCursorIndex(); cursor >= 0 && cursor < len(fp.Entries) && fp.Entries[cursor].IsSymlink {
		linkPath = fp.Vfs.Join(path, fp.Entries[cursor].Name)
	}
	if cache.path == path {
		model.FreeSpace, model.FreeSpaceKnown = cache.free, cache.freeKnown
		if cache.linkPath == linkPath {
			model.SymlinkTarget = cache.target
		}
	}
	if !model.ShowFileInfo || vtui.ActiveBackend() != "qt" || cache.querying ||
		(cache.path == path && cache.linkPath == linkPath && time.Since(cache.queriedAt) < 5*time.Second) {
		return
	}
	source := fp.Vfs
	_, local := source.(*vfs.OSVFS)
	if !local && linkPath == "" {
		return
	}
	cache.querying = true
	frames := vtui.FrameManager
	go func() {
		var free uint64
		known := false
		if local {
			if info, ok := sysinfo.FS(path); ok {
				free, known = info.Free, true
			}
		}
		target := ""
		if linkPath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			target, _ = vfs.Readlink(ctx, source, linkPath)
			cancel()
		}
		frames.PostTask(func() {
			cache.querying = false
			if fp.Vfs != source || fp.Vfs.GetPath() != path {
				frames.Redraw()
				return
			}
			cache.path, cache.linkPath, cache.free, cache.freeKnown, cache.target = path, linkPath, free, known, target
			cache.queriedAt = time.Now()
			frames.Redraw()
		})
	}()
}
