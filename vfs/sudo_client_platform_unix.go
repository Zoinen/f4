//go:build !windows

package vfs

func sudoClientSupported() bool { return true }
