package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/plugins/cloudfox"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

const (
	realCloudFoxUIEnv        = "F4_CLOUDFOX_REAL_UI"
	realCloudFoxUIConfirmed  = "CONFIRMED"
	realCloudFoxConfigEnv    = "F4_CLOUDFOX_REAL_CONFIG_DIR"
	realCloudFoxVaultEnv     = "F4_CLOUDFOX_REAL_VAULT_PASSWORD"
	realCloudFoxGoogleEnv    = "F4_CLOUDFOX_REAL_GOOGLE_CONNECTION"
	realCloudFoxYandexEnv    = "F4_CLOUDFOX_REAL_YANDEX_CONNECTION"
	realCloudFoxDialRetryEnv = "F4_CLOUDFOX_REAL_YANDEX_DIAL_RETRIES"

	realCloudFoxUIFolderPrefix = "f4-cloudfox-real-ui-test-"
)

// TestRealSavedCloudConnectionsUI exercises saved Google Drive and
// Yandex.Disk profiles through the same ManagerVFS, VFSProvider, CloudVFS,
// viewer, editor and F5 action code used by the interactive application.
//
// It is deliberately stricter to enable than an ordinary integration test:
// no profile, vault, keyring or network access happens unless
// F4_CLOUDFOX_REAL_UI has the exact value CONFIRMED and an absolute profile
// directory is supplied. The test never starts a native UI. All f4 frames,
// progress windows and errors are hosted by vtui.NewSilentScreenBuf.
//
// F4_CLOUDFOX_REAL_VAULT_PASSWORD may be left unset for the intentionally
// empty vault password. When several saved profiles exist for one provider,
// select exactly one by ID or display name with the provider-specific
// selector variable. Selectors, settings, secret references and credentials
// are never logged.
//
// F4_CLOUDFOX_REAL_YANDEX_DIAL_RETRIES is a harness-only diagnostic escape
// hatch for unstable TCP connectivity. It defaults to zero, preserving the
// exact production transport. A positive value retries only DialContext calls
// which failed before returning a connection; it never retries an HTTP
// request, response or mutation.
func TestRealSavedCloudConnectionsUI(t *testing.T) {
	if os.Getenv(realCloudFoxUIEnv) != realCloudFoxUIConfirmed {
		t.Skip("real CloudFox UI mutations require explicit confirmation")
	}

	configDir := strings.TrimSpace(os.Getenv(realCloudFoxConfigEnv))
	if configDir == "" {
		t.Fatal("real CloudFox UI config directory is required")
	}
	if !filepath.IsAbs(configDir) {
		t.Fatal("real CloudFox UI config directory must be absolute")
	}
	info, err := os.Stat(configDir)
	if err != nil || !info.IsDir() {
		t.Fatal("real CloudFox UI config directory is unavailable")
	}

	// Make every production UI route deterministic and terminal-only. The
	// root package TestMain also suppresses external/native helpers, but this
	// test does not rely on any of those routes in the first place.
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
	originalConfig := AppConfig
	t.Cleanup(func() { AppConfig = originalConfig })
	AppConfig.EditorHighlighter = "None"
	AppConfig.EditorUseEditorConfig = false
	AppConfig.EditorAutodetectCodePage = false
	AppConfig.EditorDefaultCodePage = 65001
	AppConfig.ViewerAutodetectCodePage = false
	AppConfig.ViewerDefaultCodePage = 65001
	AppConfig.ConfirmCopy = false
	AppConfig.DefaultFileOpMode = 2 // production foreground progress route

	prompt := cloudfox.MasterPasswordPromptFunc(func(ctx context.Context, _ bool) (string, error) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		return os.Getenv(realCloudFoxVaultEnv), nil
	})
	pluginOptions := cloudfox.Options{
		ConfigDir:      configDir,
		Portable:       true,
		Keyring:        cloudfox.NewKeyringStore(),
		PasswordPrompt: prompt,
	}
	pluginOptions.Factories = realCloudFoxUIFactories(t)
	plugin := cloudfox.NewPlugin(pluginOptions)
	t.Cleanup(func() {
		if err := plugin.Close(); err != nil {
			t.Errorf("close real CloudFox UI plugin: %v", err)
		}
	})

	host := newRealCloudFoxUIHost()
	if err := plugin.Init(host); err != nil {
		t.Fatalf("initialize production CloudFox registrations: %v", err)
	}
	if host.driveFactory == nil || host.uriProvider == nil || len(host.vfsProviders) == 0 {
		t.Fatal("CloudFox did not register its production drive providers")
	}

	loadCtx, cancelLoad := context.WithTimeout(context.Background(), 30*time.Second)
	connections, err := plugin.Repository().List(loadCtx)
	cancelLoad()
	if err != nil {
		t.Fatalf("load saved CloudFox profiles: %v", err)
	}

	targets := []struct {
		name        string
		provider    cloudfox.ProviderType
		selectorEnv string
	}{
		{name: "google-drive", provider: cloudfox.ProviderGoogleDrive, selectorEnv: realCloudFoxGoogleEnv},
		{name: "yandex-disk", provider: cloudfox.ProviderYandexDisk, selectorEnv: realCloudFoxYandexEnv},
	}
	for _, target := range targets {
		target := target
		t.Run(target.name, func(t *testing.T) {
			connection := selectRealCloudFoxUIConnection(t, connections, target.provider, target.selectorEnv)
			runRealCloudFoxUIProvider(t, host, connection)
		})
	}
}

