//go:build !unix && !windows

package vfs

import (
	"context"
	"os"
)

// fillPhysicalSizeCheap and fillPhysicalSize are stubs on platforms
// without a portable way to obtain per-file allocation info. Both
// leave item.PhysicalSize at 0; consumers should hide their "physical
// size" UI when the accumulated total is 0.
func fillPhysicalSizeCheap(_ *VFSItem, _ os.FileInfo)      {}
func fillPhysicalSize(_ *VFSItem, _ os.FileInfo, _ string) {}

// SupportsPhysicalSize is false on stub platforms — telling the
// scanner not to bother with the lazy Stat fallback (it would just
// return zero and add an N+1 syscall on the copy/move pre-scan).
func (v *OSVFS) SupportsPhysicalSize() bool { return false }

// FileIdentity is unavailable on stub platforms — with no portable way
// to read a stable file index, the scanner falls back to no dedup, the
// same behaviour as when Device/Inode are left zero.
func (v *OSVFS) FileIdentity(_ context.Context, _ string) (uint64, uint64, bool) {
	return 0, 0, false
}

// isReparsePoint stub — no meaningful notion of reparse points on
// platforms that hit this build path.
func isReparsePoint(_ os.FileInfo) bool { return false }
