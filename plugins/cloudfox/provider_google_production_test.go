package cloudfox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unxed/f4/vfs"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type productionProgressCapture struct {
	action string
	name   string
	pct    int
}

func (*productionProgressCapture) UpdateScan(string, int64, int64) {}
func (*productionProgressCapture) IsCancelled() bool               { return false }
func (p *productionProgressCapture) UpdateTransfer(action, name string, currentPct int, _ string, _ int, _ string) {
	p.action, p.name, p.pct = action, name, currentPct
}

func TestCopyGoogleResponseReportsThroughTaskReporter(t *testing.T) {
	capture := &productionProgressCapture{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, capture)
	var destination bytes.Buffer
	written, err := copyGoogleResponse(ctx, &destination, strings.NewReader("google-bytes"), int64(len("google-bytes")), "drive-file.bin")
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(len("google-bytes")) || destination.String() != "google-bytes" {
		t.Fatalf("copy = %d bytes %q", written, destination.String())
	}
	if capture.action != "Downloading" || capture.name != "drive-file.bin" || capture.pct != 100 {
		t.Fatalf("ReporterKey progress = %#v", capture)
	}
}

func newGoogleProductionTestBackend(t *testing.T, handler http.HandlerFunc) *googleDriveBackend {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	return &googleDriveBackend{
		service: service, items: make(map[string]*drive.File), parents: make(map[string]string),
		names: make(map[string]string), transferNames: make(map[string]string), resolved: make(map[string]string),
		downloads: newGoogleDownloadCache(),
	}
}

func writeGoogleProductionJSON(t *testing.T, writer http.ResponseWriter, raw string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	if _, err := io.WriteString(writer, raw); err != nil {
		t.Error(err)
	}
}

func TestGoogleResourceKeyLocationsRoundTrip(t *testing.T) {
	item := googleItemLocationWithResourceKey("drive", "item", "item-key")
	parsedItem, err := parseGoogleLocation(item)
	if err != nil || parsedItem.itemID != "item" || parsedItem.resourceKey != "item-key" {
		t.Fatalf("item resource-key location = %#v, %v", parsedItem, err)
	}
	shortcut := googleShortcutLocationWithResourceKeys("drive", "shortcut", "target", "shortcut-key", "target-key")
	parsedShortcut, err := parseGoogleLocation(shortcut)
	if err != nil || parsedShortcut.itemID != "shortcut" || parsedShortcut.targetID != "target" || parsedShortcut.resourceKey != "shortcut-key" || parsedShortcut.targetResourceKey != "target-key" {
		t.Fatalf("shortcut resource-key location = %#v, %v", parsedShortcut, err)
	}
}

func TestGoogleEntryExposesStrongContentRevision(t *testing.T) {
	backend := &googleDriveBackend{items: make(map[string]*drive.File), parents: make(map[string]string), names: make(map[string]string), transferNames: make(map[string]string)}
	first := backend.entryForFile(&drive.File{Id: "file-id", Name: "file.bin", MimeType: "application/octet-stream", Version: 41, Md5Checksum: "aaaa", Size: 10, Parents: []string{"root"}}, googleMyLocation)
	second := backend.entryForFile(&drive.File{Id: "file-id", Name: "file.bin", MimeType: "application/octet-stream", Version: 42, Md5Checksum: "bbbb", Size: 10, Parents: []string{"root"}}, googleMyLocation)
	if first.VFSItem.Revision == "" || second.VFSItem.Revision == "" || first.VFSItem.Revision == second.VFSItem.Revision {
		t.Fatalf("content revisions = %q and %q", first.VFSItem.Revision, second.VFSItem.Revision)
	}
	native := backend.entryForFile(&drive.File{Id: "native-id", Name: "Doc", MimeType: googleDocMime, Version: 7, Parents: []string{"root"}}, googleMyLocation)
	if native.VFSItem.Revision == "" {
		t.Fatal("native Google document has no export revision")
	}
}

