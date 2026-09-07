package cloudfox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"golang.org/x/net/webdav"
)

type localWebDAVServer struct {
	server *httptest.Server
	fs     webdav.FileSystem
}

func newLocalWebDAVServer(t *testing.T, authenticate func(*http.Request) bool) localWebDAVServer {
	t.Helper()
	fs := webdav.NewMemFS()
	if err := fs.Mkdir(context.Background(), "/root", 0o755); err != nil {
		t.Fatal(err)
	}
	handler := &webdav.Handler{Prefix: "/dav", FileSystem: fs, LockSystem: webdav.NewMemLS()}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authenticate != nil && !authenticate(r) {
			w.Header().Set("WWW-Authenticate", `Basic realm="local-webdav"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	return localWebDAVServer{server: server, fs: fs}
}

func localBasicWebDAV(t *testing.T) (localWebDAVServer, *webDAVBackend) {
	t.Helper()
	local := newLocalWebDAVServer(t, func(r *http.Request) bool {
		username, password, ok := r.BasicAuth()
		return ok && username == "alice" && password == "secret"
	})
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: local.server.Client()}, WebDAVSettings{
		BaseURL: local.server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	return local, backend
}

func createWebDAVFile(t *testing.T, backend *webDAVBackend, location, content string) {
	t.Helper()
	w, err := backend.Create(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, content); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func readWebDAVFile(t *testing.T, backend *webDAVBackend, location string) string {
	t.Helper()
	r, err := backend.Open(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	data := make([]byte, r.Size())
	if len(data) == 0 {
		return ""
	}
	if _, err := io.ReadFull(&contextReadAdapter{ctx: context.Background(), reader: r}, data); err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type contextReadAdapter struct {
	ctx    context.Context
	reader vfs.ReadAtCloser
}

func (r *contextReadAdapter) Read(p []byte) (int, error) { return r.reader.Read(r.ctx, p) }

func TestWebDAVLocalServerAllImplementedOperations(t *testing.T) {
	t.Parallel()
	_, backend := localBasicWebDAV(t)
	ctx := context.Background()

	if backend.Root() != "/" || !backend.IsRoot("/") || backend.IsRoot("/docs") {
		t.Fatalf("root helpers are inconsistent")
	}
	if normalized, err := backend.Normalize(`\docs\..\docs\report.txt`); err != nil || normalized != "/docs/report.txt" {
		t.Fatalf("Normalize = %q, %v", normalized, err)
	}
	if joined := backend.Join("/docs", "nested", "..", "report.txt"); joined != "/docs/report.txt" {
		t.Fatalf("Join = %q", joined)
	}
	if backend.Base("/docs/report.txt") != "report.txt" || backend.Dir("/docs/report.txt") != "/docs" {
		t.Fatalf("Base/Dir helpers are inconsistent")
	}

	if err := backend.MkDir(ctx, "/docs/nested"); err != nil {
		t.Fatal(err)
	}
	if err := backend.MkDir(ctx, "/docs/nested"); err != nil {
		t.Fatalf("idempotent MkDir: %v", err)
	}
	const specialName = "résumé 100% #1?.txt"
	location := "/docs/nested/" + specialName
	createWebDAVFile(t, backend, location, "0123456789")

	entry, err := backend.Stat(ctx, location)
	if err != nil {
		t.Fatal(err)
	}
	if entry.Name != specialName || entry.Location != location || entry.IsDir || entry.Size != 10 {
		t.Fatalf("Stat = %#v", entry)
	}
	var names []string
	if err := backend.ReadDir(ctx, "/docs/nested", func(chunk []RemoteEntry) {
		for _, item := range chunk {
			names = append(names, item.Name)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{specialName}) {
		t.Fatalf("ReadDir names = %v", names)
	}

	r, err := backend.Open(ctx, location)
	if err != nil {
		t.Fatal(err)
	}
	if r.Size() != 10 {
		t.Fatalf("Open size = %d", r.Size())
	}
	readAt := make([]byte, 4)
	if n, err := r.ReadAt(ctx, readAt, 3); err != nil || n != 4 || string(readAt) != "3456" {
		t.Fatalf("ReadAt = %d, %v, %q", n, err, readAt)
	}
	sequential := make([]byte, 3)
	if n, err := r.Read(ctx, sequential); err != nil || n != 3 || string(sequential) != "012" {
		t.Fatalf("Read = %d, %v, %q", n, err, sequential)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	copyLocation := "/docs/copy.txt"
	if err := backend.Copy(ctx, location, copyLocation); err != nil {
		t.Fatal(err)
	}
	if got := readWebDAVFile(t, backend, copyLocation); got != "0123456789" {
		t.Fatalf("copied content = %q", got)
	}
	movedLocation := "/docs/moved.txt"
	if err := backend.Rename(ctx, copyLocation, movedLocation); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Stat(ctx, copyLocation); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old path after MOVE = %v", err)
	}
	if got := readWebDAVFile(t, backend, movedLocation); got != "0123456789" {
		t.Fatalf("moved content = %q", got)
	}
	if err := backend.MkDir(ctx, "/docs/tree/source/nested"); err != nil {
		t.Fatal(err)
	}
	createWebDAVFile(t, backend, "/docs/tree/source/nested/child.bin", "nested payload")
	if err := backend.Copy(ctx, "/docs/tree/source", "/docs/tree/copied"); err != nil {
		t.Fatal(err)
	}
	if got := readWebDAVFile(t, backend, "/docs/tree/copied/nested/child.bin"); got != "nested payload" {
		t.Fatalf("recursively copied content = %q", got)
	}
	if err := backend.Rename(ctx, "/docs/tree/copied", "/docs/tree/moved"); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Stat(ctx, "/docs/tree/copied"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old collection after MOVE = %v", err)
	}
	if got := readWebDAVFile(t, backend, "/docs/tree/moved/nested/child.bin"); got != "nested payload" {
		t.Fatalf("recursively moved content = %q", got)
	}
	if err := backend.Remove(ctx, "/docs/tree/moved"); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Stat(ctx, "/docs/tree/moved"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recursively removed collection = %v", err)
	}
	if err := backend.Remove(ctx, movedLocation); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Stat(ctx, movedLocation); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed path = %v", err)
	}
	if err := backend.Remove(ctx, "/"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Remove root = %v", err)
	}
	if err := backend.SetAttributes(ctx, location, vfs.VFSItem{}); !errors.Is(err, ErrUnsupportedOperation) {
		t.Fatalf("SetAttributes = %v", err)
	}
	capabilities := backend.Capabilities()
	if !capabilities.HasServerSideCopy || !capabilities.HasServerSideMove || !capabilities.HasRandomAccess {
		t.Fatalf("Capabilities = %#v", capabilities)
	}
	if backend.TransferName(location) != specialName {
		t.Fatalf("TransferName = %q", backend.TransferName(location))
	}
}

func TestWebDAVLocalServerAuthenticationModes(t *testing.T) {
	t.Parallel()

	t.Run("anonymous over HTTP", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authorization := r.Header.Get("Authorization"); authorization != "" {
				t.Errorf("anonymous Authorization = %q", authorization)
			}
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/", "/", "0", true)))
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "anonymous",
		}, nil)
		if _, err := backend.Stat(context.Background(), "/"); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("basic rejection maps to permission", func(t *testing.T) {
		local := newLocalWebDAVServer(t, func(r *http.Request) bool {
			username, password, ok := r.BasicAuth()
			return ok && username == "alice" && password == "secret"
		})
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: local.server.Client()}, WebDAVSettings{
			BaseURL: local.server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "wrong"})
		if _, err := backend.Stat(context.Background(), "/"); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("Stat with bad credentials = %v", err)
		}
	})

	t.Run("bearer", func(t *testing.T) {
		local := newLocalWebDAVServer(t, func(r *http.Request) bool {
			return r.Header.Get("Authorization") == "Bearer local-token"
		})
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: local.server.Client()}, WebDAVSettings{
			BaseURL: local.server.URL + "/dav", Root: "/root", Auth: "bearer",
		}, SecretValues{"bearer_token": "local-token"})
		if _, err := backend.Stat(context.Background(), "/"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestWebDAVCustomCATLSConnection(t *testing.T) {
	t.Parallel()
	local := newLocalWebDAVServer(t, func(r *http.Request) bool {
		username, password, ok := r.BasicAuth()
		return ok && username == "alice" && password == "secret"
	})
	certificate := local.server.Certificate()
	if certificate == nil {
		t.Fatal("local TLS server has no certificate")
	}
	caPath := t.TempDir() + "/webdav-ca.pem"
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(caPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	backend := openTestWebDAV(t, &WebDAVFactory{}, WebDAVSettings{
		BaseURL: local.server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice", CustomCA: caPath,
	}, SecretValues{"password": "secret"})
	var entries []RemoteEntry
	if err := backend.ReadDir(context.Background(), "/", func(chunk []RemoteEntry) { entries = append(entries, chunk...) }); err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return strings.Compare(entries[i].Name, entries[j].Name) < 0 })
}

func TestWebDAVTLSConfigurationFailures(t *testing.T) {
	t.Parallel()
	client, err := webDAVHTTPClient("")
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("WebDAV TLS transport = %#v", client.Transport)
	}
	if _, err := webDAVHTTPClient(t.TempDir() + "/missing.pem"); err == nil || !strings.Contains(err.Error(), "read WebDAV custom CA") {
		t.Fatalf("missing custom CA = %v", err)
	}
	invalidCA := t.TempDir() + "/invalid.pem"
	if err := os.WriteFile(invalidCA, []byte("not a certificate"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := webDAVHTTPClient(invalidCA); err == nil || !strings.Contains(err.Error(), "does not contain a certificate") {
		t.Fatalf("invalid custom CA = %v", err)
	}
}

func TestWebDAVMkDirRejectsExistingFile(t *testing.T) {
	t.Parallel()
	_, backend := localBasicWebDAV(t)
	createWebDAVFile(t, backend, "/collision", "not a collection")
	if err := backend.MkDir(context.Background(), "/collision"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("MkDir over a file = %v, want os.ErrExist", err)
	}
}

func TestWebDAVStatSelectsRequestedResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(
			davResponseXML("/dav/root/unrelated.txt", "unrelated.txt", "malformed", false),
			davResponseXML("/dav/root/requested.txt", "requested.txt", "7", false),
		))
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	entry, err := backend.Stat(context.Background(), "/requested.txt")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Location != "/requested.txt" || entry.Name != "requested.txt" || entry.Size != 7 {
		t.Fatalf("Stat selected wrong response: %#v", entry)
	}
}

func TestWebDAVReadDirIgnoresResponsesOutsideRequestedDepth(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(
			davResponseXML("/dav/root/docs/", "docs", "malformed-self", true),
			davResponseXML("/dav/root/docs/direct.txt", "direct.txt", "1", false),
			davResponseXML("/dav/root/docs/sub/deep.txt", "deep.txt", "malformed-deep", false),
		))
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	var names []string
	if err := backend.ReadDir(context.Background(), "/docs", func(chunk []RemoteEntry) {
		for _, entry := range chunk {
			names = append(names, entry.Name)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"direct.txt"}) {
		t.Fatalf("ReadDir names = %v", names)
	}
}

func TestWebDAVRangeReaderMapsHTTPFailures(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer server.Close()
	readerCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &webDAVRangeReader{client: server.Client(), url: server.URL + "/gone", size: 10, etag: `"one"`, ctx: readerCtx, cancel: cancel}
	if _, err := reader.ReadAt(context.Background(), make([]byte, 1), 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadAt HTTP 404 = %v, want os.ErrNotExist", err)
	}
}

func TestWebDAVPUTMultiStatusIsPartialFailure(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(`<D:response><D:href>/failed</D:href><D:status>HTTP/1.1 423 Locked</D:status></D:response>`))
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	w, err := backend.Create(context.Background(), "/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "payload")
	closeErr := w.Close()
	if !errors.Is(closeErr, vfs.ErrOperationPartial) {
		t.Fatalf("PUT HTTP 207 = %v, want partial operation", closeErr)
	}
	var partial *vfs.PartialOperationError
	if !errors.As(closeErr, &partial) || !reflect.DeepEqual(partial.Failed, []string{"/failed"}) {
		t.Fatalf("PUT partial details = %#v", partial)
	}
}

func TestWebDAVParsesMutationMultiStatusAndRejectsAmbiguousSuccess(t *testing.T) {
	t.Parallel()
	t.Run("mixed statuses", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(
				`<D:response><D:href>/done</D:href><D:status>HTTP/1.1 204 No Content</D:status></D:response>`,
				`<D:response><D:href>/locked</D:href><D:status>HTTP/1.1 423 Locked</D:status></D:response>`,
			))
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "secret"})
		err := backend.mutation(context.Background(), http.MethodDelete, "/collection", nil, true)
		var partial *vfs.PartialOperationError
		if !errors.As(err, &partial) || !errors.Is(err, vfs.ErrOperationPartial) {
			t.Fatalf("mixed 207 = %v", err)
		}
		if !reflect.DeepEqual(partial.Completed, []string{"/done"}) || !reflect.DeepEqual(partial.Failed, []string{"/locked"}) {
			t.Fatalf("mixed 207 details = %#v", partial)
		}
	})

	t.Run("malformed multistatus", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, `<D:multistatus xmlns:D="DAV:"><D:response>`)
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "secret"})
		err := backend.mutation(context.Background(), http.MethodDelete, "/collection", nil, true)
		if !errors.Is(err, vfs.ErrOperationStateUnknown) {
			t.Fatalf("malformed 207 = %v", err)
		}
	})

	t.Run("202 is not proven complete", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "secret"})
		err := backend.mutation(context.Background(), http.MethodDelete, "/collection", nil, true)
		if !errors.Is(err, vfs.ErrOperationStateUnknown) {
			t.Fatalf("DELETE HTTP 202 = %v", err)
		}
	})

	t.Run("207 child 202 is not proven complete", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(
				`<D:response><D:href>/collection</D:href><D:status>HTTP/1.1 202 Accepted</D:status></D:response>`,
			))
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "secret"})
		err := backend.mutation(context.Background(), http.MethodDelete, "/collection", nil, true)
		if !errors.Is(err, vfs.ErrOperationStateUnknown) {
			t.Fatalf("DELETE 207 child HTTP 202 = %v", err)
		}
	})

	t.Run("409 is a DAV conflict, not destination exists", func(t *testing.T) {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "missing intermediate collection", http.StatusConflict)
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "secret"})
		err := backend.mutation(context.Background(), "COPY", "/source", nil)
		if err == nil || errors.Is(err, os.ErrExist) {
			t.Fatalf("COPY HTTP 409 = %v", err)
		}
		var httpErr *providerHTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusConflict {
			t.Fatalf("COPY HTTP 409 details = %#v, %v", httpErr, err)
		}
	})
}

func TestWebDAVRejectsRootMutations(t *testing.T) {
	t.Parallel()
	_, backend := localBasicWebDAV(t)
	ctx := context.Background()
	checks := []struct {
		name string
		err  error
	}{
		{"create", func() error { _, err := backend.Create(ctx, "/"); return err }()},
		{"rename source", backend.Rename(ctx, "/", "/renamed")},
		{"rename destination", backend.Rename(ctx, "/missing", "/")},
		{"copy source", backend.Copy(ctx, "/", "/copy")},
		{"copy destination", backend.Copy(ctx, "/missing", "/")},
	}
	for _, check := range checks {
		if !errors.Is(check.err, os.ErrPermission) {
			t.Errorf("%s root mutation = %v", check.name, check.err)
		}
	}
}

type webDAVProgressCapture struct {
	mu      sync.Mutex
	updates []string
}

func (*webDAVProgressCapture) UpdateScan(string, int64, int64) {}
func (r *webDAVProgressCapture) UpdateTransfer(action, filename string, currentPct int, _ string, _ int, _ string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, fmt.Sprintf("%s:%s:%d", action, filename, currentPct))
}
func (*webDAVProgressCapture) IsCancelled() bool { return false }
func (r *webDAVProgressCapture) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.updates...)
}

func TestWebDAVFullDownloadReportsProgress(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("data", 1024)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/file.bin", "file.bin", strconv.Itoa(len(content)), false)))
		case http.MethodGet:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK) // Deliberately ignore Range to force a full spool.
			_, _ = io.WriteString(w, content)
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	reporter := &webDAVProgressCapture{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(reporter))
	r, err := backend.Open(ctx, "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	// zoin-bot: Copy progress under the capture mutex before asserting; the
	// HTTP transport may still be delivering callbacks on another goroutine.
	updates := reporter.snapshot()
	if len(updates) < 2 || !strings.HasPrefix(updates[0], "Downloading:file.bin:") ||
		!strings.HasSuffix(updates[len(updates)-1], ":100") {
		t.Fatalf("download progress updates = %v", updates)
	}
}

func TestWebDAVFullDownloadReportsViewerProgress(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("viewer-data", 4096)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/file.bin", "file.bin", strconv.Itoa(len(content)), false)))
		case http.MethodGet:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.Header().Set("ETag", `"version-one"`)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, content)
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	var percentages []int
	ctx := context.WithValue(context.Background(), vfs.ProgressKey, vfs.ProgressCallback(func(message string, percent int) {
		if message != "Downloading file..." {
			t.Errorf("progress message = %q", message)
		}
		percentages = append(percentages, percent)
	}))
	r, err := backend.Open(ctx, "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	if len(percentages) < 2 || percentages[0] != 0 || percentages[len(percentages)-1] != 100 {
		t.Fatalf("viewer progress = %v", percentages)
	}
	for index := 1; index < len(percentages); index++ {
		if percentages[index] < percentages[index-1] {
			t.Fatalf("viewer progress is not monotonic: %v", percentages)
		}
	}
}

func TestWebDAVFullDownloadSessionCacheAndInvalidation(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	content := "first payload"
	revision := "weak-one"
	getRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		currentContent, currentRevision := content, revision
		mu.Unlock()
		switch r.Method {
		case "PROPFIND":
			response := davResponseXML("/file.txt", "file.txt", strconv.Itoa(len(currentContent)), false)
			response = strings.Replace(response, `&quot;version-one&quot;`, `W/&quot;`+currentRevision+`&quot;`, 1)
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(response))
		case http.MethodGet:
			mu.Lock()
			getRequests++
			mu.Unlock()
			w.Header().Set("ETag", `W/"`+currentRevision+`"`)
			w.Header().Set("Content-Length", strconv.Itoa(len(currentContent)))
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, currentContent)
		case http.MethodPut:
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read PUT body: %v", err)
			}
			mu.Lock()
			content = string(data)
			revision = "weak-two"
			mu.Unlock()
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if got := readWebDAVFile(t, backend, "/file.txt"); got != "first payload" {
		t.Fatalf("first read = %q", got)
	}
	if got := readWebDAVFile(t, backend, "/file.txt"); got != "first payload" {
		t.Fatalf("cached read = %q", got)
	}
	mu.Lock()
	requestsAfterCacheHit := getRequests
	mu.Unlock()
	if requestsAfterCacheHit != 1 {
		t.Fatalf("GET requests after reopen = %d, want 1", requestsAfterCacheHit)
	}
	createWebDAVFile(t, backend, "/file.txt", "second payload")
	if got := readWebDAVFile(t, backend, "/file.txt"); got != "second payload" {
		t.Fatalf("read after PUT = %q", got)
	}
	mu.Lock()
	requestsAfterInvalidation := getRequests
	mu.Unlock()
	if requestsAfterInvalidation != 2 {
		t.Fatalf("GET requests after cache invalidation = %d, want 2", requestsAfterInvalidation)
	}

	backend.downloads.mu.Lock()
	var cachedPaths []string
	for _, entry := range backend.downloads.entries {
		cachedPaths = append(cachedPaths, entry.path)
	}
	for entry := range backend.downloads.retired {
		cachedPaths = append(cachedPaths, entry.path)
	}
	backend.downloads.mu.Unlock()
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	for _, cachedPath := range cachedPaths {
		if _, err := os.Stat(cachedPath); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("cache file %q remains after Close: %v", cachedPath, err)
		}
	}
}

func TestWebDAVNoETagDoesNotReuseSameSecondSameSizeDownload(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	content := "abc"
	getRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		current := content
		mu.Unlock()
		switch r.Method {
		case "PROPFIND":
			response := davResponseXML("/file.txt", "file.txt", strconv.Itoa(len(current)), false)
			response = strings.Replace(response, `<D:getetag>&quot;version-one&quot;</D:getetag>`, "", 1)
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(response))
		case http.MethodGet:
			mu.Lock()
			getRequests++
			mu.Unlock()
			w.Header().Set("Content-Length", strconv.Itoa(len(current)))
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, current)
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if got := readWebDAVFile(t, backend, "/file.txt"); got != "abc" {
		t.Fatalf("first read=%q", got)
	}
	mu.Lock()
	content = "xyz" // same length and unchanged one-second Last-Modified value
	mu.Unlock()
	if got := readWebDAVFile(t, backend, "/file.txt"); got != "xyz" {
		t.Fatalf("second read served stale cache content %q", got)
	}
	mu.Lock()
	requests := getRequests
	mu.Unlock()
	if requests != 2 {
		t.Fatalf("GET requests=%d, want a fresh request without an ETag", requests)
	}
}

func TestWebDAVMissingGETValidatorDoesNotReusePROPFINDValidatorCacheKey(t *testing.T) {
	t.Parallel()
	content := "abc"
	getRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			response := strings.Replace(davResponseXML("/file.txt", "file.txt", "3", false),
				`&quot;version-one&quot;`, `W/&quot;stable-metadata&quot;`, 1)
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(response))
		case http.MethodGet:
			getRequests++
			w.Header().Set("Content-Length", "3")
			w.WriteHeader(http.StatusOK) // Deliberately omits ETag.
			_, _ = io.WriteString(w, content)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if got := readWebDAVFile(t, backend, "/file.txt"); got != "abc" {
		t.Fatalf("first read=%q", got)
	}
	content = "xyz"
	if got := readWebDAVFile(t, backend, "/file.txt"); got != "xyz" {
		t.Fatalf("second read served stale validator cache=%q", got)
	}
	if getRequests != 2 {
		t.Fatalf("GET requests=%d, want a fresh GET when its validator is missing", getRequests)
	}
}

func TestWebDAVRetiredCacheFileRemovedAfterLastReaderCloses(t *testing.T) {
	t.Parallel()
	cache := newWebDAVDownloadCache()
	t.Cleanup(cache.close)
	newTemp := func(content string) *providerTempReader {
		f, err := os.CreateTemp("", "f4-webdav-cache-lifecycle-*")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, content); err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
			t.Fatal(err)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			_ = f.Close()
			_ = os.Remove(f.Name())
			t.Fatal(err)
		}
		return newProviderTempReader(f, f.Name(), int64(len(content)))
	}

	reader, err := cache.install("/large.bin", `W/"one"`, newTemp("first"))
	if err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	oldPath := cache.entries["/large.bin"].path
	cache.mu.Unlock()
	cache.invalidate("/large.bin")
	if _, err := os.Stat(oldPath); err != nil {
		t.Fatalf("active retired cache file disappeared before reader Close: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired cache file remains after last reader Close: %v", err)
	}

	reader, err = cache.install("/large.bin", `W/"two"`, newTemp("second"))
	if err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	currentPath := cache.entries["/large.bin"].path
	cache.mu.Unlock()
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(currentPath); err != nil {
		t.Fatalf("current cache entry removed when its reader closed: %v", err)
	}
	cache.close()
	if _, err := os.Stat(currentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("current cache file remains after cache Close: %v", err)
	}
}

func TestWebDAVDownloadCacheEnforcesLRUBudgetWithoutEvictingActiveReader(t *testing.T) {
	t.Parallel()
	cache := newWebDAVDownloadCache()
	cache.maxEntries = 1
	cache.maxBytes = 8
	t.Cleanup(cache.close)
	newTemp := func(content string) *providerTempReader {
		f, err := os.CreateTemp("", "f4-webdav-cache-budget-*")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, content); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		return newProviderTempReader(f, f.Name(), int64(len(content)))
	}

	active, err := cache.install("/active.bin", `W/"active"`, newTemp("aaaa"))
	if err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	activePath := cache.entries["/active.bin"].path
	cache.mu.Unlock()
	private, err := cache.install("/private.bin", `W/"private"`, newTemp("bbbb"))
	if err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	_, activeCached := cache.entries["/active.bin"]
	_, privateCached := cache.entries["/private.bin"]
	entryCount, cachedBytes := len(cache.entries), cache.bytes
	cache.mu.Unlock()
	if !activeCached || privateCached || entryCount != 1 || cachedBytes != 4 {
		t.Fatalf("cache after active-pressure insert: active=%v private=%v entries=%d bytes=%d", activeCached, privateCached, entryCount, cachedBytes)
	}
	privateReader, ok := private.(*webDAVCachedReader)
	if !ok {
		t.Fatalf("private reader type=%T", private)
	}
	privatePath := privateReader.entry.path
	if err := private.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(privatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("uncached pressure response remains after Close: %v", err)
	}
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active cached response was evicted: %v", err)
	}
	if err := active.Close(); err != nil {
		t.Fatal(err)
	}
	newest, err := cache.install("/newest.bin", `W/"newest"`, newTemp("cccc"))
	if err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	newestPath := cache.entries["/newest.bin"].path
	_, oldStillCached := cache.entries["/active.bin"]
	cache.mu.Unlock()
	if oldStillCached {
		t.Fatal("inactive least-recently-used entry was not evicted")
	}
	if _, err := os.Stat(activePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inactive LRU path remains after eviction: %v", err)
	}
	if err := newest.Close(); err != nil {
		t.Fatal(err)
	}
	cache.close()
	if _, err := os.Stat(newestPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("newest cache path remains after Close: %v", err)
	}
}

func TestWebDAVPinnedFullDownloadRejectsWrongSize(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/file.bin", "file.bin", "6", false)))
		case http.MethodGet:
			w.Header().Set("ETag", `"version-one"`)
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "abc")
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	var percentages []int
	ctx := context.WithValue(context.Background(), vfs.ProgressKey, vfs.ProgressCallback(func(_ string, percent int) {
		percentages = append(percentages, percent)
	}))
	_, err := backend.Open(ctx, "/file.bin")
	if !errors.Is(err, ErrRemoteObjectChanged) {
		t.Fatalf("truncated pinned download = %v", err)
	}
	for _, percent := range percentages {
		if percent == 100 {
			t.Fatalf("failed download reported completion: %v", percentages)
		}
	}
}

func TestWebDAVRangeReadCloseCancelsRequest(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	readerCtx, cancel := context.WithCancel(context.Background())
	reader := &webDAVRangeReader{client: server.Client(), url: server.URL, size: 10, etag: `"one"`, ctx: readerCtx, cancel: cancel}
	result := make(chan error, 1)
	go func() {
		_, err := reader.ReadAt(context.Background(), make([]byte, 1), 0)
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("range request did not start")
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ReadAt after Close = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not cancel range request")
	}
}

func TestWebDAVCancellationDuringPROPFIND(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><D:multistatus xmlns:D="DAV:">`)
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() {
		close(release)
		server.CloseClientConnections()
		server.Close()
	}()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- backend.ReadDir(ctx, "/", func([]RemoteEntry) {})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("PROPFIND did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled PROPFIND = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled PROPFIND did not return")
	}
}

