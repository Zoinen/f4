package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
	"testing"
)

type nestedFolderHistoryVFS struct {
	*vfs.NullVFS
	parent vfs.VFS
	path   string
}

func (v *nestedFolderHistoryVFS) GetPath() string    { return v.path }
func (v *nestedFolderHistoryVFS) ParentVFS() vfs.VFS { return v.parent }

func TestShouldRecordFolderHistorySkipsUnqualifiedNestedAbsolutePath(t *testing.T) {
	pnl := &panel.FileSystemPanel{Vfs: &nestedFolderHistoryVFS{
		NullVFS: vfs.NewNullVFS(0),
		parent:  vfs.NewNullVFS(0),
		path:    "/home/user",
	}}

	if panel.ShouldRecordFolderHistory(pnl, pnl.Vfs.GetPath()) {
		t.Fatal("unqualified absolute path from a nested VFS must not enter local folder history")
	}
	if !panel.ShouldRecordFolderHistory(pnl, "remote-relative") {
		t.Fatal("relative nested paths should retain the existing history behavior")
	}
	if !panel.ShouldRecordFolderHistory(pnl, "netfox://site/home/user") {
		t.Fatal("persistent URI paths must remain eligible for folder history")
	}
}
