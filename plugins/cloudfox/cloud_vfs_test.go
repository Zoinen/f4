package cloudfox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/unxed/f4/vfs"
	drive "google.golang.org/api/drive/v3"
)

type fakeBackend struct {
	mu       sync.Mutex
	pages    [][]RemoteEntry
	statPath string
	closed   atomic.Int32
}

type visualTreeBackend struct {
	*fakeBackend
	directories map[string][][]RemoteEntry
	parents     map[string]string
	names       map[string]string
}

func (b *visualTreeBackend) Base(location string) string {
	if name := b.names[location]; name != "" {
		return name
	}
	return b.fakeBackend.Base(location)
}

func (b *visualTreeBackend) Dir(location string) string {
	if parent := b.parents[location]; parent != "" {
		return parent
	}
	return b.fakeBackend.Dir(location)
}

func (b *visualTreeBackend) ReadDir(ctx context.Context, location string, cb func([]RemoteEntry)) error {
	for _, page := range b.directories[location] {
		if err := ctx.Err(); err != nil {
			return err
		}
		cb(append([]RemoteEntry(nil), page...))
	}
	return nil
}

func (*fakeBackend) Root() string { return "/root" }
func (*fakeBackend) Normalize(p string) (string, error) {
	if p == "" {
		return "", errors.New("empty location")
	}
	return path.Clean("/" + p), nil
}
func (*fakeBackend) Join(first string, rest ...string) string {
	return path.Join(append([]string{first}, rest...)...)
}
func (*fakeBackend) Base(p string) string { return path.Base(p) }
func (*fakeBackend) Dir(p string) string  { return path.Dir(p) }
func (*fakeBackend) IsRoot(p string) bool { return path.Clean(p) == "/root" }
func (b *fakeBackend) ReadDir(ctx context.Context, _ string, cb func([]RemoteEntry)) error {
	for _, page := range b.pages {
		if err := ctx.Err(); err != nil {
			return err
		}
		cb(append([]RemoteEntry(nil), page...))
	}
	return nil
}
func (b *fakeBackend) Stat(_ context.Context, p string) (RemoteEntry, error) {
	b.mu.Lock()
	b.statPath = p
	b.mu.Unlock()
	return RemoteEntry{VFSItem: vfs.VFSItem{Name: path.Base(p)}, Location: p}, nil
}
func (*fakeBackend) MkDir(context.Context, string) error          { return nil }
func (*fakeBackend) Remove(context.Context, string) error         { return nil }
func (*fakeBackend) Rename(context.Context, string, string) error { return nil }
func (*fakeBackend) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return nil, os.ErrPermission
}
func (*fakeBackend) Create(context.Context, string) (io.WriteCloser, error) {
	return nopWriteCloser{bytes.NewBuffer(nil)}, nil
}
func (*fakeBackend) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return os.ErrPermission
}
func (*fakeBackend) Capabilities() vfs.VFSCapabilities { return vfs.VFSCapabilities{} }
func (b *fakeBackend) Close() error                    { b.closed.Add(1); return nil }

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type fixedCloseErrorWriter struct{ err error }

func (*fixedCloseErrorWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *fixedCloseErrorWriter) Close() error              { return w.err }

type abortCaptureWriter struct {
	aborts int
	closes int
}

func (*abortCaptureWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *abortCaptureWriter) Close() error              { w.closes++; return nil }
func (w *abortCaptureWriter) Abort() error              { w.aborts++; return nil }

type fakeTrashBackend struct{ *fakeBackend }

func (*fakeTrashBackend) MoveToTrash(context.Context, string) error { return nil }

type fakeNativeNameBackend struct{ *fakeBackend }

func (*fakeNativeNameBackend) TransferName(string) string             { return "document.docx" }
func (*fakeNativeNameBackend) IntraSessionTransferName(string) string { return "document" }

type blockingStatBackend struct {
	*fakeBackend
	started chan struct{}
	once    sync.Once
}

type contextCheckingReader struct {
	data   []byte
	offset int64
}

