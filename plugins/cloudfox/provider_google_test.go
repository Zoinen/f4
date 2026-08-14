package cloudfox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"golang.org/x/oauth2"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func googleTestConnection(t *testing.T) Connection {
	t.Helper()
	raw, err := json.Marshal(GoogleDriveSettings{ClientID: "client-id.apps.exampleusercontent.com"})
	if err != nil {
		t.Fatal(err)
	}
	return Connection{ID: testConnectionID, Name: "Google", Provider: ProviderGoogleDrive, Settings: raw}
}

func TestGoogleLocationCodecsRoundTripOpaqueIDs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw      string
		kind     string
		driveID  string
		itemID   string
		targetID string
		parent   string
		name     string
	}{
		{raw: googleRootLocation, kind: "root"},
		{raw: googleMyLocation, kind: "my"},
		{raw: googleSharedLocation, kind: "shared"},
		{raw: googleDriveLocation("drive:/with:delimiters/Диск"), kind: "drive", driveID: "drive:/with:delimiters/Диск"},
		{raw: googleItemLocation("drive:id", "item/id:100%"), kind: "item", driveID: "drive:id", itemID: "item/id:100%"},
		{raw: googleShortcutLocation("drive", "shortcut:id", "target/id"), kind: "shortcut", driveID: "drive", itemID: "shortcut:id", targetID: "target/id"},
		{raw: googleNewLocation(googleItemLocation("", "parent:id"), "Quarterly: report/2026.xlsx"), kind: "new", parent: googleItemLocation("", "parent:id"), name: "Quarterly: report/2026.xlsx"},
	}
	for _, test := range tests {
		parsed, err := parseGoogleLocation(test.raw)
		if err != nil {
			t.Errorf("parseGoogleLocation(%q): %v", test.raw, err)
			continue
		}
		if parsed.kind != test.kind || parsed.driveID != test.driveID || parsed.itemID != test.itemID || parsed.targetID != test.targetID || parsed.parent != test.parent || parsed.name != test.name {
			t.Errorf("parseGoogleLocation(%q) = %#v", test.raw, parsed)
		}
	}

	for _, raw := range []string{"", "g:unknown:value", "g:item:only-drive", "g:shortcut:a:b", "g:new:a", "g:drive:%%%"} {
		if _, err := parseGoogleLocation(raw); err == nil {
			t.Errorf("parseGoogleLocation(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestGoogleReadDirUsesMinimalSupportedFieldMask(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fields := request.URL.Query().Get("fields")
		if strings.Contains(fields, "canListChildren") {
			http.Error(writer, "Invalid field selection canListChildren", http.StatusBadRequest)
			return
		}
		if fields != googleFileListFields {
			t.Errorf("fields = %q, want %q", fields, googleFileListFields)
			http.Error(writer, "unexpected fields", http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"files":[{"id":"folder-id","name":"Folder","mimeType":"application/vnd.google-apps.folder","parents":["root"],"capabilities":{"canCopy":true,"canDelete":true,"canTrash":true}}]}`)
	}))
	defer server.Close()

	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &googleDriveBackend{
		service: service, items: make(map[string]*drive.File), parents: make(map[string]string),
		names: make(map[string]string), transferNames: make(map[string]string),
	}
	var entries []RemoteEntry
	if err := backend.ReadDir(context.Background(), googleMyLocation, func(chunk []RemoteEntry) {
		entries = append(entries, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "Folder" || !entries[0].IsDir {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestGoogleMyDriveListingNeverExposesOpaqueRootID(t *testing.T) {
	const opaqueRootID = "0ABu_Rx81coapUk9PVA"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/files" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"files":[{"id":"aob-id","name":"AOB","mimeType":"application/vnd.google-apps.folder","parents":["`+opaqueRootID+`"]}]}`)
	}))
	defer server.Close()

	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &googleDriveBackend{
		service: service, items: make(map[string]*drive.File), parents: make(map[string]string),
		names: make(map[string]string), transferNames: make(map[string]string),
	}
	cloud := testCloudVFS(t, backend)
	defer cloud.Close()

	root := cloud.GetPath()
	if err := cloud.ReadDir(context.Background(), root, func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	myDrive := cloud.Join(root, "My Drive")
	if err := cloud.SetPath(myDrive); err != nil {
		t.Fatal(err)
	}
	if err := cloud.ReadDir(context.Background(), myDrive, func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	aob := cloud.Join(myDrive, "AOB")
	if err := cloud.SetPath(aob); err != nil {
		t.Fatal(err)
	}

	want := "Test:" + string(os.PathSeparator) + "My Drive" + string(os.PathSeparator) + "AOB"
	if got := cloud.GetPath(); got != want {
		t.Fatalf("GetPath = %q, want %q", got, want)
	}
	if got := cloud.GetPath(); strings.Contains(got, opaqueRootID) {
		t.Fatalf("GetPath exposed opaque My Drive root ID: %q", got)
	}
	if got := backend.Dir(googleItemLocation("", "aob-id")); got != googleMyLocation {
		t.Fatalf("AOB parent = %q, want My Drive", got)
	}
}

func TestGoogleRangeReaderPinsETagAndValidatesContentRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-Match"); got != `"generation-one"` {
			t.Errorf("If-Match = %q", got)
		}
		switch r.Header.Get("Range") {
		case "bytes=2-4":
			w.Header().Set("Content-Range", "bytes 2-4/6")
			w.Header().Set("ETag", `"generation-one"`)
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "cde")
		case "bytes=1-2":
			w.Header().Set("Content-Range", "bytes 0-1/6")
			w.Header().Set("ETag", `"generation-one"`)
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "ab")
		default:
			http.Error(w, "unexpected range", http.StatusRequestedRangeNotSatisfiable)
		}
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	lifetime, cancel := context.WithCancel(context.Background())
	reader := &googleRangeReader{service: service, fileID: "file-id", size: 6, etag: `"generation-one"`, ctx: lifetime, cancel: cancel}
	defer reader.Close()
	buffer := make([]byte, 3)
	if n, err := reader.ReadAt(context.Background(), buffer, 2); err != nil || n != 3 || string(buffer) != "cde" {
		t.Fatalf("ReadAt = %d, %v, %q", n, err, buffer)
	}
	if _, err := reader.ReadAt(context.Background(), make([]byte, 2), 1); err == nil || !strings.Contains(err.Error(), "Content-Range") {
		t.Fatalf("mismatched Content-Range error = %v", err)
	}
	if n, err := reader.ReadAt(context.Background(), nil, 0); n != 0 || err != nil {
		t.Fatalf("zero-length ReadAt = %d, %v", n, err)
	}
	if _, err := reader.ReadAt(context.Background(), make([]byte, 1), -1); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("negative ReadAt error = %v", err)
	}
}