func TestGoogleFreshCanonicalStatHydratesFullPanelTitle(t *testing.T) {
	var gets atomic.Int32
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			http.Error(writer, "unexpected mutation", http.StatusMethodNotAllowed)
			return
		}
		gets.Add(1)
		switch request.URL.Path {
		case "/files/second-id":
			writeGoogleProductionJSON(t, writer, `{"id":"second-id","name":"Second","mimeType":"application/vnd.google-apps.folder","parents":["first-id"]}`)
		case "/files/first-id":
			writeGoogleProductionJSON(t, writer, `{"id":"first-id","name":"First","mimeType":"application/vnd.google-apps.folder","parents":["root"]}`)
		default:
			http.NotFound(writer, request)
		}
	})
	cloud := testCloudVFS(t, backend)
	defer cloud.Close()
	location := googleItemLocation("", "second-id")
	if err := cloud.SetPath(cloud.canonicalURI(location)); err != nil {
		t.Fatal(err)
	}
	if _, err := cloud.Stat(context.Background(), cloud.GetPath()); err != nil {
		t.Fatal(err)
	}
	separator := string(os.PathSeparator)
	want := "Test:" + separator + "My Drive" + separator + "First" + separator + "Second"
	if got := cloud.PanelTitle(cloud.GetPath()); got != want {
		t.Fatalf("restored PanelTitle = %q, want %q", got, want)
	}
	if got := gets.Load(); got != 2 {
		t.Fatalf("metadata GETs = %d, want current item plus one ancestor", got)
	}
}

func TestGoogleStatToleratesInaccessibleAncestorMetadata(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/files/current-id":
					writeGoogleProductionJSON(t, writer, `{"id":"current-id","name":"Current","mimeType":"application/vnd.google-apps.folder","parents":["inaccessible-parent"]}`)
				case "/files/inaccessible-parent":
					http.Error(writer, "parent unavailable", status)
				default:
					http.NotFound(writer, request)
				}
			})
			entry, err := backend.Stat(context.Background(), googleItemLocation("", "current-id"))
			if err != nil || entry.Name != "Current" || !entry.IsDir {
				t.Fatalf("Stat with inaccessible ancestor = %#v, %v", entry, err)
			}
		})
	}
}

