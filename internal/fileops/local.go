package fileops

import (
	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostfs"
	"os"
	"reflect"
	"strings"
)

// Compare filesystem identities, not spelling: a local volume may ignore case,
// and different names may be hard links. Never interpret a remote path locally.
func sameLocalFile(src, dst vfs.VFS, srcPath, dstPath string) bool {
	_, srcLocal := src.(*vfs.OSVFS)
	_, dstLocal := dst.(*vfs.OSVFS)
	if !srcLocal || !dstLocal {
		return false
	}
	srcPath, srcErr := src.Abs(srcPath)
	dstPath, dstErr := dst.Abs(dstPath)
	if srcErr != nil || dstErr != nil {
		return false
	}
	srcInfo, srcErr := hostfs.Stat(srcPath)
	dstInfo, dstErr := hostfs.Stat(dstPath)
	return srcErr == nil && dstErr == nil && os.SameFile(srcInfo, dstInfo)
}

func isLocalCaseRename(src, dst vfs.VFS, srcPath, dstPath string) bool {
	srcName, dstName := src.Base(srcPath), dst.Base(dstPath)
	if srcName == dstName || !strings.EqualFold(srcName, dstName) {
		return false
	}
	if !sameLocalFile(src, dst, src.Dir(srcPath), dst.Dir(dstPath)) {
		return false
	}
	srcPath, srcErr := src.Abs(srcPath)
	dstPath, dstErr := dst.Abs(dstPath)
	if srcErr != nil || dstErr != nil {
		return false
	}
	// Do not mistake two distinct symlinks to the same target for one entry.
	srcInfo, srcErr := hostfs.Lstat(srcPath)
	dstInfo, dstErr := hostfs.Lstat(dstPath)
	if srcErr != nil || dstErr != nil || !os.SameFile(srcInfo, dstInfo) {
		return false
	}
	// On a case-sensitive volume two hard links can genuinely have these two
	// spellings. They must not be reported as a completed case-only rename.
	entries, err := hostfs.ReadDir(src.Dir(srcPath))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Name() == dstName {
			return false
		}
	}
	return true
}

// IsLocalOSVFS reports whether v ultimately reads the machine's own file
// system, looking through the wrappers a panel stacks on top of one. Several
// operations are only offered on local paths — native properties, memory
// mapping, the system file manager — and asking the type is how they tell.
//
// The walk is reflective because a wrapper exposes no interface saying what it
// wraps: it descends into struct fields until it finds an *vfs.OSVFS or an
// *vfs.DisksVFS, or runs out.
func IsLocalOSVFS(v any) bool {
	if v == nil {
		return false
	}
	val := reflect.ValueOf(v)
	if val.Kind() == reflect.Interface {
		val = val.Elem()
	}
	if val.Kind() == reflect.Ptr {
		if _, ok := val.Interface().(*vfs.OSVFS); ok {
			return true
		}
		if _, ok := val.Interface().(*vfs.DisksVFS); ok {
			return true
		}
		val = val.Elem()
	}
	if val.Kind() == reflect.Struct {
		for i := 0; i < val.NumField(); i++ {
			field := val.Field(i)
			if field.CanInterface() {
				if IsLocalOSVFS(field.Interface()) {
					return true
				}
			}
		}
	}
	return false
}

// SameVFSInstance is deliberately stricter than cache identity. Two pooled
// remote views may share cached directory data, but an asynchronous provider
// transition belongs to the exact parent object it was started from.
func SameVFSInstance(a, b vfs.VFS) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}
