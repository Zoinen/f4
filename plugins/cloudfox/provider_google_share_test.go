package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"

	"github.com/unxed/f4/vfs"
)

const googleShareTestURL = "https://drive.google.com/file/d/file-id/view?resourcekey=test-resource-key"

func newGoogleShareTestBackend(t *testing.T, handler http.HandlerFunc) *googleDriveBackend {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return &googleDriveBackend{
		service: service,
		items:   make(map[string]*drive.File), parents: make(map[string]string),
		names: make(map[string]string), transferNames: make(map[string]string),
	}
}

func writeGoogleShareJSON(t *testing.T, writer http.ResponseWriter, raw string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(writer, raw); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func googleShareMetadata(canShare bool) string {
	return `{"id":"file-id","name":"Report.txt","mimeType":"text/plain","webViewLink":"` + googleShareTestURL + `","resourceKey":"test-resource-key","capabilities":{"canShare":` + map[bool]string{true: "true", false: "false"}[canShare] + `}}`
}

func assertGoogleShareReadQuery(t *testing.T, request *http.Request) {
	t.Helper()
	if got := request.URL.Query().Get("supportsAllDrives"); got != "true" {
		t.Errorf("supportsAllDrives = %q, want true", got)
	}
}

func assertGoogleShareResourceKey(t *testing.T, request *http.Request, fileID, resourceKey string) {
	t.Helper()
	want := fileID + "/" + resourceKey
	if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != want {
		t.Errorf("resource-key header = %q, want %q", got, want)
	}
}

func TestGoogleShareLinkInfoUsesFreshMetadataAndPaginatesPermissions(t *testing.T) {
	var permissionPages atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/files/file-id":
			assertGoogleShareReadQuery(t, request)
			if fields := request.URL.Query().Get("fields"); fields != googleShareFileFields {
				t.Errorf("file fields = %q, want %q", fields, googleShareFileFields)
			}
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case "/files/file-id/permissions":
			assertGoogleShareReadQuery(t, request)
			assertGoogleShareResourceKey(t, request, "file-id", "test-resource-key")
			if request.Method != http.MethodGet {
				t.Errorf("method = %s, want GET", request.Method)
			}
			if fields := request.URL.Query().Get("fields"); fields != googleSharePermissionList {
				t.Errorf("permission fields = %q, want %q", fields, googleSharePermissionList)
			}
			if size := request.URL.Query().Get("pageSize"); size != "100" {
				t.Errorf("pageSize = %q, want 100", size)
			}
			page := permissionPages.Add(1)
			if page == 1 {
				if token := request.URL.Query().Get("pageToken"); token != "" {
					t.Errorf("first page token = %q", token)
				}
				writeGoogleShareJSON(t, writer, `{"nextPageToken":"next","permissions":[{"id":"user","type":"user","role":"reader"}]}`)
				return
			}
			if token := request.URL.Query().Get("pageToken"); token != "next" {
				t.Errorf("second page token = %q, want next", token)
			}
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"anyone-id","type":"anyone","role":"reader","allowFileDiscovery":false,"permissionDetails":[{"permissionType":"file","role":"reader","inherited":false}]}]}`)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	})
	backend.items[googleItemLocation("", "file-id")] = &drive.File{
		Id: "file-id", Name: "stale.txt", WebViewLink: "https://stale.invalid/", Capabilities: &drive.FileCapabilities{CanShare: false},
	}

	info, err := backend.ShareLinkInfo(context.Background(), googleItemLocation("", "file-id"))
	if err != nil {
		t.Fatal(err)
	}
	if permissionPages.Load() != 2 {
		t.Fatalf("permission pages = %d, want 2", permissionPages.Load())
	}
	if info.Provider != "Google Drive" || info.ItemName != "Report.txt" || !info.CanCreate || !info.CanRevoke {
		t.Fatalf("info = %#v", info)
	}
	if !reflect.DeepEqual(info.Roles, googleShareRoles) || !reflect.DeepEqual(info.ExpirationOptions, []time.Duration{0}) {
		t.Fatalf("roles/expiration = %#v / %#v", info.Roles, info.ExpirationOptions)
	}
	if info.Link == nil || info.Link.URL != googleShareTestURL || info.Link.Role != vfs.ShareRoleViewer || !info.Link.Revocable || !info.Link.ExpiresAt.IsZero() {
		t.Fatalf("link = %#v", info.Link)
	}
}

func TestGoogleShareLinkInfoRepresentsInheritedDiscoverableAccess(t *testing.T) {
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"anyone-id","type":"anyone","role":"writer","allowFileDiscovery":true,"permissionDetails":[{"permissionType":"file","inheritedFrom":"parent-id","role":"writer","inherited":true}]}]}`)
		default:
			http.NotFound(writer, request)
		}
	})

	info, err := backend.ShareLinkInfo(context.Background(), googleItemLocation("drive-id", "file-id"))
	if err != nil {
		t.Fatal(err)
	}
	if info.CanCreate || info.CanRevoke || info.Link == nil || info.Link.Role != vfs.ShareRoleEditor || info.Link.Revocable {
		t.Fatalf("info = %#v", info)
	}
	if !reflect.DeepEqual(info.Roles, []vfs.ShareRole{vfs.ShareRoleEditor}) {
		t.Fatalf("roles = %#v, want inherited floor at Editor", info.Roles)
	}
	if !strings.Contains(info.Notice, "inherited") || !strings.Contains(info.Notice, "discovered") {
		t.Fatalf("notice = %q", info.Notice)
	}
	if strings.Contains(info.Notice, "creating a link will change it") || !strings.Contains(info.Notice, "parent") {
		t.Fatalf("inherited discovery notice promises a local conversion: %q", info.Notice)
	}
}

