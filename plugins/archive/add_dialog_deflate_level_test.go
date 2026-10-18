package archive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	zipperarchive "github.com/unxed/zipper/archive"
)

func addArchiveLevelIndex(t *testing.T, level int) int {
	t.Helper()
	for i, l := range addArchiveLevels {
		if l.Level == level {
			return i
		}
	}
	t.Fatalf("no level %d offered", level)
	return -1
}

// zipSizeAt packs src into a zip at the given level of the window and returns
// the archive's size.
func zipSizeAt(t *testing.T, src string, level int) int64 {
	t.Helper()
	s := addArchiveSettings{Name: "x.zip", Format: 0, Method: 0, Level: addArchiveLevelIndex(t, level)}
	out := filepath.Join(t.TempDir(), "x.zip")
	a, err := zipperarchive.NewArchiver(out, filepath.Dir(src), s.options())
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Archive(context.Background(), map[string]os.FileInfo{src: info}); err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	return st.Size()
}

// Fastest Deflate keeps the matches of a text: it comes out somewhat larger
// than Normal, not at twice its size (f4#2002).
func TestZipDeflateFastestCompressesText(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 8000; i++ {
		fmt.Fprintf(&b, "func handler%d(ctx context.Context, item *Item) error { return item.Save(ctx, %d) }\n", i, i%97)
	}
	text := []byte(b.String())
	src := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(src, text, 0o600); err != nil {
		t.Fatal(err)
	}

	fastest, normal := zipSizeAt(t, src, 1), zipSizeAt(t, src, 5)
	t.Logf("%d bytes: Fastest %d, Normal %d", len(text), fastest, normal)
	if fastest > normal*3/2 {
		t.Fatalf("Fastest packed %d bytes of text into %d, Normal into %d: the matches were dropped", len(text), fastest, normal)
	}
}
