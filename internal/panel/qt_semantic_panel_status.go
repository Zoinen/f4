package panel

import (
	"context"
	"time"

	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Rebuild on catalog/metadata changes; tracked selection changes update totals
// incrementally. Slow filesystem queries never run in a semantic exporter.
type nativePanelStatusCache struct {
	initialized                  bool
	catalog, metadata, selection int64
	files, directories           int
	totalFiles, totalDirectories int
	selectedSize, totalSize      int64
	path, linkPath               string
	free                         uint64
	capacity                     uint64
	source                       vfs.VFS
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
		cache.totalFiles, cache.totalDirectories = 0, 0
		for _, entry := range fp.Entries {
			if entry == nil || entry.Name == ".." {
				continue
			}
			if !entry.IsDir {
				cache.totalFiles++
				cache.totalSize += max(int64(0), entry.Size)
			} else {
				cache.totalDirectories++
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
	model.TotalFiles, model.TotalDirectories = cache.totalFiles, cache.totalDirectories
	model.SelectedSize, model.TotalSize = cache.selectedSize, cache.totalSize
	path := fp.Vfs.GetPath()
	linkPath := ""
	if cursor := fp.GetCursorIndex(); cursor >= 0 && cursor < len(fp.Entries) && fp.Entries[cursor].IsSymlink {
		linkPath = fp.Vfs.Join(path, fp.Entries[cursor].Name)
	}
	if cache.source == fp.Vfs && cache.path == path {
		model.FreeSpace, model.FreeSpaceKnown = cache.free, cache.freeKnown
		model.DiskTotalSpace = cache.capacity
		if cache.linkPath == linkPath {
			model.SymlinkTarget = cache.target
		}
	}
	if vtui.ActiveBackend() != "qt" || cache.querying ||
		(cache.source == fp.Vfs && cache.path == path && cache.linkPath == linkPath && time.Since(cache.queriedAt) < 5*time.Second) {
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
		var free, capacity uint64
		known := false
		if local {
			if info, ok := sysinfo.FS(path); ok {
				free, known = info.Free, true
				capacity = info.Total
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
			cache.capacity, cache.source = capacity, source
			cache.queriedAt = time.Now()
			frames.Redraw()
		})
	}()
}

// Keep warm status aggregates O(changed rows), including during held gestures.
// Catalog or metadata invalidation still takes the normal exporter rebuild.
func (fp *FileSystemPanel) updateNativeStatusSelection(entry *FileEntry, selected bool) {
	cache := &fp.nativeStatus
	if !cache.initialized || cache.catalog != fp.catalogRevision || cache.metadata != fp.metadataRevision || cache.selection != fp.selectionRevision {
		return
	}
	delta := 1
	if !selected {
		delta = -1
	}
	if entry.IsDir {
		cache.directories += delta
	} else {
		cache.files += delta
	}
	if !entry.IsDir || entry.SizeCalculated {
		cache.selectedSize += int64(delta) * max(int64(0), entry.Size)
	}
	cache.selection = fp.selectionRevision + 1
}