func TestGoogleShareLinkInfoDoesNotHidePublishedWorkspaceView(t *testing.T) {
	var deletes atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"published-id","type":"anyone","role":"reader","view":"published"}]}`)
		case request.Method == http.MethodDelete:
			deletes.Add(1)
			http.Error(writer, "unexpected mutation", http.StatusInternalServerError)
		default:
			http.Error(writer, "unexpected request", http.StatusBadRequest)
		}
	})

	location := googleItemLocation("", "file-id")
	info, err := backend.ShareLinkInfo(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if info.Link != nil || info.CanRevoke || !info.CanCreate || info.LinksUnenumerable || !info.UnmanagedPublicAccess {
		t.Fatalf("published view info = %#v", info)
	}
	if !strings.Contains(strings.ToLower(info.Notice), "published") || !strings.Contains(strings.ToLower(info.Notice), "google editor") {
		t.Fatalf("published view notice = %q", info.Notice)
	}
	if err := backend.RevokeShareLink(context.Background(), location); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Fatalf("revoke published view error = %v, want unsupported", err)
	}
	if deletes.Load() != 0 {
		t.Fatalf("published view triggered %d permission deletes", deletes.Load())
	}
}

func TestGoogleShareLinkInfoRejectsUnknownPublicPermissionView(t *testing.T) {
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"future-id","type":"anyone","role":"reader","view":"future-public-view"}]}`)
		default:
			http.NotFound(writer, request)
		}
	})

	_, err := backend.ShareLinkInfo(context.Background(), googleItemLocation("", "file-id"))
	if err == nil || !strings.Contains(err.Error(), "unsupported public permission view") {
		t.Fatalf("unknown public permission view error = %v", err)
	}
}

