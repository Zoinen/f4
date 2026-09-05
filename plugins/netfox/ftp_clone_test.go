package netfox

import "testing"

func TestFTPVFSCloneHasIndependentPathViewAndLifetime(t *testing.T) {
	session := &ftpSession{refs: 1}
	original := &FTPVFS{session: session, conn: session.conn, cwd: "/source"}

	cloned := original.Clone()
	clone, ok := cloned.(*FTPVFS)
	if !ok {
		t.Fatalf("clone type = %T, want *FTPVFS", cloned)
	}
	if clone == original {
		t.Fatal("FTP clone shares the source VFS")
	}
	if clone.GetPath() != original.GetPath() {
		t.Fatalf("clone path = %q, want %q", clone.GetPath(), original.GetPath())
	}

	clone.mu.Lock()
	clone.cwd = "/clone"
	clone.mu.Unlock()
	if got := original.GetPath(); got != "/source" {
		t.Fatalf("changing clone path changed source path to %q", got)
	}

	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	closedAfterOriginal := session.closed
	remainingRefs := session.refs
	session.mu.Unlock()
	if closedAfterOriginal || remainingRefs != 1 {
		t.Fatalf("closing source closed shared session: closed=%v refs=%d", closedAfterOriginal, remainingRefs)
	}
	if err := clone.Close(); err != nil {
		t.Fatal(err)
	}
	session.mu.Lock()
	closedAfterClone := session.closed
	session.mu.Unlock()
	if !closedAfterClone {
		t.Fatal("closing the last FTP view did not close the shared session")
	}
}
