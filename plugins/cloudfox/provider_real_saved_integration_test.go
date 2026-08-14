package cloudfox

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	realMutationEnv        = "F4_CLOUDFOX_REAL_MUTATION"
	realMutationConfirmed  = "CONFIRMED"
	realConfigDirEnv       = "F4_CLOUDFOX_REAL_CONFIG_DIR"
	realVaultPasswordEnv   = "F4_CLOUDFOX_REAL_VAULT_PASSWORD"
	realGoogleSelectorEnv  = "F4_CLOUDFOX_REAL_GOOGLE_CONNECTION"
	realYandexSelectorEnv  = "F4_CLOUDFOX_REAL_YANDEX_CONNECTION"
	realYandexDialRetryEnv = "F4_CLOUDFOX_REAL_YANDEX_DIAL_RETRIES"
	realLargeMiBEnv        = "F4_CLOUDFOX_REAL_LARGE_MIB"

	realFolderPrefix = "f4-cloudfox-real-test-"
)

// TestRealSavedCloudConnections exercises the real Google Drive and
// Yandex.Disk backends through credentials already saved by CloudFox.
//
// This test is intentionally impossible to enable by accident: it performs no
// config, keyring, vault or network access until F4_CLOUDFOX_REAL_MUTATION has
// the exact value CONFIRMED. The config directory must be supplied explicitly
// with F4_CLOUDFOX_REAL_CONFIG_DIR. An unset F4_CLOUDFOX_REAL_VAULT_PASSWORD
// means the intentionally empty vault password; setting it supplies a
// non-empty password without putting it on a command line.
//
// If more than one saved profile exists for a provider, select one by its ID or
// display name with F4_CLOUDFOX_REAL_GOOGLE_CONNECTION or
// F4_CLOUDFOX_REAL_YANDEX_CONNECTION. Values, settings, secret references and
// profile selectors are deliberately never printed by this test.
//
// Set F4_CLOUDFOX_REAL_LARGE_MIB to a positive number (for example 300) to add
// a large upload/download/server-copy verification. It is a separate subtest
// so the normal real-provider matrix remains reasonably quick.
//
// F4_CLOUDFOX_REAL_YANDEX_DIAL_RETRIES is a harness-only diagnostic escape
// hatch for unstable TCP connectivity. It defaults to zero, preserving exact
// production behavior. A positive value retries only DialContext calls which
// failed before returning a connection; it never retries an HTTP request,
// response or mutation.
func TestRealSavedCloudConnections(t *testing.T) {
	if os.Getenv(realMutationEnv) != realMutationConfirmed {
		t.Skip("real CloudFox mutations require explicit confirmation")
	}

	configDir := strings.TrimSpace(os.Getenv(realConfigDirEnv))
	if configDir == "" {
		t.Fatal("real CloudFox config directory is required")
	}
	if !filepath.IsAbs(configDir) {
		t.Fatal("real CloudFox config directory must be absolute")
	}
	info, err := os.Stat(configDir)
	if err != nil || !info.IsDir() {
		t.Fatal("real CloudFox config directory is unavailable")
	}

	// Both stores are injected explicitly. This prevents DefaultOptions from
	// silently choosing a different config location or prompting through vtui.
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
			factory, ok := plugin.Factory(target.provider)
			if !ok {
				t.Fatal("real CloudFox provider factory is unavailable")
			}
			if target.provider == ProviderYandexDisk {
				factory = realYandexFactoryWithDialRetries(t, factory)
			}

			t.Log("phase: open saved connection")
			openCtx, cancelOpen := context.WithTimeout(context.Background(), 2*time.Minute)
			secrets, err := plugin.Repository().Credentials(openCtx, connection)
			if err != nil {
				cancelOpen()
				t.Fatalf("unlock saved provider credentials: %v", err)
			}
			backend, err := factory.Open(openCtx, connection.Clone(), secrets.Clone())
			clearSecrets(secrets)
			cancelOpen()
			if err != nil {
				t.Fatalf("open saved provider connection: %v", err)
			}
			t.Cleanup(func() {
				if err := backend.Close(); err != nil {
					t.Errorf("close real provider backend: %v", err)
				}
			})

			runRealBackendMatrix(t, backend)
		})
	}
}