type realCloudFoxUIHost struct {
	driveFactory  func() vfs.VFS
	uriProvider   vfs.URIProvider
	vfsProviders  []vfs.VFSProvider
	driveNameSeen string
}

func newRealCloudFoxUIHost() *realCloudFoxUIHost { return &realCloudFoxUIHost{} }

func (*realCloudFoxUIHost) GetVersion() string                           { return "real-ui-test" }
func (*realCloudFoxUIHost) Log(string)                                   {}
func (*realCloudFoxUIHost) Message(string)                               {}
func (*realCloudFoxUIHost) RegisterHighlighter(vtui.HighlighterProvider) {}
func (h *realCloudFoxUIHost) RegisterVFSProvider(p vfs.VFSProvider) {
	h.vfsProviders = append(h.vfsProviders, p)
}
func (h *realCloudFoxUIHost) RegisterURIProvider(p vfs.URIProvider) error {
	if p == nil || h.uriProvider != nil {
		return errors.New("real CloudFox UI host rejected duplicate or nil URI provider")
	}
	h.uriProvider = p
	return nil
}
func (h *realCloudFoxUIHost) RegisterDrive(name string, factory func() vfs.VFS) {
	h.driveNameSeen = name
	h.driveFactory = factory
}
func (*realCloudFoxUIHost) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {}
func (*realCloudFoxUIHost) RegisterPluginMenuItem(string, func(vfs.App))                        {}
func (*realCloudFoxUIHost) RunAction(string) bool                                               { return false }

func selectRealCloudFoxUIConnection(t *testing.T, connections []cloudfox.Connection, provider cloudfox.ProviderType, selectorEnv string) cloudfox.Connection {
	t.Helper()
	selector := strings.TrimSpace(os.Getenv(selectorEnv))
	matches := make([]cloudfox.Connection, 0, 1)
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
		t.Fatalf("provider has %d saved connections; select exactly one with its selector environment variable", len(matches))
	}
	return matches[0]
}

// redactRealCloudFoxError preserves useful provider diagnostics without ever
// emitting temporary signed URLs, opaque cloud identities or token-shaped
// values which an HTTP error body may contain.
func redactRealCloudFoxError(err error) string {
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

func TestRedactRealCloudFoxError(t *testing.T) {
	got := redactRealCloudFoxError(errors.New(`Put "https://upload.example/file?token=signed-value": Authorization: Bearer bearer-value access_token="body-value"`))
	for _, forbidden := range []string{"upload.example", "signed-value", "bearer-value", "body-value", "https://"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("redacted real CloudFox error retained sensitive marker %q", forbidden)
		}
	}
	if !strings.Contains(got, "<redacted-uri>") || !strings.Contains(got, "<redacted-secret>") {
		t.Fatal("redacted real CloudFox error omitted its explicit redaction markers")
	}
}