func TestGoogleStatRefreshesExternalRevisionForSameItemID(t *testing.T) {
	var gets atomic.Int32
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/files/file-id" {
			http.NotFound(writer, request)
			return
		}
		generation := gets.Add(1) + 1
		if generation == 2 {
			writeGoogleProductionJSON(t, writer, `{"id":"file-id","name":"archive.zip","mimeType":"application/zip","version":"2","md5Checksum":"2222","size":"200","modifiedTime":"2026-08-09T01:00:00Z","parents":["root"]}`)
			return
		}
		writeGoogleProductionJSON(t, writer, `{"id":"file-id","name":"archive.zip","mimeType":"application/zip","version":"3","md5Checksum":"3333","size":"300","modifiedTime":"2026-08-09T02:00:00Z","parents":["root"]}`)
	})
	location := googleItemLocation("", "file-id")
	backend.items[location] = &drive.File{Id: "file-id", Name: "archive.zip", MimeType: "application/zip", Version: 1, Md5Checksum: "1111", Size: 100, Parents: []string{"root"}}
	first, err := backend.Stat(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.Stat(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if first.Size != 200 || second.Size != 300 || first.VFSItem.Revision == "" || second.VFSItem.Revision == "" || first.VFSItem.Revision == second.VFSItem.Revision {
		t.Fatalf("fresh external revisions: first=%d/%q second=%d/%q", first.Size, first.VFSItem.Revision, second.Size, second.VFSItem.Revision)
	}
	if gets.Load() != 2 {
		t.Fatalf("authoritative metadata GETs = %d, want 2", gets.Load())
	}
	backend.mu.RLock()
	cached := backend.items[location]
	backend.mu.RUnlock()
	if cached == nil || cached.Version != 3 || cached.Size != 300 {
		t.Fatalf("refreshed item cache = %#v", cached)
	}
}

func TestGoogleShortcutStatUsesFreshTargetContentRevision(t *testing.T) {
	var shortcutGets, targetGets atomic.Int32
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/files/shortcut-id":
			shortcutGets.Add(1)
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "shortcut-id/shortcut-key" {
				t.Errorf("shortcut resource-key header = %q", got)
			}
			writeGoogleProductionJSON(t, writer, `{"id":"shortcut-id","name":"archive-link.zip","mimeType":"application/vnd.google-apps.shortcut","resourceKey":"shortcut-key","version":"7","parents":["root"],"shortcutDetails":{"targetId":"target-id","targetMimeType":"application/zip","targetResourceKey":"target-key"}}`)
		case "/files/target-id":
			generation := targetGets.Add(1) + 10
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "target-id/target-key" {
				t.Errorf("target resource-key header = %q", got)
			}
			if generation == 11 {
				writeGoogleProductionJSON(t, writer, `{"id":"target-id","name":"archive.zip","mimeType":"application/zip","resourceKey":"target-key","version":"11","md5Checksum":"aaaa","size":"111","modifiedTime":"2026-08-09T03:00:00Z","parents":["root"]}`)
				return
			}
			writeGoogleProductionJSON(t, writer, `{"id":"target-id","name":"archive.zip","mimeType":"application/zip","resourceKey":"target-key","version":"12","md5Checksum":"bbbb","size":"222","modifiedTime":"2026-08-09T04:00:00Z","parents":["root"]}`)
		default:
			http.NotFound(writer, request)
		}
	})
	location := googleShortcutLocationWithResourceKeys("", "shortcut-id", "target-id", "shortcut-key", "target-key")
	backend.items[location] = &drive.File{
		Id: "shortcut-id", Name: "archive-link.zip", MimeType: googleShortcutMime, ResourceKey: "shortcut-key", Version: 7, Parents: []string{"root"},
		ShortcutDetails: &drive.FileShortcutDetails{TargetId: "target-id", TargetMimeType: "application/zip", TargetResourceKey: "target-key"},
	}
	first, err := backend.Stat(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.Stat(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if first.Size != 111 || second.Size != 222 || first.VFSItem.Revision == "" || second.VFSItem.Revision == "" || first.VFSItem.Revision == second.VFSItem.Revision {
		t.Fatalf("shortcut target revisions: first=%d/%q second=%d/%q", first.Size, first.VFSItem.Revision, second.Size, second.VFSItem.Revision)
	}
	if shortcutGets.Load() != 2 || targetGets.Load() != 2 {
		t.Fatalf("shortcut/target metadata GETs = %d/%d, want 2/2", shortcutGets.Load(), targetGets.Load())
	}
}

func TestGoogleCreateDirectoryCanonicalizesRecursiveNewParent(t *testing.T) {
	var creates atomic.Int32
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/parent-id":
			writeGoogleProductionJSON(t, writer, `{"id":"parent-id","name":"Parent","mimeType":"application/vnd.google-apps.folder","parents":["root"]}`)
		case request.Method == http.MethodGet && request.URL.Path == "/files":
			writeGoogleProductionJSON(t, writer, `{"files":[]}`)
		case request.Method == http.MethodPost && request.URL.Path == "/files":
			if creates.Add(1) == 1 {
				writeGoogleProductionJSON(t, writer, `{"id":"parent-id","name":"Parent","mimeType":"application/vnd.google-apps.folder","parents":["root"]}`)
			} else {
				writeGoogleProductionJSON(t, writer, `{"id":"child-id","name":"Child","mimeType":"application/vnd.google-apps.folder","parents":["parent-id"]}`)
			}
		default:
			http.NotFound(writer, request)
		}
	})
	parentGuess := backend.Join(googleMyLocation, "Parent")
	if err := backend.MkDir(context.Background(), parentGuess); err != nil {
		t.Fatal(err)
	}
	parentCanonical := backend.CanonicalLocation(parentGuess)
	parsedParent, err := parseGoogleLocation(parentCanonical)
	if err != nil || parsedParent.kind != "item" || parsedParent.itemID != "parent-id" {
		t.Fatalf("canonical parent = %q (%#v, %v)", parentCanonical, parsedParent, err)
	}
	childGuess := backend.Join(parentGuess, "Child")
	parsedChildGuess, err := parseGoogleLocation(childGuess)
	if err != nil {
		t.Fatal(err)
	}
	if parsedChildGuess.kind != "new" || parsedChildGuess.parent != parentCanonical {
		t.Fatalf("child destination parent = %q, want %q", parsedChildGuess.parent, parentCanonical)
	}
	if err := backend.MkDir(context.Background(), childGuess); err != nil {
		t.Fatal(err)
	}
	parsedChild, err := parseGoogleLocation(backend.CanonicalLocation(childGuess))
	if err != nil || parsedChild.itemID != "child-id" {
		t.Fatalf("canonical child = %#v, %v", parsedChild, err)
	}
}

