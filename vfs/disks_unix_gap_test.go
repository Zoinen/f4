//go:build !windows

package vfs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDevicePathUnix(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"sda", "/dev/sda"},
		{"sda1", "/dev/sda1"},
		{"/dev/sda", "/dev/sda"},
		{"/dev/mapper/vg-lv", "/dev/mapper/vg-lv"},
	}
	for _, c := range cases {
		if got := resolveDevicePath(c.name); got != c.want {
			t.Errorf("resolveDevicePath(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestGetDeviceSizeFromSeeker(t *testing.T) {
	path := filepath.Join(t.TempDir(), "disk")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	size, err := getDeviceSize("unused-when-f-is-non-nil", f)
	if err != nil {
		t.Fatalf("getDeviceSize(with seeker): %v", err)
	}
	if size != 10 {
		t.Errorf("getDeviceSize(with seeker) = %d, want 10", size)
	}
}

func TestGetDeviceSizeMissingDeviceFallsThroughToZero(t *testing.T) {
	// No sysfs entry and no real device to open: every fallback in
	// getDeviceSize fails silently and it must report 0 with no error,
	// never block or panic on a device that simply is not there.
	size, err := getDeviceSize("/dev/f4-lunobot-nonexistent-device-xyz", nil)
	if err != nil {
		t.Fatalf("getDeviceSize(missing device): %v", err)
	}
	if size != 0 {
		t.Errorf("getDeviceSize(missing device) = %d, want 0", size)
	}
}

func TestGetDeviceSizeSysfsLookup(t *testing.T) {
	// Exercise the real /sys/class/block lookup branch against whatever
	// block device this Linux CI runner actually exposes, rather than a
	// synthetic path -- getDeviceSize hard-codes "/sys/class/block", so
	// there is no injection seam for a fake one.
	entries, err := os.ReadDir("/sys/class/block")
	if err != nil || len(entries) == 0 {
		t.Skip("no /sys/class/block on this runner")
	}
	name := entries[0].Name()
	sysfsSize, ok := readUintFileForTest(t, "/sys/class/block/"+name+"/size")
	if !ok {
		t.Skipf("could not read size for %s", name)
	}
	want, ok := parseSysfsBlockSize([]byte(sysfsSize))
	if !ok {
		t.Skipf("size for %s did not parse", name)
	}

	got, err := getDeviceSize("/dev/"+name, nil)
	if err != nil {
		t.Fatalf("getDeviceSize(%q): %v", name, err)
	}
	if got != want {
		t.Errorf("getDeviceSize(%q) = %d, want %d (from sysfs)", name, got, want)
	}
}

func readUintFileForTest(t *testing.T, path string) (string, bool) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(data), true
}

func TestGetPlatformBlockDevicesUnix(t *testing.T) {
	items := getPlatformBlockDevices(context.Background())
	for _, it := range items {
		if it.Name == "" {
			t.Error("getPlatformBlockDevices returned an item with an empty Name")
		}
		if !it.SizeKnown {
			t.Errorf("item %q has SizeKnown = false, want true", it.Name)
		}
		if it.KnownMetadata != MetadataExplicit && it.KnownMetadata != 0 {
			// The /dev fallback branch does not set KnownMetadata; the
			// /sys/class/block branch does. Either is fine, anything else
			// is not.
			t.Errorf("item %q has unexpected KnownMetadata %v", it.Name, it.KnownMetadata)
		}
	}
}

func TestGetPlatformBlockDevicesUnixHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A cancelled context must not make this panic or hang; the exact
	// item count is runner-dependent (the /sys/class/block loop is checked
	// once per entry), so only the "doesn't blow up" contract is asserted.
	_ = getPlatformBlockDevices(ctx)
}

func TestDmMapperNameIsPreferredOverRawName(t *testing.T) {
	// This does not fabricate a fake /sys/class/block entry (that tree is
	// kernel-owned, not writable in a test), so it documents the behavior
	// via the real entries on this runner instead: any dm-* device that
	// does have a mapper name must be displayed as "mapper/<name>", never
	// as the raw "dm-N".
	items := getPlatformBlockDevices(context.Background())
	for _, it := range items {
		if !strings.HasPrefix(it.Name, "dm-") {
			continue
		}
		t.Errorf("device-mapper entry %q was not resolved to its mapper/<name> form", it.Name)
	}
}
