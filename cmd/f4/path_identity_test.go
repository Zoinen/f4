package main

import "testing"

func TestSameFolderHistoryPathKeepsURIPathOpaque(t *testing.T) {
	if !sameFolderHistoryPath("CLOUD://ABC/folder/%2E%2E", "cloud://abc/folder/%2E%2E/") {
		t.Fatal("scheme/authority case and one trailing slash should normalize")
	}
	if sameFolderHistoryPath("cloud://abc/folder/%2E%2E", "cloud://abc/") {
		t.Fatal("escaped dot segments were cleaned as filesystem paths")
	}
	if sameFolderHistoryPath("cloud://abc/Folder", "cloud://abc/folder") {
		t.Fatal("URI path identity must remain case-sensitive")
	}
}

func TestPersistentURIRecognitionDoesNotRequireLoadedPlugin(t *testing.T) {
	if !isPersistentURIPath("temporarily-unavailable://profile/path") {
		t.Fatal("valid unloaded-plugin URI was treated as a local path")
	}
	if isPersistentURIPath("ordinary/relative/path") {
		t.Fatal("relative path was treated as a persistent URI")
	}
}
