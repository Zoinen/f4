//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package winex11drag

import (
	"reflect"
	"testing"

	"github.com/unxed/vtui"
)

func TestToUnixPath(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{`Z:\home\me\a.txt`, "/home/me/a.txt"},
		{`z:/home/me/a.txt`, "/home/me/a.txt"},
		{`Z:\`, "/"},
		{`C:\Users\me\a.txt`, `C:\Users\me\a.txt`},
		{"/home/me/a.txt", "/home/me/a.txt"},
		{"Z:", "Z:"},
		{"", ""},
	} {
		if got := ToUnixPath(tc.in); got != tc.want {
			t.Errorf("ToUnixPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUnixPayloadKeepsTheRestAndTheCallersSlice(t *testing.T) {
	in := vtui.DragPayload{Kinds: []string{"files"}, Paths: []string{`Z:\a\b`, `C:\c`}, Text: "t"}
	out := unixPayload(in)
	if want := []string{"/a/b", `C:\c`}; !reflect.DeepEqual(out.Paths, want) {
		t.Errorf("Paths = %q, want %q", out.Paths, want)
	}
	if in.Paths[0] != `Z:\a\b` {
		t.Errorf("the caller's Paths were modified: %q", in.Paths)
	}
	if !reflect.DeepEqual(out.Kinds, in.Kinds) || out.Text != in.Text {
		t.Errorf("other fields changed: %+v", out)
	}
	if got := unixPayload(vtui.DragPayload{Text: "x"}); got.Paths != nil || got.Text != "x" {
		t.Errorf("a payload without paths changed: %+v", got)
	}
}
