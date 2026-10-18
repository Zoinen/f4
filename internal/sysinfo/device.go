package sysinfo

import "strings"

// DeviceClass is the kind of storage a path lives on. Anything we cannot
// prove stays DeviceUnknown: callers must treat it as the most conservative
// case (one worker, no assumptions about speed).
type DeviceClass int

const (
	DeviceUnknown DeviceClass = iota
	DeviceHDD
	DeviceSSD
	DeviceNetwork
)

// String returns the language-neutral class name.
func (c DeviceClass) String() string {
	switch c {
	case DeviceHDD:
		return "HDD"
	case DeviceSSD:
		return "SSD"
	case DeviceNetwork:
		return "network"
	}
	return "unknown"
}

// DeviceInfo identifies the physical storage behind a path. Keys name the
// physical disks (several for LVM/RAID) and are stable and sorted, so two
// paths on the same disk — even on different partitions — share a key.
type DeviceInfo struct {
	Keys  []string
	Class DeviceClass
	Name  string // "nvme0n1", "sda+sdb", "nfs"; "" when unknown
}

// networkFSTypes are the filesystem types whose data lives on another host.
var networkFSTypes = map[string]bool{
	"nfs": true, "nfs4": true, "cifs": true, "smb3": true, "smbfs": true,
	"afpfs": true, "webdav": true, "9p": true, "ceph": true, "glusterfs": true,
	"fuse.sshfs": true, "fuse.rclone": true, "fuse.s3fs": true,
	"fuse.gvfsd-fuse": true, "davfs": true, "davfs2": true, "ncpfs": true,
}

// IsNetworkFS reports whether fsType (as in /proc/mounts or f_fstypename)
// is a network filesystem.
func IsNetworkFS(fsType string) bool {
	return networkFSTypes[strings.ToLower(fsType)]
}

// mergeClasses combines the classes of the disks behind one volume: the
// slowest member decides, and a single unknown member makes the whole
// volume unknown.
func mergeClasses(classes []DeviceClass) DeviceClass {
	if len(classes) == 0 {
		return DeviceUnknown
	}
	out := DeviceSSD
	for _, c := range classes {
		switch c {
		case DeviceHDD:
			out = DeviceHDD
		case DeviceSSD:
		default:
			return DeviceUnknown
		}
	}
	return out
}