func TestGoogleRangeReaderFallsBackWhenRangeIsIgnored(t *testing.T) {
	content := "complete response"
	var requests atomic.Int32
	var progress atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if got := r.Header.Get("Range"); got != "bytes=2-5" {
			t.Errorf("Range = %q", got)
		}
		w.Header().Set("ETag", `"generation-one"`)
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, content)
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	lifetime, cancel := context.WithCancel(context.Background())
	reader := &googleRangeReader{service: service, fileID: "file-id", size: int64(len(content)), etag: `"generation-one"`, ctx: lifetime, cancel: cancel}
	defer reader.Close()

	buffer := make([]byte, 4)
	readCtx := context.WithValue(context.Background(), vfs.ProgressKey, vfs.ProgressCallback(func(_ string, percent int) {
		progress.Store(int32(percent))
	}))
	if n, err := reader.ReadAt(readCtx, buffer, 2); err != nil || n != 4 || string(buffer) != "mple" {
		t.Fatalf("first ReadAt = %d, %v, %q", n, err, buffer)
	}
	if n, err := reader.ReadAt(context.Background(), buffer, 9); err != nil || n != 4 || string(buffer) != "resp" {
		t.Fatalf("local ReadAt = %d, %v, %q", n, err, buffer)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("HTTP requests = %d, want one full-response fallback", got)
	}
	if got := progress.Load(); got != 100 {
		t.Fatalf("download progress = %d, want 100", got)
	}
}