func (r *contextCheckingReader) Size() int64 { return int64(len(r.data)) }
func (*contextCheckingReader) Close() error  { return nil }
func (r *contextCheckingReader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
func (r *contextCheckingReader) Read(ctx context.Context, p []byte) (int, error) {
	n, err := r.ReadAt(ctx, p, r.offset)
	r.offset += int64(n)
	return n, err
}

type readableFakeBackend struct{ *fakeBackend }

func (*readableFakeBackend) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return &contextCheckingReader{data: []byte("cloud data")}, nil
}

func (b *blockingStatBackend) Stat(ctx context.Context, _ string) (RemoteEntry, error) {
	b.once.Do(func() { close(b.started) })
	<-ctx.Done()
	return RemoteEntry{}, ctx.Err()
}

func testCloudVFS(t *testing.T, backend Backend) *CloudVFS {
	t.Helper()
	pool := newSessionPool()
	ready := make(chan struct{})
	close(ready)
	session := &pooledSession{pool: pool, key: "test", ready: ready, backend: backend, refs: 1}
	pool.sessions[session.key] = session
	c, err := newCloudVFS(Connection{ID: testConnectionID, Name: "Test", Provider: ProviderGoogleDrive}, nil, session, backend.Root())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCloudVFSBackendSnapshotConcurrentPoolClose(t *testing.T) {
	cloud := testCloudVFS(t, &fakeBackend{})
	pool := cloud.session.pool
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range 16 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			for range 1000 {
				backend, err := cloud.backend()
				if err == nil && backend == nil {
					t.Errorf("backend returned a nil backend without an error")
					return
				}
			}
		}()
	}
	close(start)
	if err := pool.close(); err != nil {
		t.Fatal(err)
	}
	workers.Wait()
	if backend, err := cloud.backend(); !errors.Is(err, os.ErrClosed) || backend != nil {
		t.Fatalf("backend after pool close = (%T, %v), want (nil, os.ErrClosed)", backend, err)
	}
}

func TestCloudWriteHandleConcurrentCloseReturnsCommitErrorToEveryCaller(t *testing.T) {
	commitErr := errors.New("commit failed")
	var finishes atomic.Int32
	handle := &cloudWriteHandle{
		WriteCloser: &fixedCloseErrorWriter{err: commitErr},
		finish:      func() { finishes.Add(1) },
	}
	errorsSeen := make(chan error, 16)
	var callers sync.WaitGroup
	for range 16 {
		callers.Add(1)
		go func() {
			defer callers.Done()
			errorsSeen <- handle.Close()
		}()
	}
	callers.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if !errors.Is(err, commitErr) {
			t.Fatalf("Close returned %v, want commit error", err)
		}
	}
	if finishes.Load() != 1 {
		t.Fatalf("operation lifetime finished %d times, want once", finishes.Load())
	}
}

func TestCloudWriteHandleAbortDelegatesWithoutCommitFinish(t *testing.T) {
	writer := &abortCaptureWriter{}
	commitFinishes, abortFinishes := 0, 0
	handle := &cloudWriteHandle{
		WriteCloser: writer,
		finish:      func() { commitFinishes++ },
		abortFinish: func() { abortFinishes++ },
	}
	if err := handle.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if writer.aborts != 1 || writer.closes != 0 || commitFinishes != 0 || abortFinishes != 1 {
		t.Fatalf("abort lifecycle writer=%#v commitFinish=%d abortFinish=%d", writer, commitFinishes, abortFinishes)
	}
}

func TestSessionPoolCloseCancelsFactoryOpen(t *testing.T) {
	pool := newSessionPool()
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		_, err := pool.acquire(context.Background(), Connection{ID: "opening"}, func(ctx context.Context) (Backend, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		})
		result <- err
	}()
	<-started
	if err := pool.close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("factory Open returned %v, want cancellation from pool close", err)
	}
}

func TestSessionPoolCloseCancelsInFlightBackendOperation(t *testing.T) {
	backend := &blockingStatBackend{fakeBackend: &fakeBackend{}, started: make(chan struct{})}
	cloud := testCloudVFS(t, backend)
	result := make(chan error, 1)
	go func() {
		_, err := cloud.Stat(context.Background(), cloud.GetPath())
		result <- err
	}()
	<-backend.started
	if err := cloud.session.pool.close(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("in-flight Stat returned %v, want cancellation from pool close", err)
	}
}

