package app

import (
	"fmt"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/panel"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTempBookmarks(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "bookmarks.ini")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestLoadBookmarks_MissingFile(t *testing.T) {
	set, err := panel.LoadBookmarks(filepath.Join(t.TempDir(), "does_not_exist.ini"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if set != (panel.BookmarkSet{}) {
		t.Fatalf("expected empty set for missing file, got %#v", set)
	}
}

// Real-world fixture: a verbatim slice of an actual far2l bookmarks.ini,
// sections out of order and with gaps, exactly as far2l writes them.
const realFar2lBookmarksFixture = `[9]
Path=/home/sogonov/f4
Plugin=
PluginData=
PluginFile=

[6]
Path=/mnt/d/!!wrkstk/reps/ssh/ski-analyzer
Plugin=
PluginData=
PluginFile=

[8]
Path=/home/sogonov/scc
Plugin=
PluginData=
PluginFile=
`

func TestLoadBookmarks_RealFar2lFile(t *testing.T) {
	p := writeTempBookmarks(t, realFar2lBookmarksFixture)
	set, err := panel.LoadBookmarks(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := map[int]string{
		6: "/mnt/d/!!wrkstk/reps/ssh/ski-analyzer",
		8: "/home/sogonov/scc",
		9: "/home/sogonov/f4",
	}
	for slot, path := range want {
		if set[slot].Path != path {
			t.Errorf("slot %d: got %q, want %q", slot, set[slot].Path, path)
		}
	}
	for _, slot := range []int{0, 1, 2, 3, 4, 5, 7} {
		if !set[slot].IsEmpty() {
			t.Errorf("slot %d should be empty, got %#v", slot, set[slot])
		}
	}
}

func TestSaveBookmarks_OmitsEmptySlots(t *testing.T) {
	var set panel.BookmarkSet
	set[4] = panel.Bookmark{Path: "/tmp/only"}

	p := filepath.Join(t.TempDir(), "out.ini")
	if err := panel.SaveBookmarks(p, set); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(data)
	if !strings.Contains(got, "[4]\n") {
		t.Fatalf("expected section [4], got:\n%s", got)
	}
	for i := 0; i <= 9; i++ {
		if i == 4 {
			continue
		}
		if strings.Contains(got, fmt.Sprintf("[%d]", i)) {
			t.Errorf("empty slot %d must not be written, got:\n%s", i, got)
		}
	}
}

func TestSaveBookmarks_AscendingOrder(t *testing.T) {
	var set panel.BookmarkSet
	set[9] = panel.Bookmark{Path: "/nine"}
	set[6] = panel.Bookmark{Path: "/six"}
	set[8] = panel.Bookmark{Path: "/eight"}

	p := filepath.Join(t.TempDir(), "order.ini")
	if err := panel.SaveBookmarks(p, set); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, _ := os.ReadFile(p)
	got := string(data)

	i6, i8, i9 := strings.Index(got, "[6]"), strings.Index(got, "[8]"), strings.Index(got, "[9]")
	if i6 < 0 || i8 < 0 || i9 < 0 {
		t.Fatalf("missing sections, got:\n%s", got)
	}
	if i6 >= i8 || i8 >= i9 {
		t.Fatalf("sections must be written in ascending order, got:\n%s", got)
	}
}

func TestSaveBookmarks_AlphabeticalKeys(t *testing.T) {
	var set panel.BookmarkSet
	set[1] = panel.Bookmark{Path: "/home/user"}

	p := filepath.Join(t.TempDir(), "keys.ini")
	if err := panel.SaveBookmarks(p, set); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, _ := os.ReadFile(p)
	want := `[1]
Path=/home/user
Plugin=
PluginData=
PluginFile=
`
	if string(data) != want {
		t.Fatalf("key order/content mismatch:\nGOT:\n%s\nWANT:\n%s", data, want)
	}
}

func TestSaveBookmarks_TrailingNewlineAndBlankSeparator(t *testing.T) {
	var set panel.BookmarkSet
	set[0] = panel.Bookmark{Path: "/zero"}
	set[3] = panel.Bookmark{Path: "/three"}

	p := filepath.Join(t.TempDir(), "sep.ini")
	if err := panel.SaveBookmarks(p, set); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, _ := os.ReadFile(p)
	want := "[0]\nPath=/zero\nPlugin=\nPluginData=\nPluginFile=\n" +
		"\n" +
		"[3]\nPath=/three\nPlugin=\nPluginData=\nPluginFile=\n"
	if string(data) != want {
		t.Fatalf("byte layout mismatch:\nGOT:\n%q\nWANT:\n%q", data, want)
	}
	if !strings.HasSuffix(string(data), "\n") {
		t.Errorf("file must end with a newline")
	}
}

func TestBookmarks_RoundTrip(t *testing.T) {
	var in panel.BookmarkSet
	in[0] = panel.Bookmark{Path: "/home/sogonov/f4"}
	in[2] = panel.Bookmark{Path: "/mnt/d/!!wrkstk/данные"}
	in[7] = panel.Bookmark{
		Path:       "/some/mount",
		Plugin:     "NetRocks",
		PluginData: "sftp://host/dir",
		PluginFile: "file.txt",
	}

	p := filepath.Join(t.TempDir(), "rt.ini")
	if err := panel.SaveBookmarks(p, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := panel.LoadBookmarks(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round-trip mismatch:\ngot  %#v\nwant %#v", out, in)
	}
}

func TestBookmarks_SkipsForeignSections(t *testing.T) {
	p := writeTempBookmarks(t, `[SomeOther]
Path=/should/be/ignored

[3]
Path=/real/one
Plugin=
PluginData=
PluginFile=

[UserMenu/Whatever]
Label=nope
`)
	set, err := panel.LoadBookmarks(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if set[3].Path != "/real/one" {
		t.Fatalf("slot 3: got %q, want %q", set[3].Path, "/real/one")
	}
	for i := range set {
		if i == 3 {
			continue
		}
		if !set[i].IsEmpty() {
			t.Errorf("foreign section leaked into slot %d: %#v", i, set[i])
		}
	}
}

func TestBookmark_IsEmpty(t *testing.T) {
	cases := []struct {
		b    panel.Bookmark
		want bool
	}{
		{panel.Bookmark{}, true},
		{panel.Bookmark{Path: "x"}, false},
		{panel.Bookmark{Plugin: "x"}, false},
	}
	for _, c := range cases {
		if got := c.b.IsEmpty(); got != c.want {
			t.Errorf("IsEmpty(%#v) = %v, want %v", c.b, got, c.want)
		}
	}
}

func TestExpandPathEnv_BookmarkPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	const undefined = "F4_BOOKMARK_UNDEFINED_412"
	oldUndefined, hadUndefined := os.LookupEnv(undefined)
	if err := os.Unsetenv(undefined); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadUndefined {
			_ = os.Setenv(undefined, oldUndefined)
		} else {
			_ = os.Unsetenv(undefined)
		}
	})

	cases := []struct {
		name string
		path string
		want string
	}{
		{"dollar variable", filepath.Join("$HOME", "x"), filepath.Join(home, "x")},
		{"braced dollar variable", filepath.Join("${HOME}", "x"), filepath.Join(home, "x")},
		{"percent variable", filepath.Join("%USERPROFILE%", "x"), filepath.Join(home, "x")},
		{"tilde", "~", home},
		{"tilde subdirectory", filepath.Join("~", "sub"), filepath.Join(home, "sub")},
		{"literal", filepath.Join(home, "literal"), filepath.Join(home, "literal")},
		// Unknown variables stay literal. This prevents a typo such as
		// $HMOE/x from unexpectedly becoming the unrelated rooted path /x.
		{"undefined dollar variable", filepath.Join("$"+undefined, "x"), filepath.Join("$"+undefined, "x")},
		{"undefined percent variable", filepath.Join("%"+undefined+"%", "x"), filepath.Join("%"+undefined+"%", "x")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := panel.ExpandPathEnv(tc.path); got != tc.want {
				t.Errorf("expandPathEnv(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestBookmarks_RoundTripPreservesExpandablePath(t *testing.T) {
	var in panel.BookmarkSet
	in[4] = panel.Bookmark{Path: filepath.Join("$HOME", "portable")}
	p := filepath.Join(t.TempDir(), "bookmarks.ini")

	if err := panel.SaveBookmarks(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := panel.LoadBookmarks(p)
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("round trip expanded a stored bookmark:\ngot  %#v\nwant %#v", out, in)
	}
}

func TestTruncPathLeft(t *testing.T) {
	const long = "/mnt/d/!!wrkstk/reps/ssh/ski-analyzer"
	cases := []struct {
		path  string
		width int
		want  string
	}{
		{"/home/user", 40, "/home/user"}, // fits, untouched
		{"/home/user", 10, "/home/user"}, // exactly fits
		{long, 12, "…ki-analyzer"},       // ellipsis plus 11 cells of tail
		{"/a/b/c", 1, "…"},
		{"/a/b/c", 0, ""},
	}
	for _, c := range cases {
		if got := dialog.TruncPathLeft(c.path, c.width); got != c.want {
			t.Errorf("dialog.TruncPathLeft(%q, %d) = %q, want %q", c.path, c.width, got, c.want)
		}
	}
	// The tail is what identifies a path, so it must survive the cut.
	got := dialog.TruncPathLeft(long, 20)
	if !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "ski-analyzer") {
		t.Errorf("dialog.TruncPathLeft(%q, 20) = %q, want an ellipsis plus the tail", long, got)
	}
	if w := len([]rune(got)); w > 20 {
		t.Errorf("dialog.TruncPathLeft returned %d cells, want at most 20: %q", w, got)
	}
}

func TestBookmarksFilePath_HasExpectedSuffix(t *testing.T) {
	p := panel.BookmarksFilePath()
	want := filepath.Join("f4", "settings", "bookmarks.ini")
	if !strings.HasSuffix(p, want) {
		t.Errorf("BookmarksFilePath()=%q, want suffix %q", p, want)
	}
}
