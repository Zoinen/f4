//go:build !windows

package terminal

import (
	"reflect"
	"testing"
)

func TestParseFileClipboardJSON(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"two files", `["/Users/a/one.txt","/Users/a/dir/two.png"]`, []string{"/Users/a/one.txt", "/Users/a/dir/two.png"}},
		{"nothing", `[]`, nil},
		{"relative names are dropped", `["one.txt","/Users/a/two.txt"]`, []string{"/Users/a/two.txt"}},
		{"cleaned", `["/Users/a//dir/../one.txt"]`, []string{"/Users/a/one.txt"}},
		{"not json", `Cannot read`, nil},
		{"not an array", `{"a":1}`, nil},
		{"empty output", ``, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseFileClipboardJSON([]byte(tc.in)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("parseFileClipboardJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
