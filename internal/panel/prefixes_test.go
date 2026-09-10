package panel

import (
	"testing"
)

func TestPlatformDriveCommandPrefixAliases(t *testing.T) {
	if got := platformDriveCommandPrefix("Windows Registry"); got != "reg" {
		t.Fatalf("Windows Registry prefix = %q, want reg", got)
	}
	if got := platformDriveCommandPrefix("windows registry"); got != "reg" {
		t.Fatalf("case-insensitive Windows Registry prefix = %q, want reg", got)
	}
	if got := platformDriveCommandPrefix("Physical Disks"); got != "" {
		t.Fatalf("unexpected platform drive prefix = %q", got)
	}
}