func TestGoogleIdentityPreservingWriteUpdatesExistingIDInPlace(t *testing.T) {
	payload := []byte("edited Google Drive contents")
	var patchCalls, postCalls, deleteCalls atomic.Int32
	var uploaded []byte
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPatch && request.URL.Path == "/upload/drive/v3/files/existing-id":
			patchCalls.Add(1)
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "existing-id/existing-key" {
				t.Errorf("update resource-key header = %q", got)
			}
			if request.URL.Query().Get("uploadType") == "resumable" {
				writer.Header().Set("Location", server.URL+"/identity-preserving-upload")
				writer.WriteHeader(http.StatusOK)
				return
			}
			var err error
			uploaded, err = io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
			}
			writeGoogleProductionJSON(t, writer, `{"id":"existing-id","name":"editable.bin","mimeType":"application/octet-stream","resourceKey":"existing-key","version":"2","size":"28","parents":["root"]}`)
		case request.Method == http.MethodPut && request.URL.Path == "/identity-preserving-upload":
			var err error
			uploaded, err = io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
			}
			writeGoogleProductionJSON(t, writer, `{"id":"existing-id","name":"editable.bin","mimeType":"application/octet-stream","resourceKey":"existing-key","version":"2","size":"28","parents":["root"]}`)
		case request.Method == http.MethodPost:
			postCalls.Add(1)
			http.Error(writer, "must not create replacement", http.StatusInternalServerError)
		case request.Method == http.MethodDelete:
			deleteCalls.Add(1)
			http.Error(writer, "must not delete original", http.StatusInternalServerError)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	location := googleItemLocationWithResourceKey("", "existing-id", "existing-key")
	backend := &googleDriveBackend{
		service: service,
		items: map[string]*drive.File{location: {
			Id: "existing-id", Name: "editable.bin", MimeType: "application/octet-stream", ResourceKey: "existing-key", Version: 1, Size: 12, Parents: []string{"root"},
		}},
		parents: map[string]string{location: googleMyLocation}, names: map[string]string{location: "editable.bin"},
		transferNames: map[string]string{location: "editable.bin"}, resolved: make(map[string]string), downloads: newGoogleDownloadCache(),
	}
	cloud := testCloudVFS(t, backend)
	defer cloud.Close()
	if !cloud.GetCapabilities().HasIdentityPreservingWrite {
		t.Fatal("Google CloudVFS did not advertise identity-preserving writes")
	}
	publicPath := cloud.canonicalURI(location)
	file, err := cloud.Create(context.Background(), publicPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if patchCalls.Load() != 1 || postCalls.Load() != 0 || deleteCalls.Load() != 0 {
		t.Fatalf("PATCH/POST/DELETE calls = %d/%d/%d, want 1/0/0", patchCalls.Load(), postCalls.Load(), deleteCalls.Load())
	}
	if !strings.Contains(string(uploaded), string(payload)) {
		t.Fatalf("in-place update payload = %q", uploaded)
	}
	canonical, err := cloud.Abs(publicPath)
	if err != nil {
		t.Fatal(err)
	}
	canonicalLocation, err := cloud.resolvePath(canonical)
	if err != nil || canonicalLocation != location {
		t.Fatalf("identity-preserving canonical path = %q, %v; want %q", canonical, err, location)
	}
	backend.mu.RLock()
	updated := backend.items[location]
	backend.mu.RUnlock()
	if updated == nil || updated.Id != "existing-id" || updated.Name != "editable.bin" || updated.Version != 2 {
		t.Fatalf("cached in-place result = %#v", updated)
	}
}

