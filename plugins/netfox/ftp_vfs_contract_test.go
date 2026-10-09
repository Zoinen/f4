package netfox

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestFTPSessionAndTimeoutConnectionContracts(t *testing.T) {
	session := &ftpSession{refs: 1}
	session.retain()
	if err := session.release(); err != nil || session.refs != 1 {
		t.Fatalf("first release err=%v refs=%d", err, session.refs)
	}
	if err := session.release(); err != nil || !session.closed {
		t.Fatalf("final release err=%v closed=%v", err, session.closed)
	}
	session.retain()
	if session.refs != 0 {
		t.Errorf("retained closed session refs=%d", session.refs)
	}

	left, right := net.Pipe()
	t.Cleanup(func() { _ = left.Close(); _ = right.Close() })
	conn := &timeoutConn{Conn: left, timeout: time.Second}
	go func() { _, _ = right.Write([]byte("in")) }()
	buf := make([]byte, 2)
	if n, err := conn.Read(buf); n != 2 || err != nil || string(buf) != "in" {
		t.Fatalf("timeout Read=(%d,%v,%q)", n, err, buf)
	}
	go func() { _, _ = right.Read(make([]byte, 3)) }()
	if n, err := conn.Write([]byte("out")); n != 3 || err != nil {
		t.Fatalf("timeout Write=(%d,%v)", n, err)
	}
}

func TestFTPVFSLocalContractsWithoutServer(t *testing.T) {
	session := &ftpSession{refs: 1}
	v := &FTPVFS{session: session, cwd: "/home/user", title: "user@example"}
	if v.GetTitle() != "user@example" || v.GetPath() != "/home/user" || v.IsAtRoot() {
		t.Error("unexpected FTP identity")
	}
	if v.SessionKey() != session || v.operationConn() != nil || v.encodePath("a/b") != "a/b" {
		t.Error("unexpected FTP session contract")
	}
	unlock := v.operationLock()
	unlock()
	if v.pathLocked("file") != "/home/user/file" || v.pathLocked("/tmp/file") != "/tmp/file" {
		t.Error("pathLocked returned an unexpected path")
	}
	if got, err := v.Abs("file"); err != nil || got != "/home/user/file" {
		t.Errorf("Abs=(%q,%v)", got, err)
	}
	if v.Join("/home", "user") != "/home/user" || v.Base("/home/user") != "user" || v.Dir("/home/user") != "/home" || !v.IsAbs("/x") || v.IsAbs("x") {
		t.Error("FTP path helpers returned unexpected values")
	}
	if _, err := v.Search(context.Background(), "/", "x"); err != nil {
		t.Errorf("Search error=%v", err)
	}
	if err := v.SetAttributes(context.Background(), "/x", vfs.VFSItem{}); err == nil {
		t.Error("SetAttributes unexpectedly succeeded")
	}
	if got := v.GetCapabilities(); !got.HasUnixPermissions || got.HasWrite {
		t.Errorf("FTP capabilities=%#v", got)
	}
	if v.ParentVFS() != nil {
		t.Error("unexpected FTP parent")
	}
	clone := v.Clone().(*FTPVFS)
	if clone.GetPath() != v.GetPath() || clone.SessionKey() != session {
		t.Error("Clone did not preserve FTP view state")
	}
	if err := clone.Close(); err != nil {
		t.Fatal(err)
	}
	if err := v.Close(); err != nil || v.Close() != nil {
		t.Errorf("Close errors: %v", err)
	}
	var nilVFS *FTPVFS
	if err := nilVFS.Close(); err != nil {
		t.Errorf("nil Close=%v", err)
	}
}
