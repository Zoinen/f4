//go:build linux

package sysinfo

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fakeSysfs builds a minimal /sys: devices live under devices/..., and
// dev/block/MAJ:MIN are symlinks to them, exactly as in the real tree.
type fakeSysfs struct {
	t    *testing.T
	root string
}

func newFakeSysfs(t *testing.T) *fakeSysfs {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "dev", "block"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &fakeSysfs{t: t, root: root}
}

func (f *fakeSysfs) mkdir(rel string) string {
	p := filepath.Join(f.root, rel)
	if err := os.MkdirAll(p, 0o755); err != nil {
		f.t.Fatal(err)
	}
	return p
}

func (f *fakeSysfs) write(p, content string) {
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeSysfs) link(target, devnum string) {
	if err := os.Symlink(target, filepath.Join(f.root, "dev", "block", devnum)); err != nil {
		f.t.Fatal(err)
	}
}

// disk adds a whole disk with the given queue/rotational value ("" = file absent).
func (f *fakeSysfs) disk(rel, devnum, rotational string) string {
	d := f.mkdir(rel)
	if rotational != "" {
		f.mkdir(filepath.Join(rel, "queue"))
		f.write(filepath.Join(d, "queue", "rotational"), rotational+"\n")
	}
	f.link(d, devnum)
	return d
}

// partition adds a partition directory inside disk dir.
func (f *fakeSysfs) partition(diskDir, name, devnum string) string {
	p := filepath.Join(diskDir, name)
	if err := os.MkdirAll(p, 0o755); err != nil {
		f.t.Fatal(err)
	}
	f.write(filepath.Join(p, "partition"), "1\n")
	f.link(p, devnum)
	return p
}

func TestBlockDeviceWholeDisks(t *testing.T) {
	f := newFakeSysfs(t)
	f.disk("devices/pci0/nvme/nvme0n1", "259:0", "0")
	f.disk("devices/pci0/ata1/sda", "8:0", "1")
	cases := []struct {
		maj, min uint32
		class    DeviceClass
		name     string
	}{{259, 0, DeviceSSD, "nvme0n1"}, {8, 0, DeviceHDD, "sda"}}
	for _, c := range cases {
		got, ok := blockDevice(f.root, c.maj, c.min)
		if !ok || got.Class != c.class || got.Name != c.name || !reflect.DeepEqual(got.Keys, []string{c.name}) {
			t.Errorf("%d:%d = %+v ok=%v, want %v %q", c.maj, c.min, got, ok, c.class, c.name)
		}
	}
}

func TestBlockDevicePartitionsShareTheDiskKey(t *testing.T) {
	f := newFakeSysfs(t)
	d := f.disk("devices/pci0/ata1/sda", "8:0", "1")
	f.partition(d, "sda1", "8:1")
	f.partition(d, "sda2", "8:2")
	a, ok1 := blockDevice(f.root, 8, 1)
	b, ok2 := blockDevice(f.root, 8, 2)
	if !ok1 || !ok2 {
		t.Fatalf("partitions not resolved: %v %v", ok1, ok2)
	}
	if !reflect.DeepEqual(a, b) || a.Name != "sda" || a.Class != DeviceHDD {
		t.Errorf("sda1=%+v sda2=%+v, want the same HDD sda", a, b)
	}
}

func TestBlockDeviceStackedVolumeUsesAllSlaves(t *testing.T) {
	f := newFakeSysfs(t)
	sda := f.disk("devices/pci0/ata1/sda", "8:0", "1")
	nvme := f.disk("devices/pci0/nvme/nvme0n1", "259:0", "0")
	dm := f.mkdir("devices/virtual/block/dm-0")
	f.mkdir("devices/virtual/block/dm-0/slaves")
	for _, s := range []string{sda, nvme} {
		if err := os.Symlink(s, filepath.Join(dm, "slaves", filepath.Base(s))); err != nil {
			t.Fatal(err)
		}
	}
	f.link(dm, "253:0")
	got, ok := blockDevice(f.root, 253, 0)
	if !ok {
		t.Fatal("dm-0 not resolved")
	}
	if !reflect.DeepEqual(got.Keys, []string{"nvme0n1", "sda"}) || got.Name != "nvme0n1+sda" {
		t.Errorf("keys/name = %v %q", got.Keys, got.Name)
	}
	if got.Class != DeviceHDD {
		t.Errorf("class = %v, want HDD (the slowest member decides)", got.Class)
	}
}

func TestBlockDeviceUnknownCases(t *testing.T) {
	f := newFakeSysfs(t)
	f.disk("devices/pci0/usb/sdb", "8:16", "") // no queue/rotational
	f.disk("devices/virtual/block/loop0", "7:0", "0")
	for _, c := range []struct{ maj, min uint32 }{{8, 16}, {7, 0}} {
		got, ok := blockDevice(f.root, c.maj, c.min)
		if !ok || got.Class != DeviceUnknown {
			t.Errorf("%d:%d = %+v ok=%v, want unknown class", c.maj, c.min, got, ok)
		}
	}
	if _, ok := blockDevice(f.root, 0, 42); ok {
		t.Error("anonymous device 0:42 has no sysfs entry and must not resolve")
	}
}

func TestDeviceClassesAndNetworkFS(t *testing.T) {
	if got := mergeClasses(nil); got != DeviceUnknown {
		t.Errorf("mergeClasses(nil) = %v", got)
	}
	if got := mergeClasses([]DeviceClass{DeviceSSD, DeviceUnknown}); got != DeviceUnknown {
		t.Errorf("SSD+unknown = %v, want unknown", got)
	}
	for _, c := range []struct {
		c DeviceClass
		s string
	}{{DeviceHDD, "HDD"}, {DeviceSSD, "SSD"}, {DeviceNetwork, "network"}, {DeviceUnknown, "unknown"}} {
		if c.c.String() != c.s {
			t.Errorf("%d.String() = %q, want %q", c.c, c.c.String(), c.s)
		}
	}
	for fsType, want := range map[string]bool{"nfs4": true, "CIFS": true, "fuse.sshfs": true, "ext4": false, "btrfs": false, "": false} {
		if IsNetworkFS(fsType) != want {
			t.Errorf("IsNetworkFS(%q) = %v, want %v", fsType, !want, want)
		}
	}
}

func TestDeviceOnRealPath(t *testing.T) {
	info, ok := Device(t.TempDir())
	if !ok || len(info.Keys) == 0 {
		t.Fatalf("Device(tempdir) = %+v ok=%v, want at least one key", info, ok)
	}
	if _, ok := Device(filepath.Join(t.TempDir(), "missing")); ok {
		t.Error("a missing path must report ok=false")
	}
}
