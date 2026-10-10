package vfs

import "testing"

func TestUNCToSMBURI(t *testing.T) {
	cases := []struct {
		in      string
		slashes bool
		want    string
		ok      bool
	}{
		{`\\host`, false, "smb://host", true},
		{`\\host\`, false, "smb://host", true},
		{`\\host\share`, false, "smb://host/share", true},
		{`\\host\share\dir one\file.txt`, false, "smb://host/share/dir%20one/file.txt", true},
		{`\\user@host\share`, false, "smb://user@host/share", true},
		{`\\192.168.0.5\c$`, false, "smb://192.168.0.5/c$", true},
		{"//host/share/dir", true, "smb://host/share/dir", true},
		{"//host/share/dir", false, "", false},
		{`\\?\C:\x`, false, "", false},
		{`\\.\pipe\x`, false, "", false},
		{`\\`, false, "", false},
		{`\\\share`, false, "", false},
		{`\\ho st\share`, false, "", false},
		{"/host/share", true, "", false},
		{"smb://host", true, "", false},
		{"", true, "", false},
	}
	for _, c := range cases {
		got, ok := UNCToSMBURI(c.in, c.slashes)
		if ok != c.ok || got != c.want {
			t.Errorf("UNCToSMBURI(%q, %v) = %q, %v; want %q, %v", c.in, c.slashes, got, ok, c.want, c.ok)
		}
	}
}
