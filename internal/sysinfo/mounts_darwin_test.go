//go:build darwin

package sysinfo

import (
	"reflect"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinUserMounts(t *testing.T) {
	volume := func(path, device, fs string, flags uint32) syscall.Statfs_t {
		var stat syscall.Statfs_t
		for target, value := range map[*[1024]int8]string{
			&stat.Mntonname: path, &stat.Mntfromname: device,
		} {
			for i, b := range []byte(value) {
				target[i] = int8(b)
			}
		}
		for i, b := range []byte(fs) {
			stat.Fstypename[i] = int8(b)
		}
		stat.Flags = flags
		return stat
	}
	stats := []syscall.Statfs_t{
		volume("/", "/dev/disk3s1s1", "apfs", 0),
		volume("/System/Volumes/Data", "/dev/disk3s5", "apfs", 0),
		volume("/Volumes/Recovery", "/dev/disk3s3", "apfs", unix.MNT_DONTBROWSE),
		volume("/Volumes", "map", "autofs", 0),
		volume("/Volumes/", "map", "autofs", 0),
		volume("/Volumes-old/USB", "/dev/disk8", "exfat", 0),
		volume("/Volumes/Фото USB", "/dev/disk6s1", "exfat", 0),
		volume("/Volumes/Read Only", "/dev/disk7s1", "msdos", unix.MNT_RDONLY),
		volume("/Volumes/Network", "//server/share", "smbfs", 0),
		volume("/Volumes/Фото USB", "/dev/disk6s1", "exfat", 0),
	}
	want := []MountEntry{
		{Device: "/dev/disk6s1", MountPoint: "/Volumes/Фото USB", FSType: "exfat"},
		{Device: "/dev/disk7s1", MountPoint: "/Volumes/Read Only", FSType: "msdos"},
		{Device: "//server/share", MountPoint: "/Volumes/Network", FSType: "smbfs"},
	}
	if got := darwinUserMounts(stats); !reflect.DeepEqual(got, want) {
		t.Fatalf("user mounts = %#v, want %#v", got, want)
	}
	if got := darwinUserMounts(stats[:6]); len(got) != 0 {
		t.Fatalf("removed volumes survived refresh: %#v", got)
	}
}

func TestUserMountsIncludesMountedVolumes(t *testing.T) {
	count, err := syscall.Getfsstat(nil, unix.MNT_NOWAIT)
	if err != nil {
		t.Fatal(err)
	}
	stats := make([]syscall.Statfs_t, count+16)
	count, err = syscall.Getfsstat(stats, unix.MNT_NOWAIT)
	if err != nil {
		t.Fatal(err)
	}
	drives := GetPlatformDrives()
	for _, stat := range stats[:count] {
		path := int8SliceToString(stat.Mntonname[:])
		if !strings.HasPrefix(path, "/Volumes/") || stat.Flags&unix.MNT_DONTBROWSE != 0 {
			continue
		}
		found := false
		for _, drive := range drives {
			if drive.InfoPath == path {
				found = true
				if drive.UnmountDevice != "" {
					t.Errorf("volume %q advertises unsupported unmount", path)
				}
				if drive.Factory == nil {
					t.Errorf("volume %q has no navigation factory", path)
				}
			}
		}
		if !found {
			t.Errorf("mounted volume %q is missing from drives", path)
		}
	}
}