func TestWebDAVRecursiveMkDirReportsCancellationAfterPartialCreation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	requests := 0
	client := &http.Client{Transport: mutationRoundTripper(func(req *http.Request) (*http.Response, error) {
		requests++
		cancel()
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}, nil
	})}
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: client}, WebDAVSettings{
		BaseURL: "https://dav.example", Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	err := backend.MkDir(ctx, "/created/not-created")
	var partial *vfs.PartialOperationError
	if !errors.As(err, &partial) || !errors.Is(err, vfs.ErrOperationPartial) || !errors.Is(err, context.Canceled) {
		t.Fatalf("partially cancelled MKCOL = %v", err)
	}
	if requests != 1 || !reflect.DeepEqual(partial.Completed, []string{"/created"}) ||
		!reflect.DeepEqual(partial.Failed, []string{"/created/not-created"}) {
		t.Fatalf("partially cancelled MKCOL details: requests=%d, error=%#v", requests, partial)
	}
}

func TestWebDAVRangeReaderFallsBackWhenServerLaterReturnsFullBody(t *testing.T) {
	t.Parallel()
	const content = "0123456789"
	var mu sync.Mutex
	getRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/file.bin", "file.bin", strconv.Itoa(len(content)), false)))
		case http.MethodGet:
			mu.Lock()
			getRequests++
			requestNumber := getRequests
			mu.Unlock()
			w.Header().Set("ETag", `"version-one"`)
			if requestNumber == 1 {
				w.Header().Set("Content-Range", "bytes 0-0/10")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.WriteString(w, content[:1])
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK) // Intermediary stopped honoring Range.
			_, _ = io.WriteString(w, content)
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	r, err := backend.Open(context.Background(), "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 4)
	if n, err := r.ReadAt(context.Background(), buffer, 3); err != nil || n != 4 || string(buffer) != "3456" {
		t.Fatalf("fallback ReadAt = %d, %v, %q", n, err, buffer)
	}
	if n, err := r.ReadAt(context.Background(), buffer, 6); err != nil || n != 4 || string(buffer) != "6789" {
		t.Fatalf("cached-handle ReadAt = %d, %v, %q", n, err, buffer)
	}
	_ = r.Close()
	if got := readWebDAVFile(t, backend, "/file.bin"); got != content {
		t.Fatalf("session-cached reopen = %q", got)
	}
	mu.Lock()
	requests := getRequests
	mu.Unlock()
	if requests != 2 {
		t.Fatalf("GET requests = %d, want probe + one fallback", requests)
	}
}

func TestWebDAVRangeReaderContinuesShortPartialResponses(t *testing.T) {
	t.Parallel()
	const content = "0123456789"
	var mu sync.Mutex
	var ranges []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/file.bin", "file.bin", strconv.Itoa(len(content)), false)))
		case http.MethodGet:
			rangeHeader := r.Header.Get("Range")
			mu.Lock()
			ranges = append(ranges, rangeHeader)
			mu.Unlock()
			var start, end int
			if _, err := fmt.Sscanf(rangeHeader, "bytes=%d-%d", &start, &end); err != nil {
				http.Error(w, "bad range", http.StatusBadRequest)
				return
			}
			if end > start+1 {
				end = start + 1 // Deliberately cap each response at two bytes.
			}
			w.Header().Set("ETag", `"version-one"`)
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(content)))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, content[start:end+1])
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	r, err := backend.Open(context.Background(), "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close() }()
	buffer := make([]byte, 6)
	if n, err := r.ReadAt(context.Background(), buffer, 2); err != nil || n != 6 || string(buffer) != "234567" {
		t.Fatalf("short-part continuation = %d, %v, %q", n, err, buffer)
	}
	mu.Lock()
	gotRanges := append([]string(nil), ranges...)
	mu.Unlock()
	wantRanges := []string{"bytes=0-0", "bytes=2-7", "bytes=4-7", "bytes=6-7"}
	if !reflect.DeepEqual(gotRanges, wantRanges) {
		t.Fatalf("ranges = %v, want %v", gotRanges, wantRanges)
	}
}