// realCloudFoxUIFactories returns nil for the default, exact production
// transport. NewPlugin interprets nil as its ordinary built-in factory set.
// Only the explicit diagnostic opt-in constructs an otherwise identical set
// whose Yandex factory owns a dial-retrying HTTP transport.
func realCloudFoxUIFactories(t *testing.T) []cloudfox.BackendFactory {
	t.Helper()
	rawRetries := strings.TrimSpace(os.Getenv(realCloudFoxDialRetryEnv))
	if rawRetries == "" || rawRetries == "0" {
		return nil
	}
	retries, err := strconv.Atoi(rawRetries)
	if err != nil || retries < 1 || retries > 10 {
		t.Fatal("real Yandex UI dial retries must be an integer from 0 through 10")
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
	transport.DialContext = realCloudFoxUIRetryTCPDialContext(baseDial, retries, 250*time.Millisecond)
	yandex := &cloudfox.YandexDiskFactory{HTTPClient: &http.Client{Transport: transport}}
	return []cloudfox.BackendFactory{
		&cloudfox.GoogleDriveFactory{},
		yandex,
		&cloudfox.S3Factory{},
		&cloudfox.WebDAVFactory{},
	}
}

type realCloudFoxUIDialContextFunc func(context.Context, string, string) (net.Conn, error)

func realCloudFoxUIRetryTCPDialContext(base realCloudFoxUIDialContextFunc, retries int, initialBackoff time.Duration) realCloudFoxUIDialContextFunc {
	if retries <= 0 {
		return base
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if !strings.HasPrefix(strings.ToLower(network), "tcp") {
			return base(ctx, network, address)
		}
		for attempt := 0; ; attempt++ {
			connection, err := base(ctx, network, address)
			// Once a connection exists, the transport may make the request
			// observable. Never retry that result, even if a non-conforming
			// dialer returned both a connection and an error.
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

func TestRealCloudFoxUIRetryTCPDialContextStopsBeforeHTTPRequestCanExist(t *testing.T) {
	wantErr := errors.New("test dial failure")
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	calls := 0
	dial := realCloudFoxUIRetryTCPDialContext(func(context.Context, string, string) (net.Conn, error) {
		calls++
		if calls < 3 {
			return nil, wantErr
		}
		return client, nil
	}, 2, 0)
	connection, err := dial(context.Background(), "tcp", "unused")
	if err != nil || connection != client || calls != 3 {
		t.Fatalf("TCP dial result connection=%t calls=%d err=%v", connection == client, calls, err)
	}

	secondClient, secondServer := net.Pipe()
	t.Cleanup(func() {
		_ = secondClient.Close()
		_ = secondServer.Close()
	})
	calls = 0
	dial = realCloudFoxUIRetryTCPDialContext(func(context.Context, string, string) (net.Conn, error) {
		calls++
		return secondClient, wantErr
	}, 10, 0)
	connection, err = dial(context.Background(), "tcp", "unused")
	if connection != secondClient || !errors.Is(err, wantErr) || calls != 1 {
		t.Fatalf("established dial was retried: connection=%t calls=%d err-match=%t", connection == secondClient, calls, errors.Is(err, wantErr))
	}

	calls = 0
	dial = realCloudFoxUIRetryTCPDialContext(func(context.Context, string, string) (net.Conn, error) {
		calls++
		return nil, wantErr
	}, 10, 0)
	if _, err := dial(context.Background(), "udp", "unused"); !errors.Is(err, wantErr) || calls != 1 {
		t.Fatalf("non-TCP dial was retried: calls=%d err-match=%t", calls, errors.Is(err, wantErr))
	}
}

func runRealCloudFoxUIProvider(t *testing.T, host *realCloudFoxUIHost, connection cloudfox.Connection) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	// Open the profile exactly as a user does from the CloudFox drive: list
	// ManagerVFS first (which establishes the displayed-row identity), then
	// let the registered production VFSProvider mount the connection.
	manager := host.driveFactory()
	if manager == nil {
		t.Fatal("production CloudFox drive factory returned nil")
	}
	t.Cleanup(func() { _ = manager.Close() })
	managerItems, err := readRealCloudFoxUIDir(ctx, manager, manager.GetPath(), connection.Provider)
	if err != nil {
		t.Fatalf("list production CloudFox connection manager: %v", err)
	}
	foundRow := false
	for _, item := range managerItems {
		if item.Name == connection.Name && item.IsDir {
			foundRow = true
			break
		}
	}
	if !foundRow {
		t.Fatal("saved connection is not exposed as a folder by the CloudFox manager")
	}

	connectionPath := manager.Join(manager.GetPath(), connection.Name)
	var mounted vfs.VFS
	for _, provider := range host.vfsProviders {
		if provider.CanOpen(ctx, manager, connectionPath) {
			mounted, err = provider.Open(ctx, manager, connectionPath)
			break
		}
	}
	if err != nil {
		t.Fatalf("open saved connection through production VFS provider: %v", err)
	}
	if mounted == nil {
		t.Fatal("no production VFS provider accepted the saved connection folder")
	}
	t.Cleanup(func() { _ = mounted.Close() })
	if !strings.HasPrefix(mounted.GetPath(), connection.Name+":") || strings.Contains(mounted.GetPath(), "cloud://") || strings.Contains(strings.ToLower(mounted.GetPath()), strings.ToLower(connection.ID)) {
		t.Fatalf("mounted CloudVFS exposed a non-visual path")
	}

	writeRoot := mounted
	if connection.Provider == cloudfox.ProviderGoogleDrive {
		rootItems, err := readRealCloudFoxUIDir(ctx, writeRoot, writeRoot.GetPath(), connection.Provider)
		if err != nil {
			t.Fatalf("list Google Drive virtual root: %v", err)
		}
		myDrivePath := ""
		for _, item := range rootItems {
			if item.Name != "My Drive" || !item.IsDir {
				continue
			}
			myDrivePath = writeRoot.Join(writeRoot.GetPath(), item.Name)
			break
		}
		if myDrivePath == "" {
			t.Fatal("Google Drive virtual root does not expose canonical My Drive")
		}
		if err := writeRoot.SetPath(myDrivePath); err != nil {
			t.Fatalf("enter Google My Drive through CloudVFS: %v", err)
		}
	}
	if _, err := statRealCloudFoxUI(ctx, writeRoot, writeRoot.GetPath(), connection.Provider); err != nil {
		t.Fatalf("stat writable cloud root: %v", err)
	}

	folderName := realCloudFoxUIFolderPrefix + string(connection.Provider) + "-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	workspaceCandidate := writeRoot.Join(writeRoot.GetPath(), folderName)
	if workspaceCandidate == "" || workspaceCandidate == writeRoot.GetPath() || !strings.HasPrefix(folderName, realCloudFoxUIFolderPrefix) {
		t.Fatal("CloudVFS produced an unsafe real UI workspace target")
	}

	workspacePath := ""
	creationTried := false
	t.Cleanup(func() {
		if creationTried {
			cleanupRealCloudFoxUIWorkspace(t, writeRoot, connection.Provider, folderName, workspacePath)
		}
	})
	creationTried = true
	if err := writeRoot.MkDir(ctx, workspaceCandidate); err != nil {
		// A timed-out mutation may have reached the service. Discovering the
		// exact unique row is read-only and lets cleanup remain safe even while
		// the original mutation error is reported.
		if discovered, ok := findRealCloudFoxUIWorkspace(ctx, writeRoot, connection.Provider, folderName); ok {
			workspacePath = discovered
		}
		t.Fatalf("create isolated real UI workspace: %v", err)
	}
	workspacePath = requireRealCloudFoxUIWorkspace(t, ctx, writeRoot, connection.Provider, folderName)
	workspaceStat, err := statRealCloudFoxUI(ctx, writeRoot, workspacePath, connection.Provider)
	if err != nil || !workspaceStat.IsDir || workspaceStat.Name != folderName {
		t.Fatalf("stat canonical isolated real UI workspace: %v", err)
	}

	workspace := writeRoot.Clone()
	if workspace == nil || workspace == writeRoot {
		t.Fatal("CloudVFS did not provide an independent workspace clone")
	}
	t.Cleanup(func() { _ = workspace.Close() })
	if err := workspace.SetPath(workspacePath); err != nil {
		t.Fatalf("enter isolated real UI workspace: %v", err)
	}
	workspacePath = workspace.GetPath()

	initial := realCloudFoxUITextFixture(connection.Provider)
	fileName := "viewer-editor.txt"
	fileCandidate := workspace.Join(workspace.GetPath(), fileName)
	if err := writeRealCloudFoxUIFile(ctx, workspace, fileCandidate, initial); err != nil {
		t.Fatalf("create real UI fixture: %s", redactRealCloudFoxError(err))
	}
	filePath := requireRealCloudFoxUIFile(t, ctx, workspace, connection.Provider, fileName, int64(len(initial)))

	// Keep the UI surfaces independent. In particular, an editor regression
	// must not prevent the same live run from collecting viewer and F5 results.
	t.Run("f3-viewer", func(t *testing.T) {
		runRealCloudFoxViewer(t, workspace, filePath, initial)
	})
	t.Run("f4-editor", func(t *testing.T) {
		edited := append(append([]byte(nil), initial...), []byte("editor-save-marker\n")...)
		runRealCloudFoxEditor(t, host, workspace, workspacePath, filePath, fileName, initial, edited, connection.Provider)
	})
	t.Run("f5-copy-roundtrip", func(t *testing.T) {
		runRealCloudFoxF5RoundTrip(t, workspace, connection.Provider)
	})
}

func realCloudFoxUITextFixture(provider cloudfox.ProviderType) []byte {
	var b strings.Builder
	b.WriteString("CloudFox real headless viewer/editor fixture\n")
	for line := 0; line < 1800; line++ {
		_, _ = fmt.Fprintf(&b, "%04d provider=%s abcdefghijklmnopqrstuvwxyz 0123456789\n", line, provider)
	}
	b.WriteString("fixture-tail\n")
	return []byte(b.String())
}

func writeRealCloudFoxUIFile(ctx context.Context, filesystem vfs.VFS, path string, payload []byte) error {
	w, err := filesystem.Create(ctx, path)
	if err != nil {
		return err
	}
	if len(payload) != 0 {
		if _, err := w.Write(payload); err != nil {
			_ = w.Close()
			return err
		}
	}
	return w.Close()
}

func requireRealCloudFoxUIFile(t *testing.T, ctx context.Context, filesystem vfs.VFS, provider cloudfox.ProviderType, name string, expectedSize int64) string {
	t.Helper()
	items, err := readRealCloudFoxUIDir(ctx, filesystem, filesystem.GetPath(), provider)
	if err != nil {
		t.Fatalf("list isolated real UI workspace: %v", err)
	}
	count := 0
	for _, item := range items {
		if item.Name != name {
			continue
		}
		count++
		if item.IsDir || item.Size != expectedSize {
			t.Fatalf("real UI fixture metadata mismatch: directory=%t size=%d want=%d", item.IsDir, item.Size, expectedSize)
		}
	}
	if count != 1 {
		t.Fatalf("isolated real UI workspace contains %d exact fixture rows, want 1", count)
	}
	path := filesystem.Join(filesystem.GetPath(), name)
	stat, err := statRealCloudFoxUI(ctx, filesystem, path, provider)
	if err != nil || stat.IsDir || stat.Size != expectedSize {
		t.Fatalf("stat canonical real UI fixture: %v", err)
	}
	return path
}

func runRealCloudFoxViewer(t *testing.T, filesystem vfs.VFS, path string, expected []byte) {
	t.Helper()
	realCloudFoxUIResetScreen()
	pf := realCloudFoxUIBarePanels(t)
	defer pf.Close()

	actionOpenViewer(pf, filesystem, path)
	var viewer *ViewerView
	realCloudFoxUIWait(t, 3*time.Minute, "F3 viewer to open", func() bool {
		viewer, _ = findOpenedViewer(filesystem, path)
		return viewer != nil
	})
	defer viewer.Close()
	if viewer.backend.Size() != int64(len(expected)) {
		t.Fatalf("F3 viewer size=%d, want=%d", viewer.backend.Size(), len(expected))
	}

	head := realCloudFoxUIViewerRead(t, viewer, 0, 257)
	if !bytes.Equal(head, expected[:len(head)]) {
		t.Fatal("F3 viewer returned unexpected leading content")
	}
	offset := int64(len(expected)/2 + 31)
	middle := realCloudFoxUIViewerRead(t, viewer, offset, 4093)
	if !bytes.Equal(middle, expected[offset:int(offset)+len(middle)]) {
		t.Fatal("F3 viewer returned unexpected content after an arbitrary seek")
	}

	viewer.SetPosition(0, 0, 119, 38)
	if !viewer.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_END}) {
		t.Fatal("F3 viewer did not handle End navigation")
	}
	realCloudFoxUIWait(t, 2*time.Minute, "F3 viewer tail seek", func() bool {
		return !viewer.Busy && viewer.TopOffset > 0
	})
}

func realCloudFoxUIViewerRead(t *testing.T, viewer *ViewerView, offset int64, length int) []byte {
	t.Helper()
	var data []byte
	realCloudFoxUIWait(t, 2*time.Minute, "F3 viewer range read", func() bool {
		var err error
		data, err = viewer.backend.ReadAt(offset, length)
		if err == nil || errors.Is(err, io.EOF) {
			return true
		}
		if errors.Is(err, piecetable.ErrLoading) {
			return false
		}
		t.Fatalf("F3 viewer range read: %s", redactRealCloudFoxError(err))
		return false
	})
	return data
}

func runRealCloudFoxEditor(t *testing.T, host *realCloudFoxUIHost, filesystem vfs.VFS, workspacePath, path, name string, initial, expected []byte, provider cloudfox.ProviderType) {
	t.Helper()
	realCloudFoxUIResetScreen()
	pf := realCloudFoxUIBarePanels(t)
	defer pf.Close()

	actionOpenEditor(pf, filesystem, path)
	var editor *EditorView
	realCloudFoxUIWait(t, 3*time.Minute, "F4 editor to open", func() bool {
		editor, _ = findOpenedEditor(filesystem, path)
		return editor != nil
	})
	defer func() {
		if editor != nil && !editor.IsDone() {
			editor.Close()
		}
	}()
	loaded := realCloudFoxUIEditorBytes(t, editor)
	if !bytes.Equal(loaded, initial) {
		t.Fatal("F4 editor opened with unexpected content")
	}

	editor.SetText(string(expected))
	saved := make(chan struct{}, 1)
	editor.SaveToFile(func() { saved <- struct{}{} })
	realCloudFoxUIWait(t, 5*time.Minute, "F4 editor save", func() bool {
		select {
		case <-saved:
			return true
		default:
			if !editor.saving {
				t.Fatal("F4 editor save stopped without completing successfully")
			}
			return false
		}
	})
	editor.Close()

	verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelVerify()
	items, err := readRealCloudFoxUIDir(verifyCtx, filesystem, filesystem.GetPath(), provider)
	if err != nil {
		t.Fatalf("list workspace after F4 editor save: %v", err)
	}
	for _, item := range items {
		if item.Name == name+".f4tmp" {
			t.Fatal("F4 editor left its remote temporary file behind")
		}
	}
	restoredVisualFile := requireRealCloudFoxUIFile(t, verifyCtx, filesystem, provider, name, int64(len(expected)))
	_ = restoredVisualFile

	// Recreate the workspace through the registered standalone visual-path
	// provider, exactly as panel history and bookmarks do after restart.
	reopenCtx, cancelReopen := context.WithTimeout(context.Background(), 2*time.Minute)
	var standalone vfs.VFSProvider
	for _, candidate := range host.vfsProviders {
		if marker, ok := candidate.(vfs.StandalonePathProvider); ok && marker.OpensStandalonePaths() && candidate.CanOpen(reopenCtx, nil, workspacePath) {
			standalone = candidate
			break
		}
	}
	if standalone == nil {
		cancelReopen()
		t.Fatal("no provider accepted the persisted visual cloud path")
	}
	reopened, err := standalone.Open(reopenCtx, nil, workspacePath)
	cancelReopen()
	if err != nil {
		t.Fatalf("restore edited workspace through production visual-path provider: %v", err)
	}
	defer reopened.Close()
	reopenedPath := requireRealCloudFoxUIFile(t, verifyCtx, reopened, provider, name, int64(len(expected)))

	actionOpenEditor(pf, reopened, reopenedPath)
	var reopenedEditor *EditorView
	realCloudFoxUIWait(t, 3*time.Minute, "saved F4 editor file to reopen", func() bool {
		reopenedEditor, _ = findOpenedEditor(reopened, reopenedPath)
		return reopenedEditor != nil
	})
	defer reopenedEditor.Close()
	if got := realCloudFoxUIEditorBytes(t, reopenedEditor); !bytes.Equal(got, expected) {
		t.Fatal("reopened F4 editor content does not match the saved content")
	}
}

func realCloudFoxUIEditorBytes(t *testing.T, editor *EditorView) []byte {
	t.Helper()
	var data []byte
	realCloudFoxUIWait(t, 2*time.Minute, "F4 editor content to load", func() bool {
		var err error
		data, err = editor.pt.Bytes()
		if err == nil {
			return true
		}
		if errors.Is(err, piecetable.ErrLoading) {
			return false
		}
		t.Fatalf("read F4 editor content: %s", redactRealCloudFoxError(err))
		return false
	})
	return data
}

func runRealCloudFoxF5RoundTrip(t *testing.T, workspace vfs.VFS, provider cloudfox.ProviderType) {
	t.Helper()
	uploadDir := t.TempDir()
	name := "f5-roundtrip.bin"
	payload := bytes.Repeat([]byte("CloudFox-F5-roundtrip-0123456789\n"), 4096)
	if err := os.WriteFile(filepath.Join(uploadDir, name), payload, 0o600); err != nil {
		t.Fatalf("create local F5 fixture: %v", err)
	}

	realCloudFoxUIResetScreen()
	local := vfs.NewOSVFS(uploadDir)
	cloudPanelVFS := workspace.Clone()
	if cloudPanelVFS == nil || cloudPanelVFS == workspace {
		t.Fatal("CloudVFS did not clone for the F5 passive panel")
	}
	defer local.Close()
	defer cloudPanelVFS.Close()

	pf := realCloudFoxUIPanels(t, local, cloudPanelVFS)
	defer pf.Close()
	left := pf.panels[0].(*FileSystemPanel)
	right := pf.panels[1].(*FileSystemPanel)
	realCloudFoxUIWaitPanelName(t, left, name, 30*time.Second)
	pf.activeIdx = 0
	left.SetFocus(true)
	right.SetFocus(false)

	// actionCopyMove is the production F5 handler registered as File.Copy.
	actionCopyMove(pf, false)
	realCloudFoxUIWaitFileOp(t, 5*time.Minute)
	verifyCtx, cancelVerify := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelVerify()
	remotePath := requireRealCloudFoxUIFile(t, verifyCtx, workspace, provider, name, int64(len(payload)))
	if got := readRealCloudFoxUIFile(t, workspace, remotePath); !bytes.Equal(got, payload) {
		t.Fatal("F5 local-to-cloud copy changed file content")
	}

	// Remove the upload source so the reverse F5 has an empty destination and
	// cannot enter an overwrite-confirmation branch.
	if err := os.Remove(filepath.Join(uploadDir, name)); err != nil {
		t.Fatalf("remove local upload fixture before reverse F5: %v", err)
	}
	realCloudFoxUIWaitPanelName(t, right, name, 2*time.Minute)
	pf.activeIdx = 1
	left.SetFocus(false)
	right.SetFocus(true)
	actionCopyMove(pf, false)
	realCloudFoxUIWaitFileOp(t, 5*time.Minute)
	got, err := os.ReadFile(filepath.Join(uploadDir, name))
	if err != nil {
		t.Fatalf("read cloud-to-local F5 result: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("F5 cloud-to-local copy changed file content")
	}
}

func readRealCloudFoxUIFile(t *testing.T, filesystem vfs.VFS, path string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	f, err := filesystem.Open(ctx, path)
	if err != nil {
		t.Fatalf("open copied real UI file: %s", redactRealCloudFoxError(err))
	}
	defer f.Close()
	data := make([]byte, f.Size())
	n, err := f.ReadAt(ctx, data, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("read copied real UI file: %s", redactRealCloudFoxError(err))
	}
	return data[:n]
}

func realCloudFoxUIResetScreen() {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	SetDefaultF4Palette()
}

func realCloudFoxUIBarePanels(t *testing.T) *PanelsFrame {
	t.Helper()
	left := vfs.NewOSVFS(t.TempDir())
	right := vfs.NewOSVFS(t.TempDir())
	pf := realCloudFoxUIPanels(t, left, right)
	t.Cleanup(func() {
		_ = left.Close()
		_ = right.Close()
	})
	return pf
}

func realCloudFoxUIPanels(t *testing.T, leftVFS, rightVFS vfs.VFS) *PanelsFrame {
	t.Helper()
	pf := NewPanelsFrame()
	pf.panels[0] = NewFileSystemPanel(0, 0, 60, 37, leftVFS)
	pf.panels[1] = NewFileSystemPanel(60, 0, 60, 37, rightVFS)
	pf.activeIdx = 0
	pf.ResizeConsole(120, 40)
	pf.panels[0].SetFocus(true)
	pf.panels[1].SetFocus(false)
	vtui.FrameManager.Push(pf)
	return pf
}

func realCloudFoxUIWaitPanelName(t *testing.T, panel *FileSystemPanel, name string, timeout time.Duration) {
	t.Helper()
	realCloudFoxUIWait(t, timeout, "file panel to load target row", func() bool {
		if panel.isLoading {
			return false
		}
		panel.SelectName(name)
		return panel.GetSelectedName() == name
	})
}

func realCloudFoxUIWaitFileOp(t *testing.T, timeout time.Duration) {
	t.Helper()
	started := false
	realCloudFoxUIWait(t, timeout, "F5 file operation", func() bool {
		running := false
		for _, screen := range vtui.FrameManager.Screens {
			for _, frame := range screen.Frames {
				if dialog, ok := frame.(*FileOpProgressDialog); ok && !dialog.IsDone() {
					running = true
					started = true
				}
			}
		}
		return started && !running
	})
}

func realCloudFoxUIWait(t *testing.T, timeout time.Duration, description string, ready func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if realCloudFoxUIHasErrorDialog() {
			t.Fatalf("%s surfaced an error dialog", description)
		}
		if ready() {
			return
		}
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %s", description)
		}
	}
}

