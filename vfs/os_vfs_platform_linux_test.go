//go:build linux

package vfs

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// NTFS counts 100 ns ticks from 1601-01-01; the expectations come from the
// documented distance of the Unix epoch from that date.
func TestNTFSFiletime(t *testing.T) {
	cases := []struct {
		name string
		at   time.Time
		want uint64
	}{
		{"unix epoch", time.Unix(0, 0), 116444736000000000},
		{"ntfs epoch", time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC), 0},
		{"one second after unix epoch", time.Unix(1, 0), 116444736000000000 + 10_000_000},
		{"sub-tick nanoseconds are truncated", time.Unix(0, 199), 116444736000000000 + 1},
		{"half a second", time.Unix(0, 500_000_000), 116444736000000000 + 5_000_000},
	}
	for _, c := range cases {
		got := ntfsFiletime(c.at)
		if len(got) != 8 {
			t.Fatalf("%s: %d bytes, want 8", c.name, len(got))
		}
		if v := binary.LittleEndian.Uint64(got); v != c.want {
			t.Errorf("%s: %d, want %d", c.name, v, c.want)
		}
	}
}

// A filesystem that does not know the attribute (nearly all of them) must not
// turn a copy into an error or a sudo retry: setting is best effort.
func TestApplyPlatformAttributesBirthTimeIsBestEffort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	item := VFSItem{BTime: time.Unix(1_600_000_000, 0), SetBTime: true}
	if err := applyPlatformAttributes(path, item); err != nil {
		t.Fatalf("applyPlatformAttributes: %v", err)
	}
	if err := applyPlatformAttributes(filepath.Join(filepath.Dir(path), "missing"), item); err != nil {
		t.Fatalf("missing path: %v", err)
	}
	if (&OSVFS{}).SupportsSetBTime() {
		t.Error("SupportsSetBTime must stay false on Linux")
	}
}