func TestGoogleCreateShareLinkCreatesExplicitLinkOnlyPermission(t *testing.T) {
	var mutations atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[]}`)
		case request.Method == http.MethodPost && request.URL.Path == "/files/file-id/permissions":
			mutations.Add(1)
			assertGoogleShareReadQuery(t, request)
			assertGoogleShareResourceKey(t, request, "file-id", "test-resource-key")
			if _, exists := request.URL.Query()["sendNotificationEmail"]; exists {
				t.Error("sendNotificationEmail must be omitted for anyone permission")
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body["type"] != "anyone" || body["role"] != "reader" {
				t.Errorf("permission body = %#v", body)
			}
			value, exists := body["allowFileDiscovery"]
			if !exists || value != false {
				t.Errorf("allowFileDiscovery = %#v, present=%v", value, exists)
			}
			writeGoogleShareJSON(t, writer, `{"id":"anyone-id","type":"anyone","role":"reader","allowFileDiscovery":false}`)
		default:
			http.Error(writer, "unexpected request", http.StatusBadRequest)
		}
	})

	link, err := backend.CreateShareLink(context.Background(), googleItemLocation("", "file-id"), vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	if mutations.Load() != 1 || link.URL != googleShareTestURL || link.Role != vfs.ShareRoleViewer || !link.Revocable || !link.ExpiresAt.IsZero() {
		t.Fatalf("mutations/link = %d / %#v", mutations.Load(), link)
	}
}

func TestGoogleCreateShareLinkUpdatesDirectPermissionWithoutChangingImmutableType(t *testing.T) {
	var posts, patches atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			// Direct My Drive grants can omit permissionDetails. They must still
			// be updated rather than duplicated.
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"anyone-id","type":"anyone","role":"commenter","allowFileDiscovery":true}]}`)
		case request.Method == http.MethodPatch && request.URL.Path == "/files/file-id/permissions/anyone-id":
			patches.Add(1)
			assertGoogleShareReadQuery(t, request)
			assertGoogleShareResourceKey(t, request, "file-id", "test-resource-key")
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if _, exists := body["type"]; exists {
				t.Errorf("immutable permission type was sent: %#v", body)
			}
			if body["role"] != "writer" || body["allowFileDiscovery"] != false {
				t.Errorf("permission patch = %#v", body)
			}
			writeGoogleShareJSON(t, writer, `{"id":"anyone-id","type":"anyone","role":"writer","allowFileDiscovery":false}`)
		case request.Method == http.MethodPost:
			posts.Add(1)
			http.Error(writer, "unexpected POST", http.StatusBadRequest)
		default:
			http.Error(writer, "unexpected request", http.StatusBadRequest)
		}
	})

	link, err := backend.CreateShareLink(context.Background(), googleItemLocation("", "file-id"), vfs.ShareLinkRequest{Role: vfs.ShareRoleEditor})
	if err != nil {
		t.Fatal(err)
	}
	if posts.Load() != 0 || patches.Load() != 1 || link.Role != vfs.ShareRoleEditor || !link.Revocable {
		t.Fatalf("posts/patches/link = %d/%d/%#v", posts.Load(), patches.Load(), link)
	}
}

func TestGoogleCreateShareLinkHonorsInheritedFloor(t *testing.T) {
	tests := []struct {
		name           string
		request        vfs.ShareRole
		wantRole       vfs.ShareRole
		wantMutation   bool
		wantPermission bool
	}{
		{name: "cannot reduce", request: vfs.ShareRoleViewer, wantPermission: true},
		{name: "equal is no-op", request: vfs.ShareRoleCommenter, wantRole: vfs.ShareRoleCommenter},
		{name: "increase creates direct grant", request: vfs.ShareRoleEditor, wantRole: vfs.ShareRoleEditor, wantMutation: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var mutations atomic.Int32
			backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
				switch {
				case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
					writeGoogleShareJSON(t, writer, googleShareMetadata(true))
				case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
					writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"anyone-id","type":"anyone","role":"commenter","permissionDetails":[{"permissionType":"file","inheritedFrom":"parent-id","role":"commenter","inherited":true}]}]}`)
				case request.Method == http.MethodPost && request.URL.Path == "/files/file-id/permissions":
					mutations.Add(1)
					writeGoogleShareJSON(t, writer, `{"id":"direct-id","type":"anyone","role":"writer"}`)
				default:
					http.Error(writer, "unexpected request", http.StatusBadRequest)
				}
			})
			link, err := backend.CreateShareLink(context.Background(), googleItemLocation("drive-id", "file-id"), vfs.ShareLinkRequest{Role: test.request})
			if test.wantPermission {
				if !errors.Is(err, os.ErrPermission) {
					t.Fatalf("error = %v, want permission", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if link.Role != test.wantRole || link.Revocable {
				t.Fatalf("link = %#v", link)
			}
			if got := mutations.Load() != 0; got != test.wantMutation {
				t.Fatalf("mutation = %v, want %v", got, test.wantMutation)
			}
		})
	}
}

func TestGoogleCreateShareLinkValidatesRequestBeforeNetwork(t *testing.T) {
	var requests atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Error(writer, "unexpected", http.StatusInternalServerError)
	})
	location := googleItemLocation("", "file-id")
	if _, err := backend.CreateShareLink(context.Background(), location, vfs.ShareLinkRequest{Role: vfs.ShareRoleUploader}); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("invalid role error = %v", err)
	}
	if _, err := backend.CreateShareLink(context.Background(), location, vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer, ExpiresIn: time.Hour}); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("expiration error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("network requests = %d, want 0", requests.Load())
	}
}

func TestGoogleCreateShareLinkRejectsAmbiguousDirectPublicPermissions(t *testing.T) {
	var mutations atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"link-only","type":"anyone","role":"reader","allowFileDiscovery":false},{"id":"discoverable","type":"anyone","role":"writer","allowFileDiscovery":true}]}`)
		default:
			mutations.Add(1)
			http.Error(writer, "unexpected mutation", http.StatusBadRequest)
		}
	})
	_, err := backend.CreateShareLink(context.Background(), googleItemLocation("", "file-id"), vfs.ShareLinkRequest{Role: vfs.ShareRoleCommenter})
	if err == nil || !strings.Contains(err.Error(), "multiple direct public permissions") {
		t.Fatalf("error = %v", err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("mutations = %d, want 0", mutations.Load())
	}
}

