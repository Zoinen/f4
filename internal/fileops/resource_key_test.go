package fileops

import (
	"reflect"
	"runtime"
	"testing"

	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

// fakeStorage replaces the device and volume lookups with a table keyed by
// path, so the tests never look at the real disks.
func fakeStorage(t *testing.T, devices map[string]sysinfo.DeviceInfo, volumes map[string]string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the Windows queue key is the drive letter, not the device")
	}
	oldDevice, oldVolume := deviceOf, volumeOf
	t.Cleanup(func() { deviceOf, volumeOf = oldDevice, oldVolume })
	deviceOf = func(path string) (sysinfo.DeviceInfo, bool) {
		info, ok := devices[path]
		return info, ok
	}
	volumeOf = func(path string) string { return volumes[path] }
}

func TestGetResourceKeys(t *testing.T) {
	fakeStorage(t,
		map[string]sysinfo.DeviceInfo{
			"/hdd1":    {Keys: []string{"sda"}, Class: sysinfo.DeviceHDD},
			"/hdd2":    {Keys: []string{"sdb"}, Class: sysinfo.DeviceHDD},
			"/raid":    {Keys: []string{"sda", "sdb"}, Class: sysinfo.DeviceHDD},
			"/ssd1":    {Keys: []string{"nvme0n1"}, Class: sysinfo.DeviceSSD},
			"/ssd2":    {Keys: []string{"nvme0n1"}, Class: sysinfo.DeviceSSD},
			"/nfs":     {Keys: []string{"net:/mnt/nfs"}, Class: sysinfo.DeviceNetwork},
			"/tmpfs":   {Keys: []string{"dev:0:45"}},
			"/nokeys":  {Class: sysinfo.DeviceHDD},
			"/ssd-nov": {Keys: []string{"nvme1n1"}, Class: sysinfo.DeviceSSD},
		},
		map[string]string{"/ssd1": "/", "/ssd2": "/home"},
	)

	cases := []struct {
		path string
		want []string
	}{
		{"/hdd1", []string{"disk:sda"}},
		{"/hdd2", []string{"disk:sdb"}},
		{"/raid", []string{"disk:sda", "disk:sdb"}},
		{"/ssd1", []string{"volume:/"}},
		{"/ssd2", []string{"volume:/home"}},
		{"/nfs", []string{"net:/mnt/nfs"}},
		{"/tmpfs", []string{"local_disk"}},
		{"/nokeys", []string{"local_disk"}},
		{"/ssd-nov", []string{"local_disk"}},
		{"/not-examined", []string{"local_disk"}},
	}
	for _, c := range cases {
		if got := GetResourceKeys(vfs.NewOSVFS(c.path)); !reflect.DeepEqual(got, c.want) {
			t.Errorf("GetResourceKeys(%s) = %q, want %q", c.path, got, c.want)
		}
	}
	if got := GetResourceKeys(nil); got != nil {
		t.Errorf("GetResourceKeys(nil) = %q, want none", got)
	}
}

func TestResourceKeysForMergesWithoutRepeats(t *testing.T) {
	fakeStorage(t,
		map[string]sysinfo.DeviceInfo{
			"/hdd1": {Keys: []string{"sda"}, Class: sysinfo.DeviceHDD},
			"/hdd2": {Keys: []string{"sdb"}, Class: sysinfo.DeviceHDD},
			"/raid": {Keys: []string{"sda", "sdb"}, Class: sysinfo.DeviceHDD},
		}, nil)

	a, b, raid := vfs.NewOSVFS("/hdd1"), vfs.NewOSVFS("/hdd2"), vfs.NewOSVFS("/raid")
	cases := []struct {
		name string
		in   []vfs.VFS
		want []string
	}{
		{"two disks", []vfs.VFS{a, b}, []string{"disk:sda", "disk:sdb"}},
		{"same disk twice", []vfs.VFS{a, a}, []string{"disk:sda"}},
		{"raid overlaps its members", []vfs.VFS{a, raid}, []string{"disk:sda", "disk:sdb"}},
		{"nil is skipped", []vfs.VFS{nil, b}, []string{"disk:sdb"}},
		{"only nil", []vfs.VFS{nil}, nil},
	}
	for _, c := range cases {
		if got := ResourceKeysFor(c.in...); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: ResourceKeysFor = %q, want %q", c.name, got, c.want)
		}
	}
}

// Copies between disjoint disks must not queue behind each other, while a
// copy that shares a disk with a running one must (the class of the bug in
// unxed/f4#1894: one "local_disk" key made F7 and every other disk wait).
func TestResourceKeysDisjointDisksDoNotConflict(t *testing.T) {
	fakeStorage(t,
		map[string]sysinfo.DeviceInfo{
			"/hdd1": {Keys: []string{"sda"}, Class: sysinfo.DeviceHDD},
			"/hdd2": {Keys: []string{"sdb"}, Class: sysinfo.DeviceHDD},
			"/hdd3": {Keys: []string{"sdc"}, Class: sysinfo.DeviceHDD},
		}, nil)

	running := ResourceKeysFor(vfs.NewOSVFS("/hdd1"), vfs.NewOSVFS("/hdd2"))
	overlaps := func(keys []string) bool {
		for _, k := range keys {
			for _, r := range running {
				if k == r {
					return true
				}
			}
		}
		return false
	}
	if overlaps(ResourceKeysFor(vfs.NewOSVFS("/hdd3"))) {
		t.Error("an operation on an unrelated disk must not wait for the running copy")
	}
	if !overlaps(ResourceKeysFor(vfs.NewOSVFS("/hdd2"), vfs.NewOSVFS("/hdd3"))) {
		t.Error("an operation sharing a disk with the running copy must wait")
	}
}