func TestWebDAVRangeReaderDetectsChangedSizeFrom416(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Range", "bytes */6")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	}))
	defer server.Close()
	readerCtx, cancel := context.WithCancel(context.Background())
	reader := &webDAVRangeReader{client: server.Client(), url: server.URL, size: 5, etag: `"one"`, ctx: readerCtx, cancel: cancel}
	defer func() { _ = reader.Close() }()
	if _, err := reader.ReadAt(context.Background(), make([]byte, 1), 0); !errors.Is(err, ErrRemoteObjectChanged) {
		t.Fatalf("416 after size change = %v", err)
	}
}

func TestWebDAVRetriesCanonicalCollectionPROPFIND(t *testing.T) {
	t.Parallel()
	var methods, bodies, depths []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		methods = append(methods, r.Method)
		bodies = append(bodies, string(data))
		depths = append(depths, r.Header.Get("Depth"))
		switch r.URL.Path {
		case "/dav/root/dir":
			http.Redirect(w, r, "/dav/root/dir/", http.StatusMovedPermanently)
		case "/dav/root/dir/":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(
				davResponseXML("/dav/root/dir/", "dir", "0", true),
				davResponseXML("/dav/root/dir/child.txt", "child.txt", "1", false),
			))
		default:
			http.Error(w, "unexpected path", http.StatusNotFound)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	entry, err := backend.Stat(context.Background(), "/dir")
	if err != nil || !entry.IsDir {
		t.Fatalf("canonical Stat = %#v, %v", entry, err)
	}
	var entries []RemoteEntry
	if err := backend.ReadDir(context.Background(), "/dir", func(chunk []RemoteEntry) { entries = append(entries, chunk...) }); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(methods, []string{"PROPFIND", "PROPFIND", "PROPFIND"}) || !reflect.DeepEqual(depths, []string{"0", "0", "1"}) ||
		len(bodies) != 3 || bodies[0] == "" || bodies[0] != bodies[1] || bodies[1] != bodies[2] {
		t.Fatalf("canonical retry methods=%v depths=%v bodies=%v", methods, depths, bodies)
	}
	if len(entries) != 1 || entries[0].Name != "child.txt" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestWebDAVMkDirUsesCanonicalCollectionURI(t *testing.T) {
	t.Parallel()
	var canonicalRequests, slashlessRequests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "MKCOL" {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/dav/root/created/" {
			canonicalRequests++
			w.WriteHeader(http.StatusCreated)
			return
		}
		slashlessRequests++
		w.Header().Set("Location", "/dav/root/created/")
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if err := backend.MkDir(context.Background(), "/created"); err != nil {
		t.Fatal(err)
	}
	if canonicalRequests != 1 || slashlessRequests != 0 {
		t.Fatalf("canonical MKCOL requests=%d slashless requests=%d", canonicalRequests, slashlessRequests)
	}
}

func TestWebDAVDirectoryMutationsUseCollectionURLs(t *testing.T) {
	t.Parallel()
	type mutationRequest struct {
		method, path, destination, overwrite string
	}
	var requests []mutationRequest
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			if strings.TrimSuffix(r.URL.Path, "/") == "/dav/root/copy" {
				http.Error(w, "missing", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML(r.URL.Path+"/", "source", "0", true))) // #nosec G705 -- the local test server returns its controlled request path to exercise collection-URL handling.
			return
		}
		requests = append(requests, mutationRequest{method: r.Method, path: r.URL.Path, destination: r.Header.Get("Destination"), overwrite: r.Header.Get("Overwrite")})
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusNoContent)
		} else {
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	ctx := context.Background()
	if err := backend.Copy(ctx, "/source", "/copy"); err != nil {
		t.Fatal(err)
	}
	if err := backend.Rename(vfs.WithDestinationOverwrite(ctx, false), "/source", "/moved"); err != nil {
		t.Fatal(err)
	}
	if err := backend.Remove(ctx, "/source"); err != nil {
		t.Fatal(err)
	}
	if len(requests) != 3 {
		t.Fatalf("mutation requests = %#v", requests)
	}
	for _, request := range requests {
		if request.path != "/dav/root/source/" {
			t.Errorf("%s source path = %q", request.method, request.path)
		}
		if (request.method == "COPY" || request.method == "MOVE") && !strings.HasSuffix(request.destination, "/") {
			t.Errorf("%s Destination = %q", request.method, request.destination)
		}
		if request.method == "COPY" && request.overwrite != "F" {
			t.Errorf("COPY Overwrite = %q", request.overwrite)
		}
		if request.method == "MOVE" && request.overwrite != "F" {
			t.Errorf("MOVE Overwrite = %q", request.overwrite)
		}
	}
}

