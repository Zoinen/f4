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
	directories   []plughost.MediaSourceRegistration
	directoryIDs  []string
}

func (r *registrationRecorder) RegisterDirectory(reg plughost.MediaSourceRegistration) plughost.DirectorySourceDescriptor {
	r.directories = append(r.directories, reg)
	return plughost.DirectorySourceDescriptor{ResourceID: "directory-resource", SourceKey: "directory-key", Version: "observation"}
}
func (r *registrationRecorder) CommitDirectoryPanel(_ string, _ int64, ids []string) {
	r.directoryIDs = append([]string(nil), ids...)
}

func TestPagedDirectoryPreviewAuthorityIsNegotiated(t *testing.T) {
	old := semantic.DirectoryPreviewsEnabled.Load()
	t.Cleanup(func() { semantic.DirectoryPreviewsEnabled.Store(old) })
	filesystem := newRegistrationMediaVFS(t)
	broker := installRegistrationRecorder(t)
	p := &FileSystemPanel{Vfs: filesystem, Entries: []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "album", IsDir: true}},
	}}
	p.updateSemanticRevisions()
	semantic.DirectoryPreviewsEnabled.Store(false)
	rows, _, ok := p.semanticPagedRows(0, 2)
	if !ok || rows[1].DirectorySource != nil {
		t.Fatal("older peer received directory authority")
	}
	semantic.DirectoryPreviewsEnabled.Store(true)
	rows, _, ok = p.semanticPagedRows(0, 2)
	if !ok || rows[0].DirectorySource != nil || rows[1].DirectorySource == nil {
		t.Fatalf("directory rows: %#v", rows)
	}
	if len(broker.directories) != 1 || broker.directories[0].FS != filesystem || len(broker.directoryIDs) != 1 {
		t.Fatal("directory authority was not committed")
	}
	if rows[1].Source != nil || rows[1].MinimalToMap()["directorySource"] == nil {
		t.Fatal("directory authority confused with image source")
	}
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