func TestCloudReadHandleOutlivesOpenCallContext(t *testing.T) {
	cloud := testCloudVFS(t, &readableFakeBackend{fakeBackend: &fakeBackend{}})
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	openCtx, cancelOpen := context.WithCancel(context.Background())
	handle, err := cloud.Open(openCtx, cloud.GetPath())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Close() }()

	// Viewer opening runs inside a progress task. Completion cancels that
	// task, but the returned handle must remain usable by subsequent paints.
	cancelOpen()
	buffer := make([]byte, 5)
	if n, err := handle.ReadAt(context.Background(), buffer, 0); err != nil || n != len(buffer) || string(buffer) != "cloud" {
		t.Fatalf("ReadAt after Open context cancellation = %d, %v, %q", n, err, buffer)
	}
}

func TestCloudVFSResolvesF5DestinationWithTrailingSlash(t *testing.T) {
	cloud := testCloudVFS(t, &fakeBackend{})
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	directoryTarget := cloud.GetPath() + "/"
	if !cloud.IsAbs(directoryTarget) {
		t.Fatalf("F5 destination %q is not recognized as absolute", directoryTarget)
	}
	fileTarget := cloud.Join(directoryTarget, "upload.txt")
	location, err := cloud.resolvePath(fileTarget)
	if err != nil {
		t.Fatal(err)
	}
	if location != "/root/upload.txt" {
		t.Fatalf("resolved upload location = %q, want /root/upload.txt", location)
	}
}

func TestCloudVFSPanelTitleShowsFullPlatformPath(t *testing.T) {
	cloud := testCloudVFS(t, &fakeBackend{})
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	separator := string(os.PathSeparator)
	if got, want := cloud.PanelTitle(cloud.GetPath()), "Test:"+separator; got != want {
		t.Fatalf("root PanelTitle = %q, want %q", got, want)
	}
	target := cloud.Join(cloud.GetPath(), "First", "Second")
	if err := cloud.SetPath(target); err != nil {
		t.Fatal(err)
	}
	if got, want := cloud.PanelTitle(cloud.GetPath()), "Test:"+separator+"First"+separator+"Second"; got != want {
		t.Fatalf("nested PanelTitle = %q, want %q", got, want)
	}
}