func TestWebDAVExplicitOverwriteIntentBecomesAtomicHeaders(t *testing.T) {
	t.Parallel()
	t.Run("PUT no-replace", func(t *testing.T) {
		var ifNoneMatch string
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPut {
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
				return
			}
			ifNoneMatch = r.Header.Get("If-None-Match")
			if ifNoneMatch == "*" {
				w.WriteHeader(http.StatusPreconditionFailed) // Simulate a concurrent creator.
				return
			}
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "secret"})
		writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), false), "/race.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(writer, "payload")
		if err := writer.Close(); !errors.Is(err, os.ErrExist) {
			t.Fatalf("conditional PUT collision=%v, want os.ErrExist", err)
		}
		if ifNoneMatch != "*" {
			t.Fatalf("If-None-Match=%q, want *", ifNoneMatch)
		}
	})

	t.Run("PUT overwrite", func(t *testing.T) {
		var ifNoneMatch string
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ifNoneMatch = r.Header.Get("If-None-Match")
			w.WriteHeader(http.StatusNoContent)
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "secret"})
		writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), true), "/replace.txt")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.WriteString(writer, "payload")
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		if ifNoneMatch != "" {
			t.Fatalf("overwrite PUT unexpectedly sent If-None-Match=%q", ifNoneMatch)
		}
	})

	t.Run("COPY", func(t *testing.T) {
		var destinationStats int
		var overwrites []string
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case "PROPFIND":
				if r.URL.Path != "/source.txt" {
					destinationStats++
					http.Error(w, "missing", http.StatusNotFound)
					return
				}
				w.WriteHeader(http.StatusMultiStatus)
				_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/source.txt", "source.txt", "1", false)))
			case "COPY":
				overwrites = append(overwrites, r.Header.Get("Overwrite"))
				w.WriteHeader(http.StatusCreated)
			default:
				http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			}
		}))
		defer server.Close()
		backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
			BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
		}, SecretValues{"password": "secret"})
		if err := backend.Copy(vfs.WithDestinationOverwrite(context.Background(), false), "/source.txt", "/missing.txt"); err != nil {
			t.Fatal(err)
		}
		if err := backend.Copy(vfs.WithDestinationOverwrite(context.Background(), true), "/source.txt", "/existing.txt"); err != nil {
			t.Fatal(err)
		}
		if destinationStats != 0 || !reflect.DeepEqual(overwrites, []string{"F", "T"}) {
			t.Fatalf("destination Stat requests=%d Overwrite headers=%v", destinationStats, overwrites)
		}
	})
}

