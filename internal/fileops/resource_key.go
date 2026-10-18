package fileops

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

// localDiskKey is the single shared queue for local storage we cannot tell
// apart: unknown devices, or a platform sysinfo.Device does not cover yet.
const localDiskKey = "local_disk"

// Storage lookups behind the queue keys; tests replace them so no real disk
// is examined.
var (
	deviceOf = sysinfo.Device
	volumeOf = func(path string) string {
		if fs, ok := sysinfo.FS(path); ok {
			return fs.Mount
		}
		return ""
	}
)

// GetResourceKeys returns the queue resources an operation on v occupies:
// one key per physical disk for a spinning disk (seeks make parallel work on
// it slower), one per volume for an SSD, one per mount for network storage.
// Operations with disjoint keys run side by side; an unknown device keeps
// the shared "local_disk" key.
func GetResourceKeys(v vfs.VFS) []string {
	if v == nil {
		return nil
	}
	if _, ok := v.(*vfs.OSVFS); ok {
		if runtime.GOOS == "windows" {
			if vol := filepath.VolumeName(v.GetPath()); vol != "" {
				return []string{vol}
			}
			return nil
		}
		return osResourceKeys(v.GetPath())
	}
	if parent := v.ParentVFS(); parent != nil {
		return GetResourceKeys(parent)
	}
	return []string{fmt.Sprintf("%p", v)}
}

func osResourceKeys(path string) []string {
	info, ok := deviceOf(path)
	if !ok || len(info.Keys) == 0 {
		return []string{localDiskKey}
	}
	switch info.Class {
	case sysinfo.DeviceHDD:
		keys := make([]string, 0, len(info.Keys))
		for _, k := range info.Keys {
			keys = append(keys, "disk:"+k)
		}
		return keys
	case sysinfo.DeviceSSD:
		if mount := volumeOf(path); mount != "" {
			return []string{"volume:" + mount}
		}
	case sysinfo.DeviceNetwork:
		return info.Keys
	}
	return []string{localDiskKey}
}

// ResourceKeysFor merges the keys of every VFS an operation touches,
// without repeats, in the order given.
func ResourceKeysFor(vfses ...vfs.VFS) []string {
	var keys []string
	seen := map[string]bool{}
	for _, v := range vfses {
		for _, k := range GetResourceKeys(v) {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return keys
}
