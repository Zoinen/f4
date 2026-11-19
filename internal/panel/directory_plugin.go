package panel

import (
	"context"
	"fmt"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type pluginDirectoryMount interface {
	OpenPanelDirectory(context.Context, string, bool) (vfs.VFS, string, error)
	ClosePanelMount()
	PanelParentPath(string) string
}

func hasPanelVFSParent(filesystem vfs.VFS) bool {
	if snapshot, ok := filesystem.(interface{ HasParentVFS() bool }); ok {
		return snapshot.HasParentVFS()
	}
	return filesystem.ParentVFS() != nil
}

func closePanelMount(filesystem vfs.VFS) {
	if mount, ok := filesystem.(pluginDirectoryMount); ok {
		mount.ClosePanelMount()
		return
	}
	_ = filesystem.Close()
}

func (fp *FileSystemPanel) navigatePluginDirectory(target string, entry bool) bool {
	source, ok := fp.Vfs.(pluginDirectoryMount)
	if !ok {
		return false
	}
	leaving := target == ".." && fp.Vfs.IsAtRoot()
	if leaving && !hasPanelVFSParent(fp.Vfs) {
		return true
	}
	pendingTarget := target
	if entry {
		// Keep session persistence pointed at the current qualified directory
		// until the worker has resolved the destination.
		pendingTarget = fp.Vfs.GetPath()
	}
	var selection string
	return fp.openVFSAsync(pendingTarget, func(ctx context.Context) (vfs.VFS, error) {
		next, nextSelection, err := source.OpenPanelDirectory(ctx, target, entry)
		selection = nextSelection
		return next, err
	}, func(next vfs.VFS) {
		source.ClosePanelMount()
		if leaving && fp.ProviderEntryName != "" {
			selection = fp.ProviderEntryName
			fp.ProviderEntryName = ""
		}
		fp.Vfs = next
		fp.PendingSelection = selection
		fp.showCurrentVFSLoadingRows()
		fp.ReadDirectory()
	}, func(err error) {
		vtui.DebugLog("[FIX:uri-navigation] directory open failed: %v", err)
		fp.showDirectoryError(" Error ", fmt.Sprintf("Cannot access folder:\n%v", err))
	})
}