func TestWebDAVDigestUploadReplaysBodyAndReportsProgress(t *testing.T) {
	t.Parallel()
	const (
		username = "DOMAIN\\alice"
		password = "secret"
		realm    = "upload-dav"
		nonce    = "abcdef0123456789"
	)
	var requests int
	var uploaded string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		authorization := r.Header.Get("Authorization")
		if authorization == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="upload-dav", nonce="abcdef0123456789", algorithm=SHA-512-256, qop="auth"`)
			http.Error(w, "authenticate", http.StatusUnauthorized)
			return
		}
		params := parseAuthParams(strings.TrimPrefix(authorization, "Digest "))
		ha1, _ := digestHash("SHA-512-256", username+":"+realm+":"+password)
		ha2, _ := digestHash("SHA-512-256", r.Method+":"+r.URL.RequestURI())
		want, _ := digestHash("SHA-512-256", ha1+":"+nonce+":"+params["nc"]+":"+params["cnonce"]+":auth:"+ha2)
		if params["username"] != username || params["response"] != want {
			t.Errorf("Digest params = %#v, response want %q", params, want)
		}
		data, _ := io.ReadAll(r.Body)
		uploaded = string(data)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "digest", Username: username, AllowInsecureDigest: true,
	}, SecretValues{"password": password})
	reporter := &webDAVProgressCapture{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(reporter))
	w, err := backend.Create(ctx, "/upload.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "digest payload")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || uploaded != "digest payload" {
		t.Fatalf("requests=%d uploaded=%q", requests, uploaded)
	}
	updates := reporter.snapshot()
	if len(updates) < 2 || updates[0] != "Uploading:upload.txt:0" ||
		!strings.HasSuffix(updates[len(updates)-1], ":100") {
		t.Fatalf("upload progress = %v", updates)
	}
}

func TestWebDAVDigestUploadReportsProgressWhenChallengeDrainsBody(t *testing.T) {
	t.Parallel()
	payload := bytes.Repeat([]byte("digest-progress-"), 64<<10)
	firstDrained := make(chan struct{})
	releaseChallenge := make(chan struct{})
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read Digest PUT body: %v", err)
		}
		if !bytes.Equal(data, payload) {
			t.Errorf("Digest PUT body length=%d, want %d", len(data), len(payload))
		}
		if r.Header.Get("Authorization") == "" {
			close(firstDrained)
			<-releaseChallenge
			w.Header().Set("WWW-Authenticate", `Digest realm="dav", nonce="first", algorithm=SHA-256, qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "digest", Username: "alice", AllowInsecureDigest: true,
	}, SecretValues{"password": "secret"})
	reporter := &webDAVProgressCapture{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(reporter))
	writer, err := backend.Create(ctx, "/large.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	closeResult := make(chan error, 1)
	go func() { closeResult <- writer.Close() }()
	select {
	case <-firstDrained:
	case <-time.After(5 * time.Second):
		t.Fatal("Digest challenge did not drain the first PUT body")
	}
	updatesWhileBlocked := reporter.snapshot()
	progressed := false
	for _, update := range updatesWhileBlocked {
		if !strings.HasSuffix(update, ":0") {
			progressed = true
			break
		}
	}
	close(releaseChallenge)
	if !progressed {
		t.Fatalf("first unauthenticated PUT drained with frozen progress: %v", updatesWhileBlocked)
	}
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Digest authenticated retry did not finish")
	}
	updates := reporter.snapshot()
	if requests != 2 || len(updates) == 0 || !strings.HasSuffix(updates[len(updates)-1], ":100") {
		t.Fatalf("Digest retry requests=%d progress=%v", requests, updates)
	}
}

