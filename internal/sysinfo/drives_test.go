package sysinfo

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestDriveRegistryReplacementAndSnapshots(t *testing.T) {
	restore := SnapshotDrives()
	t.Cleanup(restore)

	factory := func() vfs.VFS { return nil }
	SetDrives([]DriveEntry{{Name: "root", Factory: factory, InfoPath: "/"}})
	RegisterDrive("root", factory)
	RegisterDrive("tmp", factory)

	got := DriveRegistrySnapshot()
	if len(got) != 2 || got[0].Name != "root" || got[1].Name != "tmp" {
		t.Fatalf("registry = %#v, want root and tmp in registration order", got)
	}
	if got[0].Factory == nil || got[1].Factory == nil {
		t.Fatal("registered factories must be retained")
	}
	if got[0].InfoPath != "/" {
		t.Fatalf("replacement lost InfoPath: %#v", got[0])
	}

	got[0].Name = "mutated"
	if fresh := Drives(); fresh[0].Name != "root" {
		t.Fatalf("snapshot mutation leaked into registry: %#v", fresh)
	}
}
