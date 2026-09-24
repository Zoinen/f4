//go:build !windows

package terminal

import (
	"reflect"
	"testing"
)

func TestAttachPayloadRoundTrip(t *testing.T) {
	cases := []struct {
		name                  string
		editPath, left, right string
		viewPaths             []string
		wantWire              string
		wantEdit              string
		wantLeft, wantRight   string
		wantView              []string
	}{
		{name: "plain", wantWire: "ATTACH"},
		{
			name: "one directory", left: "/home/u/dir",
			wantWire: "ATTACH\nCWD /home/u/dir", wantLeft: "/home/u/dir",
		},
		{
			// Both panels going to the same place is what a plain start looks
			// like, and it stays a one-line datagram.
			name: "same on both sides", left: "/home/u/dir", right: "/home/u/dir",
			wantWire: "ATTACH\nCWD /home/u/dir", wantLeft: "/home/u/dir",
		},
		{
			name: "two directories", left: "/home/u/a", right: "/home/u/b",
			wantWire: "ATTACH\nCWD /home/u/a\nCWD2 /home/u/b",
			wantLeft: "/home/u/a", wantRight: "/home/u/b",
		},
		// -e keeps the single-line wire format an older daemon understands.
		{
			name: "edit", editPath: "/tmp/f.txt", left: "/home/u/a", right: "/home/u/b",
			wantWire: "ATTACH /tmp/f.txt", wantEdit: "/tmp/f.txt",
		},
		// `f4 file` (issue #991): the file rides on a line of its own after
		// the directories, where an older daemon skips it.
		{
			name: "view", left: "/home/u/a/f.txt", right: "/home/u", viewPaths: []string{"/home/u/a/f.txt"},
			wantWire: "ATTACH\nCWD /home/u/a/f.txt\nCWD2 /home/u\nVIEW /home/u/a/f.txt",
			wantLeft: "/home/u/a/f.txt", wantRight: "/home/u", wantView: []string{"/home/u/a/f.txt"},
		},
		{
			name: "several files", viewPaths: []string{"/a/1.txt", "/b/2.png"},
			wantWire: "ATTACH\nVIEW /a/1.txt\nVIEW /b/2.png", wantView: []string{"/a/1.txt", "/b/2.png"},
		},
		// A line break in a name would read as further lines of the datagram.
		{
			name: "line break in a name", viewPaths: []string{"/a/bad\nCWD /etc", "/a/good"},
			wantWire: "ATTACH\nVIEW /a/good", wantView: []string{"/a/good"},
		},
		// Next to -e nothing else is sent, for the same older daemon.
		{
			name: "edit and view", editPath: "/tmp/f.txt", viewPaths: []string{"/tmp/g.txt"},
			wantWire: "ATTACH /tmp/f.txt", wantEdit: "/tmp/f.txt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wire := string(attachPayload(tc.editPath, tc.left, tc.right, tc.viewPaths))
			if wire != tc.wantWire {
				t.Fatalf("attachPayload = %q, want %q", wire, tc.wantWire)
			}
			edit, left, right, view := parseAttachPayload(wire)
			if edit != tc.wantEdit || left != tc.wantLeft || right != tc.wantRight || !reflect.DeepEqual(view, tc.wantView) {
				t.Fatalf("parseAttachPayload(%q) = (%q, %q, %q, %q), want (%q, %q, %q, %q)",
					wire, edit, left, right, view, tc.wantEdit, tc.wantLeft, tc.wantRight, tc.wantView)
			}
		})
	}
}

// A datagram from a client that speaks of something this build knows nothing
// about must still attach, with the lines it does understand.
func TestParseAttachPayloadIgnoresUnknownLines(t *testing.T) {
	edit, left, right, view := parseAttachPayload("ATTACH\nCWD /home/u/a\nSOMETHING new\nCWD2 /home/u/b")
	if edit != "" || left != "/home/u/a" || right != "/home/u/b" || view != nil {
		t.Fatalf("got (%q, %q, %q, %q), want (\"\", \"/home/u/a\", \"/home/u/b\", nil)", edit, left, right, view)
	}
}
