//go:build darwin

package sysinfo

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// FsInfo populates FSInfo for the filesystem holding path on macOS via
// syscall.Statfs. Darwin's Statfs_t exposes fs type name (e.g. "apfs",
// "hfs") and mount point. Pathconf and getattrlist supply volume metadata.
// `ok=false` if path is
// empty or the syscall fails.
func FS(path string) (FSInfo, bool) {
	if path == "" {
		return FSInfo{}, false
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return FSInfo{}, false
	}
	bs := uint64(st.Bsize)
	info := FSInfo{
		Total:       uint64(st.Blocks) * bs,
		Free:        uint64(st.Bavail) * bs,
		Type:        int8SliceToString(st.Fstypename[:]),
		Mount:       int8SliceToString(st.Mntonname[:]),
		ClusterSize: bs,
	}
	// _PC_NAME_MAX is 4 on Darwin; query the filesystem, not NAME_MAX.
	if maxName, err := unix.Pathconf(path, 4); err == nil && maxName > 0 {
		info.MaxFilename = maxName
	}
	info.Flags = darwinMountFlags(st.Flags)
	if data, ok := darwinVolumeAttribute(path, unix.ATTR_VOL_NAME); ok {
		info.Label = darwinVolumeName(data)
	}
	if data, ok := darwinVolumeAttribute(path, unix.ATTR_VOL_UUID); ok && len(data) >= 20 {
		id := data[4:20]
		if string(id) != strings.Repeat("\x00", 16) {
			info.Serial = fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
		}
	}
	if data, ok := darwinVolumeAttribute(path, unix.ATTR_VOL_CAPABILITIES); ok && len(data) >= 36 {
		caps := binary.LittleEndian.Uint32(data[4:8])
		valid := binary.LittleEndian.Uint32(data[20:24])
		for _, cap := range []struct {
			bit  uint32
			name string
		}{
			{bit: 0x100, name: "case_sensitive"},
			{bit: 0x200, name: "case_preserved"},
		} {
			if caps&valid&cap.bit != 0 {
				info.Flags += "," + cap.name
			}
		}
	}
	return info, true
}

func darwinVolumeAttribute(path string, attribute uint32) ([]byte, bool) {
	api := loadDarwinSystemAPI()
	if api == nil {
		return nil, false
	}
	attrs := darwinAttrlist{Count: 5, Volume: attribute | unix.ATTR_VOL_INFO}
	buffer := make([]byte, 4096)
	status := api.getAttr(
		path,
		&attrs,
		&buffer[0],
		uintptr(len(buffer)),
		0,
	)
	if status != 0 {
		darwinInfoDebug("getattrlist attribute %#x failed: %d", attribute, status)
		return nil, false
	}
	size := binary.LittleEndian.Uint32(buffer[:4])
	if size < 4 || size > uint32(len(buffer)) {
		return nil, false
	}
	return buffer[:size], true
}

func darwinVolumeName(data []byte) string {
	if len(data) < 12 {
		return ""
	}
	// attrreference offsets are signed and relative to the reference at byte 4.
	// #nosec G115 -- signed ABI field; preserve its two's-complement bits.
	start := int64(int32(binary.LittleEndian.Uint32(data[4:8]))) + 4
	length := int64(binary.LittleEndian.Uint32(data[8:12]))
	end := start + length
	if start < 12 || length < 1 || end > int64(len(data)) {
		return ""
	}
	return strings.TrimRight(string(data[start:end]), "\x00")
}

func darwinMountFlags(flags uint32) string {
	result := []string{"rw"}
	if flags&unix.MNT_RDONLY != 0 {
		result[0] = "ro"
	}
	for _, flag := range []struct {
		bit  uint32
		name string
	}{
		{bit: unix.MNT_NOSUID, name: "nosuid"},
		{bit: unix.MNT_NOEXEC, name: "noexec"},
		{bit: unix.MNT_LOCAL, name: "local"},
		{bit: unix.MNT_JOURNALED, name: "journaled"},
		{bit: unix.MNT_DONTBROWSE, name: "nobrowse"},
	} {
		if flags&flag.bit != 0 {
			result = append(result, flag.name)
		}
	}
	return strings.Join(result, ",")
}

// int8SliceToString converts a NUL-terminated C-style char buffer
// (BSD APIs return them as [N]int8) to a Go string.
func int8SliceToString(s []int8) string {
	n := 0
	for n < len(s) && s[n] != 0 {
		n++
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		// #nosec G115 -- this intentionally preserves the raw bits of Darwin's C char buffer, including bytes >= 0x80.
		b[i] = byte(s[i])
	}
	return string(b)
}
