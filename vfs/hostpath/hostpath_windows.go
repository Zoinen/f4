//go:build windows

// See hostpath_posix.go for the package doc.
//
// WINE.md Stage E4: path semantics now genuinely branch on personality.
// In posix mode (vfs/hostmode.Posix()), every function speaks plain POSIX
// path syntax via the standard "path" package -- forward slashes only, no
// drive letters, no \\?\ of any kind. In windows mode, behavior is
// byte-for-byte what it was before this package existed (path/filepath).
package hostpath

import (
	"os"
	stdpath "path"
	"path/filepath"

	"github.com/unxed/f4/vfs/hostmode"
)

func Join(elem ...string) string {
	if hostmode.Posix() {
		return stdpath.Join(elem...)
	}
	return filepath.Join(elem...)
}

func Dir(path string) string {
	if hostmode.Posix() {
		return stdpath.Dir(path)
	}
	return filepath.Dir(path)
}

func Base(path string) string {
	if hostmode.Posix() {
		return stdpath.Base(path)
	}
	return filepath.Base(path)
}

func Clean(path string) string {
	if hostmode.Posix() {
		return stdpath.Clean(path)
	}
	return filepath.Clean(path)
}

func IsAbs(path string) bool {
	if hostmode.Posix() {
		return stdpath.IsAbs(path)
	}
	return filepath.IsAbs(path)
}

// VolumeName always returns "" in posix mode: there are no drive letters,
// no UNC roots, nothing "volume-shaped" -- exactly like on real POSIX. This
// is the one function every caller that special-cases Windows volumes
// (queue_manager.go's filepath.VolumeName, action_registry.go's drive-root
// construction) needs to keep working correctly against: an empty volume
// name is what tells them "there is no drive here, don't try to build one".
func VolumeName(path string) string {
	if hostmode.Posix() {
		return ""
	}
	return filepath.VolumeName(path)
}

// Abs in posix mode never needs a "current drive" concept. Every real call
// site already joins a relative path onto v.currentPath (itself absolute in
// posix mode) before calling Abs, so reaching here with a still-relative
// path isn't expected in practice; the fallback below treats it as relative
// to POSIX root rather than pulling in a getwd dependency for a path that
// should never arrive un-anchored.
func Abs(path string) (string, error) {
	if hostmode.Posix() {
		if stdpath.IsAbs(path) {
			return stdpath.Clean(path), nil
		}
		return stdpath.Clean("/" + path), nil
	}
	// filepath.Abs delegates to Win32 GetFullPathNameW, which strips
	// trailing dots and spaces from every path component -- names that stay
	// reachable only through the \\?\ forms built downstream. Absolute
	// paths therefore resolve lexically: Clean keeps a "folder." component
	// intact. Drive-relative paths ("C:foo") still go through the OS, since
	// only it knows each drive's current directory.
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if len(path) >= 2 && path[1] == ':' {
		return filepath.Abs(path)
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, path), nil
}

func EvalSymlinks(path string) (string, error) {
	if hostmode.Posix() {
		// Real symlink resolution in posix mode goes through hostfs's
		// libwinescape-backed Readlink chain (Stage E3), not here --
		// filepath.EvalSymlinks would ask Win32 to resolve a path that
		// isn't in Win32 syntax to begin with. Stage E5 wires this properly;
		// until then, identity is the safe default (matches what happens
		// today when EvalSymlinks's caller in os_vfs.go already found the
		// direct os.Stat/Lstat succeeded and never needed this branch).
		return path, nil
	}
	return filepath.EvalSymlinks(path)
}

const Separator = filepath.Separator