func realCloudFoxUIHasErrorDialog() bool {
	if vtui.FrameManager == nil {
		return false
	}
	for _, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			if frame == nil || frame.IsDone() {
				continue
			}
			title := strings.ToLower(frame.GetTitle())
			if strings.Contains(title, "error") || strings.Contains(title, "ошиб") {
				return true
			}
		}
	}
	return false
}

func statRealCloudFoxUI(ctx context.Context, filesystem vfs.VFS, path string, provider cloudfox.ProviderType) (vfs.VFSItem, error) {
	var item vfs.VFSItem
	err := retryRealCloudFoxUIRead(ctx, provider, func() error {
		var err error
		item, err = filesystem.Stat(ctx, path)
		return err
	})
	return item, err
}

func readRealCloudFoxUIDir(ctx context.Context, filesystem vfs.VFS, path string, provider cloudfox.ProviderType) ([]vfs.VFSItem, error) {
	var items []vfs.VFSItem
	err := retryRealCloudFoxUIRead(ctx, provider, func() error {
		var attempt []vfs.VFSItem
		err := filesystem.ReadDir(ctx, path, func(chunk []vfs.VFSItem) {
			attempt = append(attempt, chunk...)
		})
		if err == nil {
			items = attempt
		}
		return err
	})
	return items, err
}