func TestGoogleIdentityPreservingWriteUpdatesShortcutTargetNotShortcut(t *testing.T) {
	payload := []byte("updated through shortcut")
	var targetGets, targetPatches, shortcutMutations, creates, deletes atomic.Int32
	var uploaded []byte
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/target-id":
			targetGets.Add(1)
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "target-id/target-key" {
				t.Errorf("target GET resource-key header = %q", got)
			}
			writeGoogleProductionJSON(t, writer, `{"id":"target-id","name":"target.bin","mimeType":"application/octet-stream","resourceKey":"target-key","version":"1","size":"8","parents":["root"]}`)
		case request.Method == http.MethodPatch && request.URL.Path == "/upload/drive/v3/files/target-id":
			targetPatches.Add(1)
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "target-id/target-key" {
				t.Errorf("target PATCH resource-key header = %q", got)
			}
			if request.URL.Query().Get("uploadType") == "resumable" {
				writer.Header().Set("Location", server.URL+"/shortcut-target-upload")
				writer.WriteHeader(http.StatusOK)
				return
			}
			var err error
			uploaded, err = io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
			}
			writeGoogleProductionJSON(t, writer, `{"id":"target-id","name":"target.bin","mimeType":"application/octet-stream","resourceKey":"target-key","version":"2","size":"24","parents":["root"]}`)
		case request.Method == http.MethodPut && request.URL.Path == "/shortcut-target-upload":
			var err error
			uploaded, err = io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
			}
			writeGoogleProductionJSON(t, writer, `{"id":"target-id","name":"target.bin","mimeType":"application/octet-stream","resourceKey":"target-key","version":"2","size":"24","parents":["root"]}`)
		case strings.Contains(request.URL.Path, "shortcut-id") && request.Method != http.MethodGet:
			shortcutMutations.Add(1)
			http.Error(writer, "shortcut identity must not be mutated", http.StatusInternalServerError)
		case request.Method == http.MethodPost:
			creates.Add(1)
			http.Error(writer, "must not create replacement", http.StatusInternalServerError)
		case request.Method == http.MethodDelete:
			deletes.Add(1)
			http.Error(writer, "must not delete shortcut or target", http.StatusInternalServerError)
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	shortcutLocation := googleShortcutLocationWithResourceKeys("", "shortcut-id", "target-id", "shortcut-key", "target-key")
	shortcut := &drive.File{
		Id: "shortcut-id", Name: "editable-link.bin", MimeType: googleShortcutMime, ResourceKey: "shortcut-key", Parents: []string{"root"},
		ShortcutDetails: &drive.FileShortcutDetails{TargetId: "target-id", TargetMimeType: "application/octet-stream", TargetResourceKey: "target-key"},
	}
	backend := &googleDriveBackend{
		service: service, items: map[string]*drive.File{shortcutLocation: shortcut},
		parents: map[string]string{shortcutLocation: googleMyLocation}, names: map[string]string{shortcutLocation: "editable-link.bin"},
		transferNames: map[string]string{shortcutLocation: "editable-link.bin"}, resolved: make(map[string]string), downloads: newGoogleDownloadCache(),
	}
	cloud := testCloudVFS(t, backend)
	defer cloud.Close()
	file, err := cloud.Create(vfs.WithDestinationOverwrite(context.Background(), true), cloud.canonicalURI(shortcutLocation))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if targetGets.Load() != 1 || targetPatches.Load() != 1 || shortcutMutations.Load() != 0 || creates.Load() != 0 || deletes.Load() != 0 {
		t.Fatalf("target GET/PATCH, shortcut mutations, create/delete = %d/%d, %d, %d/%d", targetGets.Load(), targetPatches.Load(), shortcutMutations.Load(), creates.Load(), deletes.Load())
	}
	if !strings.Contains(string(uploaded), string(payload)) {
		t.Fatalf("shortcut target update payload = %q", uploaded)
	}
	canonical, err := cloud.Abs(cloud.canonicalURI(shortcutLocation))
	if err != nil {
		t.Fatal(err)
	}
	canonicalLocation, err := cloud.resolvePath(canonical)
	if err != nil || canonicalLocation != shortcutLocation {
		t.Fatalf("shortcut identity changed after target update: %q, %v", canonical, err)
	}
	backend.mu.RLock()
	stillShortcut := backend.items[shortcutLocation]
	backend.mu.RUnlock()
	if stillShortcut == nil || stillShortcut.Id != "shortcut-id" || stillShortcut.MimeType != googleShortcutMime || stillShortcut.ShortcutDetails == nil || stillShortcut.ShortcutDetails.TargetId != "target-id" {
		t.Fatalf("shortcut metadata after target update = %#v", stillShortcut)
	}
}

func TestGoogleCreateNoReplaceRejectsConcurrentDestinationWithoutMutation(t *testing.T) {
	var mutationCalls atomic.Int32
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		mutationCalls.Add(1)
		http.Error(writer, "no mutation expected", http.StatusInternalServerError)
	})
	location := googleItemLocation("", "concurrent-id")
	backend.items[location] = &drive.File{Id: "concurrent-id", Name: "race.bin", MimeType: "application/octet-stream", Parents: []string{"root"}}
	_, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), false), location)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("no-replace Create error = %v, want os.ErrExist", err)
	}
	if mutationCalls.Load() != 0 {
		t.Fatalf("no-replace Create made %d API call(s)", mutationCalls.Load())
	}
}