func TestWebDAVFailedUploadDoesNotReportCompletion(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		http.Error(w, "storage failure", http.StatusInternalServerError)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	reporter := &webDAVProgressCapture{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(reporter))
	w, err := backend.Create(ctx, "/failed.bin")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write(bytes.Repeat([]byte{0x7a}, 512<<10))
	if err := w.Close(); !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("failed PUT = %v", err)
	}
	updates := reporter.snapshot()
	if len(updates) < 2 || updates[0] != "Uploading:failed.bin:0" {
		t.Fatalf("failed upload progress = %v", updates)
	}
	for _, update := range updates {
		if strings.HasSuffix(update, ":100") {
			t.Fatalf("failed upload reported completion: %v", updates)
		}
	}
}

func TestWebDAVRejectsUnsupportedDigestQOP(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Digest realm="dav", nonce="nonce", algorithm=SHA-256, qop="auth-int"`)
		http.Error(w, "authenticate", http.StatusUnauthorized)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "digest", Username: "alice", AllowInsecureDigest: true,
	}, SecretValues{"password": "secret"})
	_, err := backend.Stat(context.Background(), "/")
	if err == nil || !strings.Contains(err.Error(), "does not offer qop=auth") {
		t.Fatalf("auth-int-only challenge = %v", err)
	}
}

func TestWebDAVSelectsStrongestSupportedDigestChallenge(t *testing.T) {
	t.Parallel()
	challenge, err := parseDigestChallenge([]string{
		`Basic realm="fallback", Digest realm="dav", nonce="md5", algorithm=MD5, qop="auth"`,
		`Digest realm="dav", nonce="unsupported", algorithm=SHA-999, qop="auth"`,
		`Digest realm="dav", nonce="strong", algorithm=SHA-512-256, qop="auth", Digest realm="dav", nonce="sha", algorithm=SHA-256, qop="auth"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if challenge.algorithm != "SHA-512-256" || challenge.nonce != "strong" || challenge.qop != "auth" {
		t.Fatalf("selected challenge = %#v", challenge)
	}
}