func TestGoogleSessionDownloadCacheReusesOnlyMatchingRevision(t *testing.T) {
	cache := newGoogleDownloadCache()
	defer cache.close()
	f, err := os.CreateTemp("", "f4-cloudfox-cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	if _, err := io.WriteString(f, "cached payload"); err != nil {
		f.Close()
		os.Remove(path)
		t.Fatal(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		os.Remove(path)
		t.Fatal(err)
	}
	handle, err := cache.install("file-id", "revision-1", newProviderTempReader(f, path, int64(len("cached payload"))))
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, ok := cache.open("file-id", "revision-1")
	if !ok {
		t.Fatal("unchanged revision was not served from the session cache")
	}
	buffer := make([]byte, len("cached payload"))
	if n, err := reopened.ReadAt(context.Background(), buffer, 0); err != nil || n != len(buffer) || string(buffer) != "cached payload" {
		t.Fatalf("cached ReadAt = %d, %v, %q", n, err, buffer)
	}
	_ = reopened.Close()
	if _, ok := cache.open("file-id", "revision-2"); ok {
		t.Fatal("changed revision incorrectly reused stale cached content")
	}

	cache.close()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cache file still exists after session close: %v", err)
	}
}

func TestGoogleNativeExportNaming(t *testing.T) {
	t.Parallel()

	backend := &googleDriveBackend{
		items: make(map[string]*drive.File), parents: make(map[string]string), names: make(map[string]string), transferNames: make(map[string]string),
	}
	tests := []struct {
		mime     string
		name     string
		wantName string
		wantMime string
		wantExt  string
	}{
		{googleDocMime, "Report", "Report.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx"},
		{googleSheetMime, "Budget.XLSX", "Budget.XLSX", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx"},
		{googleSlidesMime, "Review", "Review.pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx"},
	}
	for index, test := range tests {
		file := &drive.File{Id: string(rune('a' + index)), Name: test.name, MimeType: test.mime, Parents: []string{"root"}}
		entry := backend.entryForFile(file, googleMyLocation)
		if entry.Name != test.wantName || entry.TransferName != test.wantName || entry.IsDir {
			t.Errorf("entryForFile(%q) = %#v", test.mime, entry)
		}
		extension, exportMime, ok := googleExport(test.mime)
		if !ok || extension != test.wantExt || exportMime != test.wantMime {
			t.Errorf("googleExport(%q) = %q, %q, %v", test.mime, extension, exportMime, ok)
		}
		if got := backend.TransferName(entry.Location); got != test.wantName {
			t.Errorf("TransferName(%q) = %q", entry.Location, got)
		}
		if stripped := stripGoogleExportExtension(test.wantName, test.mime); stripped != strings.TrimSuffix(test.wantName, test.wantExt) && strings.ToLower(test.wantName) != "budget.xlsx" {
			t.Errorf("stripGoogleExportExtension(%q) = %q", test.wantName, stripped)
		}
	}
	if _, _, ok := googleExport("application/octet-stream"); ok {
		t.Fatal("binary file unexpectedly has a Google export")
	}

	shortcut := &drive.File{Id: "shortcut", Name: "Folder link", MimeType: googleShortcutMime, Parents: []string{"root"}, ShortcutDetails: &drive.FileShortcutDetails{TargetId: "folder", TargetMimeType: googleFolderMime}}
	entry := backend.entryForFile(shortcut, googleMyLocation)
	if !entry.IsSymlink || !entry.IsDir {
		t.Fatalf("folder shortcut = %#v", entry)
	}
}

func TestGoogleCopyRejectsBinaryReplacementOfNativeObject(t *testing.T) {
	t.Parallel()

	var apiCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		apiCalls.Add(1)
		http.Error(w, "Copy/Delete must not be called", http.StatusInternalServerError)
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}

	sourceLocation := googleItemLocation("", "binary-source")
	destinationLocation := googleItemLocation("", "native-destination")
	backend := &googleDriveBackend{
		service: service,
		items: map[string]*drive.File{
			sourceLocation: {
				Id: "binary-source", Name: "payload.bin", MimeType: "application/octet-stream", Parents: []string{"root"},
			},
			destinationLocation: {
				Id: "native-destination", Name: "Important document", MimeType: googleDocMime, Parents: []string{"root"},
			},
		},
		parents:       make(map[string]string),
		names:         make(map[string]string),
		transferNames: make(map[string]string),
	}

	err = backend.Copy(context.Background(), sourceLocation, destinationLocation)
	if !errors.Is(err, ErrReadOnlyObject) {
		t.Fatalf("Copy error = %v, want ErrReadOnlyObject", err)
	}
	if got := apiCalls.Load(); got != 0 {
		t.Fatalf("Copy/Delete API was called %d time(s)", got)
	}
}

