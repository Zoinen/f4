package panel

import (
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"path/filepath"
	"runtime"
	"strings"
)

// Retain entry objects (including calculated metadata and selection) across
// observations of the same directory. The completed listing is authoritative.
func (fp *FileSystemPanel) reconcileDirectoryEntries(next []*FileEntry) bool {
	previous := make(map[string]*FileEntry, len(fp.Entries))
	oldRows := make(map[string]int, len(fp.Entries))
	ranges := []extui.M{}
	for i, entry := range fp.Entries {
		previous[entry.Name] = entry
		oldRows[entry.Name] = i
	}
	changed := len(next) != len(fp.Entries)
	cursor := fp.GetCursorIndex()
	cursorName := ""
	if cursor < len(fp.Entries) {
		cursorName = fp.Entries[cursor].Name
	}
	nextCursor := min(cursor, max(0, len(next)-1))
	for i, entry := range next {
		if entry.Name == cursorName {
			nextCursor = i
		}
		old := previous[entry.Name]
		previousItem := vfs.VFSItem{}
		if old != nil {
			previousItem = old.VFSItem
			if old.IsDir && old.SizeCalculated {
				previousItem.Size = entry.Size
			}
		}
		_, strength := plughost.MediaVersion(entry.VFSItem, false, fp.catalogRevision)
		reusable := entry.IsDir || strength != "session"
		if old != nil && sameDirectoryItem(previousItem, entry.VFSItem) && reusable {
			next[i] = old
			oldRow := oldRows[entry.Name]
			var last extui.M
			if len(ranges) > 0 {
				last = ranges[len(ranges)-1]
			}
			if last != nil && last["oldIndex"].(int)+last["count"].(int) == oldRow && last["index"].(int)+last["count"].(int) == i {
				last["count"] = last["count"].(int) + 1
			} else {
				ranges = append(ranges, extui.M{"oldIndex": oldRow, "index": i, "count": 1})
			}
		} else {
			changed = true
		}
		if i >= len(fp.Entries) || fp.Entries[i].Name != entry.Name {
			changed = true
		}
	}
	if changed {
		fp.catalogRefreshDelta = extui.M{"baseCatalogRevision": fp.catalogRevision, "oldTotalCount": len(fp.Entries), "ranges": ranges}
		fp.Entries = next
		fp.CursorIdx = nextCursor
	}
	return changed
}

func sameDirectoryItem(a, b vfs.VFSItem) bool {
	// Access time can change merely because our preview decoder read a file.
	// It does not change the displayed row or its media version.
	a.ATime, b.ATime = a.MTime, b.MTime
	if !a.MTime.Equal(b.MTime) || !a.CTime.Equal(b.CTime) {
		return false
	}
	b.MTime, b.CTime, b.ATime = a.MTime, a.CTime, a.ATime
	return a == b
}

// Refresh every live view of an affected directory, not every side in the
// workspace. Parent views may show changed directory stats or folder previews.
type panelOperationLocation struct {
	owner      *PanelsFrame
	filesystem vfs.VFS
	directory  string
}

func refreshOperationLocations(owner *PanelsFrame, filesystem vfs.VFS, directory string) {
	refreshOperationViews(panelOperationLocation{owner, filesystem, directory})
}

func refreshOperationViews(locations ...panelOperationLocation) {
	seen := map[*FileSystemPanel]bool{}
	for _, location := range locations {
		owner, filesystem, directory := location.owner, location.filesystem, location.directory
		if filesystem == nil {
			continue
		}
		targetFS, targetPath := fileops.TransferIdentity(filesystem, directory)
		_, localTarget := targetFS.(*vfs.OSVFS)
		normalize := func(path string) string {
			if localTarget {
				path = filepath.Clean(path)
				if runtime.GOOS == "windows" {
					path = strings.ToLower(path)
				}
			}
			return path
		}
		targetPath = normalize(targetPath)
		parentPath := normalize(targetFS.Dir(targetPath))
		visit := func(frame *PanelsFrame) {
			if frame == nil || frame.Closed {
				return
			}
			for _, panel := range frame.Panels {
				fp, ok := panel.(*FileSystemPanel)
				if !ok || fp.Vfs == nil || seen[fp] {
					continue
				}
				candidate, path := fileops.TransferIdentity(fp.Vfs, fp.Vfs.GetPath())
				_, local := candidate.(*vfs.OSVFS)
				if !(local && localTarget) && !vfs.SameSession(candidate, targetFS) {
					continue
				}
				path = normalize(path)
				if path != targetPath && path != parentPath {
					continue
				}
				seen[fp] = true
				fp.ReadDirectory()
			}
		}
		visit(owner)
		if vtui.FrameManager != nil {
			for _, screen := range vtui.FrameManager.Screens {
				if screen == nil {
					continue
				}
				for _, frame := range screen.Frames {
					if pf, ok := frame.(*PanelsFrame); ok {
						visit(pf)
					}
				}
			}
		}
	}
}