func TestWebDAVDigestAuthorizationAlgorithmMatrix(t *testing.T) {
	t.Parallel()
	algorithms := []string{"MD5", "MD5-SESS", "SHA-256", "SHA-256-SESS", "SHA-512-256", "SHA-512-256-SESS"}
	for _, algorithm := range algorithms {
		algorithm := algorithm
		t.Run(algorithm, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, "https://dav.example/root/file.txt?download=1", nil)
			if err != nil {
				t.Fatal(err)
			}
			challenge := &digestChallenge{realm: "dav", nonce: "nonce", opaque: "opaque", algorithm: algorithm, qop: "auth"}
			authorization, err := digestAuthorization("alice", "secret", req, challenge, 7)
			if err != nil {
				t.Fatal(err)
			}
			params := parseAuthParams(strings.TrimPrefix(authorization, "Digest "))
			ha1, _ := digestHash(algorithm, "alice:dav:secret")
			if strings.HasSuffix(algorithm, "-SESS") {
				ha1, _ = digestHash(algorithm, ha1+":nonce:"+params["cnonce"])
			}
			ha2, _ := digestHash(algorithm, http.MethodGet+":"+req.URL.RequestURI())
			want, _ := digestHash(algorithm, ha1+":nonce:00000007:"+params["cnonce"]+":auth:"+ha2)
			if params["algorithm"] != algorithm || params["response"] != want || params["opaque"] != "opaque" ||
				params["nc"] != "00000007" || params["qop"] != "auth" || params["cnonce"] == "" {
				t.Fatalf("Digest %s params = %#v, response want %q", algorithm, params, want)
			}
		})
	}

	t.Run("legacy MD5 without qop", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "https://dav.example/root", nil)
		challenge := &digestChallenge{realm: "dav", nonce: "legacy", algorithm: "MD5"}
		authorization, err := digestAuthorization("alice", "secret", req, challenge, 1)
		if err != nil {
			t.Fatal(err)
		}
		params := parseAuthParams(strings.TrimPrefix(authorization, "Digest "))
		ha1, _ := digestHash("MD5", "alice:dav:secret")
		ha2, _ := digestHash("MD5", http.MethodGet+":"+req.URL.RequestURI())
		want, _ := digestHash("MD5", ha1+":legacy:"+ha2)
		if params["response"] != want || params["qop"] != "" || params["nc"] != "" {
			t.Fatalf("legacy Digest params = %#v", params)
		}
	})
}

