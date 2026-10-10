//go:build !lite

package netfox

import (
	"testing"
)

func TestSFTPVFS_Abs(t *testing.T) {
	v := &SFTPVFS{
		path: "/home/user/test",
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"file.txt", "/home/user/test/file.txt"},
		{"sub/dir", "/home/user/test/sub/dir"},
		{"/etc/passwd", "/etc/passwd"},
		{"/", "/"},
	}

	for _, tt := range tests {
		got, _ := v.Abs(tt.input)
		if got != tt.expected {
			t.Errorf("SFTPVFS.Abs(%q): expected %q, got %q", tt.input, tt.expected, got)
		}
	}
}

func TestFTPVFS_Abs(t *testing.T) {
	v := &FTPVFS{
		cwd: "/pub/files",
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"data.zip", "/pub/files/data.zip"},
		{"/root.txt", "/root.txt"},
		{"", "/pub/files"},
	}

	for _, tt := range tests {
		got, _ := v.Abs(tt.input)
		if got != tt.expected {
			t.Errorf("FTPVFS.Abs(%q): expected %q, got %q", tt.input, tt.expected, got)
		}
	}
}

func TestSFTPVFS_UtilityMethods(t *testing.T) {
	parent := &netFoxVFSWrapper{NewNetFoxVFS(t.TempDir() + "/db.json")}
	v := &SFTPVFS{
		parent: parent,
		path:   "/home/user",
		title:  "user@server",
	}

	// 1. GetPath
	if v.GetPath() != "/home/user" {
		t.Errorf("Expected path /home/user, got %q", v.GetPath())
	}

	// 2. IsAtRoot
	if v.IsAtRoot() {
		t.Error("Expected IsAtRoot to be false for /home/user")
	}
	v.path = "/"
	if !v.IsAtRoot() {
		t.Error("Expected IsAtRoot to be true for /")
	}

	// 3. ParentVFS
	if v.ParentVFS() != parent {
		t.Errorf("ParentVFS mismatch")
	}

	// 4. Base & Dir
	if v.Base("/etc/passwd") != "passwd" {
		t.Errorf("Base mismatch: got %q", v.Base("/etc/passwd"))
	}
	if v.Dir("/etc/passwd") != "/etc" {
		t.Errorf("Dir mismatch: got %q", v.Dir("/etc/passwd"))
	}

	// 5. GetTitle
	if v.GetTitle() != "user@server" {
		t.Errorf("GetTitle mismatch: got %q", v.GetTitle())
	}

	// 6. GetCapabilities
	caps := v.GetCapabilities()
	if !caps.HasRandomAccess || !caps.HasUnixPermissions {
		t.Error("SFTPVFS should support random access and Unix permissions")
	}
}

func TestFTPVFS_UtilityMethods(t *testing.T) {
	parent := &netFoxVFSWrapper{NewNetFoxVFS(t.TempDir() + "/db.json")}
	v := &FTPVFS{
		parent: parent,
		cwd:    "/pub/files",
		title:  "ftp.server",
	}

	// 1. GetPath
	if v.GetPath() != "/pub/files" {
		t.Errorf("Expected path /pub/files, got %q", v.GetPath())
	}

	// 2. IsAtRoot
	if v.IsAtRoot() {
		t.Error("Expected IsAtRoot to be false for /pub/files")
	}
	v.cwd = "."
	if !v.IsAtRoot() {
		t.Error("Expected IsAtRoot to be true for .")
	}

	// 3. ParentVFS
	if v.ParentVFS() != parent {
		t.Errorf("ParentVFS mismatch")
	}

	// 4. Base & Dir
	if v.Base("/etc/passwd") != "passwd" {
		t.Errorf("Base mismatch: got %q", v.Base("/etc/passwd"))
	}
	if v.Dir("/etc/passwd") != "/etc" {
		t.Errorf("Dir mismatch: got %q", v.Dir("/etc/passwd"))
	}

	// 5. GetTitle
	if v.GetTitle() != "ftp.server" {
		t.Errorf("GetTitle mismatch: got %q", v.GetTitle())
	}

	// 6. GetCapabilities
	caps := v.GetCapabilities()
	if !caps.HasUnixPermissions {
		t.Error("FTPVFS should support Unix permissions")
	}
	if caps.HasRandomAccess {
		t.Error("FTPVFS should not support random access")
	}
}
