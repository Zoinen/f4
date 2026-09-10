package fileops

import (
	"testing"
)

func TestPersistentURIRecognitionDoesNotRequireLoadedPlugin(t *testing.T) {
	if !IsPersistentURIPath("temporarily-unavailable://profile/path") {
		t.Fatal("valid unloaded-plugin URI was treated as a local path")
	}
	if IsPersistentURIPath("ordinary/relative/path") {
		t.Fatal("relative path was treated as a persistent URI")
	}
}