func TestWebDAVDigestStaleNonceResetsCounter(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		params := parseAuthParams(strings.TrimPrefix(r.Header.Get("Authorization"), "Digest "))
		switch requests {
		case 1:
			w.Header().Set("WWW-Authenticate", `Digest realm="dav", nonce="first", algorithm=SHA-256, qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
		case 2:
			if params["nonce"] != "first" || params["nc"] != "00000001" {
				t.Errorf("first authenticated request = %#v", params)
			}
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/", "/", "0", true)))
		case 3:
			if params["nonce"] != "first" || params["nc"] != "00000002" {
				t.Errorf("preemptive authenticated request = %#v", params)
			}
			w.Header().Set("WWW-Authenticate", `Digest realm="dav", nonce="second", algorithm=SHA-256, qop="auth", stale=true`)
			w.WriteHeader(http.StatusUnauthorized)
		case 4:
			if params["nonce"] != "second" || params["nc"] != "00000001" {
				t.Errorf("stale-nonce retry = %#v", params)
			}
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/", "/", "0", true)))
		default:
			http.Error(w, "too many requests", http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "digest", Username: "alice", AllowInsecureDigest: true,
	}, SecretValues{"password": "secret"})
	if _, err := backend.Stat(context.Background(), "/"); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Stat(context.Background(), "/"); err != nil {
		t.Fatal(err)
	}
	if requests != 4 {
		t.Fatalf("Digest requests = %d", requests)
	}
}

func TestWebDAVReadRedirectCannotLeaveConfiguredRoot(t *testing.T) {
	t.Parallel()
	outsideRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PROPFIND" && r.URL.Path == "/dav/root/file.txt":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/file.txt", "file.txt", "1", false)))
		case r.Method == http.MethodGet && r.URL.Path == "/dav/root/file.txt":
			http.Redirect(w, r, "/outside/credential-target", http.StatusTemporaryRedirect)
		case r.URL.Path == "/outside/credential-target":
			outsideRequests++
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "x")
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if _, err := backend.Open(context.Background(), "/file.txt"); err == nil {
		t.Fatal("out-of-root read redirect unexpectedly succeeded")
	}
	if outsideRequests != 0 {
		t.Fatalf("out-of-root redirect target received %d authenticated request(s)", outsideRequests)
	}
}

func TestWebDAVMutationRedirectMatrix(t *testing.T) {
	t.Parallel()
	methods := []string{"MKCOL", http.MethodPut, http.MethodDelete, "COPY", "MOVE"}
	statuses := []int{http.StatusMovedPermanently, http.StatusFound, http.StatusSeeOther, http.StatusTemporaryRedirect, http.StatusPermanentRedirect}
	for _, method := range methods {
		method := method
		for _, status := range statuses {
			status := status
			t.Run(fmt.Sprintf("%s_%d", method, status), func(t *testing.T) {
				targetRequests := 0
				spoolPath := ""
				sourcePath, targetPath := "/source", "/target"
				if method == "MKCOL" {
					sourcePath, targetPath = "/source/", "/target/"
				}
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == targetPath {
						targetRequests++
						w.WriteHeader(http.StatusNoContent)
						return
					}
					if r.URL.Path != sourcePath || r.Method != method {
						http.Error(w, "unexpected", http.StatusNotFound)
						return
					}
					http.Redirect(w, r, targetPath+"?token=webdav-redirect-secret", status)
				}))
				defer server.Close()
				backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
					BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
				}, SecretValues{"password": "secret"})
				var err error
				switch method {
				case "MKCOL":
					err = backend.MkDir(context.Background(), "/source")
				case http.MethodPut:
					var writer io.WriteCloser
					writer, err = backend.Create(context.Background(), "/source")
					if err == nil {
						if spool, ok := writer.(*providerSpoolWriter); ok {
							spoolPath = spool.path
						}
						_, _ = io.WriteString(writer, "payload")
						err = writer.Close()
					}
				default:
					err = backend.mutation(context.Background(), method, "/source", nil)
				}
				if !errors.Is(err, vfs.ErrOperationStateUnknown) {
					t.Fatalf("redirected %s HTTP %d = %v", method, status, err)
				}
				if strings.Contains(err.Error(), "webdav-redirect-secret") || strings.Contains(err.Error(), "?token=") {
					t.Fatalf("redirected %s HTTP %d leaked Location query: %v", method, status, err)
				}
				if targetRequests != 0 {
					t.Fatalf("redirected %s HTTP %d contacted target %d time(s)", method, status, targetRequests)
				}
				if spoolPath != "" {
					if _, statErr := os.Stat(spoolPath); !errors.Is(statErr, os.ErrNotExist) {
						t.Fatalf("redirected PUT left upload spool %q: %v", spoolPath, statErr)
					}
				}
			})
		}
	}
}

func TestWebDAVExternalServerLifecycle(t *testing.T) {
	baseURL := strings.TrimSpace(os.Getenv("F4_WEBDAV_TEST_URL"))
	if baseURL == "" {
		t.Skip("F4_WEBDAV_TEST_URL is not set")
	}
	auth := strings.TrimSpace(os.Getenv("F4_WEBDAV_TEST_AUTH"))
	if auth == "" {
		auth = "basic"
	}
	username := os.Getenv("F4_WEBDAV_TEST_USERNAME")
	password := os.Getenv("F4_WEBDAV_TEST_PASSWORD")
	settings := WebDAVSettings{BaseURL: baseURL, Root: "/", Auth: auth, Username: username, AllowInsecureDigest: auth == "digest" && strings.HasPrefix(baseURL, "http://")}
	backend := openTestWebDAV(t, &WebDAVFactory{}, settings, SecretValues{"password": password, "bearer_token": password})
	root := fmt.Sprintf("/f4-cloudfox-e2e-%d", time.Now().UnixNano())
	ctx := context.Background()
	if err := backend.MkDir(ctx, root+"/nested"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Remove(context.Background(), root) })
	file := root + "/nested/file.txt"
	createWebDAVFile(t, backend, file, "external WebDAV payload")
	if got := readWebDAVFile(t, backend, file); got != "external WebDAV payload" {
		t.Fatalf("read = %q", got)
	}
	treeSource := root + "/tree/source/nested"
	if err := backend.MkDir(ctx, treeSource); err != nil {
		t.Fatal(err)
	}
	createWebDAVFile(t, backend, treeSource+"/child.txt", "external nested payload")
	treeCopy := root + "/tree/copied"
	if err := backend.Copy(ctx, root+"/tree/source", treeCopy); err != nil {
		t.Fatal(err)
	}
	if got := readWebDAVFile(t, backend, treeCopy+"/nested/child.txt"); got != "external nested payload" {
		t.Fatalf("recursive copy read = %q", got)
	}
	treeMoved := root + "/tree/moved"
	if err := backend.Rename(ctx, treeCopy, treeMoved); err != nil {
		t.Fatal(err)
	}
	if got := readWebDAVFile(t, backend, treeMoved+"/nested/child.txt"); got != "external nested payload" {
		t.Fatalf("recursive move read = %q", got)
	}
	if err := backend.Remove(ctx, treeMoved); err != nil {
		t.Fatal(err)
	}
	copyPath := root + "/copied.txt"
	if err := backend.Copy(ctx, file, copyPath); err != nil {
		t.Fatal(err)
	}
	movedPath := root + "/moved.txt"
	if err := backend.Rename(ctx, copyPath, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := backend.Remove(ctx, movedPath); err != nil {
		t.Fatal(err)
	}
	if err := backend.Remove(ctx, file); err != nil {
		t.Fatal(err)
	}
	if err := backend.Remove(ctx, root); err != nil {
		t.Fatal(err)
	}
}
