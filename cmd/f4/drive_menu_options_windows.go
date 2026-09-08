//go:build windows

package main

import (
	"strings"

	"golang.org/x/sys/windows"
)

func driveMenuIsSubstitute(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	drive, err := windows.UTF16PtrFromString(path[:2])
	if err != nil {
		return false
	}
	// QueryDosDevice reports a normal volume as a \Device\... target. A
	// SUBST mapping is a DOS-device link under \??\, including a mapping to
	// an UNC path. This is the same distinction Far makes before choosing
	// the displayed drive type.
	target := make([]uint16, 32768)
	n, err := windows.QueryDosDevice(drive, &target[0], uint32(len(target)))
	if err != nil || n == 0 {
		return false
	}
	return strings.HasPrefix(strings.ToLower(windows.UTF16ToString(target[:n])), `\??\`)
}

func driveMenuPlatformKind(path string) driveMenuKind {
	if driveMenuIsSubstitute(path) {
		return driveMenuKindSubstitute
	}
	root, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return driveMenuKindUnknown
	}
	switch windows.GetDriveType(root) {
	case windows.DRIVE_RAMDISK:
		return driveMenuKindRAM
	case windows.DRIVE_FIXED:
		return driveMenuKindFixed
	case windows.DRIVE_REMOVABLE:
		return driveMenuKindRemovable
	case windows.DRIVE_CDROM:
		return driveMenuKindCD
	case windows.DRIVE_REMOTE:
		return driveMenuKindRemote
	default:
		return driveMenuKindUnknown
	}
}