func TestGoogleShareLinkCanBeCopiedWhenCurrentUserCannotModifyIt(t *testing.T) {
	var mutations atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(false))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"anyone-id","type":"anyone","role":"reader"}]}`)
		default:
			mutations.Add(1)
			http.Error(writer, "mutation is not allowed", http.StatusForbidden)
		}
	})
	location := googleItemLocation("", "file-id")
	info, err := backend.ShareLinkInfo(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if info.CanCreate || info.CanRevoke || info.Link == nil || info.Link.URL != googleShareTestURL || info.Link.Revocable {
		t.Fatalf("info = %#v", info)
	}
	if _, err := backend.CreateShareLink(context.Background(), location, vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer}); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("create error = %v, want permission", err)
	}
	if err := backend.RevokeShareLink(context.Background(), location); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("revoke error = %v, want permission", err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("mutations = %d, want 0", mutations.Load())
	}
}

func TestGoogleRevokeShareLinkIsIdempotentWhenRestricted(t *testing.T) {
	var mutations atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[]}`)
		default:
			mutations.Add(1)
			http.Error(writer, "unexpected mutation", http.StatusBadRequest)
		}
	})
	if err := backend.RevokeShareLink(context.Background(), googleItemLocation("", "file-id")); err != nil {
		t.Fatal(err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("mutations = %d, want 0", mutations.Load())
	}
}

func TestGoogleRevokeShareLinkDeletesOnlyDirectPermission(t *testing.T) {
	var deletes atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"anyone-id","type":"anyone","role":"reader","permissionDetails":[{"permissionType":"file","role":"reader","inherited":false}]}]}`)
		case request.Method == http.MethodDelete && request.URL.Path == "/files/file-id/permissions/anyone-id":
			deletes.Add(1)
			assertGoogleShareReadQuery(t, request)
			assertGoogleShareResourceKey(t, request, "file-id", "test-resource-key")
			writeGoogleShareJSON(t, writer, `{}`)
		default:
			http.Error(writer, "unexpected request", http.StatusBadRequest)
		}
	})
	if err := backend.RevokeShareLink(context.Background(), googleItemLocation("", "file-id")); err != nil {
		t.Fatal(err)
	}
	if deletes.Load() != 1 {
		t.Fatalf("deletes = %d, want 1", deletes.Load())
	}
}

func TestGoogleRevokeShareLinkDeletesEveryDirectPublicPermission(t *testing.T) {
	deleted := make(chan string, 2)
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"first","type":"anyone","role":"reader"},{"id":"second","type":"anyone","role":"writer","allowFileDiscovery":true}]}`)
		case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, "/files/file-id/permissions/"):
			deleted <- strings.TrimPrefix(request.URL.Path, "/files/file-id/permissions/")
			writeGoogleShareJSON(t, writer, `{}`)
		default:
			http.Error(writer, "unexpected request", http.StatusBadRequest)
		}
	})
	if err := backend.RevokeShareLink(context.Background(), googleItemLocation("", "file-id")); err != nil {
		t.Fatal(err)
	}
	close(deleted)
	got := make(map[string]bool)
	for id := range deleted {
		got[id] = true
	}
	if !reflect.DeepEqual(got, map[string]bool{"first": true, "second": true}) {
		t.Fatalf("deleted permissions = %#v", got)
	}
}

