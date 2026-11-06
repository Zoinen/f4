package panel

import (
	"context"
	"reflect"
	"strings"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// entrySizeText is what an entry shows where a size goes: the Size column,
// the status line, the bottom frame and the semantic panel model. It follows
// far2l's FormatStr_Size with the default DirNameStyle and ShowSymlinkSize=0,
// and Far3's, which also tells a junction from a symlink: a counted folder
// shows its size, ".." shows Up, a link shows its kind instead of the size
// of the link itself, any other directory shows Folder, and a file shows
// its size.
func entrySizeText(e *FileEntry) string {
	if e.IsDir && e.SizeCalculated {
		text := fileops.FormatIntWithSpaces(e.Size)
		if e.SizeApproximate {
			text = "~" + text
		}
		return text
	}
	if e.Name == ".." {
		return i18n.Msg("Panel.UpDir")
	}
	switch vfs.LinkKindOf(&e.VFSItem) {
	case vfs.LinkJunction:
		return i18n.Msg("Panel.Junction")
	case vfs.LinkSymlink:
		return i18n.Msg("Panel.Symlink")
	}
	if e.IsDir {
		return i18n.Msg("Panel.Folder")
	}
	return fileops.FormatIntWithSpaces(e.Size)
}

// guiSizeText includes the approximation marker and suffix in the panel's
// 11-character size field. Binary unit changes discard fractional digits.
func guiSizeText(size int64, approximate bool) string {
	prefix := ""
	if approximate {
		prefix = "~"
	}
	const units = "KMGTPE"
	suffix := ""
	for unit := 0; ; unit++ {
		text := prefix + fileops.FormatIntWithSpaces(size) + suffix
		if len(text) <= 11 || unit == len(units) {
			return text
		}
		size /= 1024
		suffix = " " + string(units[unit])
	}
}

type directorySizeKey struct {
	identity any
	path     string
}

type directorySize struct {
	bytes, dirBytes int64
}

func directorySizeCacheKey(source vfs.VFS, path string) (directorySizeKey, bool) {
	var identity any = source
	if shared, ok := source.(vfs.SessionIdentity); ok {
		if key := shared.SessionKey(); key != nil && reflect.TypeOf(key).Comparable() {
			identity = key
		}
	}
	if identity == nil || !reflect.TypeOf(identity).Comparable() {
		return directorySizeKey{}, false
	}
	if absolute, err := source.Abs(path); err == nil && absolute != "" {
		path = absolute
	}
	return directorySizeKey{identity: identity, path: path}, true
}

// CalculateDirectorySize remembers each completed subtree without a second
// traversal or retaining its files. Capture source and path on the UI thread.
func (fp *FileSystemPanel) CalculateDirectorySize(ctx context.Context, source vfs.VFS, path string, cb vfs.ScanCallback) (vfs.OpStats, error) {
	return fp.CalculateDirectorySizes(ctx, source, path, []string{""}, cb)
}

// CalculateDirectorySizes also supports F3 on a panel's parent row.
func (fp *FileSystemPanel) CalculateDirectorySizes(ctx context.Context, source vfs.VFS, base string, names []string, cb vfs.ScanCallback) (vfs.OpStats, error) {
	vtui.DebugLog("[FIX:directory-size] scanning %q roots=%d", base, len(names))
	opts := vfs.ScanOptions{FollowSymlinkDirs: true, OnDirectorySize: func(path string, stats vfs.OpStats) {
		key, ok := directorySizeCacheKey(source, source.Dir(path))
		if !ok {
			return
		}
		name := strings.Clone(source.Base(path))
		fp.sizeCacheMu.Lock()
		if fp.sizeCache == nil {
			fp.sizeCache = make(map[directorySizeKey]map[string]directorySize)
		}
		children := fp.sizeCache[key]
		if children == nil {
			children = make(map[string]directorySize)
			fp.sizeCache[key] = children
		}
		children[name] = directorySize{bytes: stats.Bytes, dirBytes: stats.DirBytes}
		fp.sizeCacheMu.Unlock()
	}}
	stats, err := vfs.CalculateStatsWithOptions(ctx, source, base, names, opts, cb)
	vtui.DebugLog("[FIX:directory-size] scan %q: bytes=%d error=%v", base, stats.Bytes, err)
	return stats, err
}

func (fp *FileSystemPanel) restoreDirectorySizes(entries []*FileEntry) {
	if fp.Vfs == nil {
		return
	}
	fp.restoreDirectorySizesFor(fp.Vfs, fp.Vfs.GetPath(), entries, config.App.DirSizeCountsFolders)
}

func (fp *FileSystemPanel) restoreDirectorySizesFor(source vfs.VFS, base string, entries []*FileEntry, withFolders bool) {
	fp.sizeCacheMu.Lock()
	empty := len(fp.sizeCache) == 0
	fp.sizeCacheMu.Unlock()
	if empty {
		return
	}
	// Resolve the directory once, outside the lock. Joining/normalizing a full
	// path for every child allocated 2 strings per folder in the navigation path.
	// Parent buckets share that path and use the existing entry names directly.
	key, ok := directorySizeCacheKey(source, base)
	if !ok {
		return
	}
	fp.sizeCacheMu.Lock()
	defer fp.sizeCacheMu.Unlock()
	children := fp.sizeCache[key]
	if len(children) == 0 {
		return
	}
	for _, e := range entries {
		if e == nil || !e.IsDir || e.Name == ".." || e.SizeCalculated {
			continue
		}
		if size, ok := children[e.Name]; ok {
			e.Size = size.bytes
			if withFolders {
				e.Size += size.dirBytes
			}
			e.SizeCalculated = true
			e.SizeApproximate = true
		}
	}
}
