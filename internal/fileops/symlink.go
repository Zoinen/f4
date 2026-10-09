package fileops

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/unxed/f4/vfs"
)

// copySymlinkAsLink recreates the link at srcPath as a link at destPath, for a
// copy with "Copy symlink contents" turned off and for every move (#722).
//
// handled is false when either side cannot read or make links, or the source
// cannot say where it points (a junction, say): the caller then copies what
// the link points at, as f4 always did, rather than invent a target.
func copySymlinkAsLink(ctx context.Context, srcVfs vfs.VFS, srcPath string, dstVfs vfs.VFS, destPath string, state *FileOpState, stat vfs.VFSItem) (bool, error) {
	srcLinks, ok := srcVfs.(vfs.SymlinkVFS)
	if !ok {
		return false, nil
	}
	dstLinks, ok := dstVfs.(vfs.SymlinkVFS)
	if !ok {
		return false, nil
	}
	target, err := srcLinks.Readlink(ctx, srcPath)
	if err != nil || target == "" {
		return false, nil
	}

	for {
		existing, err := vfs.Lstat(ctx, dstVfs, destPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				break
			}
			if state.tolerateWrite(destPath, err) {
				return true, nil
			}
			return true, err
		}
		if existing.IsDir && !existing.IsSymlink {
			err := fmt.Errorf("cannot overwrite folder with link: %s", dstVfs.Base(destPath))
			if state.tolerateWrite(destPath, err) {
				return true, nil
			}
			return true, err
		}
		if state.SkipAll {
			state.skipItem(srcPath, destPath)
			return true, nil
		}
		if !state.OverwriteAll {
			choice, remember := AskOverwrite(ctx, destPath, stat, existing, state.Anchor)
			switch choice {
			case 1: // Overwrite
				if remember {
					state.OverwriteAll = true
				}
			case 2: // Skip
				if remember {
					state.SkipAll = true
				}
				state.skipItem(srcPath, destPath)
				return true, nil
			case 3: // Rename
				newName := AskRename(ctx, dstVfs.Base(destPath), state.Anchor)
				if newName == "" {
					return true, context.Canceled
				}
				destPath = dstVfs.Join(dstVfs.Dir(destPath), newName)
				continue
			case 4, 5: // Append and Resume mean nothing for a link: ask again.
				continue
			default:
				return true, context.Canceled
			}
		}
		if err := dstVfs.Remove(ctx, destPath); err != nil {
			if state.tolerateWrite(destPath, err) {
				return true, nil
			}
			return true, err
		}
		break
	}

	if err := dstLinks.Symlink(ctx, target, destPath); err != nil {
		if state.tolerateWrite(destPath, err) {
			return true, nil
		}
		return true, err
	}
	state.fileCopied(srcPath, destPath)
	if state.Tracker != nil {
		state.Tracker.FileDone()
		if state.UpdateUI != nil {
			state.UpdateUI(true)
		}
	}
	return true, nil
}
