package panel

import (
	"github.com/unxed/f4/vfs"
	"testing"
)

func TestMetadataFingerprintTracksOpaqueRevision(t *testing.T) {
	item := vfs.VFSItem{Name: "image.jpg", Size: 10, SizeKnown: true, Revision: "opaque-revision"}
	filesystem := vfs.NewOSVFS(t.TempDir())
	panel := &FileSystemPanel{Vfs: filesystem, Entries: []*FileEntry{{VFSItem: item}}}
	firstCatalog, firstMetadata, _ := panel.semanticFingerprints()
	panel.Entries[0].Revision = "opaque-revision-2"
	secondCatalog, secondMetadata, _ := panel.semanticFingerprints()
	if firstCatalog != secondCatalog {
		t.Fatal("VFSItem.Revision unexpectedly changed the base catalog fingerprint")
	}
	if firstMetadata == secondMetadata {
		t.Fatal("metadata fingerprint ignored VFSItem.Revision")
	}
}
