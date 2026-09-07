package cloudfox

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	drive "google.golang.org/api/drive/v3"
)

const (
	realGoogleNativeEnv       = "F4_CLOUDFOX_REAL_GOOGLE_NATIVE"
	realGoogleNativeConfirmed = "CONFIRMED"
)

// TestRealSavedGoogleNativeObjects exercises Google Workspace exports and
// shortcuts through an already-saved CloudFox Google Drive connection.
//
// It is deliberately gated twice. Both F4_CLOUDFOX_REAL_MUTATION and
// F4_CLOUDFOX_REAL_GOOGLE_NATIVE must have the exact value CONFIRMED before
// this test reads configuration, unlocks credentials, performs network I/O or
// mutates Drive. F4_CLOUDFOX_REAL_CONFIG_DIR and the optional saved-connection
// selector/password variables have the same meanings as in
// TestRealSavedCloudConnections.
//
// Every mutation is confined to one uniquely named folder in My Drive. The
// cleanup guard verifies the generated name and canonical item ID against a
// fresh My Drive listing before deletion, then proves that exact pair is gone.
func TestRealSavedGoogleNativeObjects(t *testing.T) {
	if os.Getenv(realMutationEnv) != realMutationConfirmed {
		t.Skip("real CloudFox mutations require explicit confirmation")
	}
	if os.Getenv(realGoogleNativeEnv) != realGoogleNativeConfirmed {
		t.Skip("real Google Workspace mutations require separate explicit confirmation")
	}

	backend := openRealSavedGoogleBackend(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Log("phase: inspect Google Drive virtual roots")
	rootEntries := readRealDirectory(t, ctx, backend, googleRootLocation)
	rootByLocation := make(map[string]RemoteEntry, len(rootEntries))
	for _, entry := range rootEntries {
		rootByLocation[entry.Location] = entry
	}
	for _, expected := range []struct {
		location string
		name     string
	}{
		{location: googleMyLocation, name: "My Drive"},
		{location: googleSharedLocation, name: "Shared drives"},
	} {
		entry, ok := rootByLocation[expected.location]
		if !ok || !entry.IsDir || entry.Name != expected.name || entry.TransferName != expected.name {
			t.Fatalf("Google virtual root metadata mismatch for %q", expected.name)
		}
		stat, err := backend.Stat(ctx, expected.location)
		if err != nil || !stat.IsDir || stat.Name != expected.name {
			t.Fatalf("stat Google virtual root %q: %v", expected.name, err)
		}
	}
	sharedDrives := readRealDirectory(t, ctx, backend, googleSharedLocation)
	for _, entry := range sharedDrives {
		if !entry.IsDir || entry.Location == "" || entry.Name == "" || entry.TransferName != entry.Name {
			t.Fatal("Google shared-drive listing returned invalid metadata")
		}
		stat, err := backend.Stat(ctx, entry.Location)
		if err != nil || !stat.IsDir || stat.Name != entry.Name || stat.Location != entry.Location {
			t.Fatalf("stat listed Google shared drive: %v", err)
		}
	}

	t.Log("phase: create isolated Google Workspace folder")
	uuid, err := newUUID()
	if err != nil {
		t.Fatalf("generate isolated Google Workspace folder name: %v", err)
	}
	folderName := realFolderPrefix + "google-native-" + strings.ReplaceAll(uuid, "-", "")
	folderCandidate := backend.Join(googleMyLocation, folderName)
	if folderCandidate == "" || folderCandidate == googleMyLocation || backend.IsRoot(folderCandidate) {
		t.Fatal("provider produced an unsafe Google Workspace folder location")
	}
	// Register cleanup before the first mutation. MkDir can return an unknown
	// operation-state error after the server committed the folder; in that case
	// cleanup discovers only this UUID-qualified exact name in My Drive before
	// touching it.
	workspace := ""
	t.Cleanup(func() {
		cleanupRealGoogleNativeWorkspace(t, backend, workspace, folderName)
	})
	if err := backend.MkDir(ctx, folderCandidate); err != nil {
		t.Fatalf("create isolated Google Workspace folder: %v", err)
	}
	workspaceEntry := findRealGoogleListedEntry(t, ctx, backend, googleMyLocation, folderName)
	if !workspaceEntry.IsDir || workspaceEntry.IsSymlink || workspaceEntry.Location == "" {
		t.Fatal("isolated Google Workspace folder has invalid metadata")
	}
	workspace = workspaceEntry.Location
	assertRealWorkspaceTarget(t, ctx, backend, googleMyLocation, workspace, folderName, workspaceEntry)
	parsedWorkspace, err := parseGoogleLocation(workspace)
	if err != nil || parsedWorkspace.kind != "item" || parsedWorkspace.itemID == "" {
		t.Fatalf("isolated Google Workspace folder has invalid canonical identity: %v", err)
	}

	nativeSpecs := []realGoogleNativeSpec{
		{rawName: "native-document", mimeType: googleDocMime, extension: ".docx", requiredPart: "word/document.xml"},
		{rawName: "native-spreadsheet", mimeType: googleSheetMime, extension: ".xlsx", requiredPart: "xl/workbook.xml"},
		{rawName: "native-presentation", mimeType: googleSlidesMime, extension: ".pptx", requiredPart: "ppt/presentation.xml"},
	}

	t.Log("phase: create native Google Workspace objects and shortcut targets")
	for index := range nativeSpecs {
		created := createRealGoogleFile(t, ctx, backend, &drive.File{
			Name:     nativeSpecs[index].rawName,
			MimeType: nativeSpecs[index].mimeType,
			Parents:  []string{parsedWorkspace.itemID},
		})
		nativeSpecs[index].itemID = created.Id
	}

	filePayload := []byte("CloudFox real Google Drive shortcut target\n")
	fileTargetCandidate := backend.Join(workspace, "shortcut-target.txt")
	writeRealBytes(t, ctx, backend, fileTargetCandidate, filePayload)
	fileTarget := findRealGoogleListedEntry(t, ctx, backend, workspace, "shortcut-target.txt")
	parsedFileTarget := mustParseRealGoogleItem(t, fileTarget.Location)

	folderTargetCandidate := backend.Join(workspace, "shortcut-folder-target")
	if err := backend.MkDir(ctx, folderTargetCandidate); err != nil {
		t.Fatalf("create Google folder shortcut target: %v", err)
	}
	folderTarget := findRealGoogleListedEntry(t, ctx, backend, workspace, "shortcut-folder-target")
	if !folderTarget.IsDir || folderTarget.IsSymlink {
		t.Fatal("Google folder shortcut target has invalid metadata")
	}
	parsedFolderTarget := mustParseRealGoogleItem(t, folderTarget.Location)
	folderChildPayload := []byte("CloudFox folder shortcut child\n")
	writeRealBytes(t, ctx, backend, backend.Join(folderTarget.Location, "through-folder-shortcut.txt"), folderChildPayload)

	fileShortcut := createRealGoogleFile(t, ctx, backend, &drive.File{
		Name:     "file-shortcut.txt",
		MimeType: googleShortcutMime,
		Parents:  []string{parsedWorkspace.itemID},
		ShortcutDetails: &drive.FileShortcutDetails{
			TargetId: parsedFileTarget.itemID,
		},
	})
	folderShortcut := createRealGoogleFile(t, ctx, backend, &drive.File{
		Name:     "folder-shortcut",
		MimeType: googleShortcutMime,
		Parents:  []string{parsedWorkspace.itemID},
		ShortcutDetails: &drive.FileShortcutDetails{
			TargetId: parsedFolderTarget.itemID,
		},
	})

	t.Log("phase: verify native metadata, transfer names and OOXML exports")
	listed := indexRealGoogleEntries(readRealDirectory(t, ctx, backend, workspace))
	for index := range nativeSpecs {
		spec := &nativeSpecs[index]
		displayName := spec.rawName + spec.extension
		entry := requireRealGoogleEntry(t, listed, displayName)
		if entry.IsDir || entry.IsSymlink || entry.TransferName != displayName {
			t.Fatalf("native Google Workspace metadata mismatch for %q", displayName)
		}
		if backend.TransferName(entry.Location) != displayName {
			t.Fatalf("external transfer name mismatch for %q", displayName)
		}
		if backend.IntraSessionTransferName(entry.Location) != spec.rawName {
			t.Fatalf("intra-session transfer name mismatch for %q", displayName)
		}
		parsed := mustParseRealGoogleItem(t, entry.Location)
		if parsed.itemID != spec.itemID {
			t.Fatalf("native Google Workspace identity mismatch for %q", displayName)
		}
		metadata, err := backend.getFile(ctx, parsed.itemID)
		if err != nil || metadata.MimeType != spec.mimeType || metadata.Name != spec.rawName {
			t.Fatalf("native Google Workspace type mismatch for %q: %v", displayName, err)
		}
		assertRealGoogleOOXMLExport(t, ctx, backend, entry.Location, spec)
		spec.location = entry.Location
	}

	t.Log("phase: verify Google Drive shortcuts")
	listed = indexRealGoogleEntries(readRealDirectory(t, ctx, backend, workspace))
	fileShortcutEntry := requireRealGoogleEntry(t, listed, "file-shortcut.txt")
	assertRealGoogleShortcutMetadata(t, backend, fileShortcutEntry, fileShortcut.Id, parsedFileTarget.itemID, false)
	assertRealGoogleBytes(t, ctx, backend, fileShortcutEntry.Location, filePayload)

	folderShortcutEntry := requireRealGoogleEntry(t, listed, "folder-shortcut")
	assertRealGoogleShortcutMetadata(t, backend, folderShortcutEntry, folderShortcut.Id, parsedFolderTarget.itemID, true)
	shortcutChildren := indexRealGoogleEntries(readRealDirectory(t, ctx, backend, folderShortcutEntry.Location))
	shortcutChild := requireRealGoogleEntry(t, shortcutChildren, "through-folder-shortcut.txt")
	assertRealGoogleBytes(t, ctx, backend, shortcutChild.Location, folderChildPayload)

	t.Log("phase: verify server-side copies preserve native types")
	copier, ok := interface{}(backend).(BackendCopier)
	if !ok || !backend.Capabilities().HasServerSideCopy {
		t.Fatal("Google backend does not expose its advertised server-side copy")
	}
	for index := range nativeSpecs {
		spec := &nativeSpecs[index]
		copyRawName := spec.rawName + "-copy"
		copyDisplayName := copyRawName + spec.extension
		if err := copier.Copy(ctx, spec.location, backend.Join(workspace, copyDisplayName)); err != nil {
			t.Fatalf("copy native Google Workspace object %q: %v", spec.rawName, err)
		}
		copyEntry := findRealGoogleListedEntry(t, ctx, backend, workspace, copyDisplayName)
		parsedCopy := mustParseRealGoogleItem(t, copyEntry.Location)
		metadata, err := backend.getFile(ctx, parsedCopy.itemID)
		if err != nil || metadata.MimeType != spec.mimeType || metadata.Name != copyRawName {
			t.Fatalf("copied native Google Workspace type mismatch for %q: %v", copyDisplayName, err)
		}
		if copyEntry.TransferName != copyDisplayName || backend.TransferName(copyEntry.Location) != copyDisplayName || backend.IntraSessionTransferName(copyEntry.Location) != copyRawName {
			t.Fatalf("copied native Google Workspace transfer names mismatch for %q", copyDisplayName)
		}
		assertRealGoogleOOXMLExport(t, ctx, backend, copyEntry.Location, spec)
	}

	t.Log("phase: reject native-object overwrite without orphaning files")
	before := listRealGoogleChildren(t, ctx, backend, parsedWorkspace.itemID)
	for _, spec := range nativeSpecs {
		assertRealGoogleNativeCreateRejected(t, ctx, backend, spec.location)
		assertRealGoogleNativeCreateRejected(t, ctx, backend, backend.Join(workspace, spec.rawName))
	}
	after := listRealGoogleChildren(t, ctx, backend, parsedWorkspace.itemID)
	if !equalRealGoogleChildren(before, after) {
		t.Fatal("rejected native-object overwrite changed or orphaned a Google Drive child")
	}
	for _, spec := range nativeSpecs {
		metadata, err := backend.getFile(ctx, spec.itemID)
		if err != nil || metadata.MimeType != spec.mimeType || metadata.Name != spec.rawName {
			t.Fatalf("rejected overwrite changed native object %q: %v", spec.rawName, err)
		}
		assertRealGoogleOOXMLExport(t, ctx, backend, spec.location, &spec)
	}
}

type realGoogleNativeSpec struct {
	rawName      string
	mimeType     string
	extension    string
	requiredPart string
	itemID       string
	location     string
}

type realGoogleChild struct {
	name     string
	mimeType string
}

func cleanupRealGoogleNativeWorkspace(t *testing.T, backend *googleDriveBackend, workspace, folderName string) {
	t.Helper()
	if folderName == "" || !strings.HasPrefix(folderName, realFolderPrefix+"google-native-") {
		t.Error("refusing unsafe Google Workspace cleanup target: generated name mismatch")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var children []RemoteEntry
	if err := backend.ReadDir(ctx, googleMyLocation, func(chunk []RemoteEntry) {
		children = append(children, chunk...)
	}); err != nil {
		t.Errorf("list My Drive during Google Workspace cleanup: %v", err)
		return
	}
	matches := make([]RemoteEntry, 0, 1)
	for _, child := range children {
		if child.Name == folderName && child.IsDir && !child.IsSymlink {
			matches = append(matches, child)
		}
	}
	if len(matches) == 0 {
		if workspace != "" {
			t.Error("isolated Google Workspace folder disappeared before cleanup proof")
		}
		return
	}
	if len(matches) != 1 {
		t.Error("refusing unsafe Google Workspace cleanup target: exact-name membership mismatch")
		return
	}
	target := matches[0]
	if workspace != "" && target.Location != workspace {
		t.Error("refusing unsafe Google Workspace cleanup target: canonical identity mismatch")
		return
	}
	parsed, err := parseGoogleLocation(target.Location)
	if err != nil || parsed.kind != "item" || parsed.itemID == "" || target.Location == googleMyLocation || backend.IsRoot(target.Location) {
		t.Errorf("refusing unsafe Google Workspace cleanup target: invalid canonical identity: %v", err)
		return
	}
	if err := backend.Remove(ctx, target.Location); err != nil {
		t.Errorf("remove isolated Google Workspace folder: %v", err)
		return
	}
	children = children[:0]
	if err := backend.ReadDir(ctx, googleMyLocation, func(chunk []RemoteEntry) {
		children = append(children, chunk...)
	}); err != nil {
		t.Errorf("prove Google Workspace cleanup in My Drive: %v", err)
		return
	}
	for _, child := range children {
		if child.Name == folderName || child.Location == target.Location {
			t.Error("isolated Google Workspace folder remains after exact name+identity cleanup")
			return
		}
	}
}

func openRealSavedGoogleBackend(t *testing.T) *googleDriveBackend {
	t.Helper()
	configDir := strings.TrimSpace(os.Getenv(realConfigDirEnv))
	if configDir == "" {
		t.Fatal("real CloudFox config directory is required")
	}
	if !filepath.IsAbs(configDir) {
		t.Fatal("real CloudFox config directory must be absolute")
	}
	info, err := os.Stat(configDir) // #nosec G703 -- the opted-in real-provider test intentionally loads the operator-supplied absolute config directory.
	if err != nil || !info.IsDir() {
		t.Fatal("real CloudFox config directory is unavailable")
	}

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
	})
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close real CloudFox plugin: %v", err)
		}
	})

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	connections, err := plugin.Repository().List(loadCtx)
	cancelLoad()
	if err != nil {
		t.Fatalf("load saved CloudFox connections: %v", err)
	}
	connection := selectRealConnection(t, connections, ProviderGoogleDrive, realGoogleSelectorEnv)
	factory, ok := plugin.Factory(ProviderGoogleDrive)
	if !ok {
		t.Fatal("real Google Drive provider factory is unavailable")
	}
	openCtx, cancelOpen := context.WithTimeout(context.Background(), 2*time.Minute)
	secrets, err := plugin.Repository().Credentials(openCtx, connection)
	if err != nil {
		cancelOpen()
		t.Fatalf("unlock saved Google Drive credentials: %v", err)
	}
	opened, err := factory.Open(openCtx, connection.Clone(), secrets.Clone())
	clearSecrets(secrets)
	cancelOpen()
	if err != nil {
		t.Fatalf("open saved Google Drive connection: %v", err)
	}
	backend, ok := opened.(*googleDriveBackend)
	if !ok {
		_ = opened.Close()
		t.Fatal("saved Google Drive connection returned an unexpected backend type")
	}
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close real Google Drive backend: %v", err)
		}
	})
	return backend
}

