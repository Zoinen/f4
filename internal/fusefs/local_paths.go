package fusefs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/vfs"
)

// LocalPaths resolves a complete selection through an existing in-process
// mount with the same VFS session. Registry labels alone cannot authenticate a
// connection, so detached mounts must be browsed by their actual OS paths.
func LocalPaths(source vfs.VFS, paths []string) ([]string, bool, bool) {
	for _, mount := range List() {
		if result, ok := mount.localPaths(source, paths); ok {
			return result, mount.ReadOnly, true
		}
	}
	return nil, false, false
}

func (m *Mount) localPaths(source vfs.VFS, paths []string) ([]string, bool) {
	if m.bridge == nil || len(paths) == 0 {
		return nil, false
	}
	m.bridge.mu.RLock()
	matches := !m.bridge.closed && vfs.SameSession(source, m.bridge.v)
	m.bridge.mu.RUnlock()
	// A stat re-enters FUSE and may require the bridge lock exclusively.
	if !matches {
		return nil, false
	}
	select {
	case <-m.done:
		return nil, false
	default:
	}
	var result []string
	for _, path := range paths {
		rel, err := filepath.Rel(filepath.FromSlash(m.RootPath), filepath.FromSlash(path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, false
		}
		local := filepath.Join(m.MountPoint, rel)
		if _, err := os.Lstat(local); err != nil {
			return nil, false
		}
		// Intermediate symlinks must not escape the mount. Preserve the final
		// symlink itself so menu actions can rename/delete that link.
		parent, err := filepath.EvalSymlinks(filepath.Dir(local))
		root, rootErr := filepath.EvalSymlinks(m.MountPoint)
		if err != nil || rootErr != nil {
			return nil, false
		}
		inside, err := filepath.Rel(root, parent)
		if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
			return nil, false
		}
		result = append(result, local)
	}
	return result, true
}