func TestGoogleCreateUploadsStreamToMyDrive(t *testing.T) {
	payload := []byte("local file contents")
	var uploaded []byte
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/drive/v3/files":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"files":[]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/upload/drive/v3/files":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			if r.URL.Query().Get("uploadType") == "resumable" {
				w.Header().Set("Location", server.URL+"/upload-session")
				w.WriteHeader(http.StatusOK)
				return
			}
			uploaded = append(uploaded, body...)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"uploaded","size":"19"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/upload-session":
			var err error
			uploaded, err = io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"uploaded","size":"19"}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &googleDriveBackend{
		service: service, items: make(map[string]*drive.File), parents: make(map[string]string),
		names: make(map[string]string), transferNames: make(map[string]string), downloads: newGoogleDownloadCache(),
	}
	writer, err := backend.Create(context.Background(), googleNewLocation(googleMyLocation, "upload.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(uploaded, payload) {
		t.Fatalf("uploaded request does not contain payload: %q", uploaded)
	}
}

func TestGoogleCopyMissingResultIDDoesNotDeleteDestination(t *testing.T) {
	t.Parallel()

	var deleteCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			deleteCalls.Add(1)
			http.Error(w, "destination must not be deleted", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	service, err := drive.NewService(context.Background(), option.WithHTTPClient(server.Client()), option.WithEndpoint(server.URL+"/"))
	if err != nil {
		t.Fatal(err)
	}

	sourceLocation := googleItemLocation("", "binary-source")
	destinationLocation := googleItemLocation("", "binary-destination")
	backend := &googleDriveBackend{
		service: service,
		items: map[string]*drive.File{
			sourceLocation:      {Id: "binary-source", Name: "source.bin", MimeType: "application/octet-stream", Parents: []string{"root"}},
			destinationLocation: {Id: "binary-destination", Name: "destination.bin", MimeType: "application/octet-stream", Parents: []string{"root"}},
		},
		parents:       make(map[string]string),
		names:         make(map[string]string),
		transferNames: make(map[string]string),
	}

	err = backend.Copy(context.Background(), sourceLocation, destinationLocation)
	if !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("Copy error = %v, want ErrOperationStateUnknown", err)
	}
	if got := deleteCalls.Load(); got != 0 {
		t.Fatalf("destination delete was called %d time(s)", got)
	}
}

