package archive

import (
	"context"
	"path/filepath"
	"testing"
)

func TestArchiveAutoQueueContext(t *testing.T) {
	if autoQueueRequested(context.WithValue(context.Background(), autoQueueContextKey{}, false)) || autoQueueRequested(context.Background()) {
		t.Fatal("ordinary contexts must not request automatic queueing")
	}
	if !autoQueueRequested(WithAutoQueue(context.Background())) {
		t.Fatal("WithAutoQueue context was not recognised")
	}
}

func TestArchiveRootPathCleaning(t *testing.T) {
	if got := cleanArchiveRootPath(filepath.Join("tmp", "bundle.zip")); got != filepath.Clean(filepath.Join("tmp", "bundle.zip")) {
		t.Fatalf("local root = %q", got)
	}
	if got := cleanArchiveRootPath("sftp://host/path/archive.zip///"); got != "sftp://host/path/archive.zip" {
		t.Fatalf("URI root = %q", got)
	}
}

func TestArchiveRelativePathLocalBoundaries(t *testing.T) {
	root := filepath.Join("tmp", "bundle.zip")
	inside := filepath.Join(root, "folder", "file.txt")
	if got, ok := archiveRelativePath(inside, root); !ok || got != filepath.ToSlash(filepath.Join("folder", "file.txt")) {
		t.Fatalf("inside relative path = %q, %v", got, ok)
	}
	if got, ok := archiveRelativePath(filepath.Join("tmp", "other.zip"), root); ok || got != "" {
		t.Fatalf("outside relative path = %q, %v", got, ok)
	}
}

func TestArchiveRelativePathURIChildBoundary(t *testing.T) {
	root := "sftp://host/archive.zip"
	if got, ok := archiveRelativePath(root, root); !ok || got != "." {
		t.Fatalf("URI root relative path = %q, %v", got, ok)
	}
	if got, ok := archiveRelativePath(root+"/dir/file", root); !ok || got != "dir/file" {
		t.Fatalf("URI child relative path = %q, %v", got, ok)
	}
	if _, ok := archiveRelativePath(root+"-copy/file", root); ok {
		t.Fatal("URI prefix without a separator must not be owned")
	}
}

func TestArchiveInnerPathCleaning(t *testing.T) {
	cases := []struct {
		input string
		want  string
		ok    bool
	}{
		{input: "", want: ".", ok: true},
		{input: `folder\\file.txt`, want: "folder/file.txt", ok: true},
		{input: `/folder/file.txt`, want: "folder/file.txt", ok: true},
		{input: `../outside`, ok: false},
	}
	for _, tc := range cases {
		got, err := cleanArchiveInnerPath(tc.input)
		if (err == nil) != tc.ok || tc.ok && got != tc.want {
			t.Errorf("cleanArchiveInnerPath(%q) = %q, %v", tc.input, got, err)
		}
	}
}

func TestArchiveExtractionPathCleaningRejectsEscapes(t *testing.T) {
	valid, err := cleanArchiveExtractionPath(`folder\\file.txt`)
	if err != nil || valid != "folder/file.txt" {
		t.Fatalf("valid extraction path = %q, %v", valid, err)
	}
	for _, input := range []string{"/absolute", `C:\\absolute`, "../outside", "folder/../../outside", "bad\x00name"} {
		if _, err := cleanArchiveExtractionPath(input); err == nil {
			t.Errorf("cleanArchiveExtractionPath(%q) accepted an unsafe path", input)
		}
	}
}

func TestArchiveExtractionSelection(t *testing.T) {
	selected := map[string]bool{"folder": true, "exact.txt": true}
	for name, want := range map[string]bool{
		"folder/file.txt": true,
		"exact.txt":       true,
		"other.txt":       false,
		"folderish/file":  false,
	} {
		if got := archiveExtractionPathSelected(name, selected); got != want {
			t.Errorf("selection for %q = %v, want %v", name, got, want)
		}
	}
	if !archiveExtractionPathSelected("anything", map[string]bool{".": true}) {
		t.Fatal("dot selection must include every entry")
	}
}

func TestArchiveExtractionRelativePath(t *testing.T) {
	if got, err := archiveExtractionRelativePath("folder/file.txt", "."); err != nil || got != "folder/file.txt" {
		t.Fatalf("root extraction path = %q, %v", got, err)
	}
	if got, err := archiveExtractionRelativePath("folder/file.txt", "folder"); err != nil || got != "file.txt" {
		t.Fatalf("nested extraction path = %q, %v", got, err)
	}
	if got, err := archiveExtractionRelativePath("folder", "folder"); err != nil || got != "." {
		t.Fatalf("directory extraction path = %q, %v", got, err)
	}
	if _, err := archiveExtractionRelativePath("other/file.txt", "folder"); err == nil {
		t.Fatal("entry outside selected folder must be rejected")
	}
}

func TestArchivePathJoiningVariants(t *testing.T) {
	if got := archivePathJoin(filepath.Join("tmp", "bundle.zip"), "folder/file.txt"); got != filepath.Join("tmp", "bundle.zip", "folder", "file.txt") {
		t.Fatalf("local archivePathJoin = %q", got)
	}
	if got := archivePathJoin("sftp://host/bundle.zip/", `folder\\file.txt`); got != "sftp://host/bundle.zip/folder/file.txt" {
		t.Fatalf("URI archivePathJoin = %q", got)
	}
	if got := archivePathJoin("root", "."); got != "root" {
		t.Fatalf("empty archivePathJoin = %q", got)
	}
}

func TestArchiveVFSPathAPI(t *testing.T) {
	root := filepath.Join("tmp", "bundle.zip")
	v := &ArchiveVFS{arcPath: root, innerPath: "folder"}
	if v.IsAtRoot() {
		t.Fatal("folder view must not be at root")
	}
	path := filepath.Join(root, "folder", "file.txt")
	if !v.IsAbs(path) || v.Base(path) != "file.txt" || v.Dir(path) != filepath.Join(root, "folder") {
		t.Fatalf("path API mismatch for %q", path)
	}
	if got, err := v.Abs("child.txt"); err != nil || got != filepath.Join(root, "folder", "child.txt") {
		t.Fatalf("relative Abs = %q, %v", got, err)
	}
	if got := v.Join(v.GetPath(), "child.txt"); got != filepath.Join(root, "folder", "child.txt") {
		t.Fatalf("Join = %q", got)
	}
	if _, err := v.Abs("sftp://other/path"); err == nil {
		t.Fatal("foreign URI must not be accepted")
	}
}
