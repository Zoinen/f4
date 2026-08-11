package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

type transferNameOSVFS struct{ *vfs.OSVFS }

func (s *transferNameOSVFS) TransferName(srcPath string, _ vfs.VFS) string {
	switch s.Base(srcPath) {
	case "document.gdoc [a1b2]":
		return "document.docx"
	case "nested [c3d4]":
		return "nested"
	case "sheet.gsheet [e5f6]":
		return "sheet.xlsx"
	default:
		return ""
	}
}

type unsafeTransferNameOSVFS struct {
	*vfs.OSVFS
	name string
}

func (s *unsafeTransferNameOSVFS) TransferName(string, vfs.VFS) string { return s.name }

func TestTransferItemNameFallsBackForInvalidProviderPath(t *testing.T) {
	source := &transferNameOSVFS{OSVFS: vfs.NewOSVFS(t.TempDir())}
	destination := vfs.NewOSVFS(t.TempDir())
	if got := transferItemName(source, source.Join(source.GetPath(), "ordinary.txt"), destination, "ordinary.txt"); got != "ordinary.txt" {
		t.Fatalf("fallback name = %q", got)
	}
}

func TestTransferItemNameNeverReturnsAPath(t *testing.T) {
	destination := vfs.NewOSVFS(t.TempDir())
	for _, tc := range []struct {
		provider string
		fallback string
	}{
		{provider: "../escape", fallback: "../also-unsafe"},
		{provider: `nested\\escape`, fallback: `nested\\fallback`},
		{provider: ".", fallback: ".."},
	} {
		source := &unsafeTransferNameOSVFS{OSVFS: vfs.NewOSVFS(t.TempDir()), name: tc.provider}
		got := transferItemName(source, "ignored", destination, tc.fallback)
		if !isSafeTransferItemName(destination, got) {
			t.Fatalf("provider=%q fallback=%q produced unsafe name %q", tc.provider, tc.fallback, got)
		}
	}
}

func TestTransferItemNameSanitizesPortableInvalidNames(t *testing.T) {
	source := vfs.NewOSVFS(t.TempDir())
	destination := vfs.NewOSVFS(t.TempDir())
	for _, name := range []string{"bad<name", "bad?name", "CON", "CON.txt", "trailing.", "trailing "} {
		got := transferItemName(source, source.Join(source.GetPath(), name), destination, name)
		if got == name {
			t.Errorf("unsafe name %q was not changed", name)
		}
		if !isSafeTransferItemName(destination, got) {
			t.Errorf("unsafe name %q produced invalid destination name %q", name, got)
		}
	}
}

func TestBulkCopyRequiresIdentityAfterNameSanitization(t *testing.T) {
	source := vfs.NewOSVFS(t.TempDir())
	destination := vfs.NewOSVFS(t.TempDir())
	if transferNamesAreIdentity(source, destination, source.GetPath(), []string{"CON"}) {
		t.Fatal("bulk copy was allowed to bypass destination-name sanitization")
	}
	if !transferNamesAreIdentity(source, destination, source.GetPath(), []string{"ordinary.txt"}) {
		t.Fatal("ordinary identity transfer name unexpectedly disabled bulk copy")
	}
}

func TestRecursiveCopyAppliesTransferNamesAtEveryDepth(t *testing.T) {
	sourceRoot := t.TempDir()
	destinationRoot := t.TempDir()
	sourceDir := filepath.Join(sourceRoot, "root")
	if err := os.MkdirAll(filepath.Join(sourceDir, "nested [c3d4]"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "document.gdoc [a1b2]"), []byte("doc"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "nested [c3d4]", "sheet.gsheet [e5f6]"), []byte("sheet"), 0o644); err != nil {
		t.Fatal(err)
	}

	source := &transferNameOSVFS{OSVFS: vfs.NewOSVFS(sourceRoot)}
	destination := vfs.NewOSVFS(destinationRoot)
	target := filepath.Join(destinationRoot, "copied")
	if err := recursiveCopy(context.Background(), source, sourceDir, destination, target, &FileOpState{}, 0); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(target, "document.docx"),
		filepath.Join(target, "nested", "sheet.xlsx"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("mapped destination %q missing: %v", path, err)
		}
	}
}