func createRealGoogleFile(t *testing.T, ctx context.Context, backend *googleDriveBackend, metadata *drive.File) *drive.File {
	t.Helper()
	created, err := backend.service.Files.Create(metadata).
		SupportsAllDrives(true).
		Fields(googleFileFields).
		Context(ctx).
		Do()
	if err != nil {
		t.Fatalf("create isolated Google Drive test object: %v", err)
	}
	if created == nil || created.Id == "" || created.Name == "" || created.MimeType == "" {
		t.Fatal("Google Drive returned incomplete metadata for a created test object")
	}
	return created
}

func findRealGoogleListedEntry(t *testing.T, ctx context.Context, backend Backend, parent, name string) RemoteEntry {
	t.Helper()
	matches := make([]RemoteEntry, 0, 1)
	for _, entry := range readRealDirectory(t, ctx, backend, parent) {
		if entry.Name == name {
			matches = append(matches, entry)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("Google Drive listing contains %d entries named %q, want exactly one", len(matches), name)
	}
	return matches[0]
}

func indexRealGoogleEntries(entries []RemoteEntry) map[string][]RemoteEntry {
	indexed := make(map[string][]RemoteEntry, len(entries))
	for _, entry := range entries {
		indexed[entry.Name] = append(indexed[entry.Name], entry)
	}
	return indexed
}

func requireRealGoogleEntry(t *testing.T, indexed map[string][]RemoteEntry, name string) RemoteEntry {
	t.Helper()
	matches := indexed[name]
	if len(matches) != 1 {
		t.Fatalf("Google Drive listing contains %d entries named %q, want exactly one", len(matches), name)
	}
	if matches[0].Location == "" {
		t.Fatalf("Google Drive entry %q has no canonical location", name)
	}
	return matches[0]
}

func mustParseRealGoogleItem(t *testing.T, location string) googleLocation {
	t.Helper()
	parsed, err := parseGoogleLocation(location)
	if err != nil || parsed.kind != "item" || parsed.itemID == "" {
		t.Fatalf("Google Drive item has invalid canonical location: %v", err)
	}
	return parsed
}

func assertRealGoogleOOXMLExport(t *testing.T, ctx context.Context, backend Backend, location string, spec *realGoogleNativeSpec) {
	t.Helper()
	payload := readRealGoogleBytes(t, ctx, backend, location)
	if len(payload) < 4 || !bytes.Equal(payload[:4], []byte{'P', 'K', 3, 4}) {
		t.Fatalf("Google Workspace %s export does not have an OOXML ZIP signature", spec.extension)
	}
	archive, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatalf("open Google Workspace %s export as OOXML ZIP: %v", spec.extension, err)
	}
	parts := make(map[string]bool, len(archive.File))
	for _, file := range archive.File {
		parts[file.Name] = true
	}
	if !parts["[Content_Types].xml"] || !parts[spec.requiredPart] {
		t.Fatalf("Google Workspace %s export is missing required OOXML parts", spec.extension)
	}

	reader, err := backend.Open(ctx, location)
	if err != nil {
		t.Fatalf("reopen Google Workspace %s export for random reads: %v", spec.extension, err)
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close Google Workspace random-read handle: %v", err)
		}
	}()
	if reader.Size() != int64(len(payload)) {
		t.Fatalf("Google Workspace %s export size changed between opens", spec.extension)
	}
	offsets := []int64{0, int64(len(payload) / 2), int64(max(0, len(payload)-257))}
	for _, offset := range offsets {
		length := min(257, len(payload)-int(offset))
		if length <= 0 {
			continue
		}
		buffer := make([]byte, length)
		n, readErr := reader.ReadAt(ctx, buffer, offset)
		if n != length || (readErr != nil && !errors.Is(readErr, io.EOF)) || !bytes.Equal(buffer[:n], payload[offset:offset+int64(n)]) {
			t.Fatalf("Google Workspace %s random read mismatch at offset %d: bytes=%d/%d error=%v", spec.extension, offset, n, length, readErr)
		}
	}
}

