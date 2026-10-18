//go:build linux

package sysinfo

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const sysfsRoot = "/sys"

// maxSlaveDepth bounds the walk through stacked block devices
// (disk → LVM → dm-crypt → md ...).
const maxSlaveDepth = 8

// Device describes the physical storage holding path. ok=false when the
// path cannot be examined at all.
func Device(path string) (DeviceInfo, bool) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return DeviceInfo{}, false
	}
	if fs, ok := FS(path); ok && IsNetworkFS(fs.Type) {
		return DeviceInfo{Keys: []string{"net:" + fs.Mount}, Class: DeviceNetwork, Name: fs.Type}, true
	}
	dev := uint64(st.Dev)
	if info, ok := blockDevice(sysfsRoot, unix.Major(dev), unix.Minor(dev)); ok {
		return info, true
	}
	// Anonymous devices (tmpfs, btrfs, overlay, FUSE) have no sysfs block
	// entry: still give them a key so equal paths compare equal.
	return DeviceInfo{Keys: []string{fmt.Sprintf("dev:%d:%d", unix.Major(dev), unix.Minor(dev))}}, true
}

// blockDevice resolves a major:minor pair to the physical disks behind it
// using the sysfs tree rooted at root (a parameter so tests can fake it).
func blockDevice(root string, major, minor uint32) (DeviceInfo, bool) {
	dir, err := filepath.EvalSymlinks(filepath.Join(root, "dev", "block", fmt.Sprintf("%d:%d", major, minor)))
	if err != nil {
		return DeviceInfo{}, false
	}
	disks := map[string]DeviceClass{}
	collectDisks(dir, disks, 0)
	if len(disks) == 0 {
		return DeviceInfo{}, false
	}
	names := make([]string, 0, len(disks))
	classes := make([]DeviceClass, 0, len(disks))
	for n := range disks {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		classes = append(classes, disks[n])
	}
	return DeviceInfo{Keys: names, Class: mergeClasses(classes), Name: strings.Join(names, "+")}, true
}

// collectDisks adds the whole disks behind the block device at dir:
// a partition maps to its parent disk, a stacked device (dm-*, md*) to the
// disks of its slaves.
func collectDisks(dir string, disks map[string]DeviceClass, depth int) {
	if depth > maxSlaveDepth {
		return
	}
	if entries, err := os.ReadDir(filepath.Join(dir, "slaves")); err == nil && len(entries) > 0 {
		for _, e := range entries {
			slave, err := filepath.EvalSymlinks(filepath.Join(dir, "slaves", e.Name()))
			if err != nil {
				disks[e.Name()] = DeviceUnknown
				continue
			}
			collectDisks(slave, disks, depth+1)
		}
		return
	}
	if _, err := os.Stat(filepath.Join(dir, "partition")); err == nil {
		dir = filepath.Dir(dir)
	}
	disks[filepath.Base(dir)] = diskClass(dir)
}

// diskClass reads queue/rotational. Virtual devices (loop, zram, ram) say
// "non-rotational" regardless of what backs them, so they stay unknown.
func diskClass(dir string) DeviceClass {
	if strings.Contains(filepath.ToSlash(dir), "/devices/virtual/") {
		return DeviceUnknown
	}
	b, err := os.ReadFile(filepath.Join(dir, "queue", "rotational"))
	if err != nil {
		return DeviceUnknown
	}
	switch strings.TrimSpace(string(b)) {
	case "1":
		return DeviceHDD
	case "0":
		return DeviceSSD
	}
	return DeviceUnknown
}
