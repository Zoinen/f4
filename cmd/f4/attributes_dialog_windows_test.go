//go:build windows

package main

import (
	"path/filepath"
	"testing"
)

func TestShowNativePropertiesOS_MissingPathFailsBeforeShell(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.exe")
	if err := showNativePropertiesOS(missing); err == nil {
		t.Fatal("opening native properties for a missing path unexpectedly succeeded")
	}
}
