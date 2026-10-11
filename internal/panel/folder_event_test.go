package panel

import "testing"

func TestRaiseFolderChangedRemembersTheFolder(t *testing.T) {
	raiseFolderChanged(nil, "/a") // no panel: nothing to remember, nothing to fail
	fp := &FileSystemPanel{}
	raiseFolderChanged(fp, "")
	if fp.folderEventPath != "" {
		t.Errorf("an empty path was remembered as %q", fp.folderEventPath)
	}
	raiseFolderChanged(fp, "/a")
	if fp.folderEventPath != "/a" {
		t.Errorf("folderEventPath = %q, want /a", fp.folderEventPath)
	}
	raiseFolderChanged(fp, "/a") // a refresh of the same folder
	raiseFolderChanged(fp, "/b")
	if fp.folderEventPath != "/b" {
		t.Errorf("folderEventPath = %q, want /b", fp.folderEventPath)
	}
}
