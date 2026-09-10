//go:build windows

package vfs

import (
	"context"
	"io"
	"os"
	"testing"
)

func TestResolveDevicePath(t *testing.T) {
	if got := resolveDevicePath("PhysicalDrive0"); got != "\\\\.\\PhysicalDrive0" {
		t.Fatalf("resolveDevicePath() = %q, want \\\\.\\PhysicalDrive0", got)
	}
	if got := resolveDevicePath("\\\\.\\PhysicalDrive0"); got != "\\\\.\\PhysicalDrive0" {
		t.Fatalf("resolveDevicePath(existing prefix) = %q", got)
	}
}

func TestGetDeviceSizeUsesSeekableFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "device")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("payload"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(2, io.SeekStart); err != nil {
		t.Fatal(err)
	}

	got, err := getDeviceSize("unused", f)
	if err != nil {
		t.Fatalf("getDeviceSize() error = %v", err)
	}
	if got != int64(len("payload")) {
		t.Fatalf("getDeviceSize() = %d, want %d", got, len("payload"))
	}
	pos, err := f.Seek(0, io.SeekCurrent)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 0 {
		t.Fatalf("size probe left file offset at %d, want 0", pos)
	}
}

func TestGetDeviceSizeRejectsEmbeddedNUL(t *testing.T) {
	got, err := getDeviceSize("bad\x00path", nil)
	if err != nil {
		t.Fatalf("getDeviceSize() error = %v, want nil", err)
	}
	if got != 0 {
		t.Fatalf("getDeviceSize() = %d, want 0", got)
	}
}

func TestGetPlatformBlockDevicesHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := getPlatformBlockDevices(ctx); len(got) != 0 {
		t.Fatalf("getPlatformBlockDevices(canceled) = %#v, want empty", got)
	}
}
