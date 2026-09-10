package panel

import (
	"github.com/unxed/f4/internal/plughost"
	semantic "github.com/unxed/f4/internal/semantic"
	vfs "github.com/unxed/f4/vfs"
	testing "testing"
)

func TestSemanticStaticEntryRegistersBrokerSourceDescriptor(t *testing.T) {
	filesystem := newRegistrationMediaVFS(t)
	broker := installRegistrationRecorder(t)
	panel := &FileSystemPanel{Vfs: filesystem, Entries: []*FileEntry{{VFSItem: mediaRegistrationItem}}}
	panel.updateSemanticRevisions()
	static := panel.semanticStaticPanelData("vfs")
	if len(static.entries) != 1 || static.entries[0].Source == nil {
		t.Fatalf("semantic entries = %#v", static.entries)
	}
	if len(broker.registrations) != 1 || broker.registrations[0].Item.Revision != mediaRegistrationItem.Revision || broker.registrations[0].FS != filesystem {
		t.Fatal("wrong media registration")
	}
	source := static.entries[0].Source
	if source.ResourceID == "" || source.SourceKey == "" || source.Version != "sentinel-version" ||
		source.VersionStrength != "strong" || source.AccessProfile != "nativeRange" || source.StorageClass != "network" {
		t.Fatalf("semantic source = %#v", source)
	}
}

func TestDeferredSemanticStaticCatalogRegistersOnlyImageSources(t *testing.T) {
	previousCapability := semantic.SetPanelCatalogMetadataEnabled(true)
	t.Cleanup(func() { semantic.SetPanelCatalogMetadataEnabled(previousCapability) })

	filesystem := newRegistrationMediaVFS(t)
	broker := installRegistrationRecorder(t)

	panel := &FileSystemPanel{
		Vfs: filesystem,
		Entries: []*FileEntry{
			{VFSItem: vfs.VFSItem{Name: "image.jpg", Size: 10, SizeKnown: true}},
			{VFSItem: vfs.VFSItem{Name: "notes.txt", Size: 20, SizeKnown: true}},
			{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true}},
		},
	}
	panel.updateSemanticRevisions()
	static := panel.semanticStaticPanelData("vfs")
	if len(static.entries) != 3 {
		t.Fatalf("semantic entries = %#v", static.entries)
	}
	if static.entries[0].Source == nil || !static.entries[0].IsImage {
		t.Fatalf("image source was omitted: %#v", static.entries[0])
	}
	if static.entries[1].Source != nil || static.entries[1].IsImage {
		t.Fatalf("non-image source was registered: %#v", static.entries[1])
	}
	if static.entries[2].Source != nil {
		t.Fatalf("directory source was registered: %#v", static.entries[2])
	}
	if ids := broker.registrations; len(ids) != 1 {
		t.Fatalf("broker resources = %d, want one image resource", len(ids))
	}
}

var mediaRegistrationItem = vfs.VFSItem{Name: "image.jpg", Size: 10, SizeKnown: true, Revision: "opaque-revision"}

func newRegistrationMediaVFS(t *testing.T) vfs.VFS { return vfs.NewOSVFS(t.TempDir()) }

type registrationRecorder struct {
	registrations []plughost.MediaSourceRegistration
	committed     []string
}

func (r *registrationRecorder) Register(reg plughost.MediaSourceRegistration) plughost.ImageSourceDescriptor {
	r.registrations = append(r.registrations, reg)
	return plughost.ImageSourceDescriptor{ResourceID: "sentinel-resource", SourceKey: "sentinel-key", Version: "sentinel-version", VersionStrength: "strong", AccessProfile: "nativeRange", StorageClass: "network"}
}
func (r *registrationRecorder) CommitPanel(_ string, _ int64, ids []string) {
	r.committed = append([]string(nil), ids...)
}
func installRegistrationRecorder(t *testing.T) *registrationRecorder {
	old := currentPanelMediaRegistry
	r := &registrationRecorder{}
	currentPanelMediaRegistry = func() panelMediaRegistry { return r }
	t.Cleanup(func() { currentPanelMediaRegistry = old })
	return r
}
