package app

import (
	"github.com/unxed/f4/internal/fileops"
	"testing"
)

// The subject is fileops.SameFolderHistoryPath, which is still here. It asks the
// same question internal/fileops answers for paths, from the panel side.

func TestSameFolderHistoryPathKeepsURIPathOpaque(t *testing.T) {
	if !fileops.SameFolderHistoryPath("CLOUD://ABC/folder/%2E%2E", "cloud://abc/folder/%2E%2E/") {
		t.Fatal("scheme/authority case and one trailing slash should normalize")
	}
	if fileops.SameFolderHistoryPath("cloud://abc/folder/%2E%2E", "cloud://abc/") {
		t.Fatal("escaped dot segments were cleaned as filesystem paths")
	}
	if fileops.SameFolderHistoryPath("cloud://abc/Folder", "cloud://abc/folder") {
		t.Fatal("URI path identity must remain case-sensitive")
	}
}