func TestCloudVFSPublicContractUsesOnlyVisualPaths(t *testing.T) {
	backend := &visualTreeBackend{
		fakeBackend: &fakeBackend{},
		parents:     map[string]string{"/opaque/folder-id": "/root", "/opaque/file-id": "/opaque/folder-id"},
		names:       map[string]string{"/opaque/folder-id": "Photos", "/opaque/file-id": "vacation.jpg"},
		directories: map[string][][]RemoteEntry{
			"/root":             {{{VFSItem: vfs.VFSItem{Name: "Photos", IsDir: true}, Location: "/opaque/folder-id"}}},
			"/opaque/folder-id": {{{VFSItem: vfs.VFSItem{Name: "vacation.jpg"}, Location: "/opaque/file-id"}}},
		},
	}
	cloud := testCloudVFS(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	separator := string(os.PathSeparator)
	root := "Test:" + separator
	if got := cloud.GetPath(); got != root {
		t.Fatalf("root GetPath = %q, want %q", got, root)
	}
	if strings.Contains(cloud.GetPath(), "cloud://") || strings.Contains(cloud.GetPath(), testConnectionID) {
		t.Fatalf("root path exposed internal identity: %q", cloud.GetPath())
	}
	if err := cloud.ReadDir(context.Background(), cloud.GetPath(), func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	folder := cloud.Join(cloud.GetPath(), "Photos")
	if want := root + "Photos"; folder != want {
		t.Fatalf("joined folder = %q, want %q", folder, want)
	}
	if location, err := cloud.resolvePath(folder); err != nil || location != "/opaque/folder-id" {
		t.Fatalf("visual folder resolved to %q, %v", location, err)
	}
	if err := cloud.SetPath(folder); err != nil {
		t.Fatal(err)
	}
	if err := cloud.ReadDir(context.Background(), cloud.GetPath(), func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	file := cloud.Join(cloud.GetPath(), "vacation.jpg")
	if want := root + "Photos" + separator + "vacation.jpg"; file != want {
		t.Fatalf("joined file = %q, want %q", file, want)
	}
	if location, err := cloud.resolvePath(file); err != nil || location != "/opaque/file-id" {
		t.Fatalf("visual file resolved to %q, %v", location, err)
	}
	if got := cloud.Join(cloud.GetPath(), ".."); got != root {
		t.Fatalf("Join(current, ..) = %q, want %q", got, root)
	}
	if location, err := cloud.resolvePath(".."); err != nil || location != "/root" {
		t.Fatalf("relative .. resolved to %q, %v", location, err)
	}
	for _, public := range []string{cloud.GetPath(), folder, file, cloud.Dir(file)} {
		if strings.Contains(public, "cloud://") || strings.Contains(public, testConnectionID) || strings.Contains(public, "%2F") {
			t.Fatalf("public operation exposed internal path %q", public)
		}
		foreignSeparator := "/"
		if os.PathSeparator == '/' {
			foreignSeparator = "\\"
		}
		if strings.Contains(public, foreignSeparator) {
			t.Fatalf("public path %q contains non-native separator %q", public, foreignSeparator)
		}
	}
}

func TestCloudVFSRestoresVisualPathByResolvingEachDirectory(t *testing.T) {
	backend := &visualTreeBackend{
		fakeBackend: &fakeBackend{},
		parents:     map[string]string{"/ids/first": "/root", "/ids/second": "/ids/first"},
		names:       map[string]string{"/ids/first": "First", "/ids/second": "Second"},
		directories: map[string][][]RemoteEntry{
			"/root":      {{{VFSItem: vfs.VFSItem{Name: "First", IsDir: true}, Location: "/ids/first"}}},
			"/ids/first": {{{VFSItem: vfs.VFSItem{Name: "Second", IsDir: true}, Location: "/ids/second"}}},
		},
	}
	cloud := testCloudVFS(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if err := cloud.restoreVisualPath(context.Background(), []string{"First", "Second"}); err != nil {
		t.Fatal(err)
	}
	separator := string(os.PathSeparator)
	want := "Test:" + separator + "First" + separator + "Second"
	if got := cloud.GetPath(); got != want {
		t.Fatalf("restored GetPath = %q, want %q", got, want)
	}
	if got := cloud.currentLocation(); got != "/ids/second" {
		t.Fatalf("restored internal location = %q, want /ids/second", got)
	}
}

func TestGooglePanelTitleUsesCachedDisplayHierarchy(t *testing.T) {
	first := googleItemLocation("", "folder-one")
	second := googleItemLocation("", "folder-two")
	backend := &googleDriveBackend{
		items: make(map[string]*drive.File),
		parents: map[string]string{
			first:  googleMyLocation,
			second: first,
		},
		names: map[string]string{
			googleMyLocation: "My Drive",
			first:            "First",
			second:           "Second",
		},
		transferNames: make(map[string]string),
	}
	cloud := testCloudVFS(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if err := cloud.SetPath(cloud.publicPath(second)); err != nil {
		t.Fatal(err)
	}
	separator := string(os.PathSeparator)
	want := "Test:" + separator + "My Drive" + separator + "First" + separator + "Second"
	if got := cloud.PanelTitle(cloud.GetPath()); got != want {
		t.Fatalf("Google PanelTitle = %q, want %q", got, want)
	}
}

func TestCloudVFSDisambiguatesAcrossPagesAndResolvesOpaqueLocations(t *testing.T) {
	backend := &fakeBackend{pages: [][]RemoteEntry{
		{{VFSItem: vfs.VFSItem{Name: "report.txt"}, Location: "/ids/object-a"}},
		{{VFSItem: vfs.VFSItem{Name: "report.txt"}, Location: "/ids/object-b"}},
	}}
	cloud := testCloudVFS(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	var names []string
	if err := cloud.ReadDir(context.Background(), cloud.GetPath(), func(items []vfs.VFSItem) {
		for _, item := range items {
			names = append(names, item.Name)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] == names[1] || names[0] == "report.txt" || names[1] == "report.txt" {
		t.Fatalf("duplicate aliases = %#v", names)
	}
	for _, name := range names {
		publicPath := cloud.Join(cloud.GetPath(), name)
		if _, err := cloud.Stat(context.Background(), publicPath); err != nil {
			t.Fatal(err)
		}
		backend.mu.Lock()
		resolved := backend.statPath
		backend.mu.Unlock()
		if resolved != "/ids/object-a" && resolved != "/ids/object-b" {
			t.Fatalf("alias %q resolved to %q, want opaque ID", name, resolved)
		}
		if got := cloud.TransferName(publicPath, nil); got != "report.txt" {
			t.Fatalf("TransferName(%q) = %q", name, got)
		}
	}
}

func TestDisambiguationIsStableAcrossProviderOrder(t *testing.T) {
	backend := &fakeBackend{}
	a := RemoteEntry{VFSItem: vfs.VFSItem{Name: "same"}, Location: "/id/a"}
	b := RemoteEntry{VFSItem: vfs.VFSItem{Name: "same"}, Location: "/id/b"}
	first := disambiguateEntries([][]RemoteEntry{{a}, {b}}, "/root", backend)
	second := disambiguateEntries([][]RemoteEntry{{b}, {a}}, "/root", backend)
	aliases := func(pages [][]RemoteEntry) []string {
		var out []string
		for _, page := range pages {
			for _, entry := range page {
				out = append(out, entry.Location+"="+entry.Name)
			}
		}
		sort.Strings(out)
		return out
	}
	if got, want := aliases(first), aliases(second); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("aliases depend on provider order: %v vs %v", got, want)
	}
}

func TestCloudVFSSanitizesPathLikeProviderLabelsButKeepsOpaqueIdentity(t *testing.T) {
	backend := &fakeBackend{pages: [][]RemoteEntry{{
		{VFSItem: vfs.VFSItem{Name: ".."}, Location: "/ids/parent-label"},
		{VFSItem: vfs.VFSItem{Name: "a/b\\c"}, Location: "/ids/separator-label"},
	}}}
	cloud := testCloudVFS(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	var names []string
	if err := cloud.ReadDir(context.Background(), cloud.GetPath(), func(items []vfs.VFSItem) {
		for _, item := range items {
			names = append(names, item.Name)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 {
		t.Fatalf("names = %#v", names)
	}
	for _, name := range names {
		if name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
			t.Fatalf("unsafe panel label %q", name)
		}
		if _, err := cloud.Stat(context.Background(), cloud.Join(cloud.GetPath(), name)); err != nil {
			t.Fatal(err)
		}
		backend.mu.Lock()
		resolved := backend.statPath
		backend.mu.Unlock()
		if resolved != "/ids/parent-label" && resolved != "/ids/separator-label" {
			t.Fatalf("sanitized label resolved to %q", resolved)
		}
	}
}

func TestCloudVFSCloneSharesSessionUntilLastClose(t *testing.T) {
	backend := &fakeBackend{}
	cloud := testCloudVFS(t, backend)
	clone, ok := cloud.Clone().(*CloudVFS)
	if !ok || clone == cloud {
		t.Fatalf("Clone = %T %p", clone, clone)
	}
	if !vfs.SameSession(cloud, clone) {
		t.Fatal("clones do not share session identity")
	}
	if err := cloud.Close(); err != nil {
		t.Fatal(err)
	}
	if backend.closed.Load() != 0 {
		t.Fatal("backend closed while clone was alive")
	}
	if err := clone.Close(); err != nil {
		t.Fatal(err)
	}
	if backend.closed.Load() != 1 {
		t.Fatalf("backend closes = %d, want 1", backend.closed.Load())
	}
}

func TestCloudVFSTransferNameIsDestinationAware(t *testing.T) {
	backend := &fakeNativeNameBackend{fakeBackend: &fakeBackend{pages: [][]RemoteEntry{{{
		VFSItem: vfs.VFSItem{Name: "document.docx"}, Location: "/ids/native", TransferName: "document.docx",
	}}}}}
	cloud := testCloudVFS(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if err := cloud.ReadDir(context.Background(), cloud.GetPath(), func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	source := cloud.Join(cloud.GetPath(), "document.docx")
	if got := cloud.TransferName(source, nil); got != "document.docx" {
		t.Fatalf("external transfer name = %q", got)
	}
	clone := cloud.Clone().(*CloudVFS)
	t.Cleanup(func() {
		if err := clone.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if got := cloud.TransferName(source, clone); got != "document" {
		t.Fatalf("same-session transfer name = %q", got)
	}
}

func TestTrashCapabilityIsExposedOnlyByTrashBackends(t *testing.T) {
	plain := testCloudVFS(t, &fakeBackend{})
	t.Cleanup(func() {
		if err := plain.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if _, ok := wrapCloudVFS(plain).(vfs.TrashVFS); ok {
		t.Fatal("plain backend unexpectedly exposes TrashVFS")
	}
	trash := testCloudVFS(t, &fakeTrashBackend{fakeBackend: &fakeBackend{}})
	t.Cleanup(func() {
		if err := trash.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if _, ok := wrapCloudVFS(trash).(vfs.TrashVFS); !ok {
		t.Fatal("trash backend does not expose TrashVFS")
	}
}

func TestManagerRowsCannotSerializeProfileOrSecret(t *testing.T) {
	repo := &Repository{Connections: NewConnectionStore(path.Join(t.TempDir(), "CloudFox.json"))}
	connection := testConnection("Opaque profile", ProviderS3)
	connection.SecretRef = "keyring:v1:opaque"
	if _, err := repo.Save(context.Background(), connection, nil, ""); err != nil {
		t.Fatal(err)
	}
	manager := NewManagerVFS(repo, nil)
	separator := string(os.PathSeparator)
	foreignSeparator := "/"
	if os.PathSeparator == '/' {
		foreignSeparator = "\\"
	}
	for label, publicPath := range map[string]string{
		"root": manager.GetPath(),
		"join": manager.Join(ManagerRoot, connection.Name),
	} {
		if !strings.HasPrefix(publicPath, DriveName+":"+separator) || strings.Contains(publicPath, foreignSeparator) {
			t.Fatalf("manager %s path is not native: %q", label, publicPath)
		}
	}
	foreignInput := DriveName + ":/" + connection.Name
	if os.PathSeparator == '/' {
		foreignInput = DriveName + ":\\" + connection.Name
	}
	if absolute, err := manager.Abs(foreignInput); err != nil || absolute != managerVisualRoot()+connection.Name {
		t.Fatalf("manager Abs did not normalize foreign separators: %q, %v", absolute, err)
	}
	var rows []vfs.VFSItem
	if err := manager.ReadDir(context.Background(), ManagerRoot, func(chunk []vfs.VFSItem) {
		rows = append(rows, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	var saved vfs.VFSItem
	for _, row := range rows {
		if row.Name == connection.Name {
			saved = row
			break
		}
	}
	if saved.Name == "" || !saved.IsDir || saved.IsExecutable || !saved.NoExtension {
		t.Fatalf("saved connection row = %#v, want virtual directory", saved)
	}
	stat, err := manager.Stat(context.Background(), manager.Join(ManagerRoot, connection.Name))
	if err != nil {
		t.Fatal(err)
	}
	if !stat.IsDir || stat.IsExecutable {
		t.Fatalf("saved connection stat = %#v, want virtual directory", stat)
	}
	provider := &connectionProvider{}
	if !provider.OpensVirtualDirectories() {
		t.Fatal("CloudFox connection provider does not open virtual directories")
	}
	if _, err := manager.Open(context.Background(), manager.Join(ManagerRoot, connection.Name)); !errors.Is(err, ErrManagerReadOnly) {
		t.Fatalf("Open = %v", err)
	}
	if _, err := manager.Create(context.Background(), manager.Join(ManagerRoot, connection.Name)); !errors.Is(err, ErrManagerReadOnly) {
		t.Fatalf("Create = %v", err)
	}
}
