//go:build linux || dragonfly || freebsd || netbsd || openbsd || solaris || illumos

package vfs

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var _ TrashVFS = (*OSVFS)(nil)

// MoveToTrash implements the FreeDesktop.org Trash specification. It never
// invokes sudo and never falls back to a cross-device copy or Remove.
func (v *OSVFS) MoveToTrash(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	abs, err := v.Abs(path)
	if err != nil {
		return err
	}
	return moveToFreedesktopTrash(ctx, abs)
}

type freedesktopTrash struct {
	root          string
	files         string
	info          string
	pathBase      string
	absolutePaths bool
}

func moveToFreedesktopTrash(ctx context.Context, source string) error {
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	source = filepath.Clean(source)
	sourceInfo, err := os.Lstat(source)
	if err != nil {
		return err
	}
	sourceDev, _, ok := unixStatIdentity(sourceInfo)
	if !ok {
		return fmt.Errorf("cannot determine source filesystem for trash: %s", source)
	}

	var homeTrash freedesktopTrash
	homeRoot, homeErr := freedesktopHomeTrashRoot()
	if homeErr == nil {
		if isPathWithin(homeRoot, source) {
			homeErr = fmt.Errorf("home trash is inside the selected item")
		} else {
			homeTrash, homeErr = prepareHomeTrashAt(homeRoot)
		}
	}
	if homeErr == nil {
		if info, statErr := os.Lstat(homeTrash.root); statErr == nil {
			if homeDev, _, statOK := unixStatIdentity(info); statOK && homeDev == sourceDev {
				return moveIntoFreedesktopTrash(ctx, source, homeTrash)
			}
		}
	}

	mountRoot, err := filesystemMountRoot(source, sourceDev)
	if err != nil {
		return err
	}
	if source == mountRoot {
		return fmt.Errorf("cannot move a filesystem mount root to trash: %s", source)
	}
	volumeTrash, err := prepareVolumeTrash(mountRoot)
	if err != nil {
		if homeErr != nil {
			return fmt.Errorf("home trash unavailable (%v); volume trash unavailable: %w", homeErr, err)
		}
		return err
	}
	return moveIntoFreedesktopTrash(ctx, source, volumeTrash)
}

func freedesktopHomeTrashRoot() (string, error) {
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(dataHome) {
		return "", fmt.Errorf("trash data home is not absolute: %s", dataHome)
	}
	return filepath.Join(dataHome, "Trash"), nil
}

func prepareHomeTrashAt(root string) (freedesktopTrash, error) {
	if err := ensurePrivateTrashDir(root); err != nil {
		return freedesktopTrash{}, err
	}
	t := freedesktopTrash{root: root, files: filepath.Join(root, "files"), info: filepath.Join(root, "info"), absolutePaths: true}
	if err := ensureTrashSubdirs(t); err != nil {
		return freedesktopTrash{}, err
	}
	return t, nil
}

func prepareVolumeTrash(mountRoot string) (freedesktopTrash, error) {
	uid := strconv.Itoa(os.Getuid())
	shared := filepath.Join(mountRoot, ".Trash")
	root := ""
	if info, err := os.Lstat(shared); err == nil {
		// The specification requires falling back to .Trash-$uid when the
		// shared directory is untrusted; an unsafe .Trash must never be
		// followed, but it need not disable a safe private volume trash.
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 && info.Mode()&os.ModeSticky != 0 {
			root = filepath.Join(shared, uid)
			if err := ensurePrivateTrashDir(root); err != nil {
				return freedesktopTrash{}, err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return freedesktopTrash{}, err
	}
	if root == "" {
		root = filepath.Join(mountRoot, ".Trash-"+uid)
		if err := ensurePrivateTrashDir(root); err != nil {
			return freedesktopTrash{}, err
		}
	}
	t := freedesktopTrash{
		root:          root,
		files:         filepath.Join(root, "files"),
		info:          filepath.Join(root, "info"),
		pathBase:      mountRoot,
		absolutePaths: false,
	}
	if err := ensureTrashSubdirs(t); err != nil {
		return freedesktopTrash{}, err
	}
	return t, nil
}

func ensureTrashSubdirs(t freedesktopTrash) error {
	if err := ensurePrivateTrashDir(t.files); err != nil {
		return err
	}
	return ensurePrivateTrashDir(t.info)
}

func ensurePrivateTrashDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	_, owner, ok := unixStatIdentity(info)
	if !ok || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("unsafe trash directory: %s", path)
	}
	if owner != uint32(os.Getuid()) {
		return fmt.Errorf("trash directory is owned by uid %d, not uid %d: %s", owner, os.Getuid(), path)
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("trash directory permissions are too broad (%#o): %s", info.Mode().Perm(), path)
	}
	return nil
}

