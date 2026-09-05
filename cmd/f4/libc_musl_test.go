//go:build linux && goffi_musl

package main

import "testing"

// Compiled only into musl builds, where buildLibc must name musl -- that is
// what makes the updater ask for the musl release asset. Selecting both
// libc_default.go and libc_musl.go, or neither, is already a compile error
// (duplicate or undefined constant); what this test adds is the case those
// cannot catch, a pair of constraints that are each valid but inverted.
func TestBuildLibcIsMusl(t *testing.T) {
	if buildLibc != "musl" {
		t.Fatalf("buildLibc = %q under -tags goffi_musl, want \"musl\"", buildLibc)
	}
	suffixes := updateAssetSuffixes("linux", "amd64", currentLibc)
	if len(suffixes) == 0 || suffixes[0] != "-linux-musl-amd64.tar.gz" {
		t.Fatalf("musl build prefers %v, want the musl asset first", suffixes)
	}
}
