//go:build !windows

package main

func driveMenuPlatformKind(path string) driveMenuKind {
	if path == "" {
		return driveMenuKindUnknown
	}
	return driveMenuKindFixed
}