func TestGoogleRevokeShareLinkRejectsInheritedPermission(t *testing.T) {
	var deletes atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case "/files/file-id/permissions":
			if request.Method == http.MethodDelete {
				deletes.Add(1)
			}
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"anyone-id","type":"anyone","role":"reader","permissionDetails":[{"permissionType":"file","inheritedFrom":"parent-id","role":"reader","inherited":true}]}]}`)
		default:
			http.NotFound(writer, request)
		}
	})
	err := backend.RevokeShareLink(context.Background(), googleItemLocation("drive-id", "file-id"))
	if !errors.Is(err, os.ErrPermission) || deletes.Load() != 0 {
		t.Fatalf("error/deletes = %v / %d", err, deletes.Load())
	}
}

func TestGoogleSharingTargetsShortcutOriginalAndRejectsSyntheticRoots(t *testing.T) {
	var targetGets atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/files/shortcut-id":
			writeGoogleShareJSON(t, writer, `{"id":"shortcut-id","name":"Shortcut","mimeType":"application/vnd.google-apps.shortcut","shortcutDetails":{"targetId":"target-id","targetMimeType":"application/vnd.google-apps.folder","targetResourceKey":"target-key"},"webViewLink":"https://drive.google.com/open?id=shortcut-id","capabilities":{"canShare":true}}`)
		case "/files/target-id":
			targetGets.Add(1)
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "target-id/target-key" {
				t.Errorf("resource-key header = %q", got)
			}
			writeGoogleShareJSON(t, writer, `{"id":"target-id","name":"Target","mimeType":"application/vnd.google-apps.folder","webViewLink":"https://drive.google.com/drive/folders/target-id","capabilities":{"canShare":true}}`)
		case "/files/target-id/permissions":
			assertGoogleShareResourceKey(t, request, "target-id", "target-key")
			writeGoogleShareJSON(t, writer, `{"permissions":[]}`)
		default:
			http.Error(writer, "unexpected request", http.StatusBadRequest)
		}
	})
	if _, err := backend.ShareLinkInfo(context.Background(), googleShortcutLocation("drive-id", "shortcut-id", "target-id")); err != nil {
		t.Fatal(err)
	}
	if targetGets.Load() != 1 {
		t.Fatalf("target GETs = %d, want 1", targetGets.Load())
	}
	for _, location := range []string{googleRootLocation, googleMyLocation, googleSharedLocation, googleDriveLocation("drive-id")} {
		if _, err := backend.ShareLinkInfo(context.Background(), location); !errors.Is(err, ErrShareLinksUnsupported) {
			t.Errorf("ShareLinkInfo(%q) error = %v", location, err)
		}
	}
}

func TestGoogleSharingFollowsShortcutReturnedForItemLocation(t *testing.T) {
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/files/shortcut-id":
			writeGoogleShareJSON(t, writer, `{"id":"shortcut-id","name":"Shortcut","mimeType":"application/vnd.google-apps.shortcut","shortcutDetails":{"targetId":"target-id","targetMimeType":"text/plain"},"webViewLink":"https://drive.google.com/open?id=shortcut-id","capabilities":{"canShare":true}}`)
		case "/files/target-id":
			writeGoogleShareJSON(t, writer, `{"id":"target-id","name":"Target.txt","mimeType":"text/plain","webViewLink":"https://drive.google.com/open?id=target-id","capabilities":{"canShare":true}}`)
		case "/files/target-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[]}`)
		default:
			http.Error(writer, "unexpected request", http.StatusBadRequest)
		}
	})
	info, err := backend.ShareLinkInfo(context.Background(), googleItemLocation("", "shortcut-id"))
	if err != nil {
		t.Fatal(err)
	}
	if info.ItemName != "Target.txt" {
		t.Fatalf("item name = %q, want target", info.ItemName)
	}
}