func TestGoogleRemoveInvalidatesCachedItemBeforeStat(t *testing.T) {
	var metadataGets atomic.Int32
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "file-id/file-key" {
			t.Errorf("resource-key header = %q", got)
		}
		switch request.Method {
		case http.MethodDelete:
			writer.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			metadataGets.Add(1)
			http.Error(writer, "gone", http.StatusNotFound)
		default:
			http.Error(writer, "unexpected method", http.StatusMethodNotAllowed)
		}
	})
	location := googleItemLocationWithResourceKey("", "file-id", "file-key")
	backend.items[location] = &drive.File{Id: "file-id", Name: "stale.txt", MimeType: "text/plain", ResourceKey: "file-key", Parents: []string{"root"}}
	backend.names[location] = "stale.txt"
	if err := backend.Remove(context.Background(), location); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Stat(context.Background(), location); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat after Remove = %v, want os.ErrNotExist", err)
	}
	if got := metadataGets.Load(); got != 1 {
		t.Fatalf("metadata GETs after Remove = %d, want authoritative lookup", got)
	}
}

func TestGoogleRenameReplacementRemapsDeletedDestinationIdentity(t *testing.T) {
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPatch && request.URL.Path == "/files/destination-id":
			writeGoogleProductionJSON(t, writer, `{"id":"destination-id","name":"destination.txt.f4bak-destinat","mimeType":"text/plain","parents":["root"]}`)
		case request.Method == http.MethodPatch && request.URL.Path == "/files/source-id":
			writeGoogleProductionJSON(t, writer, `{"id":"source-id","name":"destination.txt","mimeType":"text/plain","parents":["root"]}`)
		case request.Method == http.MethodDelete && request.URL.Path == "/files/destination-id":
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(writer, request)
		}
	})
	source := googleItemLocation("", "source-id")
	destination := googleItemLocation("", "destination-id")
	backend.items[source] = &drive.File{Id: "source-id", Name: "destination.txt.f4tmp", MimeType: "text/plain", Parents: []string{"root"}}
	backend.items[destination] = &drive.File{Id: "destination-id", Name: "destination.txt", MimeType: "text/plain", Parents: []string{"root"}}
	if err := backend.Rename(context.Background(), source, destination); err != nil {
		t.Fatal(err)
	}
	if got := backend.CanonicalLocation(destination); got != source {
		t.Fatalf("replacement destination remap = %q, want %q", got, source)
	}
	file, err := backend.fileForLocation(context.Background(), destination)
	if err != nil || file.Id != "source-id" || file.Name != "destination.txt" {
		t.Fatalf("reopen replaced destination = %#v, %v", file, err)
	}
	cloud := testCloudVFS(t, backend)
	defer cloud.Close()
	resolved, err := cloud.resolvePath(cloud.canonicalURI(destination))
	if err != nil || resolved != source {
		t.Fatalf("CloudVFS replacement remap = %q, %v; want %q", resolved, err, source)
	}
	canonicalPublic, err := cloud.Abs(cloud.canonicalURI(destination))
	if err != nil {
		t.Fatal(err)
	}
	canonicalLocation, err := cloud.resolvePath(canonicalPublic)
	if err != nil || canonicalLocation != source {
		t.Fatalf("canonical replacement URI = %q, %v; want location %q", canonicalPublic, err, source)
	}
}