func readRealGoogleBytes(t *testing.T, ctx context.Context, backend Backend, location string) []byte {
	t.Helper()
	reader, err := backend.Open(ctx, location)
	if err != nil {
		t.Fatalf("open Google Drive object: %v", err)
	}
	reportedSize := reader.Size()
	var payload bytes.Buffer
	buffer := make([]byte, 256<<10)
	for {
		n, readErr := reader.Read(ctx, buffer)
		if n > 0 {
			_, _ = payload.Write(buffer[:n])
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = reader.Close()
			t.Fatalf("read Google Drive object: %v", readErr)
		}
		if n == 0 {
			_ = reader.Close()
			t.Fatal("read Google Drive object made no progress")
		}
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close Google Drive object: %v", err)
	}
	if reportedSize != int64(payload.Len()) {
		t.Fatalf("Google Drive object reported %d bytes but returned %d", reportedSize, payload.Len())
	}
	return payload.Bytes()
}

func assertRealGoogleBytes(t *testing.T, ctx context.Context, backend Backend, location string, expected []byte) {
	t.Helper()
	payload := readRealGoogleBytes(t, ctx, backend, location)
	if !bytes.Equal(payload, expected) {
		t.Fatal("Google Drive shortcut resolved to unexpected bytes")
	}
	reader, err := backend.Open(ctx, location)
	if err != nil {
		t.Fatalf("reopen Google Drive shortcut target: %v", err)
	}
	defer func() { _ = reader.Close() }()
	if len(expected) != 0 {
		buffer := make([]byte, min(17, len(expected)))
		n, readErr := reader.ReadAt(ctx, buffer, int64(len(expected)-len(buffer)))
		if n != len(buffer) || (readErr != nil && !errors.Is(readErr, io.EOF)) || !bytes.Equal(buffer[:n], expected[len(expected)-len(buffer):]) {
			t.Fatalf("Google Drive shortcut random read mismatch: bytes=%d/%d error=%v", n, len(buffer), readErr)
		}
	}
}