func TestGoogleShareLinkInfoRequiresBrowserLinkAndHonorsCancellation(t *testing.T) {
	t.Run("missing webViewLink", func(t *testing.T) {
		var permissionRequests atomic.Int32
		backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
			if strings.HasSuffix(request.URL.Path, "/permissions") {
				permissionRequests.Add(1)
			}
			writeGoogleShareJSON(t, writer, `{"id":"file-id","name":"No link","mimeType":"text/plain","capabilities":{"canShare":true}}`)
		})
		if _, err := backend.ShareLinkInfo(context.Background(), googleItemLocation("", "file-id")); err == nil || !strings.Contains(err.Error(), "browser link") {
			t.Fatalf("error = %v", err)
		}
		if permissionRequests.Load() != 0 {
			t.Fatalf("permission requests = %d, want 0", permissionRequests.Load())
		}
	})
	t.Run("canceled", func(t *testing.T) {
		backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
			<-request.Context().Done()
		})
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := backend.ShareLinkInfo(ctx, googleItemLocation("", "file-id")); !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context canceled", err)
		}
	})
}

func TestGoogleShareErrorsPreservePermissionAndMutationState(t *testing.T) {
	t.Run("metadata forbidden", func(t *testing.T) {
		backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(writer, `{"error":{"code":403,"message":"sharing denied","errors":[{"reason":"insufficientFilePermissions","message":"sharing denied"}]}}`)
		})
		if _, err := backend.ShareLinkInfo(context.Background(), googleItemLocation("", "file-id")); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("error = %v, want permission", err)
		}
	})
	t.Run("indeterminate create", func(t *testing.T) {
		backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
			switch {
			case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
				writeGoogleShareJSON(t, writer, googleShareMetadata(true))
			case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
				writeGoogleShareJSON(t, writer, `{"permissions":[]}`)
			case request.Method == http.MethodPost:
				http.Error(writer, "backend failed", http.StatusInternalServerError)
			default:
				http.NotFound(writer, request)
			}
		})
		_, err := backend.CreateShareLink(context.Background(), googleItemLocation("", "file-id"), vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
		var unknown *vfs.UnknownOperationStateError
		if !errors.As(err, &unknown) {
			t.Fatalf("error = %T %v, want unknown operation state", err, err)
		}
	})
}

func TestGoogleShareMutationsAreSerialized(t *testing.T) {
	firstMutation := make(chan struct{})
	secondMutation := make(chan struct{})
	release := make(chan struct{})
	var mutations atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[]}`)
		case request.Method == http.MethodPost:
			number := mutations.Add(1)
			if number == 1 {
				close(firstMutation)
				<-release
			} else if number == 2 {
				close(secondMutation)
			}
			writeGoogleShareJSON(t, writer, `{"id":"anyone-id","type":"anyone","role":"reader"}`)
		default:
			http.NotFound(writer, request)
		}
	})

	type result struct {
		link vfs.ShareLink
		err  error
	}
	results := make(chan result, 2)
	create := func() {
		link, err := backend.CreateShareLink(context.Background(), googleItemLocation("", "file-id"), vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
		results <- result{link: link, err: err}
	}
	go create()
	select {
	case <-firstMutation:
	case <-time.After(5 * time.Second):
		t.Fatal("first mutation did not start")
	}
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledResult := make(chan error, 1)
	go func() {
		_, err := backend.CreateShareLink(canceledCtx, googleItemLocation("", "file-id"), vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
		canceledResult <- err
	}()
	select {
	case err := <-canceledResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled sharing request remained blocked behind mutation gate")
	}
	go create()
	select {
	case <-secondMutation:
		t.Fatal("second permission mutation overlapped the first")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	for range 2 {
		select {
		case got := <-results:
			if got.err != nil || got.link.URL != googleShareTestURL {
				t.Fatalf("result = %#v", got)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("serialized mutation did not finish")
		}
	}
	if mutations.Load() != 2 {
		t.Fatalf("mutations = %d, want 2", mutations.Load())
	}
}

func TestGoogleShareIgnoresMetadataViewAndCreatesRealPermission(t *testing.T) {
	var creates atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"metadata-view","type":"anyone","role":"reader","view":"metadata","permissionDetails":[{"permissionType":"file","role":"reader","inherited":true}]}]}`)
		case request.Method == http.MethodPost && request.URL.Path == "/files/file-id/permissions":
			creates.Add(1)
			writeGoogleShareJSON(t, writer, `{"id":"public-reader","type":"anyone","role":"reader"}`)
		default:
			http.NotFound(writer, request)
		}
	})
	location := googleItemLocation("", "file-id")
	info, err := backend.ShareLinkInfo(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if info.Link != nil || !info.CanCreate {
		t.Fatalf("metadata-only view became a share link: %#v", info)
	}
	if _, err := backend.CreateShareLink(context.Background(), location, vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer}); err != nil {
		t.Fatal(err)
	}
	if creates.Load() != 1 {
		t.Fatalf("permission creates = %d, want 1", creates.Load())
	}
}

