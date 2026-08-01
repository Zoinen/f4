//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// fsInfo returns filesystem info for the drive containing path.
// ok=false if the value can't be determined.
func fsInfo(path string) (FSInfo, bool) {
	if path == "" {
		return FSInfo{}, false
	}
	info := FSInfo{}

	// Drive root — e.g. "C:\\" — is what most Volume APIs expect.
	root := filepath.VolumeName(path)
	if root != "" && !strings.HasSuffix(root, `\`) {
		root += `\`
	}
	if root == "" {
		root = path
	}
	info.Mount = root

	rootPtr, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return info, false
	}
	var freeAvail, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(rootPtr, &freeAvail, &totalBytes, &totalFree); err != nil {
		return info, false
	}
	info.Total = totalBytes
	info.Free = freeAvail

	// Volume label / serial / max filename length / flags / fs name.
	var (
		volumeName         [windows.MAX_PATH + 1]uint16
		fsName             [windows.MAX_PATH + 1]uint16
		serialNumber       uint32
		maxComponentLength uint32
		fileSystemFlags    uint32
	)
	if err := windows.GetVolumeInformation(
		rootPtr,
		&volumeName[0], uint32(len(volumeName)),
		&serialNumber,
		&maxComponentLength,
		&fileSystemFlags,
		&fsName[0], uint32(len(fsName)),
	); err == nil {
		info.Label = windows.UTF16ToString(volumeName[:])
		info.Type = windows.UTF16ToString(fsName[:])
		info.MaxFilename = int(maxComponentLength)
		info.Serial = fmt.Sprintf("%04X-%04X", serialNumber>>16, serialNumber&0xFFFF)
		info.Flags = decodeVolumeFlags(fileSystemFlags)
	}
	return info, true
}

func decodeVolumeFlags(f uint32) string {
	// Names come from WinBase.h FILE_* constants. Kept to the ones
	// far2l's info panel typically surfaces; the raw hex tail keeps
	// bits we didn't decode visible for the curious.
	type flag struct {
		bit uint32
		s   string
	}
	all := []flag{
		{0x00000001, "case_sensitive"},
		{0x00000002, "case_preserved"},
		{0x00000004, "unicode"},
		{0x00000008, "acls"},
		{0x00000010, "compressed_files"},
		{0x00000040, "quotas"},
		{0x00000080, "sparse"},
		{0x00000100, "reparse_points"},
		{0x00008000, "compressed_vol"},
		{0x00010000, "objects_ids"},
		{0x00020000, "encryption"},
		{0x00040000, "streams"},
		{0x02000000, "readonly"},
	}
	var parts []string
	for _, e := range all {
		if f&e.bit != 0 {
			parts = append(parts, e.s)
			f &^= e.bit
		}
	}
	if f != 0 {
		parts = append(parts, fmt.Sprintf("0x%X", f))
	}
	return strings.Join(parts, ",")
}
