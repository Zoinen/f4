//go:build !unix

package fishplus

import "io/fs"

func statOwner(fs.FileInfo) (uid, gid int) { return 0, 0 }