func TestGoogleShareRejectsPlainHTTPWebViewLink(t *testing.T) {
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		writeGoogleShareJSON(t, writer, `{"id":"file-id","name":"file.txt","mimeType":"text/plain","webViewLink":"http://drive.invalid/file","capabilities":{"canShare":true}}`)
	})
	if _, err := backend.ShareLinkInfo(context.Background(), googleItemLocation("", "file-id")); err == nil || strings.Contains(err.Error(), "http://") {
		t.Fatalf("plain HTTP webViewLink error = %v", err)
	}
}

func TestGoogleShareRevokeTreatsPermissionNotFoundAsUnknownUntilRelisted(t *testing.T) {
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"already-gone","type":"anyone","role":"reader","permissionDetails":[{"role":"reader","inherited":false}]}]}`)
		case request.Method == http.MethodDelete:
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(writer, `{"error":{"code":404,"message":"not found"}}`)
		default:
			http.NotFound(writer, request)
		}
	})
	if err := backend.RevokeShareLink(context.Background(), googleItemLocation("", "file-id")); !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("ambiguous revoke error = %v", err)
	}
}

func TestGoogleShareCanRemoveDirectDiscoverabilityAboveInheritedEditor(t *testing.T) {
	var updates atomic.Int32
	backend := newGoogleShareTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id":
			writeGoogleShareJSON(t, writer, googleShareMetadata(true))
		case request.Method == http.MethodGet && request.URL.Path == "/files/file-id/permissions":
			writeGoogleShareJSON(t, writer, `{"permissions":[{"id":"inherited","type":"anyone","role":"writer","allowFileDiscovery":false,"permissionDetails":[{"role":"writer","inherited":true}]},{"id":"direct","type":"anyone","role":"writer","allowFileDiscovery":true,"permissionDetails":[{"role":"writer","inherited":false}]}]}`)
		case request.Method == http.MethodPatch && request.URL.Path == "/files/file-id/permissions/direct":
			updates.Add(1)
			var permission drive.Permission
			if err := json.NewDecoder(request.Body).Decode(&permission); err != nil {
				t.Fatal(err)
			}
			if permission.AllowFileDiscovery {
				t.Fatal("update retained discoverability")
			}
			writeGoogleShareJSON(t, writer, `{"id":"direct","type":"anyone","role":"writer","allowFileDiscovery":false}`)
		default:
			http.NotFound(writer, request)
		}
	})
	location := googleItemLocation("", "file-id")
	info, err := backend.ShareLinkInfo(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if !info.CanCreate || !info.LinkInherited || !info.LinkDiscoverable || info.LinkDiscoverabilityInherited {
		t.Fatalf("mixed inherited/direct state = %#v", info)
	}
	if _, err := backend.CreateShareLink(context.Background(), location, vfs.ShareLinkRequest{Role: vfs.ShareRoleEditor}); err != nil {
		t.Fatal(err)
	}
	if updates.Load() != 1 {
		t.Fatalf("permission updates = %d, want 1", updates.Load())
	}
}
