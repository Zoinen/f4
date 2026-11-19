package plughost

import (
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// PanelEntryPath joins a name from the listing to its published directory.
// The extracted providers publish device URIs or CloudFox visual paths.
func (v *rpcURIVFS) PanelEntryPath(directory, name string) string {
	switch v.scheme {
	case "ios", "android":
		if directory == v.scheme+"://" {
			if name == ".." {
				return directory
			}
			if v.scheme == "android" {
				device := vfs.DevicePath{Scheme: v.scheme, Device: name}
				return strings.TrimSuffix(device.Root(), "/")
			}
			return directory + name
		}
		return (vfs.DevicePath{}).Join(directory, name)
	case "cloud":
		// A visual root already ends with the platform separator.
		root, tail, ok := strings.Cut(directory, ":"+string(os.PathSeparator))
		if ok {
			normalized := strings.ReplaceAll(tail+"/"+name, "\\", "/")
			return root + ":" + string(os.PathSeparator) +
				strings.ReplaceAll(strings.TrimPrefix(path.Clean("/"+normalized), "/"), "/", string(os.PathSeparator))
		}
	}
	return path.Join(directory, name)
}

func (v *rpcURIVFS) HasParentVFS() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.mount.HasParent != nil {
		return *v.mount.HasParent
	}
	// Older standalone device plugins predate this snapshot field.
	return (v.scheme == "ios" || v.scheme == "android") &&
		strings.HasPrefix(v.mount.Path, v.scheme+"://") && v.mount.Path != v.scheme+"://"
}

// OpenPanelDirectory resolves a directory on a worker and returns a new mount.
// A cancelled or stale navigation can discard it without altering the panel's
// current mount.
func (v *rpcURIVFS) OpenPanelDirectory(ctx context.Context, target string, relativeEntry bool) (dest vfs.VFS, selection string, err error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if target == ".." && v.IsAtRoot() {
		parent, err := v.relatedContext(ctx, "parent")
		if parent == nil {
			if err == nil {
				err = os.ErrNotExist
			}
			return nil, "", err
		}
		var previous string
		if err := parent.call(ctx, f4plugin.URIRequest{Operation: "base", Path: v.GetPath()}, &previous); err != nil || previous == "" {
			previous = ".."
		}
		return parent, previous, nil
	}
	oldPath := v.GetPath()
	selection = ".."
	if relativeEntry {
		var joined string
		err = v.call(ctx, f4plugin.URIRequest{Operation: "join", Elements: []string{oldPath, target}}, &joined)
		if err != nil {
			return nil, "", err
		}
		if target == ".." {
			err = v.call(ctx, f4plugin.URIRequest{Operation: "base", Path: oldPath}, &selection)
			if err != nil {
				return nil, "", err
			}
		}
		target = joined
	}
	clone, err := v.relatedContext(ctx, "clone")
	if err != nil {
		return nil, "", err
	}
	if clone == nil {
		return nil, "", fmt.Errorf("VFS plugin returned an empty navigation mount")
	}
	operation := "set"
	if relativeEntry {
		operation = "setOptimistic"
	}
	if err := clone.setPathContext(ctx, target, operation); err != nil {
		clone.ClosePanelMount()
		return nil, "", err
	}
	return clone, selection, nil
}

func (v *rpcURIVFS) PanelParentPath(directory string) string {
	if v.IsAtRoot() {
		return directory
	}
	return v.PanelEntryPath(directory, "..")
}

func (v *rpcURIVFS) ClosePanelMount() {
	go func() {
		if err := v.Close(); err != nil {
			vtui.DebugLog("[FIX:uri-navigation] mount close failed: %v", err)
		}
	}()
}
