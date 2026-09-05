//go:build unix

package vfs

import (
	"context"
	"os"
	"syscall"
)

// fillPhysicalSizeCheap populates PhysicalSize only when the answer
// is already in memory alongside FileInfo — on Unix, stat.Blocks is
// part of Stat_t, which lstat() had to load anyway. So calling this
// from the ReadDir loop is free. Windows and stub variants no-op.
//
// info can legitimately be nil — DirEntry.Info() returns nil,err when
// the entry vanished between readdir and lstat, common in /tmp/ or
// build trees. We simply leave PhysicalSize at zero rather than panic.
func fillPhysicalSizeCheap(item *VFSItem, info os.FileInfo) {
	if info == nil {
		return
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		// stat.Blocks is always in 512-byte units per POSIX, regardless
		// of the fs's own block size. Handles sparse files (fewer blocks
		// than Size) and transparent-compression fs (btrfs/zfs — Blocks
		// reflects the compressed footprint).
		item.PhysicalSize = int64(stat.Blocks) * 512
		// Device / Inode let the scanner dedup hard links (same inode
		// reached through multiple paths). Also free — Stat_t is
		// already loaded. Windows and stubs leave these zero, so the
		// scanner just doesn't dedup there.
		// #nosec G115 -- Device is an opaque equality key; sign extension of a signed dev_t is lossless for identity comparisons.
		item.Device = uint64(stat.Dev)
		item.Inode = uint64(stat.Ino)
	}
}

// fillPhysicalSize does whatever it takes to populate PhysicalSize.
// On Unix this is the same as fillPhysicalSizeCheap. On Windows the
// two diverge — see os_vfs_physical_windows.go.
func fillPhysicalSize(item *VFSItem, info os.FileInfo, _ string) {
	fillPhysicalSizeCheap(item, info)
}

// SupportsPhysicalSize is the PhysicalSizer capability answer for
// this platform. Declared here (rather than a single unconditional
// method in os_vfs.go) so the answer is co-located with the actual
// implementation of fillPhysicalSize — no risk of OSVFS claiming a
// capability on a platform where the stub version leaves the field
// at zero.
func (v *OSVFS) SupportsPhysicalSize() bool { return true }

// FileIdentity resolves (device, inode) for path via Lstat so the
// scanner can dedup hard links through the FileIdentifier capability.
// OSVFS already stamps VFSItem.Device/Inode cheaply in the ReadDir
// path here (fillPhysicalSizeCheap reads the same Stat_t), so the
// scanner rarely needs to call this on Unix; it exists to keep the
// capability honest and to cover any item that reaches the dedup check
// without identity. Lstat (not Stat) so a symlink reports its own
// identity, never the target's.
func (v *OSVFS) FileIdentity(_ context.Context, path string) (device, inode uint64, ok bool) {
	info, err := os.Lstat(prepareOSPath(path))
	if err != nil {
		return 0, 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, false
	}
	// #nosec G115 -- Device is an opaque equality key; sign extension of a signed dev_t is lossless for identity comparisons.
	return uint64(stat.Dev), uint64(stat.Ino), true
}

// isReparsePoint reports whether info describes a reparse-point-like
// entry. There's no direct Unix equivalent — plain symlinks are
// already covered by Mode()&ModeSymlink at the caller — so this is a
// no-op stub. The Windows implementation (os_vfs_physical_windows.go)
// covers junctions which Go doesn't always mark as ModeSymlink.
func isReparsePoint(_ os.FileInfo) bool { return false }