func TestGoogleShortcutResourceKeysSurviveRestoreAndDirectoryTraversal(t *testing.T) {
	var shortcutGets, targetGets, lists atomic.Int32
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/shortcut-id":
			shortcutGets.Add(1)
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "shortcut-id/shortcut-key" {
				t.Errorf("shortcut resource-key header = %q", got)
			}
			writeGoogleProductionJSON(t, writer, `{"id":"shortcut-id","name":"Folder link","mimeType":"application/vnd.google-apps.shortcut","resourceKey":"shortcut-key","parents":["root"],"shortcutDetails":{"targetId":"target-id","targetMimeType":"application/vnd.google-apps.folder","targetResourceKey":"target-key"}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/files/target-id":
			targetGets.Add(1)
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "target-id/target-key" {
				t.Errorf("target resource-key header = %q", got)
			}
			writeGoogleProductionJSON(t, writer, `{"id":"target-id","name":"Target","mimeType":"application/vnd.google-apps.folder","resourceKey":"target-key","parents":["root"]}`)
		case request.Method == http.MethodGet && request.URL.Path == "/files":
			lists.Add(1)
			if got := request.Header.Get("X-Goog-Drive-Resource-Keys"); got != "target-id/target-key" {
				t.Errorf("children-list resource-key header = %q", got)
			}
			writeGoogleProductionJSON(t, writer, `{"files":[]}`)
		default:
			http.NotFound(writer, request)
		}
	})
	location := googleShortcutLocationWithResourceKeys("", "shortcut-id", "target-id", "shortcut-key", "target-key")
	entry, err := backend.Stat(context.Background(), location)
	if err != nil || !entry.IsDir || !entry.IsSymlink || entry.Location != location {
		t.Fatalf("restored shortcut Stat = %#v, %v", entry, err)
	}
	if err := backend.ReadDir(context.Background(), location, func([]RemoteEntry) {}); err != nil {
		t.Fatal(err)
	}
	// Stat fetches the target once for its authoritative content revision;
	// ReadDir fetches it again as the directory being traversed.
	if shortcutGets.Load() != 1 || targetGets.Load() != 2 || lists.Load() != 1 {
		t.Fatalf("shortcut/target/list calls = %d/%d/%d", shortcutGets.Load(), targetGets.Load(), lists.Load())
	}
}

func TestGoogleNativeExportRefreshesRevisionBeforeUsingSessionCache(t *testing.T) {
	var metadataCalls atomic.Int32
	var exportCalls atomic.Int32
	backend := newGoogleProductionTestBackend(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/files/native-id":
			version := metadataCalls.Add(1)
			writeGoogleProductionJSON(t, writer, `{"id":"native-id","name":"Report","mimeType":"application/vnd.google-apps.document","version":"`+string(rune('0'+version))+`","modifiedTime":"2026-08-08T00:00:0`+string(rune('0'+version))+`Z","parents":["root"]}`)
		case request.Method == http.MethodGet && request.URL.Path == "/files/native-id/export":
			if exportCalls.Add(1) == 1 {
				_, _ = io.WriteString(writer, "first")
			} else {
				_, _ = io.WriteString(writer, "second")
			}
		default:
			http.NotFound(writer, request)
		}
	})
	location := googleItemLocation("", "native-id")
	backend.items[location] = &drive.File{Id: "native-id", Name: "Report", MimeType: googleDocMime, Version: 0, Parents: []string{"root"}}
	read := func() string {
		r, err := backend.Open(context.Background(), location)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		data := make([]byte, r.Size())
		n, err := r.ReadAt(context.Background(), data, 0)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Fatal(err)
		}
		return string(data[:n])
	}
	if got := read(); got != "first" {
		t.Fatalf("first export = %q", got)
	}
	if got := read(); got != "second" {
		t.Fatalf("second export reused stale session cache: %q", got)
	}
	if metadataCalls.Load() != 2 || exportCalls.Load() != 2 {
		t.Fatalf("metadata/export calls = %d/%d, want 2/2", metadataCalls.Load(), exportCalls.Load())
	}
}

func newGoogleCacheTestTemp(t *testing.T, payload string) *providerTempReader {
	t.Helper()
	file, err := os.CreateTemp("", "f4-google-cache-production-*")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	if _, err := io.WriteString(file, payload); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		t.Fatal(err)
	}
	return newProviderTempReader(file, path, int64(len(payload)))
}

func googleCacheReaderPath(t *testing.T, reader vfs.ReadAtCloser) string {
	t.Helper()
	local, ok := reader.(interface{ LocalPath() (string, bool) })
	if !ok {
		t.Fatalf("cached reader %T has no local backing lease", reader)
	}
	path, valid := local.LocalPath()
	if !valid || path == "" {
		t.Fatal("cached reader returned an invalid local backing lease")
	}
	return path
}

func TestGoogleDownloadCacheInvalidationRetainsActiveBackingLease(t *testing.T) {
	cache := newGoogleDownloadCache()
	reader, err := cache.install("file-id", "revision-one", newGoogleCacheTestTemp(t, "payload"))
	if err != nil {
		t.Fatal(err)
	}
	path := googleCacheReaderPath(t, reader)
	cache.invalidate("file-id")
	cache.mu.Lock()
	entryCount, cachedBytes := len(cache.entries), cache.bytes
	cache.mu.Unlock()
	if entryCount != 0 || cachedBytes != 0 {
		t.Fatalf("cache after invalidation: entries=%d bytes=%d", entryCount, cachedBytes)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("active backing file disappeared during invalidation: %v", err)
	}
	buffer := make([]byte, len("payload"))
	if n, err := reader.ReadAt(context.Background(), buffer, 0); err != nil || n != len(buffer) || string(buffer) != "payload" {
		t.Fatalf("active read after invalidation = %d, %v, %q", n, err, buffer)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired backing remains after last reader Close: %v", err)
	}
	cache.close()
}

func TestGoogleDownloadCacheEvictsInactiveLRUWithinBudget(t *testing.T) {
	cache := newGoogleDownloadCache()
	cache.maxEntries = 1
	cache.maxBytes = 64
	first, err := cache.install("first", "one", newGoogleCacheTestTemp(t, "first"))
	if err != nil {
		t.Fatal(err)
	}
	firstPath := googleCacheReaderPath(t, first)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := cache.install("second", "two", newGoogleCacheTestTemp(t, "second"))
	if err != nil {
		t.Fatal(err)
	}
	secondPath := googleCacheReaderPath(t, second)
	if _, err := os.Stat(firstPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inactive LRU backing remains after eviction: %v", err)
	}
	cache.mu.Lock()
	_, firstCached := cache.entries["first"]
	_, secondCached := cache.entries["second"]
	entryCount, cachedBytes := len(cache.entries), cache.bytes
	cache.mu.Unlock()
	if firstCached || !secondCached || entryCount != 1 || cachedBytes != int64(len("second")) {
		t.Fatalf("bounded cache state: first=%t second=%t entries=%d bytes=%d", firstCached, secondCached, entryCount, cachedBytes)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(secondPath); err != nil {
		t.Fatalf("current cached backing disappeared when its reader closed: %v", err)
	}
	cache.close()
	if _, err := os.Stat(secondPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cached backing remains after session close: %v", err)
	}
}

func TestGoogleDownloadCacheKeepsActiveOldEntryAndMakesPressureInsertPrivate(t *testing.T) {
	cache := newGoogleDownloadCache()
	cache.maxEntries = 1
	cache.maxBytes = 64
	active, err := cache.install("active", "one", newGoogleCacheTestTemp(t, "active"))
	if err != nil {
		t.Fatal(err)
	}
	activePath := googleCacheReaderPath(t, active)
	private, err := cache.install("private", "two", newGoogleCacheTestTemp(t, "private"))
	if err != nil {
		t.Fatal(err)
	}
	privatePath := googleCacheReaderPath(t, private)
	cache.mu.Lock()
	_, activeCached := cache.entries["active"]
	_, privateCached := cache.entries["private"]
	entryCount, cachedBytes := len(cache.entries), cache.bytes
	cache.mu.Unlock()
	if !activeCached || privateCached || entryCount != 1 || cachedBytes != int64(len("active")) {
		t.Fatalf("active-pressure cache state: active=%t private=%t entries=%d bytes=%d", activeCached, privateCached, entryCount, cachedBytes)
	}
	if err := private.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(privatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private pressure backing remains after Close: %v", err)
	}
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active cached backing was evicted: %v", err)
	}
	if err := active.Close(); err != nil {
		t.Fatal(err)
	}
	cache.close()
	if _, err := os.Stat(activePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active cached backing remains after cache Close: %v", err)
	}
}

func TestCloudVFSReadDirReplacesStaleAliasSnapshot(t *testing.T) {
	backend := &fakeBackend{pages: [][]RemoteEntry{{{
		VFSItem: vfs.VFSItem{Name: "gone.txt"}, Location: "/ids/gone",
	}}}}
	cloud := testCloudVFS(t, backend)
	defer cloud.Close()
	if err := cloud.ReadDir(context.Background(), cloud.GetPath(), func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	warmed := cloud.Join(cloud.GetPath(), "gone.txt")
	warmedLocation, err := cloud.resolvePath(warmed)
	if err != nil || warmedLocation != "/ids/gone" {
		t.Fatalf("warmed alias path = %q (%v)", warmed, err)
	}
	backend.pages = nil
	if err := cloud.ReadDir(context.Background(), cloud.GetPath(), func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	refreshed := cloud.Join(cloud.GetPath(), "gone.txt")
	refreshedLocation, err := cloud.resolvePath(refreshed)
	if err != nil {
		t.Fatal(err)
	}
	if refreshedLocation == "/ids/gone" {
		t.Fatalf("removed row retained stale alias: %q", refreshed)
	}
}
