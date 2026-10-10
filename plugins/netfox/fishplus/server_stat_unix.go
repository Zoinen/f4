//go:build unix

package fishplus

import (
	"io/fs"
	"syscall"
)

// statOwner is the owner a listing carries beyond os.FileInfo. The access and
// change times are not read: their field names differ between the unixes, the
// package keeps to the standard library, and a listing announces them as the
// modification time when it has nothing better (see server_fs.go).
func statOwner(fi fs.FileInfo) (uid, gid int) {
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return 0, 0
}