func selectRealConnection(t *testing.T, connections []Connection, provider ProviderType, selectorEnv string) Connection {
	t.Helper()
	selector := strings.TrimSpace(os.Getenv(selectorEnv))
	matches := make([]Connection, 0, 1)
	for _, connection := range connections {
		if connection.Provider != provider {
			continue
		}
		if selector == "" || strings.EqualFold(connection.ID, selector) || strings.EqualFold(connection.Name, selector) {
			matches = append(matches, connection.Clone())
		}
	}
	if len(matches) == 0 {
		if selector == "" {
			t.Fatal("no saved connection exists for this provider")
		}
		t.Fatal("the requested saved provider connection was not found")
	}
	if len(matches) != 1 {
		t.Fatalf("provider has %d saved connections; select exactly one with the provider selector environment variable", len(matches))
	}
	return matches[0]
}

// redactRealProviderError keeps provider failures useful in opt-in test logs
// while stripping signed URLs, opaque cloud identities and token-shaped values.
func redactRealProviderError(err error) string {
	if err == nil {
		return "none"
	}
	words := strings.Fields(strings.TrimSpace(err.Error()))
	redactNext := 0
	for index, word := range words {
		if redactNext > 0 {
			words[index] = "<redacted-secret>"
			redactNext--
			continue
		}
		lower := strings.ToLower(word)
		if strings.Contains(lower, "://") {
			words[index] = "<redacted-uri>"
			continue
		}
		for _, marker := range []string{"access_token", "refresh_token", "client_secret", "authorization", "oauth_token", "x-amz-signature", "x-goog-signature"} {
			if strings.Contains(lower, marker) {
				words[index] = "<redacted-secret>"
				redactNext = 2
				break
			}
		}
	}
	result := strings.Join(words, " ")
	if len(result) > 500 {
		result = result[:500] + "..."
	}
	return result
}

func TestRedactRealProviderError(t *testing.T) {
	got := redactRealProviderError(errors.New(`Put "https://upload.example/file?token=signed-value": Authorization: Bearer bearer-value access_token="body-value"`))
	for _, forbidden := range []string{"upload.example", "signed-value", "bearer-value", "body-value", "https://"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redacted real provider error retained sensitive marker %q", forbidden)
		}
	}
	if !strings.Contains(got, "<redacted-uri>") || !strings.Contains(got, "<redacted-secret>") {
		t.Fatal("redacted real provider error omitted its explicit redaction markers")
	}
}

