//go:build !linux || !goffi_musl

package main

import "testing"

// The counterpart to TestBuildLibcIsMusl: every other build must claim no
// libc, so the updater keeps asking for the generic artifact it always has.
func TestBuildLibcIsUnset(t *testing.T) {
	if buildLibc != "" {
		t.Fatalf("buildLibc = %q in a default build, want empty", buildLibc)
	}
}
