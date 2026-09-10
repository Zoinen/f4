//go:build windows

package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestCommandPrefixOpensWindowsRegistryDrive(t *testing.T) {
	pf := setupMockPanelsFrame(t)
	defer pf.Close()

	if !dispatchCommandPrefix(pf, "REG:") {
		t.Fatal("reg prefix was not consumed")
	}
	if _, ok := pf.getActivePanel().vfs.(*vfs.RegistryVFS); !ok {
		t.Fatalf("active panel VFS = %T, want *vfs.RegistryVFS", pf.getActivePanel().vfs)
	}
}
