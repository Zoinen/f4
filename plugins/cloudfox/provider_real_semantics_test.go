package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	drive "google.golang.org/api/drive/v3"
)

const (
	realSemanticsEnv       = "F4_CLOUDFOX_REAL_SEMANTICS"
	realSemanticsConfirmed = "CONFIRMED"
	realSemanticsPrefix    = "f4-cloudfox-real-semantics-"
)

// TestRealSavedCloudConnectionSemantics covers provider behavior which is not
// part of the ordinary CRUD matrix: the production TrashVFS wrapper, exact
// recovery followed by permanent cleanup, account/quota panel information,
// and the human platform path restored directly in a fresh provider session.
//
// It is deliberately independently opt-in. No profile, secret store or
// network access occurs unless both F4_CLOUDFOX_REAL_MUTATION and
// F4_CLOUDFOX_REAL_SEMANTICS have the exact value CONFIRMED. Every mutation is
// confined to generated top-level folders. Cleanup first proves their exact
// generated names and provider identities, including identities in Trash.
//
// F4_CLOUDFOX_REAL_YANDEX_DIAL_RETRIES retains the same harness-only meaning
// as the main live matrix: it can retry only a DialContext failure which did
// not return a TCP connection. No HTTP request, response or mutation is ever
// retried by this test.
func TestRealSavedCloudConnectionSemantics(t *testing.T) {
	if os.Getenv(realMutationEnv) != realMutationConfirmed || os.Getenv(realSemanticsEnv) != realSemanticsConfirmed {
		t.Skip("real CloudFox semantics require both explicit confirmations")
	}
	configDir := strings.TrimSpace(os.Getenv(realConfigDirEnv))
	if configDir == "" || !filepath.IsAbs(configDir) {
		t.Fatal("real CloudFox semantics require an absolute config directory")
	}
	info, err := os.Stat(configDir) // #nosec G703 -- the doubly opted-in semantics test intentionally loads the operator-supplied absolute config directory.
	if err != nil || !info.IsDir() {
		t.Fatal("real CloudFox config directory is unavailable")
	}

	yandexFactory := realYandexFactoryWithDialRetries(t, &YandexDiskFactory{})
	prompt := MasterPasswordPromptFunc(func(ctx context.Context, _ bool) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return os.Getenv(realVaultPasswordEnv), nil
	})
	plugin := NewPlugin(Options{
		ConfigDir:      configDir,
		Keyring:        NewKeyringStore(),
		PasswordPrompt: prompt,
		Factories: []BackendFactory{
			&GoogleDriveFactory{},
			yandexFactory,
			&S3Factory{},
			&WebDAVFactory{},
		},
	})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close real semantics plugin: %v", err)
		}
	})

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	connections, err := plugin.Repository().List(loadCtx)
	cancelLoad()
	if err != nil {
		t.Fatalf("load saved CloudFox connections: %v", err)
	}
	targets := []struct {
		name        string
		provider    ProviderType
		selectorEnv string
	}{
		{name: "google-drive", provider: ProviderGoogleDrive, selectorEnv: realGoogleSelectorEnv},
		{name: "yandex-disk", provider: ProviderYandexDisk, selectorEnv: realYandexSelectorEnv},
	}
	for _, target := range targets {
		target := target
		t.Run(target.name, func(t *testing.T) {
			connection := selectRealConnection(t, connections, target.provider, target.selectorEnv)
			runRealSavedSemantics(t, plugin, connection)
		})
	}
}

