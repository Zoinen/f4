package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

type nestedFolderHistoryVFS struct {
	*vfs.NullVFS
	parent vfs.VFS
	path   string
}

func (v *nestedFolderHistoryVFS) GetPath() string    { return v.path }
func (v *nestedFolderHistoryVFS) ParentVFS() vfs.VFS { return v.parent }

func TestShouldRecordFolderHistorySkipsUnqualifiedNestedAbsolutePath(t *testing.T) {
	panel := &FileSystemPanel{vfs: &nestedFolderHistoryVFS{
		NullVFS: vfs.NewNullVFS(0),
		parent:  vfs.NewNullVFS(0),
		path:    "/home/user",
	}}

	if shouldRecordFolderHistory(panel, panel.vfs.GetPath()) {
		t.Fatal("unqualified absolute path from a nested VFS must not enter local folder history")
	}
	if !shouldRecordFolderHistory(panel, "remote-relative") {
		t.Fatal("relative nested paths should retain the existing history behavior")
	}
	if !shouldRecordFolderHistory(panel, "netfox://site/home/user") {
		t.Fatal("persistent URI paths must remain eligible for folder history")
	}
}
