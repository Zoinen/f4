//go:build !windows

// Package hostpath is the single point of contact between f4 and host path
// semantics (WINE.md §13.9, Stage E1) -- Join, Dir, Base, and the rest of
// what path/filepath does, kept separate from vfs/hostfs (file operations)
// because path manipulation happens in many places with no file operation
// nearby (sorting, autocomplete, breadcrumbs).
//
// On every non-Windows GOOS, path/filepath already speaks POSIX, so this is
// a direct, inlinable forward with zero behavioral difference from calling
// path/filepath directly.
package hostpath

import "path/filepath"

func Join(elem ...string) string               { return filepath.Join(elem...) }
func Dir(path string) string                   { return filepath.Dir(path) }
func Base(path string) string                  { return filepath.Base(path) }
func Clean(path string) string                 { return filepath.Clean(path) }
func IsAbs(path string) bool                   { return filepath.IsAbs(path) }
func VolumeName(path string) string            { return filepath.VolumeName(path) }
func Abs(path string) (string, error)          { return filepath.Abs(path) }
func EvalSymlinks(path string) (string, error) { return filepath.EvalSymlinks(path) }

const Separator = filepath.Separator
