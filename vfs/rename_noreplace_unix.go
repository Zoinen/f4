//go:build aix || dragonfly || freebsd || netbsd || openbsd || solaris || illumos

package vfs

func renameNoReplace(oldPath, newPath string) error {
	return renameNoReplacePortable(oldPath, newPath)
}
