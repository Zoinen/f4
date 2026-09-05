package cloudfox

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

type visualPathTestFactory struct{ backend Backend }

func (*visualPathTestFactory) Provider() ProviderType    { return ProviderWebDAV }
func (*visualPathTestFactory) Validate(Connection) error { return nil }
func (f *visualPathTestFactory) Open(context.Context, Connection, SecretValues) (Backend, error) {
	return f.backend, nil
}

func TestNewPluginDoesNotMutateOrShareGoogleFactory(t *testing.T) {
	shared := &GoogleDriveFactory{}
	first := NewPlugin(Options{ConfigDir: t.TempDir(), Portable: true, Factories: []BackendFactory{shared}})
	second := NewPlugin(Options{ConfigDir: t.TempDir(), Portable: true, Factories: []BackendFactory{shared}})
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})

	if shared.TokenUpdate != nil {
		t.Fatal("NewPlugin mutated the caller-owned Google factory")
	}
	firstFactory, ok := first.Factory(ProviderGoogleDrive)
	if !ok {
		t.Fatal("first plugin did not register Google factory")
	}
	secondFactory, ok := second.Factory(ProviderGoogleDrive)
	if !ok {
		t.Fatal("second plugin did not register Google factory")
	}
	firstGoogle := firstFactory.(*GoogleDriveFactory)
	secondGoogle := secondFactory.(*GoogleDriveFactory)
	if firstGoogle == shared || secondGoogle == shared || firstGoogle == secondGoogle {
		t.Fatal("plugin instances share a mutable Google factory")
	}
	if firstGoogle.TokenUpdate == nil || secondGoogle.TokenUpdate == nil {
		t.Fatal("per-plugin token persistence callback was not installed")
	}
}

func TestPortablePluginKeepsExistingKeyringReferencesReadable(t *testing.T) {
	plugin := NewPlugin(Options{ConfigDir: t.TempDir(), Portable: true})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})

	if plugin.repo == nil || plugin.repo.Secrets.Keyring == nil {
		t.Fatal("portable plugin disabled the keyring needed by an existing keyring:v1 reference")
	}
	if !plugin.portable {
		t.Fatal("portable mode was not retained")
	}
}

func TestConnectionProviderReopensStandaloneVisualBookmark(t *testing.T) {
	backend := &visualTreeBackend{
		fakeBackend: &fakeBackend{},
		parents:     map[string]string{"/opaque/albums": "/root", "/opaque/year": "/opaque/albums"},
		names:       map[string]string{"/opaque/albums": "Albums", "/opaque/year": "2026"},
		directories: map[string][][]RemoteEntry{
			"/root":          {{{VFSItem: vfs.VFSItem{Name: "Albums", IsDir: true}, Location: "/opaque/albums"}}},
			"/opaque/albums": {{{VFSItem: vfs.VFSItem{Name: "2026", IsDir: true}, Location: "/opaque/year"}}},
		},
	}
	plugin := NewPlugin(Options{
		ConfigDir: t.TempDir(),
		Factories: []BackendFactory{&visualPathTestFactory{backend: backend}},
	})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	connection, err := plugin.repo.Save(context.Background(), Connection{
		Name: "My cloud", Provider: ProviderWebDAV,
		Settings: []byte(`{"base_url":"https://example.invalid/dav","auth":"anonymous"}`),
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if connection.ID == "" {
		t.Fatal("saved connection has no internal id")
	}
	separator := string(os.PathSeparator)
	bookmark := "My cloud:" + separator + "Albums" + separator + "2026"
	provider := &connectionProvider{plugin: plugin}
	if !provider.CanOpen(context.Background(), vfs.NewOSVFS(t.TempDir()), bookmark) {
		t.Fatalf("standalone provider did not recognize %q", bookmark)
	}
	opened, err := provider.Open(context.Background(), vfs.NewOSVFS(t.TempDir()), bookmark)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if got := opened.GetPath(); got != bookmark {
		t.Fatalf("reopened bookmark path = %q, want %q", got, bookmark)
	}
	if publicPath := opened.GetPath(); publicPath == "" || strings.Contains(publicPath, "cloud://") || strings.Contains(publicPath, connection.ID) {
		t.Fatalf("reopened bookmark exposed canonical URI %q", publicPath)
	}
}

func TestConnectionProviderReopensNestedVisualHistoryFromManager(t *testing.T) {
	backend := &visualTreeBackend{
		fakeBackend: &fakeBackend{},
		parents:     map[string]string{"/opaque/albums": "/root", "/opaque/year": "/opaque/albums"},
		names:       map[string]string{"/opaque/albums": "Albums", "/opaque/year": "2026"},
		directories: map[string][][]RemoteEntry{
			"/root":          {{{VFSItem: vfs.VFSItem{Name: "Albums", IsDir: true}, Location: "/opaque/albums"}}},
			"/opaque/albums": {{{VFSItem: vfs.VFSItem{Name: "2026", IsDir: true}, Location: "/opaque/year"}}},
		},
	}
	plugin := NewPlugin(Options{
		ConfigDir: t.TempDir(),
		Factories: []BackendFactory{&visualPathTestFactory{backend: backend}},
	})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	connection, err := plugin.repo.Save(context.Background(), Connection{
		Name: "My cloud", Provider: ProviderWebDAV,
		Settings: []byte(`{"base_url":"https://example.invalid/dav","auth":"anonymous"}`),
	}, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	manager := plugin.manager()
	separator := string(os.PathSeparator)
	historyPath := connection.Name + ":" + separator + "Albums" + separator + "2026"
	provider := &connectionProvider{plugin: plugin}
	if !provider.CanOpen(context.Background(), manager, historyPath) {
		t.Fatalf("provider rejected nested history path %q while manager was active", historyPath)
	}
	opened, err := provider.Open(context.Background(), manager, historyPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := opened.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if got := opened.GetPath(); got != historyPath {
		t.Fatalf("reopened manager history path = %q, want %q", got, historyPath)
	}
	if opened.ParentVFS() != manager {
		t.Fatal("nested history restore did not preserve the active CloudFox manager as parent")
	}
}

func TestConnectionProviderDoesNotRemountPathOwnedByCurrentCloudVFS(t *testing.T) {
	backend := &visualTreeBackend{
		fakeBackend: &fakeBackend{},
		parents:     map[string]string{"/opaque/albums": "/root", "/opaque/year": "/opaque/albums"},
		names:       map[string]string{"/opaque/albums": "Albums", "/opaque/year": "2026"},
	}
	cloud := testCloudVFS(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	current := cloud.publicPath("/opaque/year")
	if err := cloud.SetPath(current); err != nil {
		t.Fatal(err)
	}
	parent := cloud.Dir(current)
	provider := &connectionProvider{}
	if provider.CanOpen(context.Background(), cloud, parent) {
		t.Fatalf("standalone provider tried to remount current CloudVFS parent %q", parent)
	}
	if got := cloud.Base(current); got != "2026" {
		t.Fatalf("folder left by upward navigation = %q, want 2026", got)
	}
}
