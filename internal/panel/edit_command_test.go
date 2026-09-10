package panel

import (
	"testing"
)

func TestParsePlainEditCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		path string
		ok   bool
	}{
		{name: "plain path", cmd: "edit:notes.txt", path: "notes.txt", ok: true},
		{name: "case insensitive with spaces", cmd: " EDIT: report final.txt ", path: "report final.txt", ok: true},
		{name: "empty path", cmd: "edit:", ok: false},
		{name: "captured command", cmd: "edit:<< echo output", ok: false},
		{name: "other command", cmd: "view:notes.txt", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, ok := parsePlainEditCommand(tt.cmd)
			if path != tt.path || ok != tt.ok {
				t.Fatalf("parsePlainEditCommand(%q) = (%q, %v), want (%q, %v)", tt.cmd, path, ok, tt.path, tt.ok)
			}
		})
	}
}
