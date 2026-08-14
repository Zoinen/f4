package cloudfox

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type fakeShareBackend struct {
	*fakeBackend
	info           vfs.ShareLinkInfo
	created        vfs.ShareLink
	infoLocation   string
	createLocation string
	revokeLocation string
	request        vfs.ShareLinkRequest
}

func (b *fakeShareBackend) ShareLinkInfo(_ context.Context, location string) (vfs.ShareLinkInfo, error) {
	b.infoLocation = location
	return b.info, nil
}

func (b *fakeShareBackend) CreateShareLink(_ context.Context, location string, request vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	b.createLocation = location
	b.request = request
	return b.created, nil
}

func (b *fakeShareBackend) RevokeShareLink(_ context.Context, location string) error {
	b.revokeLocation = location
	return nil
}

func TestCloudVFSShareLinkAdapterResolvesAliasesAndCopiesCapabilities(t *testing.T) {
	backend := &fakeShareBackend{
		fakeBackend: &fakeBackend{pages: [][]RemoteEntry{{{
			VFSItem: vfs.VFSItem{Name: "shown.txt"}, Location: "/root/opaque-id",
		}}}},
		info: vfs.ShareLinkInfo{
			Provider: "Test", ItemName: "shown.txt", Roles: []vfs.ShareRole{vfs.ShareRoleViewer},
			ExpirationOptions: []time.Duration{time.Hour}, CanCreate: true,
			Link: &vfs.ShareLink{URL: "https://share.example/item?token=opaque", Role: vfs.ShareRoleViewer, Revocable: true},
		},
		created: vfs.ShareLink{URL: "https://share.example/new?token=opaque", Role: vfs.ShareRoleViewer, ExpiresAt: time.Now().Add(time.Hour)},
	}
	cloud := testCloudVFS(t, backend)
	defer cloud.Close()
	if err := cloud.ReadDir(context.Background(), cloud.GetPath(), func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	itemPath := cloud.Join(cloud.GetPath(), "shown.txt")

	info, err := cloud.ShareLinkInfo(context.Background(), itemPath)
	if err != nil {
		t.Fatal(err)
	}
	if backend.infoLocation != "/root/opaque-id" {
		t.Fatalf("info location = %q", backend.infoLocation)
	}
	// The adapter must not expose provider-owned mutable slices or pointers.
	info.Roles[0] = vfs.ShareRoleEditor
	info.ExpirationOptions[0] = 24 * time.Hour
	info.Link.URL = "https://changed.invalid/"
	if backend.info.Roles[0] != vfs.ShareRoleViewer || backend.info.ExpirationOptions[0] != time.Hour || backend.info.Link.URL != "https://share.example/item?token=opaque" {
		t.Fatalf("adapter returned provider-owned sharing state: %#v", backend.info)
	}

	request := vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer, ExpiresIn: time.Hour}
	link, err := cloud.CreateShareLink(context.Background(), itemPath, request)
	if err != nil {
		t.Fatal(err)
	}
	if backend.createLocation != "/root/opaque-id" || !reflect.DeepEqual(backend.request, request) || link.URL != backend.created.URL {
		t.Fatalf("create forwarding = location %q request %#v link %#v", backend.createLocation, backend.request, link)
	}
	if err := cloud.RevokeShareLink(context.Background(), itemPath); err != nil {
		t.Fatal(err)
	}
	if backend.revokeLocation != "/root/opaque-id" {
		t.Fatalf("revoke location = %q", backend.revokeLocation)
	}
}

func TestCloudVFSShareLinkAdapterRejectsUnsafeURLs(t *testing.T) {
	unsafe := []string{
		"",
		"relative/path",
		"ftp://share.example/file",
		"https://user:password@share.example/file",
		"https://share.example/file\nX-Fake: value",
	}
	for _, raw := range unsafe {
		t.Run(raw, func(t *testing.T) {
			backend := &fakeShareBackend{
				fakeBackend: &fakeBackend{},
				info:        vfs.ShareLinkInfo{Link: &vfs.ShareLink{URL: raw}},
				created:     vfs.ShareLink{URL: raw},
			}
			cloud := testCloudVFS(t, backend)
			defer cloud.Close()
			if _, err := cloud.ShareLinkInfo(context.Background(), cloud.GetPath()); err == nil {
				t.Fatal("unsafe info URL was accepted")
			}
			if _, err := cloud.CreateShareLink(context.Background(), cloud.GetPath(), vfs.ShareLinkRequest{}); err == nil {
				t.Fatal("unsafe created URL was accepted")
			}
		})
	}
}

func TestCloudVFSShareLinkAdapterRejectsMismatchedMutationResultsAsUnknown(t *testing.T) {
	request := vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer, ExpiresIn: time.Hour}
	tests := map[string]vfs.ShareLink{
		"stronger role": {
			URL: "https://share.example/item", Role: vfs.ShareRoleEditor, ExpiresAt: time.Now().Add(time.Hour),
		},
		"missing expiration": {
			URL: "https://share.example/item", Role: vfs.ShareRoleViewer,
		},
		"excessive expiration": {
			URL: "https://share.example/item", Role: vfs.ShareRoleViewer, ExpiresAt: time.Now().Add(24 * time.Hour),
		},
	}
	for name, created := range tests {
		t.Run(name, func(t *testing.T) {
			backend := &fakeShareBackend{fakeBackend: &fakeBackend{}, created: created}
			cloud := testCloudVFS(t, backend)
			defer cloud.Close()
			_, err := cloud.CreateShareLink(context.Background(), cloud.GetPath(), request)
			if !errors.Is(err, vfs.ErrOperationStateUnknown) {
				t.Fatalf("CreateShareLink error = %v", err)
			}
		})
	}
}

func TestCloudVFSShareLinkAdapterReportsUnsupportedBackend(t *testing.T) {
	cloud := testCloudVFS(t, &fakeBackend{})
	defer cloud.Close()
	if _, err := cloud.ShareLinkInfo(context.Background(), cloud.GetPath()); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Fatalf("ShareLinkInfo error = %v", err)
	}
	if _, err := cloud.CreateShareLink(context.Background(), cloud.GetPath(), vfs.ShareLinkRequest{}); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Fatalf("CreateShareLink error = %v", err)
	}
	if err := cloud.RevokeShareLink(context.Background(), cloud.GetPath()); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Fatalf("RevokeShareLink error = %v", err)
	}
}

var _ BackendShareLinker = (*fakeShareBackend)(nil)
