//go:build windows

package fileops

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func daclProtected(t *testing.T, path string) (bool, uint16) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatalf("security of %q: %v", path, err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatalf("DACL of %q: %v", path, err)
	}
	return control&windows.SE_DACL_PROTECTED != 0, dacl.AceCount
}

// TestCopyAccessRightsWindowsDACL pins the Windows meaning of the three
// choices (#722): the DACL, as Far Manager copies and resets it.
func TestCopyAccessRightsWindowsDACL(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	srcFile := filepath.Join(src, "f.txt")
	if err := os.WriteFile(srcFile, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A protected DACL with one entry is what no file created in a
	// temporary folder receives by inheritance.
	world, err := windows.StringToSid("S-1-1-0")
	if err != nil {
		t.Fatal(err)
	}
	acl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_WELL_KNOWN_GROUP,
			TrusteeValue: windows.TrusteeValueFromSID(world),
		},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(srcFile, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}

	copyOnce(t, src, dst, "f.txt", AccessRightsDefault)
	if protected, _ := daclProtected(t, filepath.Join(dst, "f.txt")); protected {
		t.Error("Default gave the new copy the source's protected DACL; it should inherit")
	}

	copyOnce(t, src, dst, "f.txt", AccessRightsCopy)
	if protected, aces := daclProtected(t, filepath.Join(dst, "f.txt")); !protected || aces != 1 {
		t.Errorf("Copy left protected=%v with %d entries, want the source's protected DACL of 1 entry", protected, aces)
	}

	copyOnce(t, src, dst, "f.txt", AccessRightsInherit)
	if protected, _ := daclProtected(t, filepath.Join(dst, "f.txt")); protected {
		t.Error("Inherit kept a protected DACL on the overwritten copy")
	}
}
