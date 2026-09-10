package viewer

import (
	vfs "github.com/unxed/f4/vfs"
)

type semanticExpensiveReader struct{ vfs.ReadAtCloser }

func (semanticExpensiveReader) ReadAccessProfile() vfs.ReadAccessProfile {
	return vfs.ReadAccessUnknownExpensive
}