func runRealBackendMatrix(t *testing.T, backend Backend) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	t.Log("phase: root navigation")
	root := backend.Root()
	normalizedRoot, err := backend.Normalize(root)
	if err != nil || normalizedRoot != root || !backend.IsRoot(root) {
		t.Fatalf("provider root normalization failed: %v", err)
	}
	rootStat, err := statRealReadOnly(ctx, backend, root)
	if err != nil {
		t.Fatalf("stat provider root: %v", err)
	}
	if !rootStat.IsDir {
		t.Fatal("provider root is not a directory")
	}
	rootEntries := readRealDirectory(t, ctx, backend, root)
	writeRoot := realWritableRoot(t, ctx, backend, root, rootEntries)
	if _, err := statRealReadOnly(ctx, backend, writeRoot); err != nil {
		t.Fatalf("stat provider writable root: %v", err)
	}
	_ = readRealDirectory(t, ctx, backend, writeRoot)

	t.Log("phase: create isolated workspace")
	uuid, err := newUUID()
	if err != nil {
		t.Fatalf("generate isolated real-test folder name: %v", err)
	}
	folderName := realFolderPrefix + strings.ReplaceAll(uuid, "-", "")
	folderCandidate := backend.Join(writeRoot, folderName)
	if folderCandidate == "" || folderCandidate == writeRoot || backend.IsRoot(folderCandidate) {
		t.Fatal("provider produced an unsafe real-test folder location")
	}

	workspace := ""
	creationTried := false
	t.Cleanup(func() {
		if !creationTried {
			return
		}
		cleanupRealWorkspace(t, backend, writeRoot, workspace, folderName)
	})
	creationTried = true
	if err := backend.MkDir(ctx, folderCandidate); err != nil {
		t.Fatalf("create isolated real-test folder: %v", err)
	}
	workspaceEntry, err := statRealReadOnly(ctx, backend, folderCandidate)
	if err != nil {
		t.Fatalf("stat isolated real-test folder: %v", err)
	}
	if !workspaceEntry.IsDir || workspaceEntry.Name != folderName || workspaceEntry.Location == "" {
		t.Fatal("provider returned unexpected isolated real-test folder metadata")
	}
	workspace = workspaceEntry.Location
	assertRealWorkspaceTarget(t, ctx, backend, writeRoot, workspace, folderName, workspaceEntry)

	t.Log("phase: directory and file CRUD")
	nestedCandidate := backend.Join(workspace, "nested")
	if err := backend.MkDir(ctx, nestedCandidate); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	nestedEntry, err := statRealReadOnly(ctx, backend, nestedCandidate)
	if err != nil || !nestedEntry.IsDir {
		t.Fatalf("stat nested directory: %v", err)
	}
	nested := nestedEntry.Location
	if nested == "" {
		t.Fatal("nested directory has no canonical location")
	}

	binaryInitial := realPatternBytes(64<<10, 0x41)
	binaryFinal := realPatternBytes(512<<10, 0xb7)
	files := []struct {
		name    string
		initial []byte
		final   []byte
		random  bool
	}{
		{name: "zero.bin", initial: nil, final: nil},
		{name: "text.txt", initial: []byte("CloudFox initial text\n"), final: []byte("CloudFox overwritten text\nsecond line\n")},
		{name: "юникод-文件-🙂.txt", initial: []byte("первоначальный текст\n"), final: []byte("Русский, 日本語, emoji 🙂 — overwritten\n")},
		{name: "binary.bin", initial: binaryInitial, final: binaryFinal, random: true},
	}
	finalLocations := make(map[string]string, len(files))
	for _, file := range files {
		destination := backend.Join(workspace, file.name)
		writeRealBytes(t, ctx, backend, destination, file.initial)
		initialEntry := statRealFile(t, ctx, backend, destination, int64(len(file.initial)))
		if initialEntry.Name != file.name {
			t.Fatal("created file has an unexpected name")
		}

		// Calling Create for the same destination is the provider overwrite
		// path used by the editor when it saves a changed remote file.
		writeRealBytes(t, ctx, backend, destination, file.final)
		entry := statRealFile(t, ctx, backend, destination, int64(len(file.final)))
		finalLocations[file.name] = entry.Location
		assertRealFullHash(t, ctx, backend, entry.Location, file.final)
		if file.random {
			assertRealRandomHash(t, ctx, backend, entry.Location, file.final)
		}
	}

	nestedPayload := []byte("nested CloudFox payload\n")
	nestedFile := backend.Join(nested, "child.txt")
	writeRealBytes(t, ctx, backend, nestedFile, nestedPayload)
	nestedFileEntry := statRealFile(t, ctx, backend, nestedFile, int64(len(nestedPayload)))
	assertRealFullHash(t, ctx, backend, nestedFileEntry.Location, nestedPayload)

	entries := readRealDirectory(t, ctx, backend, workspace)
	listed := make(map[string]RemoteEntry, len(entries))
	for _, entry := range entries {
		listed[entry.Name] = entry
	}
	for _, file := range files {
		entry, ok := listed[file.name]
		if !ok || entry.IsDir || entry.Location == "" {
			t.Fatal("created file is missing from directory listing")
		}
	}
	if entry, ok := listed["nested"]; !ok || !entry.IsDir || entry.Location == "" {
		t.Fatal("nested directory is missing from directory listing")
	}

	t.Log("phase: server-side copy and rename")
	capabilities := backend.Capabilities()
	if !capabilities.HasServerSideCopy {
		t.Fatal("provider does not advertise required server-side copy")
	}
	copier, ok := backend.(BackendCopier)
	if !ok {
		t.Fatal("provider advertises server-side copy without implementing it")
	}
	copyCandidate := backend.Join(workspace, "binary-copy.bin")
	if err := copier.Copy(ctx, finalLocations["binary.bin"], copyCandidate); err != nil {
		t.Fatalf("server-side copy: %v", err)
	}
	copyEntry := statRealFile(t, ctx, backend, copyCandidate, int64(len(binaryFinal)))
	assertRealFullHash(t, ctx, backend, copyEntry.Location, binaryFinal)

	renamedCandidate := backend.Join(workspace, "binary-renamed.bin")
	if err := backend.Rename(ctx, copyEntry.Location, renamedCandidate); err != nil {
		t.Fatalf("rename server-side copy: %v", err)
	}
	if _, err := statRealReadOnly(ctx, backend, copyCandidate); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old name after rename remains addressable: %v", err)
	}
	renamedEntry := statRealFile(t, ctx, backend, renamedCandidate, int64(len(binaryFinal)))
	assertRealRandomHash(t, ctx, backend, renamedEntry.Location, binaryFinal)

	if err := backend.SetAttributes(ctx, finalLocations["text.txt"], vfs.VFSItem{MTime: time.Now().Add(-time.Hour)}); !errors.Is(err, ErrUnsupportedOperation) {
		t.Fatalf("SetAttributes error = %v, want unsupported operation", err)
	}

	t.Log("phase: delete verification")
	if err := backend.Remove(ctx, renamedEntry.Location); err != nil {
		t.Fatalf("delete renamed file: %v", err)
	}
	if _, err := statRealReadOnly(ctx, backend, renamedCandidate); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted file is still addressable: %v", err)
	}

	deleteCandidate := backend.Join(workspace, "delete-me.txt")
	writeRealBytes(t, ctx, backend, deleteCandidate, []byte("delete verification\n"))
	deleteEntry := statRealFile(t, ctx, backend, deleteCandidate, int64(len("delete verification\n")))
	if err := backend.Remove(ctx, deleteEntry.Location); err != nil {
		t.Fatalf("delete ordinary file: %v", err)
	}
	if _, err := statRealReadOnly(ctx, backend, deleteCandidate); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary deleted file is still addressable: %v", err)
	}

	t.Run("large-file", func(t *testing.T) {
		t.Log("phase: optional large-file matrix")
		runRealLargeFile(t, backend, workspace, copier)
	})
}

