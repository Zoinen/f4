package fishplus

import (
	"testing"
	"time"
)

func TestParseLsEntry(t *testing.T) {
	cases := []struct {
		name    string
		variant string
		line    string
		want    Entry
	}{
		{
			name:    "GNU with an epoch",
			variant: "epoch",
			line:    "-rw-r--r-- 1 1000 100    5 1785950384 a file.txt",
			want:    Entry{Name: "a file.txt", Size: 5, Mode: 0100644, Uid: 1000, Gid: 100},
		},
		{
			name:    "a directory",
			variant: "epoch",
			line:    "drwxr-xr-x 2 0 0 4096 1785950384 sub dir",
			want:    Entry{Name: "sub dir", Size: 4096, Mode: 0040755},
		},
		{
			name:    "a symlink, whose target is not its name",
			variant: "epoch",
			line:    "lrwxrwxrwx 1 0 0 10 1785950384 link -> a file.txt",
			want:    Entry{Name: "link", Size: 10, Mode: 0120777},
		},
		{
			name:    "setuid, setgid and sticky",
			variant: "epoch",
			line:    "-rwsr-Sr-T 1 0 0 0 1785950384 odd",
			want:    Entry{Name: "odd", Mode: 0106744 | 01000},
		},
		{
			name:    "an ACL marker, which macOS and Linux both add",
			variant: "epoch",
			line:    "-rw-r--r--@ 1 501 20 7 1785950384 marked",
			want:    Entry{Name: "marked", Size: 7, Mode: 0100644, Uid: 501, Gid: 20},
		},
		{
			name:    "BSD with a full date",
			variant: "bsd",
			line:    "-rw-r--r-- 1 501 20 18765 Jul 23 00:57:04 2026 LICENSE.txt",
			want:    Entry{Name: "LICENSE.txt", Size: 18765, Mode: 0100644, Uid: 501, Gid: 20},
		},
		{
			// BusyBox, which is the reason this dialect exists: it takes
			// --full-time and nothing else that carries a year.
			name:    "full iso without a fraction",
			variant: "iso",
			line:    "drwxr-xr-x 20 0 0 4096 2026-06-05 15:09:23 +0300 a dir",
			want:    Entry{Name: "a dir", Size: 4096, Mode: 0040755},
		},
		{
			name:    "full iso with a fraction",
			variant: "iso",
			line:    "-rw-r--r-- 1 1000 1000 12 2026-08-03 19:40:32.480954998 +0300 x.txt",
			want:    Entry{Name: "x.txt", Size: 12, Mode: 0100644, Uid: 1000, Gid: 1000},
		},
		{
			name:    "a name of nothing but spaces keeps them",
			variant: "epoch",
			line:    "-rw-r--r-- 1 0 0 0 1785950384    three",
			want:    Entry{Name: "   three", Mode: 0100644},
		},
	}

	for _, tc := range cases {
		e, err := parseLsEntry(tc.line, tc.variant, false)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if e.Name != tc.want.Name {
			t.Errorf("%s: Name = %q, want %q", tc.name, e.Name, tc.want.Name)
		}
		if e.Size != tc.want.Size {
			t.Errorf("%s: Size = %d, want %d", tc.name, e.Size, tc.want.Size)
		}
		if e.Mode != tc.want.Mode {
			t.Errorf("%s: Mode = %o, want %o", tc.name, e.Mode, tc.want.Mode)
		}
		if e.Uid != tc.want.Uid || e.Gid != tc.want.Gid {
			t.Errorf("%s: ids = %d/%d, want %d/%d", tc.name, e.Uid, e.Gid, tc.want.Uid, tc.want.Gid)
		}
		if e.MTime.IsZero() {
			t.Errorf("%s: no timestamp", tc.name)
		}
	}

	// The epoch dialect is exact, so it is worth checking rather than only
	// checking that something came back.
	e, err := parseLsEntry("-rw-r--r-- 1 0 0 0 1785950384 x", "epoch", false)
	if err != nil {
		t.Fatal(err)
	}
	if !e.MTime.Equal(time.Unix(1785950384, 0)) {
		t.Errorf("MTime = %v", e.MTime)
	}

	// The iso dialect carries its zone, so it is exact too, and worth
	// checking rather than only checking that it parsed.
	e, err = parseLsEntry("-rw-r--r-- 1 0 0 0 2026-06-05 15:09:23 +0300 x", "iso", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.MTime.UTC().Format("2006-01-02T15:04:05Z"); got != "2026-06-05T12:09:23Z" {
		t.Errorf("iso MTime = %v", got)
	}

	// The lines that are not entries have to be refused rather than turned
	// into an entry with a strange name.
	for _, bad := range []string{
		"total 8",
		"",
		"-rw-r--r-- 1 0 0 0 1785950384",
		"?rw-r--r-- 1 0 0 0 1785950384 x",
		"-rw-r--r-- 1 nobody 0 0 1785950384 x",
	} {
		if _, err := parseLsEntry(bad, "epoch", false); err == nil {
			t.Errorf("%q was accepted as an entry", bad)
		}
	}

	// A search wants the path it was handed, a listing wants the name.
	if e, err := parseLsEntry("-rw-r--r-- 1 0 0 0 1785950384 /tmp/x/a file.txt", "epoch", true); err != nil {
		t.Fatal(err)
	} else if e.Name != "/tmp/x/a file.txt" {
		t.Errorf("keepPath gave %q", e.Name)
	}
}
