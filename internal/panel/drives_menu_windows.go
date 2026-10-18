//go:build windows

package panel

import (
	"encoding/binary"
	"strings"

	"golang.org/x/sys/windows"
)

// ioctlStorageGetMediaTypesEx is IOCTL_STORAGE_GET_MEDIA_TYPES_EX: device
// type IOCTL_STORAGE_BASE (0x2d), function 0x301, METHOD_BUFFERED,
// FILE_ANY_ACCESS.
const ioctlStorageGetMediaTypesEx = 0x2d<<16 | 0x301<<2

// driveMenuOpticalDeviceType asks an optical drive for its device type: the
// DeviceType member that opens the GET_MEDIA_TYPES answer. The handle asks
// for no access at all, which is enough for a FILE_ANY_ACCESS request and
// does not need a disc in the drive.
func driveMenuOpticalDeviceType(path string) (uint32, bool) {
	if len(path) < 2 || path[1] != ':' {
		return 0, false
	}
	name, err := windows.UTF16PtrFromString(`\\.\` + path[:2])
	if err != nil {
		return 0, false
	}
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return 0, false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var buf [2048]byte
	var n uint32
	if err := windows.DeviceIoControl(h, ioctlStorageGetMediaTypesEx, nil, 0, &buf[0], uint32(len(buf)), &n, nil); err != nil || n < 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(buf[:4]), true
}

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
		return driveMenuOpticalKind(driveMenuOpticalDeviceType(path))
	case windows.DRIVE_REMOTE:
		return driveMenuKindRemote
	default:
		return driveMenuKindUnknown
	}
}
