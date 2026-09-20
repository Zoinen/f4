//go:build windows

package fileops

import (
	"context"
	"errors"
	"unsafe"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
)

// On Windows the access rights of Far's "Access rights" choice are the DACL,
// which permission bits cannot express: Go's Chmod there only toggles the
// read-only attribute. This mirrors Far Manager's own implementation
// (far/copy.cpp GetSecurity/SetSecurity/ResetSecurity and
// far/platform.security.cpp set_security/reset_security).

// x/sys/windows has no wrapper for InitializeAcl, and its ACL type keeps the
// header fields private, so the documented call builds the empty ACL.
var procInitializeAcl = windows.NewLazySystemDLL("advapi32.dll").NewProc("InitializeAcl")

// aclRevision is ACL_REVISION from winnt.h.
const aclRevision = 2

func applyPlatformRights(ctx context.Context, state *FileOpState, srcVfs vfs.VFS, srcPath string, dstVfs vfs.VFS, dstPath string) {
	if state == nil || ctx.Err() != nil || !IsLocalOSVFS(dstVfs) {
		return
	}
	dst, err := dstVfs.Abs(dstPath)
	if err != nil {
		return
	}
	switch state.AccessRights {
	case AccessRightsCopy:
		if srcVfs == nil || !IsLocalOSVFS(srcVfs) {
			return
		}
		src, absErr := srcVfs.Abs(srcPath)
		if absErr != nil {
			return
		}
		err = copyDACL(src, dst)
	case AccessRightsInherit:
		err = resetDACL(dst)
	default:
		return
	}
	if err != nil {
		vtui.DebugLog("FILEOP: access rights of %q: %v", dst, err)
		state.note("RIGHTS   %s: %v", dst, err)
	}
}

// copyDACL puts the DACL of src on dst, protected or not as it was on src.
func copyDACL(src, dst string) error {
	sd, err := windows.GetNamedSecurityInfo(src, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return err
	}
	control, _, err := sd.Control()
	if err != nil {
		return err
	}
	// An absent DACL is passed on as absent, as Far passes on what
	// GetSecurityDescriptorDacl gave it.
	dacl, _, err := sd.DACL()
	if err != nil && !errors.Is(err, windows.ERROR_OBJECT_NOT_FOUND) {
		return err
	}
	var info windows.SECURITY_INFORMATION = windows.DACL_SECURITY_INFORMATION
	if control&windows.SE_DACL_PROTECTED != 0 {
		info |= windows.PROTECTED_DACL_SECURITY_INFORMATION
	} else {
		info |= windows.UNPROTECTED_DACL_SECURITY_INFORMATION
	}
	return windows.SetNamedSecurityInfo(dst, windows.SE_FILE_OBJECT, info, nil, nil, dacl, nil)
}

// resetDACL replaces the explicit DACL of path with an empty, unprotected one,
// so that what it has is what it inherits from its folder.
func resetDACL(path string) error {
	var empty windows.ACL
	if ok, _, callErr := procInitializeAcl.Call(uintptr(unsafe.Pointer(&empty)), unsafe.Sizeof(empty), aclRevision); ok == 0 {
		return callErr
	}
	return windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, nil, nil, &empty, nil)
}
