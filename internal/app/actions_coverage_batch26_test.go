package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestFileListedAsFindsExactRegularFileCoverageBatch26(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	if err := os.WriteFile(filepath.Join(dir, "entry.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !fileListedAs(context.Background(), fs, dir, "entry.txt") {
		t.Fatal("fileListedAs did not find the exact regular file")
	}
}

func TestFileListedAsRejectsDirectoryCoverageBatch26(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	if err := os.Mkdir(filepath.Join(dir, "folder"), 0o700); err != nil {
		t.Fatal(err)
	}
	if fileListedAs(context.Background(), fs, dir, "folder") {
		t.Fatal("fileListedAs treated a directory as a file")
	}
}

func TestFileListedAsIsCaseSensitiveCoverageBatch26(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	if err := os.WriteFile(filepath.Join(dir, "MixedName"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fileListedAs(context.Background(), fs, dir, "mixedname") {
		t.Fatal("fileListedAs matched a differently cased name")
	}
}

func TestFileListedAsRejectsMissingEntryCoverageBatch26(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	if fileListedAs(context.Background(), fs, dir, "missing") {
		t.Fatal("fileListedAs reported a missing entry")
	}
}

func TestFileListedAsRejectsMissingDirectoryCoverageBatch26(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	if fileListedAs(context.Background(), fs, filepath.Join(dir, "missing"), "entry.txt") {
		t.Fatal("fileListedAs reported an entry from a missing directory")
	}
}

func TestImportFar2lSettingsReportsNoCompatibleFilesCoverageBatch26(t *testing.T) {
	source := t.TempDir()
	_, err := importFar2lSettings(source, []far2lSettingFile{{name: "missing.ini", target: filepath.Join(t.TempDir(), "out.ini")}})
	if err == nil || !strings.Contains(err.Error(), "no compatible far2l settings") {
		t.Fatalf("importFar2lSettings(empty) error = %v", err)
	}
}

func TestImportFar2lSettingsSkipsMissingFilesCoverageBatch26(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "bookmarks.ini")
	if err := os.WriteFile(filepath.Join(source, "bookmarks.ini"), []byte("bookmarks"), 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := importFar2lSettings(source, []far2lSettingFile{
		{name: "missing.ini", target: filepath.Join(t.TempDir(), "missing.out")},
		{name: "bookmarks.ini", target: target},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != 1 || imported[0] != "bookmarks.ini" {
		t.Fatalf("imported = %v, want [bookmarks.ini]", imported)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "bookmarks" {
		t.Fatalf("target data = %q, want bookmarks", data)
	}
}

func TestImportFar2lSettingsPreservesOrderAndReplacesTargetsCoverageBatch26(t *testing.T) {
	source := t.TempDir()
	targetDir := t.TempDir()
	files := []far2lSettingFile{
		{name: "first.ini", target: filepath.Join(targetDir, "first.ini")},
		{name: "second.ini", target: filepath.Join(targetDir, "second.ini")},
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(source, file.name), []byte("new-"+file.name), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file.target, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	imported, err := importFar2lSettings(source, files)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(imported, ",") != "first.ini,second.ini" {
		t.Fatalf("imported = %v, want source order", imported)
	}
	for _, file := range files {
		data, err := os.ReadFile(file.target)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "new-"+file.name {
			t.Fatalf("%s was not replaced: %q", file.name, data)
		}
	}
}

func TestImportFar2lSettingsReportsReadErrorCoverageBatch26(t *testing.T) {
	source := t.TempDir()
	if err := os.Mkdir(filepath.Join(source, "broken.ini"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := importFar2lSettings(source, []far2lSettingFile{{name: "broken.ini", target: filepath.Join(t.TempDir(), "out.ini")}})
	if err == nil || !strings.Contains(err.Error(), "read broken.ini") {
		t.Fatalf("importFar2lSettings(read error) = %v", err)
	}
}

func TestImportFar2lSettingsCopiesEmptyFileCoverageBatch26(t *testing.T) {
	source := t.TempDir()
	target := filepath.Join(t.TempDir(), "empty.ini")
	if err := os.WriteFile(filepath.Join(source, "empty.ini"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	imported, err := importFar2lSettings(source, []far2lSettingFile{{name: "empty.ini", target: target}})
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != 1 || imported[0] != "empty.ini" {
		t.Fatalf("imported = %v, want [empty.ini]", imported)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("empty target has %d bytes", len(data))
	}
}