func assertRealGoogleShortcutMetadata(t *testing.T, backend *googleDriveBackend, entry RemoteEntry, shortcutID, targetID string, directory bool) {
	t.Helper()
	if !entry.IsSymlink || entry.IsDir != directory || entry.TransferName != entry.Name || backend.TransferName(entry.Location) != entry.Name {
		t.Fatalf("Google Drive shortcut metadata mismatch for %q", entry.Name)
	}
	parsed, err := parseGoogleLocation(entry.Location)
	if err != nil || parsed.kind != "shortcut" || parsed.itemID != shortcutID || parsed.targetID != targetID {
		t.Fatalf("Google Drive shortcut canonical identity mismatch for %q: %v", entry.Name, err)
	}
}

func assertRealGoogleNativeCreateRejected(t *testing.T, ctx context.Context, backend Backend, location string) {
	t.Helper()
	writer, err := backend.Create(ctx, location)
	if writer != nil {
		_ = writer.Close()
	}
	if !errors.Is(err, ErrReadOnlyObject) {
		t.Errorf("native Google Workspace Create error = %v, want ErrReadOnlyObject", err)
	}
}

func listRealGoogleChildren(t *testing.T, ctx context.Context, backend *googleDriveBackend, parentID string) map[string]realGoogleChild {
	t.Helper()
	children := make(map[string]realGoogleChild)
	pageToken := ""
	for {
		call := backend.service.Files.List().
			Q("'" + escapeGoogleQuery(parentID) + "' in parents and trashed = false").
			Spaces("drive").
			PageSize(1000).
			SupportsAllDrives(true).
			IncludeItemsFromAllDrives(true).
			Fields("nextPageToken,files(id,name,mimeType)").
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		result, err := call.Do()
		if err != nil {
			t.Fatalf("list isolated Google Drive test children: %v", err)
		}
		for _, file := range result.Files {
			if file.Id == "" {
				t.Fatal("Google Drive child has no canonical ID")
			}
			children[file.Id] = realGoogleChild{name: file.Name, mimeType: file.MimeType}
		}
		if result.NextPageToken == "" {
			return children
		}
		pageToken = result.NextPageToken
	}
}

func equalRealGoogleChildren(first, second map[string]realGoogleChild) bool {
	if len(first) != len(second) {
		return false
	}
	for id, child := range first {
		if second[id] != child {
			return false
		}
	}
	return true
}
