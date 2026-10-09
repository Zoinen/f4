package panel

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/fusefs"
)

func TestMountListHelpers(t *testing.T) {
	if got := mountMode(true); got != "ro" {
		t.Fatalf("mountMode(true) = %q; want ro", got)
	}
	if got := mountMode(false); got != "rw" {
		t.Fatalf("mountMode(false) = %q; want rw", got)
	}

	mountPoint := filepath.Join(string(filepath.Separator)+"tmp", "f4-mount")
	child := filepath.Join(mountPoint, "child")
	for _, tc := range []struct {
		name  string
		path  string
		mount string
		want  bool
	}{
		{name: "empty panel", path: "", mount: mountPoint, want: false},
		{name: "empty mount", path: child, mount: "", want: false},
		{name: "mount itself", path: mountPoint, mount: mountPoint, want: true},
		{name: "child", path: child, mount: mountPoint, want: true},
		{name: "prefix sibling", path: mountPoint + "-other", mount: mountPoint, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := withinMount(tc.path, tc.mount); got != tc.want {
				t.Fatalf("withinMount(%q, %q) = %v; want %v", tc.path, tc.mount, got, tc.want)
			}
		})
	}

	if got := hereMark(child, mountPoint); got != "  (here)" {
		t.Fatalf("hereMark(child) = %q; want mount marker", got)
	}
	if got := hereMark(filepath.Join(string(filepath.Separator)+"tmp", "other"), mountPoint); got != "" {
		t.Fatalf("hereMark(other) = %q; want empty", got)
	}
}

func TestLiveRowsFiltersForeignMounts(t *testing.T) {
	owned := &fusefs.Mount{ID: "owned"}
	rows := []mountRow{
		{point: "/foreign", source: "ssh", live: nil},
		{point: "/owned", source: "local", live: owned},
		{point: "/foreign-again", source: "fstab", live: nil},
	}

	live := liveRows(rows)
	if len(live) != 1 {
		t.Fatalf("liveRows returned %d rows; want one", len(live))
	}
	if live[0].point != "/owned" || live[0].source != "local" || live[0].live != owned {
		t.Fatalf("liveRows returned %#v; want owned mount", live[0])
	}
	if got := liveRows(nil); got != nil {
		t.Fatalf("liveRows(nil) = %#v; want nil", got)
	}
}
