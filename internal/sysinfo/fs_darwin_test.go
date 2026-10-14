//go:build darwin

package sysinfo

import (
	"encoding/binary"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinVolumeMetadata(t *testing.T) {
	info, ok := FS(t.TempDir())
	if !ok {
		t.Fatal("filesystem capacity unavailable")
	}
	if info.MaxFilename <= 0 || info.Flags == "" {
		t.Fatalf("macOS volume limitations missing: %+v", info)
	}
	if info.Type == "apfs" && (info.Label == "" || info.Serial == "") {
		t.Fatalf("APFS volume identity missing: %+v", info)
	}
	t.Logf("[FIX:darwin-info] volume %+v", info)
}

func TestDarwinVolumeNameBounds(t *testing.T) {
	data := make([]byte, 17)
	binary.LittleEndian.PutUint32(data[4:8], 8)
	binary.LittleEndian.PutUint32(data[8:12], 5)
	copy(data[12:], "Data\x00")
	if got := darwinVolumeName(data); got != "Data" {
		t.Fatalf("volume name = %q", got)
	}
	for _, tc := range []struct {
		name           string
		offset, length uint32
	}{
		{name: "negative reference", offset: 0xffffffff, length: 5},
		{name: "oversized name", offset: 8, length: 9999},
		{name: "reference inside header", offset: 0, length: 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			binary.LittleEndian.PutUint32(data[4:8], tc.offset)
			binary.LittleEndian.PutUint32(data[8:12], tc.length)
			if got := darwinVolumeName(data); got != "" {
				t.Fatalf("malformed reference accepted: %q", got)
			}
		})
	}
}

func TestDarwinMountFlagNames(t *testing.T) {
	if got := darwinMountFlags(unix.MNT_RDONLY | unix.MNT_NOSUID | unix.MNT_NOEXEC); got != "ro,nosuid,noexec" {
		t.Fatalf("mount flags = %q", got)
	}
}
