//go:build windows

package panel

import (
	"github.com/unxed/f4/vfs"
	"testing"
)

func TestCommandPrefixOpensWindowsRegistryDrive(t *testing.T) {
	pf := setupMockPanelsFrame(t)
	defer pf.Close()

	if !DispatchCommandPrefix(pf, "REG:") {
		t.Fatal("reg prefix was not consumed")
	}
	if _, ok := pf.GetActivePanel().Vfs.(*vfs.RegistryVFS); !ok {
		t.Fatalf("active panel VFS = %T, want *vfs.RegistryVFS", pf.GetActivePanel().Vfs)
	}
}