func TestGoogleAuthorizationURLUsesStateAndPKCE(t *testing.T) {
	t.Parallel()

	verifier := "verifier-with-enough-random-looking-data-0123456789"
	raw, err := GoogleAuthorizationURL(googleTestConnection(t), SecretValues{"client_secret": "client-secret"}, "http://127.0.0.1:43210/callback", "state-123", verifier)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	query := parsed.Query()
	if parsed.Scheme != "https" || query.Get("client_id") != "client-id.apps.exampleusercontent.com" || query.Get("state") != "state-123" || query.Get("redirect_uri") != "http://127.0.0.1:43210/callback" {
		t.Fatalf("authorization URL = %s", raw)
	}
	if query.Get("code_challenge") != wantChallenge || query.Get("code_challenge_method") != "S256" || query.Get("access_type") != "offline" || query.Get("prompt") != "consent" {
		t.Fatalf("OAuth options = %v", query)
	}
	if !strings.Contains(query.Get("scope"), "googleapis.com/auth/drive") {
		t.Fatalf("scope = %q", query.Get("scope"))
	}
}

type googleTestRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper googleTestRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

func TestAuthorizeGoogleDesktopCompletesLoopbackWithoutExternalNetwork(t *testing.T) {
	t.Parallel()

	var tokenForm url.Values
	tokenClient := &http.Client{Transport: googleTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || !strings.HasSuffix(request.URL.Path, "/token") {
			t.Errorf("token request = %s %s", request.Method, request.URL)
		}
		data, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		tokenForm, err = url.ParseQuery(string(data))
		if err != nil {
			return nil, err
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": {"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`)),
			Request:    request,
		}, nil
	})}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, tokenClient)
	initial := SecretValues{"client_secret": "client-secret", "preserve": "yes"}
	result, err := AuthorizeGoogleDesktop(ctx, googleTestConnection(t), initial, func(rawAuthorizationURL string) error {
		authorizationURL, err := url.Parse(rawAuthorizationURL)
		if err != nil {
			return err
		}
		query := authorizationURL.Query()
		if query.Get("state") == "" || query.Get("code_challenge") == "" {
			t.Errorf("authorization query = %v", query)
		}
		callbackURL, err := url.Parse(query.Get("redirect_uri"))
		if err != nil {
			return err
		}
		callbackQuery := callbackURL.Query()
		callbackQuery.Set("state", query.Get("state"))
		callbackQuery.Set("code", "offline-code")
		callbackURL.RawQuery = callbackQuery.Encode()
		response, err := http.Get(callbackURL.String()) // #nosec G107 -- loopback URL created by the code under test.
		if err != nil {
			return err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Errorf("callback status = %s", response.Status)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result["client_secret"] != "client-secret" || result["preserve"] != "yes" || result["access_token"] != "new-access" || result["refresh_token"] != "new-refresh" {
		t.Fatalf("secrets = %#v", result)
	}
	if _, err := time.Parse(time.RFC3339Nano, result["expires_at"]); err != nil {
		t.Fatalf("expires_at = %q: %v", result["expires_at"], err)
	}
	if !reflect.DeepEqual(initial, SecretValues{"client_secret": "client-secret", "preserve": "yes"}) {
		t.Fatalf("input secrets were mutated: %#v", initial)
	}
	if tokenForm.Get("code") != "offline-code" || tokenForm.Get("code_verifier") == "" || tokenForm.Get("redirect_uri") == "" {
		t.Fatalf("token form = %v", tokenForm)
	}
}

func TestAuthorizeGoogleDesktopRejectsStateMismatchBeforeExchange(t *testing.T) {
	t.Parallel()

	tokenCalled := false
	tokenClient := &http.Client{Transport: googleTestRoundTripper(func(request *http.Request) (*http.Response, error) {
		tokenCalled = true
		return nil, context.Canceled
	})}
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, tokenClient)
	_, err := AuthorizeGoogleDesktop(ctx, googleTestConnection(t), SecretValues{"client_secret": "secret"}, func(rawAuthorizationURL string) error {
		authorizationURL, parseErr := url.Parse(rawAuthorizationURL)
		if parseErr != nil {
			return parseErr
		}
		callbackURL, parseErr := url.Parse(authorizationURL.Query().Get("redirect_uri"))
		if parseErr != nil {
			return parseErr
		}
		query := callbackURL.Query()
		query.Set("state", "attacker-state")
		query.Set("code", "must-not-be-exchanged")
		callbackURL.RawQuery = query.Encode()
		response, requestErr := http.Get(callbackURL.String()) // #nosec G107 -- loopback URL created by the code under test.
		if requestErr != nil {
			return requestErr
		}
		_ = response.Body.Close()
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("error = %v", err)
	}
	if tokenCalled {
		t.Fatal("authorization code was exchanged after a state mismatch")
	}
}

func TestPersistGoogleTokenPreservesLatestMetadataAndTracksRotatedRef(t *testing.T) {
	api := &memoryKeyring{}
	secrets := newKeyringStoreWithAPI(api)
	repo := &Repository{
		Connections: NewConnectionStore(filepath.Join(t.TempDir(), "CloudFox.json")),
		Secrets:     SecretStores{Keyring: secrets},
	}
	initialValues := SecretValues{"refresh_token": "refresh-one"}
	candidate := googleTestConnection(t)
	candidate.ID = ""
	connection, err := repo.Save(context.Background(), candidate, &initialValues, SecretStorageKeyring)
	if err != nil {
		t.Fatal(err)
	}
	stale := connection.Clone()
	connection.Name = "Renamed in the UI"
	connection, err = repo.Save(context.Background(), connection, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	plugin := &Plugin{repo: repo}
	refreshed := &oauth2.Token{AccessToken: "access-one", RefreshToken: "refresh-one", Expiry: time.Now().Add(time.Hour)}
	tracked, err := plugin.persistGoogleToken(context.Background(), stale, refreshed)
	if err != nil {
		t.Fatal(err)
	}
	if tracked.Name != "Renamed in the UI" || tracked.SecretRef == stale.SecretRef {
		t.Fatalf("tracked connection = %+v", tracked)
	}
	latest, err := repo.Get(context.Background(), connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Name != "Renamed in the UI" {
		t.Fatalf("token refresh reverted profile name to %q", latest.Name)
	}

	refreshed.AccessToken = "access-two"
	trackedAgain, err := plugin.persistGoogleToken(context.Background(), tracked, refreshed)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := repo.Credentials(context.Background(), trackedAgain)
	if err != nil {
		t.Fatal(err)
	}
	if credentials["access_token"] != "access-two" {
		t.Fatalf("second refresh was not persisted: %#v", credentials)
	}
}

func TestPersistGoogleTokenRejectsOldOAuthClientAfterMetadataEdit(t *testing.T) {
	api := &memoryKeyring{}
	repo := &Repository{
		Connections: NewConnectionStore(filepath.Join(t.TempDir(), "CloudFox.json")),
		Secrets:     SecretStores{Keyring: newKeyringStoreWithAPI(api)},
	}
	values := SecretValues{"refresh_token": "refresh-old"}
	candidate := googleTestConnection(t)
	candidate.ID = ""
	connection, err := repo.Save(context.Background(), candidate, &values, SecretStorageKeyring)
	if err != nil {
		t.Fatal(err)
	}
	oldSession := connection.Clone()
	connection.Settings = json.RawMessage(`{"client_id":"different-client.apps.googleusercontent.com"}`)
	connection, err = repo.Save(context.Background(), connection, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	plugin := &Plugin{repo: repo}
	rotated, err := plugin.persistGoogleToken(context.Background(), oldSession, &oauth2.Token{
		AccessToken: "must-not-persist", RefreshToken: "refresh-old", Expiry: time.Now().Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID != "" {
		t.Fatalf("stale OAuth session unexpectedly rotated profile: %+v", rotated)
	}
	latest, err := repo.Get(context.Background(), connection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.SecretRef != oldSession.SecretRef {
		t.Fatalf("secret reference changed from %q to %q", oldSession.SecretRef, latest.SecretRef)
	}
}

func TestGoogleBackendCloseCancelsSessionContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	backend := &googleDriveBackend{cancel: cancel}
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("Close did not cancel the backend session context")
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
}