func retryRealCloudFoxUIRead(ctx context.Context, provider cloudfox.ProviderType, operation func() error) error {
	const attempts = 3
	for attempt := 0; attempt < attempts; attempt++ {
		err := operation()
		if err == nil || provider != cloudfox.ProviderYandexDisk || !isRealCloudFoxUINetworkError(err) || attempt == attempts-1 {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func isRealCloudFoxUINetworkError(err error) bool {
	var networkError net.Error
	return errors.As(err, &networkError)
}

func requireRealCloudFoxUIWorkspace(t *testing.T, ctx context.Context, root vfs.VFS, provider cloudfox.ProviderType, name string) string {
	t.Helper()
	path, ok := findRealCloudFoxUIWorkspace(ctx, root, provider, name)
	if !ok {
		t.Fatal("isolated real UI workspace was not returned as one exact directory row")
	}
	return path
}

func findRealCloudFoxUIWorkspace(ctx context.Context, root vfs.VFS, provider cloudfox.ProviderType, name string) (string, bool) {
	items, err := readRealCloudFoxUIDir(ctx, root, root.GetPath(), provider)
	if err != nil {
		return "", false
	}
	count := 0
	isDir := false
	for _, item := range items {
		if item.Name == name {
			count++
			isDir = item.IsDir
		}
	}
	if count != 1 || !isDir {
		return "", false
	}
	// The exact UUID name and directory bit are the public identity proof.
	// Provider IDs stay behind CloudVFS and are resolved only by the backend.
	path := root.Join(root.GetPath(), name)
	if path == "" || strings.Contains(path, "cloud://") {
		return "", false
	}
	return path, true
}

func cleanupRealCloudFoxUIWorkspace(t *testing.T, root vfs.VFS, provider cloudfox.ProviderType, name, expectedPath string) {
	t.Helper()
	if !strings.HasPrefix(name, realCloudFoxUIFolderPrefix) || strings.ContainsAny(name, "/\\") {
		t.Errorf("refusing unsafe real UI workspace cleanup")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	path, ok := findRealCloudFoxUIWorkspace(ctx, root, provider, name)
	if !ok {
		t.Errorf("cannot prove exact real UI workspace identity for cleanup")
		return
	}
	if expectedPath != "" && path != expectedPath {
		t.Errorf("refusing real UI workspace cleanup after canonical identity changed")
		return
	}
	if err := root.Remove(ctx, path); err != nil {
		t.Errorf("remove isolated real UI workspace: %v", err)
		return
	}
	items, err := readRealCloudFoxUIDir(ctx, root, root.GetPath(), provider)
	if err != nil {
		t.Errorf("verify isolated real UI workspace cleanup: %v", err)
		return
	}
	for _, item := range items {
		if item.Name == name {
			t.Errorf("isolated real UI workspace remains after cleanup")
			return
		}
	}
}