func realYandexFactoryWithDialRetries(t *testing.T, factory BackendFactory) BackendFactory {
	t.Helper()
	rawRetries := strings.TrimSpace(os.Getenv(realYandexDialRetryEnv))
	if rawRetries == "" || rawRetries == "0" {
		return factory
	}
	retries, err := strconv.Atoi(rawRetries)
	if err != nil || retries < 1 || retries > 10 {
		t.Fatal("real Yandex dial retries must be an integer from 0 through 10")
	}
	yandexFactory, ok := factory.(*YandexDiskFactory)
	if !ok {
		t.Fatal("real Yandex provider has an unexpected factory type")
	}
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		t.Fatal("default HTTP transport has an unexpected type")
	}
	transport := defaultTransport.Clone()
	baseDial := transport.DialContext
	if baseDial == nil {
		dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
		baseDial = dialer.DialContext
	}
	transport.DialContext = realRetryTCPDialContext(baseDial, retries, 250*time.Millisecond)
	clone := *yandexFactory
	clone.HTTPClient = &http.Client{Transport: transport}
	t.Logf("harness-only Yandex TCP dial retries enabled: %d", retries)
	return &clone
}

type realDialContextFunc func(context.Context, string, string) (net.Conn, error)

func realRetryTCPDialContext(base realDialContextFunc, retries int, initialBackoff time.Duration) realDialContextFunc {
	if retries <= 0 {
		return base
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if !strings.HasPrefix(strings.ToLower(network), "tcp") {
			return base(ctx, network, address)
		}
		for attempt := 0; ; attempt++ {
			connection, err := base(ctx, network, address)
			// A returned connection may already be observable by the transport.
			// Never retry it, even if a non-conforming dialer also returned an
			// error. Retrying is safe only while no connection was established,
			// before the HTTP transport can send request bytes.
			if connection != nil || err == nil || attempt >= retries {
				return connection, err
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			delay := initialBackoff << min(attempt, 3)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}
}

func TestRealRetryTCPDialContextRetriesOnlyUnestablishedTCPConnections(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("dial failed")
	calls := 0
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	dial := realRetryTCPDialContext(func(context.Context, string, string) (net.Conn, error) {
		calls++
		if calls < 3 {
			return nil, wantErr
		}
		return client, nil
	}, 2, time.Millisecond)
	connection, err := dial(context.Background(), "tcp", "example.invalid:443")
	if err != nil || connection != client || calls != 3 {
		t.Fatalf("TCP retry result: connection=%t calls=%d error=%v", connection == client, calls, err)
	}

	calls = 0
	dial = realRetryTCPDialContext(func(context.Context, string, string) (net.Conn, error) {
		calls++
		return nil, wantErr
	}, 5, time.Millisecond)
	if _, err := dial(context.Background(), "udp", "example.invalid:443"); !errors.Is(err, wantErr) || calls != 1 {
		t.Fatalf("non-TCP dial was retried: calls=%d error=%v", calls, err)
	}

	calls = 0
	connected, peer := net.Pipe()
	defer connected.Close()
	defer peer.Close()
	dial = realRetryTCPDialContext(func(context.Context, string, string) (net.Conn, error) {
		calls++
		return connected, wantErr
	}, 5, time.Millisecond)
	connection, err = dial(context.Background(), "tcp4", "example.invalid:443")
	if connection != connected || !errors.Is(err, wantErr) || calls != 1 {
		t.Fatalf("established connection was retried: connection=%t calls=%d error=%v", connection == connected, calls, err)
	}
}

func realWritableRoot(t *testing.T, ctx context.Context, backend Backend, root string, entries []RemoteEntry) string {
	t.Helper()
	if backend.Root() != googleRootLocation {
		return root
	}
	for _, entry := range entries {
		if entry.Location == googleMyLocation && entry.IsDir {
			if _, err := statRealReadOnly(ctx, backend, entry.Location); err != nil {
				t.Fatalf("stat Google My Drive: %v", err)
			}
			return entry.Location
		}
	}
	t.Fatal("Google Drive root does not contain My Drive")
	return ""
}

func readRealDirectory(t *testing.T, ctx context.Context, backend Backend, location string) []RemoteEntry {
	t.Helper()
	var entries []RemoteEntry
	if err := readDirRealReadOnly(ctx, backend, location, func(chunk []RemoteEntry) {
		entries = append(entries, chunk...)
	}); err != nil {
		t.Fatalf("read provider directory: %v", err)
	}
	return entries
}

// The live Yandex endpoint occasionally drops a TCP connection before the
// first response. Retrying a read is safe; retrying a mutation is not. Keep the
// retry seam deliberately limited to these two read-only operations and to the
// concrete Yandex backend so no other provider's behavior is hidden.
func statRealReadOnly(ctx context.Context, backend Backend, location string) (RemoteEntry, error) {
	var entry RemoteEntry
	err := retryRealYandexRead(ctx, backend, func() error {
		var err error
		entry, err = backend.Stat(ctx, location)
		return err
	})
	return entry, err
}

func readDirRealReadOnly(ctx context.Context, backend Backend, location string, onChunk func([]RemoteEntry)) error {
	return retryRealYandexRead(ctx, backend, func() error {
		// A failed paginated read may already have delivered chunks. Buffer one
		// attempt and publish it only after the whole attempt succeeds so a retry
		// cannot duplicate entries in the caller.
		var attempt [][]RemoteEntry
		err := backend.ReadDir(ctx, location, func(chunk []RemoteEntry) {
			attempt = append(attempt, append([]RemoteEntry(nil), chunk...))
		})
		if err != nil {
			return err
		}
		for _, chunk := range attempt {
			onChunk(chunk)
		}
		return nil
	})
}

func retryRealYandexRead(ctx context.Context, backend Backend, operation func() error) error {
	const attempts = 3
	for attempt := 0; attempt < attempts; attempt++ {
		err := operation()
		if err == nil || !isRealYandexTCPFailure(backend, err) || attempt == attempts-1 {
			return err
		}
		delay := time.Duration(attempt+1) * 250 * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func isRealYandexTCPFailure(backend Backend, err error) bool {
	if _, ok := backend.(*yandexDiskBackend); !ok {
		return false
	}
	var networkError *net.OpError
	return errors.As(err, &networkError)
}

func writeRealBytes(t *testing.T, ctx context.Context, backend Backend, location string, payload []byte) {
	t.Helper()
	w, err := backend.Create(ctx, location)
	if err != nil {
		t.Fatalf("create remote file: %s", redactRealProviderError(err))
	}
	if len(payload) != 0 {
		if _, err := w.Write(payload); err != nil {
			_ = w.Close()
			t.Fatalf("write remote file: %s", redactRealProviderError(err))
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("commit remote file: %s", redactRealProviderError(err))
	}
}

func statRealFile(t *testing.T, ctx context.Context, backend Backend, location string, size int64) RemoteEntry {
	t.Helper()
	entry, err := statRealReadOnly(ctx, backend, location)
	if err != nil {
		t.Fatalf("stat remote file: %v", err)
	}
	if entry.IsDir || entry.Location == "" || entry.Size != size {
		t.Fatalf("remote file metadata mismatch: directory=%t size=%d, want size=%d", entry.IsDir, entry.Size, size)
	}
	return entry
}

func assertRealFullHash(t *testing.T, ctx context.Context, backend Backend, location string, expected []byte) {
	t.Helper()
	want := sha256.Sum256(expected)
	got, size, reportedSize, err := hashRealRemote(ctx, backend, location)
	if err != nil {
		t.Fatalf("read and hash remote file: %s", redactRealProviderError(err))
	}
	if got != want || size != int64(len(expected)) || reportedSize != int64(len(expected)) {
		t.Fatalf("remote full read mismatch: bytes=%d reported=%d want=%d hash-match=%t", size, reportedSize, len(expected), got == want)
	}
}

func hashRealRemote(ctx context.Context, backend Backend, location string) ([sha256.Size]byte, int64, int64, error) {
	reader, err := backend.Open(ctx, location)
	if err != nil {
		return [sha256.Size]byte{}, 0, 0, err
	}
	reportedSize := reader.Size()
	h := sha256.New()
	buffer := make([]byte, 1<<20)
	var total int64
	for {
		n, readErr := reader.Read(ctx, buffer)
		if n > 0 {
			_, _ = h.Write(buffer[:n])
			total += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = reader.Close()
			return [sha256.Size]byte{}, total, reportedSize, readErr
		}
		if n == 0 {
			_ = reader.Close()
			return [sha256.Size]byte{}, total, reportedSize, io.ErrNoProgress
		}
	}
	if err := reader.Close(); err != nil {
		return [sha256.Size]byte{}, total, reportedSize, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum, total, reportedSize, nil
}

func assertRealRandomHash(t *testing.T, ctx context.Context, backend Backend, location string, expected []byte) {
	t.Helper()
	if len(expected) < 8192 {
		t.Fatal("random-read fixture is too small")
	}
	type span struct{ offset, length int }
	spans := []span{
		{offset: 0, length: 257},
		{offset: len(expected)/3 + 17, length: 4093},
		{offset: len(expected) - 4096, length: 4096},
	}
	reader, err := backend.Open(ctx, location)
	if err != nil {
		t.Fatalf("open remote file for random reads: %s", redactRealProviderError(err))
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close random-read handle: %s", redactRealProviderError(err))
		}
	}()
	wantHash := sha256.New()
	gotHash := sha256.New()
	for _, span := range spans {
		buffer := make([]byte, span.length)
		n, readErr := reader.ReadAt(ctx, buffer, int64(span.offset))
		if n != len(buffer) || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			t.Fatalf("random remote read returned bytes=%d/%d: %s", n, len(buffer), redactRealProviderError(readErr))
		}
		_, _ = gotHash.Write(buffer[:n])
		_, _ = wantHash.Write(expected[span.offset : span.offset+span.length])
	}
	if !equalHash(gotHash, wantHash) {
		t.Fatal("random remote reads have an unexpected SHA-256 digest")
	}
}

func equalHash(first, second hash.Hash) bool {
	return string(first.Sum(nil)) == string(second.Sum(nil))
}

func realPatternBytes(size int, seed byte) []byte {
	payload := make([]byte, size)
	state := uint32(seed) + 1
	for i := range payload {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		payload[i] = byte(state)
	}
	return payload
}

func runRealLargeFile(t *testing.T, backend Backend, workspace string, copier BackendCopier) {
	t.Helper()
	rawSize := strings.TrimSpace(os.Getenv(realLargeMiBEnv))
	if rawSize == "" || rawSize == "0" {
		t.Skip("set the real large-file size environment variable to enable")
	}
	mib, err := strconv.ParseInt(rawSize, 10, 32)
	if err != nil || mib < 1 || mib > 4096 {
		t.Fatal("real large-file size must be an integer from 1 through 4096 MiB")
	}
	size := mib << 20
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	block := realPatternBytes(1<<20, 0xd3)
	originalCandidate := backend.Join(workspace, fmt.Sprintf("large-%d-mib.bin", mib))
	wantHash := writeRealPattern(t, ctx, backend, originalCandidate, size, block)
	original := statRealFile(t, ctx, backend, originalCandidate, size)
	assertRealHashValue(t, ctx, backend, original.Location, size, wantHash)
	assertRealPatternRandomReads(t, ctx, backend, original.Location, size, block)

	copyCandidate := backend.Join(workspace, fmt.Sprintf("large-%d-mib-copy.bin", mib))
	if err := copier.Copy(ctx, original.Location, copyCandidate); err != nil {
		t.Fatalf("server-side copy large file: %v", err)
	}
	copyEntry := statRealFile(t, ctx, backend, copyCandidate, size)
	assertRealHashValue(t, ctx, backend, copyEntry.Location, size, wantHash)
	assertRealPatternRandomReads(t, ctx, backend, copyEntry.Location, size, block)

	if err := backend.Remove(ctx, copyEntry.Location); err != nil {
		t.Fatalf("delete large copied file: %v", err)
	}
	if err := backend.Remove(ctx, original.Location); err != nil {
		t.Fatalf("delete large original file: %v", err)
	}
}

func writeRealPattern(t *testing.T, ctx context.Context, backend Backend, location string, size int64, block []byte) [sha256.Size]byte {
	t.Helper()
	w, err := backend.Create(ctx, location)
	if err != nil {
		t.Fatalf("create large remote file: %s", redactRealProviderError(err))
	}
	h := sha256.New()
	remaining := size
	for remaining > 0 {
		chunk := block
		if int64(len(chunk)) > remaining {
			chunk = chunk[:remaining]
		}
		if _, err := w.Write(chunk); err != nil {
			_ = w.Close()
			t.Fatalf("write large remote file: %s", redactRealProviderError(err))
		}
		_, _ = h.Write(chunk)
		remaining -= int64(len(chunk))
	}
	if err := w.Close(); err != nil {
		t.Fatalf("commit large remote file: %s", redactRealProviderError(err))
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

func assertRealHashValue(t *testing.T, ctx context.Context, backend Backend, location string, expectedSize int64, expectedHash [sha256.Size]byte) {
	t.Helper()
	got, size, reportedSize, err := hashRealRemote(ctx, backend, location)
	if err != nil {
		t.Fatalf("read and hash large remote file: %s", redactRealProviderError(err))
	}
	if got != expectedHash || size != expectedSize || reportedSize != expectedSize {
		t.Fatalf("large remote read mismatch: bytes=%d reported=%d want=%d hash-match=%t", size, reportedSize, expectedSize, got == expectedHash)
	}
}

func assertRealPatternRandomReads(t *testing.T, ctx context.Context, backend Backend, location string, size int64, block []byte) {
	t.Helper()
	offsets := []int64{0, size/2 + 31, size - 8192}
	reader, err := backend.Open(ctx, location)
	if err != nil {
		t.Fatalf("open large remote file for random reads: %s", redactRealProviderError(err))
	}
	defer func() {
		if err := reader.Close(); err != nil {
			t.Errorf("close large random-read handle: %s", redactRealProviderError(err))
		}
	}()
	wantHash := sha256.New()
	gotHash := sha256.New()
	for _, offset := range offsets {
		const length = 8192
		buffer := make([]byte, length)
		n, readErr := reader.ReadAt(ctx, buffer, offset)
		if n != length || (readErr != nil && !errors.Is(readErr, io.EOF)) {
			t.Fatalf("large random remote read returned bytes=%d/%d: %s", n, length, redactRealProviderError(readErr))
		}
		_, _ = gotHash.Write(buffer)
		for i := int64(0); i < length; i++ {
			_, _ = wantHash.Write(block[(offset+i)%int64(len(block)) : (offset+i)%int64(len(block))+1])
		}
	}
	if !equalHash(gotHash, wantHash) {
		t.Fatal("large random remote reads have an unexpected SHA-256 digest")
	}
}

func cleanupRealWorkspace(t *testing.T, backend Backend, writeRoot, workspace, folderName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var children []RemoteEntry
	if err := backend.ReadDir(ctx, writeRoot, func(chunk []RemoteEntry) {
		children = append(children, chunk...)
	}); err != nil {
		t.Errorf("list writable root during isolated real-test cleanup: %v", err)
		return
	}
	matches := make([]RemoteEntry, 0, 1)
	for _, child := range children {
		if child.Name == folderName && child.IsDir {
			matches = append(matches, child)
		}
	}
	if len(matches) == 0 {
		return
	}
	if len(matches) != 1 {
		t.Errorf("refusing ambiguous isolated real-test cleanup: %d exact folders", len(matches))
		return
	}
	entry := matches[0]
	if workspace != "" && entry.Location != workspace {
		t.Errorf("refusing isolated real-test cleanup after canonical identity changed")
		return
	}
	workspace = entry.Location
	assertRealWorkspaceTarget(t, ctx, backend, writeRoot, entry.Location, folderName, entry)
	if err := backend.Remove(ctx, entry.Location); err != nil {
		t.Errorf("remove isolated real-test folder: %v", err)
		return
	}
	// Do not prove deletion with Stat(canonical ID). Google currently retains
	// item metadata in its session cache, so such a Stat can succeed after the
	// exact remote item was permanently deleted. ReadDir is a fresh provider
	// query and proves that the exact name+identity pair left the writable root.
	children = readRealDirectory(t, ctx, backend, writeRoot)
	for _, child := range children {
		if child.Name == folderName && child.Location == workspace {
			t.Error("isolated real-test folder remains in writable root after cleanup")
			return
		}
	}
}

func assertRealWorkspaceTarget(t *testing.T, ctx context.Context, backend Backend, writeRoot, workspace, folderName string, entry RemoteEntry) {
	t.Helper()
	if folderName == "" || !strings.HasPrefix(folderName, realFolderPrefix) || entry.Name != folderName || !entry.IsDir {
		t.Fatal("refusing unsafe real-test cleanup target: folder identity mismatch")
	}
	if workspace == "" || workspace == writeRoot || backend.IsRoot(workspace) || entry.Location != workspace {
		t.Fatal("refusing unsafe real-test cleanup target: unsafe canonical location")
	}
	normalized, err := backend.Normalize(workspace)
	if err != nil || normalized != workspace {
		t.Fatalf("refusing unsafe real-test cleanup target: normalization failed: %v", err)
	}
	// Google Drive's canonical item has the account root item ID as its parent,
	// while the writable virtual root is g:my. Dir(actualItem) therefore cannot
	// prove panel ancestry. A fresh listing of the intended writable root can:
	// require the exact generated name, canonical identity and directory bit.
	children := readRealDirectory(t, ctx, backend, writeRoot)
	matches := 0
	for _, child := range children {
		if child.Name == folderName && child.Location == workspace && child.IsDir {
			matches++
		}
	}
	if matches != 1 {
		t.Fatal("refusing unsafe real-test cleanup target: writable-root membership mismatch")
	}
}
