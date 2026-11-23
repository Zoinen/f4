package panel

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostpath"
	"github.com/unxed/vtui"
)

type panelPathReveal struct {
	filesystem vfs.VFS
	path       string
	name       string
	hidden     bool
	revision   int64
}

func normalizePathInput(input, currentPath string, local bool) string {
	target := strings.TrimSpace(stripDirChangeQuotes(strings.TrimSpace(input)))
	if local && vfs.WindowsPersonality() {
		// A pasted drive path may have the slash that follows a URL authority.
		// Only drop it before a drive letter, so UNC roots stay intact.
		if len(target) > 2 && (target[0] == '/' || target[0] == '\\') {
			drivePath := target[1:]
			if len(hostpath.VolumeName(drivePath)) == 2 &&
				(len(drivePath) == 2 || drivePath[2] == '/' || drivePath[2] == '\\') {
				target = drivePath
			}
		}
		volume := hostpath.VolumeName(target)
		if volume != "" || !vfs.IsURIPath(target) {
			target = filepath.FromSlash(target)
			if len(target) == 2 && len(volume) == 2 {
				target += `\`
			}
			if volume == "" && strings.HasPrefix(target, `\`) && !strings.HasPrefix(target, `\\`) {
				target = hostpath.VolumeName(currentPath) + target
			}
		}
	}
	if local && !vfs.IsURIPath(target) {
		target = expandPathHome(target)
	}
	return target
}

func (fp *FileSystemPanel) navigateTypedLocalPath(filesystem *vfs.OSVFS, target string) bool {
	oldPath := filesystem.GetPath()
	// The background task resolves against a private path snapshot. The
	// panel keeps its filesystem and directory cache when the result arrives.
	source := vfs.NewOSVFS(oldPath)
	var selectedName string
	var selectedHidden bool
	return fp.openVFSAsync(
		"",
		func(ctx context.Context) (vfs.VFS, error) {
			resolve := source.Abs
			if hostpath.VolumeName(target) != "" {
				resolve = hostpath.Abs
			}
			path, err := resolve(target)
			if err != nil {
				return nil, err
			}
			item, err := source.Stat(ctx, path)
			if os.IsPermission(err) {
				// Preserve SetPath's handling of protected junctions and
				// accessible directories reached through elevated lookups.
				path, err = source.ResolveElevated(path)
			}
			if err != nil {
				return nil, err
			}
			if item.Name != "" && !item.IsDir {
				selectedName = vfs.NativeEntryName(path, item.Name)
				selectedHidden = item.IsHidden
				path = hostpath.Dir(path)
			}
			source.CommitPath(path)
			return source, nil
		},
		func(destination vfs.VFS) {
			path := destination.GetPath()
			filesystem.CommitPath(path)
			if selectedName != "" {
				fp.pathInputReveal = panelPathReveal{
					filesystem: filesystem,
					path:       path,
					name:       selectedName,
					hidden:     selectedHidden,
					revision:   fp.pathInputReveal.revision + 1,
				}
				fp.PendingSelection = selectedName
			} else {
				fp.PendingSelection = filesystem.Base(oldPath)
			}
			vtui.DebugLog("[FIX:path-input] directory=%q file=%q", path, selectedName)
			fp.ReadDirectory()
		},
		func(err error) {
			vtui.DebugLog("[FIX:path-input] path=%q error=%v", target, err)
			fp.showDirectoryError(" Error ", fmt.Sprintf("Cannot access path:\n%v", err))
		},
	)
}

func (fp *FileSystemPanel) cursorRevealRevision() int64 {
	reveal := fp.pathInputReveal
	if !reveal.matches(fp.Vfs, fp.Vfs.GetPath()) || fp.catalogProvisional ||
		fp.GetRawSelectedName() != reveal.name {
		return 0
	}
	return reveal.revision
}

func (reveal panelPathReveal) matches(filesystem vfs.VFS, path string) bool {
	return reveal.name != "" && reveal.path == path && fileops.SameVFSInstance(reveal.filesystem, filesystem)
}