func runRealSavedSemantics(t *testing.T, plugin *Plugin, connection Connection) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	uuid, err := newUUID()
	if err != nil {
		t.Fatalf("generate real semantics identity: %v", err)
	}
	suffix := strings.ReplaceAll(uuid, "-", "")
	titleName := realSemanticsPrefix + "title-" + suffix
	trashName := realSemanticsPrefix + "trash-" + suffix
	for _, name := range []string{titleName, trashName} {
		if !strings.HasPrefix(name, realSemanticsPrefix) || strings.ContainsAny(name, "/\\") {
			t.Fatal("generated an unsafe real semantics folder name")
		}
	}
	t.Cleanup(func() {
		cleanupRealSemanticsArtifacts(t, plugin, connection.ID, []string{titleName, trashName})
	})

	mounted, writeRoot := openRealSemanticsRoot(t, ctx, plugin, connection)
	assertNoRealSemanticsArtifacts(t, ctx, mounted, writeRoot)
	assertRealPanelInfo(t, ctx, mounted, connection.Provider)

	titlePath := createRealSemanticsDirectory(t, ctx, mounted, writeRoot, titleName)
	if err := mounted.SetPath(titlePath); err != nil {
		_ = mounted.Close()
		t.Fatalf("enter real semantics title workspace: %v", err)
	}
	firstPath := createRealSemanticsDirectory(t, ctx, mounted, mounted.GetPath(), "First")
	if err := mounted.SetPath(firstPath); err != nil {
		_ = mounted.Close()
		t.Fatalf("enter first title level: %v", err)
	}
	secondPath := createRealSemanticsDirectory(t, ctx, mounted, mounted.GetPath(), "Second")
	if err := mounted.SetPath(secondPath); err != nil {
		_ = mounted.Close()
		t.Fatalf("enter second title level: %v", err)
	}

	wantParts := []string{titleName, "First", "Second"}
	if connection.Provider == ProviderGoogleDrive {
		wantParts = append([]string{"My Drive"}, wantParts...)
	}
	assertRealPanelTitle(t, "warmed navigation", mounted, connection.Name, wantParts)
	directPath := mounted.GetPath()
	if err := mounted.Close(); err != nil {
		t.Errorf("close warmed semantics mount: %v", err)
	}

	// Reopen the persisted visual path with no surviving VFS reference, exactly
	// like restoring a panel, folder history entry, or bookmark.
	restored, err := (&connectionProvider{plugin: plugin}).Open(ctx, nil, directPath)
	if err != nil {
		t.Fatalf("restore direct nested visual path: %v", err)
	}
	assertRealPanelTitle(t, "fresh visual restore", restored, connection.Name, wantParts)
	if err := restored.Close(); err != nil {
		t.Errorf("close direct canonical restored mount: %v", err)
	}

	root, writeRoot := openRealSemanticsRoot(t, ctx, plugin, connection)
	t.Cleanup(func() {
		if err := root.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	removeRealSemanticsNormalFolder(t, ctx, root, writeRoot, titleName)

	trashPath := createRealSemanticsDirectory(t, ctx, root, writeRoot, trashName)
	markerPath := root.Join(trashPath, "recovery-marker.txt")
	marker := []byte("CloudFox exact trash recovery marker\n")
	writeRealSemanticsFile(t, ctx, root, markerPath, marker)
	if _, err := root.Stat(ctx, markerPath); err != nil {
		t.Fatalf("stat trash recovery marker: %v", err)
	}

	trasher, ok := root.(vfs.TrashVFS)
	if !ok {
		t.Fatal("real cloud VFS does not expose the provider trash capability")
	}
	if err := trasher.MoveToTrash(ctx, trashPath); err != nil {
		t.Fatalf("move generated folder to provider Trash: %v", err)
	}
	if _, found, err := findRealSemanticsEntry(ctx, root, writeRoot, trashName); err != nil {
		t.Fatalf("list writable root after MoveToTrash: %v", err)
	} else if found {
		t.Error("MoveToTrash left the generated folder visible in the writable root")
	}

	cloud := realSemanticsCloudVFS(t, root)
	backend, err := cloud.backend()
	if err != nil {
		t.Fatalf("access live backend for exact Trash verification: %v", err)
	}
	switch concrete := backend.(type) {
	case *googleDriveBackend:
		restoreRealGoogleTrash(t, ctx, cloud, concrete, trashPath, trashName)
	case *yandexDiskBackend:
		restoreRealYandexTrash(t, ctx, concrete, trashName)
	default:
		t.Fatalf("unexpected real Trash backend type %T", backend)
	}

	recoveredPath, found, err := findRealSemanticsEntry(ctx, root, writeRoot, trashName)
	if err != nil {
		t.Fatalf("list writable root after Trash recovery: %v", err)
	}
	if !found {
		t.Fatal("provider reported successful Trash recovery but the exact folder is absent")
	}
	markerItems, err := readRealSemanticsDirectory(ctx, root, recoveredPath)
	if err != nil {
		t.Fatalf("list recovered folder: %v", err)
	}
	markerCount := 0
	for _, item := range markerItems {
		if item.Name == "recovery-marker.txt" && !item.IsDir && item.Size == int64(len(marker)) {
			markerCount++
		}
	}
	if markerCount != 1 {
		t.Fatalf("recovered folder contains %d exact markers, want 1", markerCount)
	}
	assertRealSemanticsFile(t, ctx, root, root.Join(recoveredPath, "recovery-marker.txt"), marker)

	if err := root.Remove(ctx, recoveredPath); err != nil {
		t.Fatalf("permanently delete recovered generated folder: %v", err)
	}
	if _, found, err := findRealSemanticsEntry(ctx, root, writeRoot, trashName); err != nil {
		t.Fatalf("list writable root after permanent cleanup: %v", err)
	} else if found {
		t.Error("permanently deleted generated folder remains in the writable root")
	}
	assertRealSemanticsTrashAbsent(t, ctx, backend, trashName)
	t.Log("MoveToTrash, exact recovery, marker verification, and permanent cleanup completed")
}

func openRealSemanticsRoot(t *testing.T, ctx context.Context, plugin *Plugin, connection Connection) (vfs.VFS, string) {
	t.Helper()
	latest, err := plugin.Repository().Get(ctx, connection.ID)
	if err != nil {
		t.Fatalf("reload saved connection for real semantics: %v", err)
	}
	filesystem, err := plugin.openConnection(ctx, plugin.manager(), latest, "", false)
	if err != nil {
		t.Fatalf("open saved connection for real semantics: %v", err)
	}
	writeRoot := filesystem.GetPath()
	if latest.Provider == ProviderGoogleDrive {
		items, err := readRealSemanticsDirectory(ctx, filesystem, writeRoot)
		if err != nil {
			_ = filesystem.Close()
			t.Fatalf("list Google virtual root for real semantics: %v", err)
		}
		matches := 0
		for _, item := range items {
			if item.Name != "My Drive" || !item.IsDir {
				continue
			}
			writeRoot = filesystem.Join(writeRoot, item.Name)
			matches++
		}
		if matches != 1 {
			_ = filesystem.Close()
			t.Fatalf("Google virtual root contains %d canonical My Drive entries, want 1", matches)
		}
		if err := filesystem.SetPath(writeRoot); err != nil {
			_ = filesystem.Close()
			t.Fatalf("enter Google My Drive for real semantics: %v", err)
		}
	}
	return filesystem, writeRoot
}

func createRealSemanticsDirectory(t *testing.T, ctx context.Context, filesystem vfs.VFS, parent, name string) string {
	t.Helper()
	if name == "" || strings.ContainsAny(name, "/\\") {
		t.Fatal("refusing unsafe real semantics directory name")
	}
	candidate := filesystem.Join(parent, name)
	if candidate == "" || candidate == parent {
		t.Fatal("provider produced an unsafe real semantics destination")
	}
	if err := filesystem.MkDir(ctx, candidate); err != nil {
		t.Fatalf("create real semantics directory: %v", err)
	}
	path, found, err := findRealSemanticsEntry(ctx, filesystem, parent, name)
	if err != nil {
		t.Fatalf("list parent after creating real semantics directory: %v", err)
	}
	if !found {
		t.Fatal("created real semantics directory is absent from its parent")
	}
	item, err := filesystem.Stat(ctx, path)
	if err != nil || !item.IsDir || item.Name != name {
		t.Fatalf("created real semantics directory has unexpected metadata: %v", err)
	}
	return path
}

func findRealSemanticsEntry(ctx context.Context, filesystem vfs.VFS, parent, name string) (string, bool, error) {
	items, err := readRealSemanticsDirectory(ctx, filesystem, parent)
	if err != nil {
		return "", false, err
	}
	matches := 0
	path := ""
	for _, item := range items {
		if item.Name == name && item.IsDir {
			matches++
			path = filesystem.Join(parent, item.Name)
		}
	}
	if matches > 1 {
		return "", false, errors.New("real semantics folder name is not unique")
	}
	return path, matches == 1, nil
}

func readRealSemanticsDirectory(ctx context.Context, filesystem vfs.VFS, path string) ([]vfs.VFSItem, error) {
	var result []vfs.VFSItem
	err := filesystem.ReadDir(ctx, path, func(items []vfs.VFSItem) {
		result = append(result, items...)
	})
	return result, err
}

func writeRealSemanticsFile(t *testing.T, ctx context.Context, filesystem vfs.VFS, path string, payload []byte) {
	t.Helper()
	w, err := filesystem.Create(ctx, path)
	if err != nil {
		t.Fatalf("create real semantics marker: %s", redactRealProviderError(err))
	}
	if _, err := w.Write(payload); err != nil {
		_ = w.Close()
		t.Fatalf("write real semantics marker: %s", redactRealProviderError(err))
	}
	if err := w.Close(); err != nil {
		t.Fatalf("commit real semantics marker: %s", redactRealProviderError(err))
	}
}

func assertRealSemanticsFile(t *testing.T, ctx context.Context, filesystem vfs.VFS, path string, expected []byte) {
	t.Helper()
	r, err := filesystem.Open(ctx, path)
	if err != nil {
		t.Fatalf("open recovered marker: %s", redactRealProviderError(err))
	}
	defer func() { _ = r.Close() }()
	got := make([]byte, len(expected))
	n, err := r.ReadAt(ctx, got, 0)
	if n != len(expected) || (err != nil && !errors.Is(err, io.EOF)) || string(got) != string(expected) {
		t.Fatalf("recovered marker content mismatch: bytes=%d/%d error=%s", n, len(expected), redactRealProviderError(err))
	}
}

func realSemanticsCloudVFS(t *testing.T, filesystem vfs.VFS) *CloudVFS {
	t.Helper()
	switch typed := filesystem.(type) {
	case *trashCloudVFS:
		return typed.CloudVFS
	case *CloudVFS:
		return typed
	default:
		t.Fatalf("unexpected real CloudFox VFS type %T", filesystem)
		return nil
	}
}

func assertRealPanelTitle(t *testing.T, phase string, filesystem vfs.VFS, connectionName string, wantParts []string) {
	t.Helper()
	provider, ok := filesystem.(vfs.PanelTitleProvider)
	if !ok {
		t.Errorf("%s: mounted CloudVFS has no PanelTitle provider", phase)
		return
	}
	separator := string(os.PathSeparator)
	want := connectionName + ":" + separator + strings.Join(wantParts, separator)
	got := provider.PanelTitle(filesystem.GetPath())
	if got == want {
		t.Logf("%s PanelTitle reconstructed all %d platform-path components", phase, len(wantParts))
		return
	}
	prefixOK := strings.HasPrefix(got, connectionName+":"+separator)
	body := strings.TrimPrefix(got, connectionName+":"+separator)
	gotParts := []string{}
	if body != "" {
		gotParts = strings.Split(body, separator)
	}
	matched := 0
	for index := 0; index < len(gotParts) && index < len(wantParts); index++ {
		if gotParts[index] == wantParts[index] {
			matched++
		}
	}
	t.Errorf("%s PanelTitle is not the full human platform path (prefix=%t components=%d want=%d leading-matches=%d)", phase, prefixOK, len(gotParts), len(wantParts), matched)
}

func assertRealPanelInfo(t *testing.T, ctx context.Context, filesystem vfs.VFS, providerType ProviderType) {
	t.Helper()
	provider, ok := filesystem.(vfs.PanelInfoProvider)
	if !ok {
		t.Error("mounted CloudVFS has no PanelInfo provider")
		return
	}
	cloud := realSemanticsCloudVFS(t, filesystem)
	backend, err := cloud.backend()
	if err != nil {
		t.Errorf("access live backend for panel information: %v", err)
		return
	}
	_, backendImplements := backend.(BackendPanelInfo)
	req := vfs.PanelInfoRequest{Path: filesystem.GetPath()}
	key := provider.PanelInfoKey(req)
	if key == "" || key != provider.PanelInfoKey(req) {
		t.Error("PanelInfoKey is empty or unstable")
	}
	snapshot, err := provider.RefreshPanelInfo(ctx, req)
	if err != nil {
		t.Errorf("refresh live provider panel information: %v", err)
		return
	}
	hasUser, hasQuota, fields := realPanelInfoShape(snapshot)
	cached, fresh := provider.CachedPanelInfo(req)
	cachedUser, cachedQuota, cachedFields := realPanelInfoShape(cached)
	t.Logf("panel info: backend=%t authoritative=%t sections=%d fields=%d user=%t quota=%t cached-fresh=%t", backendImplements, snapshot.Authoritative, len(snapshot.Sections), fields, hasUser, hasQuota, fresh)
	if !snapshot.Authoritative || snapshot.RefreshedAt.IsZero() || !hasUser || !hasQuota {
		t.Errorf("live panel information is incomplete (backend=%t authoritative=%t timestamp=%t user=%t quota=%t)", backendImplements, snapshot.Authoritative, !snapshot.RefreshedAt.IsZero(), hasUser, hasQuota)
	}
	if !fresh || !cached.Authoritative || !cachedUser || !cachedQuota || cachedFields != fields {
		t.Errorf("cached live panel information is not a fresh equivalent snapshot (fresh=%t authoritative=%t user=%t quota=%t fields=%d/%d)", fresh, cached.Authoritative, cachedUser, cachedQuota, cachedFields, fields)
	}
	if providerType == ProviderYandexDisk && !backendImplements {
		if concrete, ok := backend.(*yandexDiskBackend); ok {
			hasLiveUser, hasLiveQuota, aboutErr := realYandexAboutShape(ctx, concrete)
			if aboutErr != nil {
				t.Errorf("read Yandex account/quota source for panel diagnosis: %v", aboutErr)
			} else {
				t.Logf("Yandex account endpoint exposes semantic panel data: user=%t quota=%t", hasLiveUser, hasLiveQuota)
			}
		}
	}
}

func realPanelInfoShape(snapshot vfs.PanelInfoSnapshot) (hasUser, hasQuota bool, fields int) {
	for _, section := range snapshot.Sections {
		for _, field := range section.Fields {
			fields++
			switch field.ID {
			case "user":
				hasUser = field.Kind == vfs.PanelInfoText && strings.TrimSpace(field.Value) != ""
			case "quota":
				hasQuota = field.Kind == vfs.PanelInfoUsage && field.TotalBytes > 0 && field.AvailableBytes <= field.TotalBytes
			}
		}
	}
	return hasUser, hasQuota, fields
}

func realYandexAboutShape(ctx context.Context, backend *yandexDiskBackend) (bool, bool, error) {
	resp, err := backend.apiRequest(ctx, http.MethodGet, "", nil, nil)
	if err != nil {
		return false, false, err
	}
	defer func() {
		_ = resp.Body.Close() // response body cleanup only
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, false, mapProviderHTTPError(resp, readSmallResponse(resp))
	}
	var about struct {
		TotalSpace uint64 `json:"total_space"`
		UsedSpace  uint64 `json:"used_space"`
		User       struct {
			UID         json.RawMessage `json:"uid"`
			Login       string          `json:"login"`
			DisplayName string          `json:"display_name"`
		} `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&about); err != nil {
		return false, false, err
	}
	hasUser := strings.TrimSpace(about.User.Login) != "" || strings.TrimSpace(about.User.DisplayName) != "" || len(about.User.UID) > 0
	hasQuota := about.TotalSpace > 0 && about.UsedSpace <= about.TotalSpace
	return hasUser, hasQuota, nil
}

func restoreRealGoogleTrash(t *testing.T, ctx context.Context, cloud *CloudVFS, backend *googleDriveBackend, location, name string) {
	t.Helper()
	internal, err := cloud.resolvePath(location)
	if err != nil {
		t.Fatalf("resolve generated Google Trash visual path: %v", err)
	}
	parsed, err := parseGoogleLocation(internal)
	if err != nil || parsed.kind != "item" || parsed.itemID == "" {
		t.Fatalf("generated Google Trash folder has an invalid canonical identity: %v", err)
	}
	file, err := backend.service.Files.Get(parsed.itemID).SupportsAllDrives(true).Fields("id,name,mimeType,trashed").Context(ctx).Do()
	if err != nil {
		t.Fatalf("verify generated Google folder in Trash: %v", mapGoogleError(err))
	}
	if file.Id != parsed.itemID || file.Name != name || file.MimeType != googleFolderMime || !file.Trashed {
		t.Fatal("Google Trash contains unexpected metadata for the generated folder")
	}
	_, err = backend.service.Files.Update(parsed.itemID, &drive.File{Trashed: false, ForceSendFields: []string{"Trashed"}}).
		SupportsAllDrives(true).Fields("id,trashed").Context(ctx).Do()
	if err != nil {
		t.Fatalf("restore exact generated Google folder from Trash: %v", googleMutationError("real semantics restore", err))
	}
}

func restoreRealYandexTrash(t *testing.T, ctx context.Context, backend *yandexDiskBackend, name string) {
	t.Helper()
	entries, err := realYandexTrashEntries(ctx, backend)
	if err != nil {
		t.Fatalf("list Yandex Trash for exact generated folder: %v", err)
	}
	matches := make([]yandexResource, 0, 1)
	for _, entry := range entries {
		if entry.Name == name && entry.Type == "dir" {
			matches = append(matches, entry)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("Yandex Trash contains %d exact generated folders, want 1", len(matches))
	}
	path, err := realYandexTrashRelativePath(matches[0].Path)
	if err != nil {
		t.Fatalf("generated Yandex Trash folder has an invalid identity: %v", err)
	}
	if err := backend.mutation(ctx, http.MethodPut, "/trash/resources/restore", url.Values{"path": {path}, "overwrite": {"false"}}); err != nil {
		t.Fatalf("restore exact generated Yandex folder from Trash: %v", err)
	}
}

func realYandexTrashEntries(ctx context.Context, backend *yandexDiskBackend) ([]yandexResource, error) {
	var result []yandexResource
	for offset := 0; ; {
		query := url.Values{
			"path":   {"/"},
			"limit":  {"1000"},
			"offset": {strconv.Itoa(offset)},
		}
		resp, err := backend.apiRequest(ctx, http.MethodGet, "/trash/resources", query, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			message := readSmallResponse(resp)
			_ = resp.Body.Close() // response body cleanup only
			return nil, mapProviderHTTPError(resp, message)
		}
		var resource yandexResource
		err = json.NewDecoder(resp.Body).Decode(&resource)
		_ = resp.Body.Close() // response body cleanup only
		if err != nil {
			return nil, err
		}
		result = append(result, resource.Embedded.Items...)
		offset += len(resource.Embedded.Items)
		if len(resource.Embedded.Items) == 0 || offset >= resource.Embedded.Total {
			return result, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
}

func realYandexTrashRelativePath(raw string) (string, error) {
	path := strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	path = strings.TrimPrefix(path, "trash:")
	if path == "" || path == "/" || strings.Contains(path, "..") {
		return "", errors.New("unsafe Yandex Trash path")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path, nil
}

func assertRealSemanticsTrashAbsent(t *testing.T, ctx context.Context, backend Backend, name string) {
	t.Helper()
	switch concrete := backend.(type) {
	case *googleDriveBackend:
		files, err := realGoogleTrashedFolders(ctx, concrete, name)
		if err != nil {
			t.Errorf("verify Google Trash cleanup: %v", err)
		} else if len(files) != 0 {
			t.Errorf("Google Trash retains %d exact generated folders", len(files))
		}
	case *yandexDiskBackend:
		entries, err := realYandexTrashEntries(ctx, concrete)
		if err != nil {
			t.Errorf("verify Yandex Trash cleanup: %v", err)
			return
		}
		matches := 0
		for _, entry := range entries {
			if entry.Name == name && entry.Type == "dir" {
				matches++
			}
		}
		if matches != 0 {
			t.Errorf("Yandex Trash retains %d exact generated folders", matches)
		}
	}
}

func assertNoRealSemanticsArtifacts(t *testing.T, ctx context.Context, filesystem vfs.VFS, writeRoot string) {
	t.Helper()
	items, err := readRealSemanticsDirectory(ctx, filesystem, writeRoot)
	if err != nil {
		t.Fatalf("audit writable root for stale real semantics artifacts: %v", err)
	}
	normalCount := 0
	for _, item := range items {
		if item.IsDir && strings.HasPrefix(item.Name, realSemanticsPrefix) {
			normalCount++
		}
	}
	cloud := realSemanticsCloudVFS(t, filesystem)
	backend, err := cloud.backend()
	if err != nil {
		t.Fatalf("access backend for stale real semantics Trash audit: %v", err)
	}
	trashCount := 0
	switch concrete := backend.(type) {
	case *googleDriveBackend:
		result, listErr := concrete.service.Files.List().
			Q("trashed = true and name contains '" + escapeGoogleQuery(realSemanticsPrefix) + "'").
			Spaces("drive").PageSize(100).SupportsAllDrives(true).IncludeItemsFromAllDrives(true).
			Fields("files(id,name,mimeType,trashed)").Context(ctx).Do()
		if listErr != nil {
			t.Fatalf("audit Google Trash for stale real semantics artifacts: %v", mapGoogleError(listErr))
		}
		for _, file := range result.Files {
			if file.Trashed && file.MimeType == googleFolderMime && strings.HasPrefix(file.Name, realSemanticsPrefix) {
				trashCount++
			}
		}
	case *yandexDiskBackend:
		entries, listErr := realYandexTrashEntries(ctx, concrete)
		if listErr != nil {
			t.Fatalf("audit Yandex Trash for stale real semantics artifacts: %v", listErr)
		}
		for _, entry := range entries {
			if entry.Type == "dir" && strings.HasPrefix(entry.Name, realSemanticsPrefix) {
				trashCount++
			}
		}
	}
	t.Logf("preflight artifact audit: writable-root=%d trash=%d", normalCount, trashCount)
	if normalCount != 0 || trashCount != 0 {
		t.Fatalf("stale real semantics artifacts exist (writable-root=%d trash=%d)", normalCount, trashCount)
	}
}

func realGoogleTrashedFolders(ctx context.Context, backend *googleDriveBackend, name string) ([]*drive.File, error) {
	result, err := backend.service.Files.List().
		Q("trashed = true and name = '" + escapeGoogleQuery(name) + "'").
		Spaces("drive").PageSize(10).SupportsAllDrives(true).IncludeItemsFromAllDrives(true).
		Fields("files(id,name,mimeType,trashed)").Context(ctx).Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	files := make([]*drive.File, 0, len(result.Files))
	for _, file := range result.Files {
		if file.Name == name && file.MimeType == googleFolderMime && file.Trashed {
			files = append(files, file)
		}
	}
	return files, nil
}

func removeRealSemanticsNormalFolder(t *testing.T, ctx context.Context, filesystem vfs.VFS, writeRoot, name string) {
	t.Helper()
	if !strings.HasPrefix(name, realSemanticsPrefix) {
		t.Fatal("refusing unsafe real semantics permanent cleanup")
	}
	path, found, err := findRealSemanticsEntry(ctx, filesystem, writeRoot, name)
	if err != nil {
		t.Fatalf("locate generated folder for permanent cleanup: %v", err)
	}
	if !found {
		return
	}
	if err := filesystem.Remove(ctx, path); err != nil {
		t.Fatalf("permanently delete generated semantics folder: %v", err)
	}
	if _, found, err := findRealSemanticsEntry(ctx, filesystem, writeRoot, name); err != nil {
		t.Fatalf("verify generated folder permanent cleanup: %v", err)
	} else if found {
		t.Fatal("generated semantics folder remains after permanent cleanup")
	}
}

func cleanupRealSemanticsArtifacts(t *testing.T, plugin *Plugin, connectionID string, names []string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	connection, err := plugin.Repository().Get(ctx, connectionID)
	if err != nil {
		t.Errorf("reload connection for real semantics cleanup: %v", err)
		return
	}
	filesystem, writeRoot := openRealSemanticsRoot(t, ctx, plugin, connection)
	t.Cleanup(func() {
		if err := filesystem.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	for _, name := range names {
		if !strings.HasPrefix(name, realSemanticsPrefix) || strings.ContainsAny(name, "/\\") {
			t.Errorf("refusing unsafe real semantics cleanup name")
			continue
		}
		path, found, findErr := findRealSemanticsEntry(ctx, filesystem, writeRoot, name)
		if findErr != nil {
			t.Errorf("locate normal real semantics cleanup target: %v", findErr)
			continue
		}
		if found {
			if removeErr := filesystem.Remove(ctx, path); removeErr != nil {
				t.Errorf("remove normal real semantics cleanup target: %v", removeErr)
				continue
			}
		}
		if _, remains, verifyErr := findRealSemanticsEntry(ctx, filesystem, writeRoot, name); verifyErr != nil {
			t.Errorf("verify normal real semantics cleanup target: %v", verifyErr)
		} else if remains {
			t.Errorf("normal real semantics cleanup target remains")
		}
	}
	cloud := realSemanticsCloudVFS(t, filesystem)
	backend, err := cloud.backend()
	if err != nil {
		t.Errorf("access provider for real semantics Trash cleanup: %v", err)
		return
	}
	for _, name := range names {
		switch concrete := backend.(type) {
		case *googleDriveBackend:
			files, listErr := realGoogleTrashedFolders(ctx, concrete, name)
			if listErr != nil {
				t.Errorf("list Google Trash during cleanup: %v", listErr)
				continue
			}
			if len(files) > 1 {
				t.Errorf("refusing ambiguous Google Trash cleanup: %d exact folders", len(files))
				continue
			}
			if len(files) == 1 {
				if deleteErr := concrete.service.Files.Delete(files[0].Id).SupportsAllDrives(true).Context(ctx).Do(); deleteErr != nil {
					t.Errorf("permanently delete exact Google Trash cleanup target: %v", googleMutationError("real semantics cleanup", deleteErr))
				}
			}
		case *yandexDiskBackend:
			entries, listErr := realYandexTrashEntries(ctx, concrete)
			if listErr != nil {
				t.Errorf("list Yandex Trash during cleanup: %v", listErr)
				continue
			}
			matches := make([]yandexResource, 0, 1)
			for _, entry := range entries {
				if entry.Name == name && entry.Type == "dir" {
					matches = append(matches, entry)
				}
			}
			if len(matches) > 1 {
				t.Errorf("refusing ambiguous Yandex Trash cleanup: %d exact folders", len(matches))
				continue
			}
			if len(matches) == 1 {
				path, pathErr := realYandexTrashRelativePath(matches[0].Path)
				if pathErr != nil {
					t.Errorf("refusing invalid Yandex Trash cleanup identity: %v", pathErr)
					continue
				}
				if deleteErr := concrete.mutation(ctx, http.MethodDelete, "/trash/resources", url.Values{"path": {path}}); deleteErr != nil {
					t.Errorf("permanently delete exact Yandex Trash cleanup target: %v", deleteErr)
				}
			}
		}
		assertRealSemanticsTrashAbsent(t, ctx, backend, name)
	}
}
