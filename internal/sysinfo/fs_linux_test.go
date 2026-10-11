//go:build linux

package sysinfo

import "testing"

func TestMountCovers(t *testing.T) {
	cases := []struct {
		mp, path string
		want     bool
	}{
		{"/", "/anything", true},
		{"/", "/", true},
		{"/home", "/home", true},
		{"/home", "/home/user", true},
		{"/home", "/homework", false}, // must not match a sibling prefix
		{"/mnt/data", "/mnt/data/sub/dir", true},
		{"/mnt/data", "/mnt/other", false},
	}
	for _, c := range cases {
		if got := mountCovers(c.mp, c.path); got != c.want {
			t.Errorf("mountCovers(%q, %q) = %v, want %v", c.mp, c.path, got, c.want)
		}
	}
}

func TestFSEmptyPath(t *testing.T) {
	if _, ok := FS(""); ok {
		t.Error("FS(\"\") ok = true, want false")
	}
}

func TestFSMissingPath(t *testing.T) {
	if _, ok := FS("/this/path/should/not/exist/on/the/ci/runner"); ok {
		t.Error("FS(missing) ok = true, want false")
	}
}

func TestFSRoot(t *testing.T) {
	// "/" always exists and is statfs-able on the Linux CI runner this
	// test runs on, exercising both the syscall.Statfs success path and
	// the /proc/mounts enrichment.
	info, ok := FS("/")
	if !ok {
		t.Fatal("FS(\"/\") ok = false, want true")
	}
	if info.Total == 0 {
		t.Error("Total = 0, want a positive filesystem size for /")
	}
	if info.ClusterSize == 0 {
		t.Error("ClusterSize = 0, want a positive block size")
	}
	if info.Mount == "" {
		t.Error("Mount = \"\", want a mount point resolved from /proc/mounts")
	}
}

func TestEnrichFromProcMountsPicksLongestPrefix(t *testing.T) {
	// / is always a mount point on a real system; enrichFromProcMounts must
	// resolve a nested path like /proc/self to whichever real mount covers
	// it (either "/" itself, or a more specific mount such as "/proc" if
	// the runner has one), not silently leave Mount empty.
	var info FSInfo
	enrichFromProcMounts("/", &info)
	if info.Mount == "" {
		t.Error("enrichFromProcMounts did not resolve a mount point for /")
	}
}