func unixStatIdentity(info os.FileInfo) (device uint64, owner uint32, ok bool) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	return uint64(stat.Dev), stat.Uid, true
}

func filesystemMountRoot(source string, sourceDevice uint64) (string, error) {
	current := source
	if info, err := os.Lstat(current); err != nil {
		return "", err
	} else if !info.IsDir() {
		current = filepath.Dir(current)
	}
	for {
		parent := filepath.Dir(current)
		if parent == current {
			return current, nil
		}
		info, err := os.Lstat(parent)
		if err != nil {
			return "", err
		}
		device, _, ok := unixStatIdentity(info)
		if !ok {
			return "", fmt.Errorf("cannot determine filesystem mount root for %s", source)
		}
		if device != sourceDevice {
			return current, nil
		}
		current = parent
	}
}

func moveIntoFreedesktopTrash(ctx context.Context, source string, trash freedesktopTrash) error {
	if isPathWithin(source, trash.root) {
		return fmt.Errorf("item is already inside the trash: %s", source)
	}
	if isPathWithin(trash.root, source) {
		return fmt.Errorf("cannot move an ancestor of the trash into itself: %s", source)
	}
	trashPath := source
	if !trash.absolutePaths {
		var err error
		trashPath, err = filepath.Rel(trash.pathBase, source)
		if err != nil || trashPath == "." || strings.HasPrefix(trashPath, ".."+string(filepath.Separator)) || trashPath == ".." {
			return fmt.Errorf("cannot express trash path relative to mount root: %s", source)
		}
	}
	// EscapedPath applies URL percent-encoding while preserving path
	// separators. The Trash specification's examples require Path to remain
	// visibly absolute/relative (for example, /home/user/file or foo/bar), so
	// treating the whole pathname as one URL segment would be incorrect.
	encodedPath := (&url.URL{Path: filepath.ToSlash(trashPath)}).EscapedPath()
	base := filepath.Base(source)
	if base == "." || base == string(filepath.Separator) || base == "" {
		return fmt.Errorf("invalid trash item name: %s", source)
	}

	for suffix := 0; suffix < 100000; suffix++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := base
		if suffix > 0 {
			name += "." + strconv.Itoa(suffix)
		}
		destination := filepath.Join(trash.files, name)
		infoPath := filepath.Join(trash.info, name+".trashinfo")
		if pathExists(destination) || pathExists(infoPath) {
			continue
		}

		contents := "[Trash Info]\nPath=" + encodedPath + "\nDeletionDate=" + time.Now().Format("2006-01-02T15:04:05") + "\n"
		if err := writeExclusiveFile(infoPath, []byte(contents), 0600); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			_ = os.Remove(infoPath)
			return err
		}
		if err := renameNoReplace(source, destination); err != nil {
			_ = os.Remove(infoPath)
			if errors.Is(err, ErrDestinationExists) {
				continue
			}
			return err
		}
		return nil
	}
	return fmt.Errorf("cannot allocate a unique name in trash for %s", source)
}

func writeExclusiveFile(path string, data []byte, mode os.FileMode) (err error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			_ = os.Remove(path)
			err = closeErr
		}
	}()
	if _, err = f.Write(data); err != nil {
		_ = os.Remove(path)
		return err
	}
	if err = f.Sync(); err != nil {
		_ = os.Remove(path)
		return err
	}
	return nil
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil || !errors.Is(err, os.ErrNotExist)
}

func isPathWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
