package cloudfox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"golang.org/x/net/webdav"
)

type edgeWebDAVHarness struct {
	server  *httptest.Server
	backend *webDAVBackend
}

type edgeCountingBody struct {
	reader *bytes.Reader
	read   int
}

func (b *edgeCountingBody) Read(p []byte) (int, error) {
	n, err := b.reader.Read(p)
	b.read += n
	return n, err
}
func (*edgeCountingBody) Close() error { return nil }

func newEdgeWebDAVHarness(t *testing.T) edgeWebDAVHarness {
	t.Helper()
	fs := webdav.NewMemFS()
	if err := fs.Mkdir(context.Background(), "/root", 0o755); err != nil {
		t.Fatal(err)
	}
	handler := &webdav.Handler{Prefix: "/dav", FileSystem: fs, LockSystem: webdav.NewMemLS()}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "edge-user" || password != "edge-secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="edge-webdav"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "edge-user",
	}, SecretValues{"password": "edge-secret"})
	return edgeWebDAVHarness{server: server, backend: backend}
}

func edgeWriteWebDAVFile(t *testing.T, backend *webDAVBackend, location string, data []byte) {
	t.Helper()
	w, err := backend.Create(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func edgeReadWebDAVFile(t *testing.T, backend *webDAVBackend, location string) []byte {
	t.Helper()
	r, err := backend.Open(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	data := make([]byte, r.Size())
	if len(data) == 0 {
		return data
	}
	n, err := r.ReadAt(context.Background(), data, 0)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if n != len(data) {
		t.Fatalf("ReadAt(%q) = %d bytes, want %d", location, n, len(data))
	}
	return data
}

func TestWebDAVEdgeSettingsAndAuthenticationMatrix(t *testing.T) {
	t.Parallel()
	factory := &WebDAVFactory{}
	validation := []struct {
		name     string
		settings WebDAVSettings
		wantErr  string
	}{
		{name: "default basic over TLS", settings: WebDAVSettings{BaseURL: "https://dav.example/dav"}},
		{name: "bearer over TLS", settings: WebDAVSettings{BaseURL: "https://dav.example/dav", Auth: "BeArEr"}},
		{name: "digest over TLS", settings: WebDAVSettings{BaseURL: "https://dav.example/dav", Auth: "digest"}},
		{name: "explicit insecure digest", settings: WebDAVSettings{BaseURL: "http://dav.example/dav", Auth: "digest", AllowInsecureDigest: true}},
		{name: "anonymous over HTTP", settings: WebDAVSettings{BaseURL: "http://dav.example/dav", Auth: "anonymous"}},
		{name: "missing host", settings: WebDAVSettings{BaseURL: "https:///dav"}, wantErr: "invalid WebDAV base URL"},
		{name: "unsupported scheme", settings: WebDAVSettings{BaseURL: "ftp://dav.example/dav"}, wantErr: "invalid WebDAV base URL"},
		{name: "userinfo", settings: WebDAVSettings{BaseURL: "https://alice:secret@dav.example/dav"}, wantErr: "invalid WebDAV base URL"},
		{name: "query", settings: WebDAVSettings{BaseURL: "https://dav.example/dav?token=secret"}, wantErr: "invalid WebDAV base URL"},
		{name: "fragment", settings: WebDAVSettings{BaseURL: "https://dav.example/dav#fragment"}, wantErr: "invalid WebDAV base URL"},
		{name: "encoded base separator", settings: WebDAVSettings{BaseURL: "https://dav.example/dav%2Ftenant"}, wantErr: "ambiguous WebDAV base URL path"},
		{name: "empty base segment", settings: WebDAVSettings{BaseURL: "https://dav.example/dav//tenant"}, wantErr: "ambiguous WebDAV base URL path"},
		{name: "encoded base NUL", settings: WebDAVSettings{BaseURL: "https://dav.example/dav%00tenant"}, wantErr: "ambiguous WebDAV base URL path"},
		{name: "encoded base control", settings: WebDAVSettings{BaseURL: "https://dav.example/dav%1ftenant"}, wantErr: "ambiguous WebDAV base URL path"},
		{name: "encoded base dot segment", settings: WebDAVSettings{BaseURL: "https://dav.example/dav/%2e%2e/tenant"}, wantErr: "ambiguous WebDAV base URL path"},
		{name: "literal base dot segment", settings: WebDAVSettings{BaseURL: "https://dav.example/dav/../tenant"}, wantErr: "ambiguous WebDAV base URL path"},
		{name: "root NUL", settings: WebDAVSettings{BaseURL: "https://dav.example/dav", Root: "bad\x00root"}, wantErr: "root contains NUL"},
		{name: "unknown auth", settings: WebDAVSettings{BaseURL: "https://dav.example/dav", Auth: "ntlm"}, wantErr: "unsupported WebDAV authentication"},
		{name: "basic over HTTP", settings: WebDAVSettings{BaseURL: "http://dav.example/dav", Auth: "basic"}, wantErr: "HTTPS is required"},
		{name: "bearer over HTTP", settings: WebDAVSettings{BaseURL: "http://dav.example/dav", Auth: "bearer"}, wantErr: "HTTPS is required"},
		{name: "digest over HTTP without confirmation", settings: WebDAVSettings{BaseURL: "http://dav.example/dav", Auth: "digest"}, wantErr: "explicit confirmation"},
	}
	for _, test := range validation {
		t.Run(test.name, func(t *testing.T) {
			err := factory.Validate(webDAVConnection(t, test.settings))
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate = %v, want error containing %q", err, test.wantErr)
			}
		})
	}

	normalized, base, err := factory.settings(webDAVConnection(t, WebDAVSettings{
		BaseURL: "  https://dav.example/dav/  ", Root: `\tenant\..\root\`, Auth: " BeArEr ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if normalized.BaseURL != "https://dav.example/dav/" || normalized.Root != "/root" || normalized.Auth != "bearer" || base.Path != "/dav" {
		t.Fatalf("normalized settings = %#v, base = %v", normalized, base)
	}

	missingSecrets := []struct {
		name     string
		settings WebDAVSettings
		secrets  SecretValues
	}{
		{name: "basic missing username", settings: WebDAVSettings{BaseURL: "https://dav.example", Auth: "basic"}, secrets: SecretValues{"password": "secret"}},
		{name: "basic missing password", settings: WebDAVSettings{BaseURL: "https://dav.example", Auth: "basic", Username: "alice"}},
		{name: "digest missing password", settings: WebDAVSettings{BaseURL: "https://dav.example", Auth: "digest", Username: "alice"}},
		{name: "bearer missing token", settings: WebDAVSettings{BaseURL: "https://dav.example", Auth: "bearer"}},
		{name: "bearer whitespace token", settings: WebDAVSettings{BaseURL: "https://dav.example", Auth: "bearer"}, secrets: SecretValues{"bearer_token": "  "}},
	}
	for _, test := range missingSecrets {
		t.Run(test.name, func(t *testing.T) {
			_, err := factory.Open(context.Background(), webDAVConnection(t, test.settings), test.secrets)
			if !errors.Is(err, ErrAuthenticationRequired) {
				t.Fatalf("Open = %v, want ErrAuthenticationRequired", err)
			}
		})
	}

	installedAuth := []struct {
		name      string
		settings  WebDAVSettings
		secrets   SecretValues
		transport any
	}{
		{name: "basic", settings: WebDAVSettings{BaseURL: "https://dav.example", Auth: "basic", Username: "alice"}, secrets: SecretValues{"password": " secret "}, transport: (*webDAVStaticAuthTransport)(nil)},
		{name: "bearer", settings: WebDAVSettings{BaseURL: "https://dav.example", Auth: "bearer"}, secrets: SecretValues{"bearer_token": " token "}, transport: (*webDAVStaticAuthTransport)(nil)},
		{name: "digest", settings: WebDAVSettings{BaseURL: "https://dav.example", Auth: "digest", Username: "alice"}, secrets: SecretValues{"password": "secret"}, transport: (*webDAVDigestTransport)(nil)},
	}
	for _, test := range installedAuth {
		t.Run(test.name+" transport", func(t *testing.T) {
			opened, err := factory.Open(context.Background(), webDAVConnection(t, test.settings), test.secrets)
			if err != nil {
				t.Fatal(err)
			}
			defer opened.Close()
			backend := opened.(*webDAVBackend)
			switch test.transport.(type) {
			case *webDAVStaticAuthTransport:
				if _, ok := backend.client.Transport.(*webDAVStaticAuthTransport); !ok {
					t.Fatalf("transport = %T", backend.client.Transport)
				}
			case *webDAVDigestTransport:
				if _, ok := backend.client.Transport.(*webDAVDigestTransport); !ok {
					t.Fatalf("transport = %T", backend.client.Transport)
				}
			}
		})
	}
}

func TestWebDAVEdgeReadDirBatchesMoreThanTwoHundredEntries(t *testing.T) {
	t.Parallel()
	responses := make([]string, 0, 206)
	responses = append(responses, davResponseXML("/dav/root/", "root", "0", true))
	for i := 0; i < 205; i++ {
		name := "item-" + strconv.Itoa(i) + ".txt"
		responses = append(responses, davResponseXML("/dav/root/"+name, name, strconv.Itoa(i), false))
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" || r.Header.Get("Depth") != "1" {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(responses...))
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})

	var chunkSizes []int
	var entries []RemoteEntry
	if err := backend.ReadDir(context.Background(), "/", func(chunk []RemoteEntry) {
		chunkSizes = append(chunkSizes, len(chunk))
		entries = append(entries, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(chunkSizes, []int{200, 5}) {
		t.Fatalf("chunk sizes = %v, want [200 5]", chunkSizes)
	}
	if len(entries) != 205 || entries[0].Name != "item-0.txt" || entries[204].Name != "item-204.txt" {
		t.Fatalf("entries = %d, first=%#v, last=%#v", len(entries), entries[0], entries[len(entries)-1])
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	chunkSizes = nil
	err := backend.ReadDir(ctx, "/", func(chunk []RemoteEntry) {
		chunkSizes = append(chunkSizes, len(chunk))
		cancel()
	})
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(chunkSizes, []int{200}) {
		t.Fatalf("cancelled ReadDir = %v, chunks = %v", err, chunkSizes)
	}
}

func TestWebDAVEdgePathHrefAndRootEscapeBehavior(t *testing.T) {
	t.Parallel()
	base, err := url.Parse("https://dav.example/dav")
	if err != nil {
		t.Fatal(err)
	}
	backend := &webDAVBackend{base: base, rootPath: "/root"}
	requestURL, err := url.Parse("https://dav.example/dav/root/dir/")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		href    string
		want    string
		wantErr error
	}{
		{name: "relative href", href: "child%20one.txt", want: "/dir/child one.txt"},
		{name: "absolute path", href: "/dav/root/top.txt", want: "/top.txt"},
		{name: "absolute URI", href: "https://dav.example/dav/root/top.txt", want: "/top.txt"},
		{name: "foreign absolute URI", href: "https://mirror.example/dav/root/top.txt", wantErr: os.ErrPermission},
		{name: "same-origin network path", href: "//dav.example/dav/root/top.txt", want: "/top.txt"},
		{name: "foreign network path", href: "//mirror.example/dav/root/top.txt", wantErr: os.ErrPermission},
		{name: "query identity", href: "/dav/root/top.txt?version=2", wantErr: os.ErrPermission},
		{name: "encoded slash", href: "/dav/root/a%2Fb.txt", wantErr: os.ErrPermission},
		{name: "encoded backslash", href: "/dav/root/a%5Cb.txt", wantErr: os.ErrPermission},
		{name: "relative escape", href: "../../../outside.txt", wantErr: os.ErrPermission},
		{name: "prefix confusion", href: "/dav/rooted/outside.txt", wantErr: os.ErrPermission},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := backend.locationFromHref(test.href, requestURL)
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("locationFromHref = %q, %v", got, err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("locationFromHref = %q, %v, want %q", got, err, test.want)
			}
		})
	}
	if normalized, err := backend.Normalize("/../../safe.txt"); err != nil || normalized != "/safe.txt" {
		t.Fatalf("Normalize traversal = %q, %v", normalized, err)
	}
	if _, err := backend.Normalize("bad\x00name"); err == nil {
		t.Fatal("Normalize accepted NUL")
	}
	u, err := backend.urlFor("/../../safe.txt")
	if err != nil || u.Path != "/dav/root/safe.txt" {
		t.Fatalf("urlFor traversal = %v, %v", u, err)
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/outside/secret.txt", "secret.txt", "1", false)))
	}))
	defer server.Close()
	remote := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	delivered := 0
	err = remote.ReadDir(context.Background(), "/", func(chunk []RemoteEntry) { delivered += len(chunk) })
	if !errors.Is(err, os.ErrPermission) || delivered != 0 {
		t.Fatalf("outside-root response = %v, delivered = %d", err, delivered)
	}
}

func TestWebDAVEdgeIgnoresForeignNamespacePropertyLookalikes(t *testing.T) {
	t.Parallel()
	response := `<?xml version="1.0"?><D:multistatus xmlns:D="DAV:" xmlns:X="urn:extension">` +
		`<D:response><D:href>/dav/root/file.txt</D:href><D:propstat><D:prop>` +
		`<D:displayname>file.txt</D:displayname><D:resourcetype><X:collection/></D:resourcetype>` +
		`<D:getcontentlength>3</D:getcontentlength><X:getetag>W/&quot;foreign-constant&quot;</X:getetag>` +
		`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response></D:multistatus>`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, response)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	entry, err := backend.Stat(context.Background(), "/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if entry.IsDir || entry.Revision != "" || entry.Size != 3 || !entry.SizeKnown {
		t.Fatalf("foreign DAV-lookalike properties affected entry: %#v", entry)
	}
}

func TestWebDAVEdgeRejectsOversizedPROPFINDBeforeDeliveringEntries(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxWebDAVPropfindResponse+1, 10))
		w.WriteHeader(http.StatusMultiStatus)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	delivered := 0
	err := backend.ReadDir(context.Background(), "/", func(chunk []RemoteEntry) { delivered += len(chunk) })
	if err == nil || !strings.Contains(err.Error(), "PROPFIND response exceeds") {
		t.Fatalf("oversized PROPFIND=%v", err)
	}
	if delivered != 0 {
		t.Fatalf("oversized PROPFIND delivered %d entries", delivered)
	}
}

func TestWebDAVEdgeRejectsEncodedSeparatorCanonicalRedirect(t *testing.T) {
	t.Parallel()
	var sourceRequests, targetRequests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/dav/root/dir":
			sourceRequests++
			w.Header().Set("Location", "/dav/root/dir%2F")
			w.WriteHeader(http.StatusMovedPermanently)
		case "/dav/root/dir%2F":
			targetRequests++
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/dir/", "dir", "0", true)))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if _, err := backend.Stat(context.Background(), "/dir"); err == nil {
		t.Fatal("encoded-separator canonical redirect unexpectedly succeeded")
	}
	if sourceRequests != 1 || targetRequests != 0 {
		t.Fatalf("source requests=%d encoded target requests=%d", sourceRequests, targetRequests)
	}
}

func TestWebDAVEdgeFullFallbackReportsViewerProgress(t *testing.T) {
	t.Parallel()
	content := bytes.Repeat([]byte("viewer-progress-"), 8192)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/file.bin", "file.bin", strconv.Itoa(len(content)), false)))
		case http.MethodGet:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK) // Ignore Range and force a full local spool.
			_, _ = w.Write(content)
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})

	var percentages []int
	ctx := context.WithValue(context.Background(), vfs.ProgressKey, vfs.ProgressCallback(func(_ string, percent int) {
		percentages = append(percentages, percent)
	}))
	r, err := backend.Open(ctx, "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if len(percentages) == 0 {
		t.Fatal("full WebDAV fallback did not report viewer progress through vfs.ProgressKey")
	}
	for i := 1; i < len(percentages); i++ {
		if percentages[i] < percentages[i-1] {
			t.Fatalf("non-monotonic progress = %v", percentages)
		}
	}
	if percentages[len(percentages)-1] != 100 {
		t.Fatalf("progress = %v, want final 100", percentages)
	}
}

func TestWebDAVEdgeStrongETagRejectsTruncatedFullFallback(t *testing.T) {
	t.Parallel()
	type observedRequest struct{ rangeHeader, ifMatch string }
	observed := make(chan observedRequest, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/file.bin", "file.bin", "10", false)))
		case http.MethodGet:
			observed <- observedRequest{rangeHeader: r.Header.Get("Range"), ifMatch: r.Header.Get("If-Match")}
			w.Header().Set("ETag", `"version-one"`)
			w.Header().Set("Content-Length", "3")
			w.WriteHeader(http.StatusOK) // Same strong ETag, but shorter than PROPFIND metadata.
			_, _ = io.WriteString(w, "abc")
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	r, err := backend.Open(context.Background(), "/file.bin")
	if err == nil {
		_ = r.Close()
		t.Fatal("Open accepted a full response shorter than strong-ETag PROPFIND metadata")
	}
	request := <-observed
	if request.rangeHeader != "bytes=0-0" || request.ifMatch != `"version-one"` {
		t.Fatalf("probe headers = %#v", request)
	}
}

func TestWebDAVEdgeWeakOrMissingETagRejectsTruncatedFullResponse(t *testing.T) {
	t.Parallel()
	for _, etag := range []string{"", `W/&quot;version-one&quot;`} {
		etag := etag
		name := "missing"
		if etag != "" {
			name = "weak"
		}
		t.Run(name, func(t *testing.T) {
			response := davResponseXML("/dav/root/file.bin", "file.bin", "10", false)
			response = strings.Replace(response, `&quot;version-one&quot;`, etag, 1)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.Method {
				case "PROPFIND":
					w.WriteHeader(http.StatusMultiStatus)
					_, _ = io.WriteString(w, davMultiStatusXML(response))
				case http.MethodGet:
					w.Header().Set("Content-Length", "3")
					w.WriteHeader(http.StatusOK)
					_, _ = io.WriteString(w, "abc")
				default:
					http.Error(w, "unexpected", http.StatusMethodNotAllowed)
				}
			}))
			defer server.Close()
			backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
				BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
			}, SecretValues{"password": "secret"})
			reader, err := backend.Open(context.Background(), "/file.bin")
			if err == nil {
				_ = reader.Close()
				t.Fatal("Open accepted a response shorter than PROPFIND metadata")
			}
			if !errors.Is(err, ErrRemoteObjectChanged) {
				t.Fatalf("truncated full response = %v, want ErrRemoteObjectChanged", err)
			}
		})
	}
}

func TestWebDAVEdgeKnownSizeBoundsFullResponseBeforeSpooling(t *testing.T) {
	t.Parallel()
	t.Run("chunked oversized", func(t *testing.T) {
		body := &edgeCountingBody{reader: bytes.NewReader(bytes.Repeat([]byte{'x'}, 1024))}
		resp := &http.Response{StatusCode: http.StatusOK, ContentLength: -1, Body: body}
		reader, err := responseToTempReader(context.Background(), resp, "oversized.bin", 1, true)
		if reader != nil {
			_ = reader.Close()
			t.Fatal("oversized response unexpectedly produced a reader")
		}
		if !errors.Is(err, ErrRemoteObjectChanged) {
			t.Fatalf("oversized response=%v, want ErrRemoteObjectChanged", err)
		}
		if body.read != 2 {
			t.Fatalf("oversized response consumed %d bytes, want expected size plus one", body.read)
		}
	})
	t.Run("conflicting content length", func(t *testing.T) {
		body := &edgeCountingBody{reader: bytes.NewReader([]byte("payload"))}
		resp := &http.Response{StatusCode: http.StatusOK, ContentLength: 7, Body: body}
		if _, err := responseToTempReader(context.Background(), resp, "mismatch.bin", 1, true); !errors.Is(err, ErrRemoteObjectChanged) {
			t.Fatalf("Content-Length mismatch=%v, want ErrRemoteObjectChanged", err)
		}
		if body.read != 0 {
			t.Fatalf("Content-Length mismatch consumed %d body bytes", body.read)
		}
	})
}

func TestWebDAVEdgeFullGETRequiresStatusOK(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/file.bin", "file.bin", "1", false)))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if reader, err := backend.Open(context.Background(), "/file.bin"); err == nil {
		_ = reader.Close()
		t.Fatal("Open accepted HTTP 204 as a file representation")
	}
}

func TestWebDAVEdgeMissingPROPFINDLengthUsesActualFullSize(t *testing.T) {
	t.Parallel()
	content := "actual-size"
	response := davResponseXML("/dav/root/file.bin", "file.bin", "10", false)
	response = strings.Replace(response, `<D:getcontentlength>10</D:getcontentlength>`, "", 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(response))
		case http.MethodGet:
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, content)
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	reader, err := backend.Open(context.Background(), "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.Size() != int64(len(content)) {
		t.Fatalf("reader size=%d, want %d", reader.Size(), len(content))
	}
}

func TestWebDAVEdgeInitialRange416DetectsChangedSize(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/file.bin", "file.bin", "5", false)))
			return
		}
		w.Header().Set("Content-Range", "bytes */0")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if _, err := backend.Open(context.Background(), "/file.bin"); !errors.Is(err, ErrRemoteObjectChanged) {
		t.Fatalf("initial 416 after size change = %v", err)
	}
}

func TestWebDAVEdgeWildcardETagDoesNotEnableRangeReader(t *testing.T) {
	t.Parallel()
	content := "hello"
	response := strings.Replace(davResponseXML("/dav/root/file.bin", "file.bin", "5", false), `&quot;version-one&quot;`, "*", 1)
	var getRequests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(response))
		case http.MethodGet:
			getRequests++
			if r.Header.Get("If-Match") != "" {
				t.Errorf("wildcard ETag was sent as If-Match: %q", r.Header.Get("If-Match"))
			}
			if r.Header.Get("Range") != "" {
				w.Header().Set("Content-Range", "bytes 0-0/5")
				w.Header().Set("ETag", "*")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.WriteString(w, "h")
				return
			}
			w.Header().Set("Content-Length", strconv.Itoa(len(content)))
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, content)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	reader, err := backend.Open(context.Background(), "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reader.(*webDAVRangeReader); ok {
		t.Fatal("wildcard ETag enabled the multi-request range reader")
	}
	buffer := make([]byte, 5)
	if n, err := reader.ReadAt(context.Background(), buffer, 0); err != nil || n != 5 || string(buffer) != "hello" {
		t.Fatalf("first wildcard response=%d, %v, %q", n, err, buffer)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	content = "world" // same invalid validator, size, and Last-Modified
	reader, err = backend.Open(context.Background(), "/file.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if n, err := reader.ReadAt(context.Background(), buffer, 0); err != nil || n != 5 || string(buffer) != "world" {
		t.Fatalf("second wildcard response=%d, %v, %q", n, err, buffer)
	}
	if getRequests != 4 {
		t.Fatalf("GET requests=%d, want two probe/full pairs without invalid-ETag cache reuse", getRequests)
	}
}

func TestWebDAVEdgeProbeRejectsGenerationDifferentFromWeakStat(t *testing.T) {
	t.Parallel()
	response := strings.Replace(davResponseXML("/dav/root/file.bin", "file.bin", "5", false),
		`&quot;version-one&quot;`, `W/&quot;old&quot;`, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PROPFIND" {
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(response))
			return
		}
		w.Header().Set("Content-Range", "bytes 0-0/5")
		w.Header().Set("ETag", `"new"`)
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.WriteString(w, "n")
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	if _, err := backend.Open(context.Background(), "/file.bin"); !errors.Is(err, ErrRemoteObjectChanged) {
		t.Fatalf("weak Stat / different strong probe = %v", err)
	}
}

func TestWebDAVEdgeCancellationDuringStreamedFullGET(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	var once sync.Once
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/slow.bin", "slow.bin", strconv.Itoa(8<<20), false)))
		case http.MethodGet:
			w.Header().Set("Content-Length", strconv.Itoa(8<<20))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(bytes.Repeat([]byte{0x5a}, 32<<10))
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			once.Do(func() { close(started) })
			<-r.Context().Done()
		default:
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		r, err := backend.Open(ctx, "/slow.bin")
		if r != nil {
			_ = r.Close()
		}
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("full GET did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Open after cancellation = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled full GET did not return")
	}
}

func TestWebDAVEdgeCancellationDuringStreamedPUT(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	releaseServer := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
			return
		}
		one := make([]byte, 1)
		_, _ = io.ReadFull(r.Body, one)
		close(started)
		select {
		case <-r.Context().Done():
		case <-releaseServer:
		}
	}))
	defer func() {
		close(releaseServer)
		server.Close()
	}()
	backend := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	ctx, cancel := context.WithCancel(context.Background())
	w, err := backend.Create(ctx, "/slow-upload.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(bytes.Repeat([]byte{0xa5}, 2<<20)); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- w.Close() }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("PUT did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || !errors.Is(err, vfs.ErrOperationStateUnknown) {
			t.Fatalf("PUT after cancellation = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled PUT did not return")
	}
}

func TestWebDAVEdgeZeroByteOverwriteAndBinaryLifecycle(t *testing.T) {
	t.Parallel()
	harness := newEdgeWebDAVHarness(t)
	backend := harness.backend
	ctx := context.Background()

	edgeWriteWebDAVFile(t, backend, "/empty.bin", nil)
	entry, err := backend.Stat(ctx, "/empty.bin")
	if err != nil || entry.IsDir || entry.Size != 0 {
		t.Fatalf("empty Stat = %#v, %v", entry, err)
	}
	empty, err := backend.Open(ctx, "/empty.bin")
	if err != nil {
		t.Fatal(err)
	}
	if empty.Size() != 0 {
		t.Fatalf("empty Size = %d", empty.Size())
	}
	if n, err := empty.ReadAt(ctx, make([]byte, 1), 0); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("empty ReadAt = %d, %v", n, err)
	}
	_ = empty.Close()

	first := append([]byte{0x00, 0xff, 0x7f, 0x80}, bytes.Repeat([]byte{0x5a, 0x00}, 4096)...)
	edgeWriteWebDAVFile(t, backend, "/binary.bin", first)
	if got := edgeReadWebDAVFile(t, backend, "/binary.bin"); !bytes.Equal(got, first) {
		t.Fatalf("binary content differs: got %d bytes, want %d", len(got), len(first))
	}
	second := []byte{0xde, 0xad, 0xbe, 0xef, 0x00}
	edgeWriteWebDAVFile(t, backend, "/binary.bin", second)
	if got := edgeReadWebDAVFile(t, backend, "/binary.bin"); !bytes.Equal(got, second) {
		t.Fatalf("overwritten content = %x, want %x", got, second)
	}
	entry, err = backend.Stat(ctx, "/binary.bin")
	if err != nil || entry.Size != int64(len(second)) {
		t.Fatalf("overwritten Stat = %#v, %v", entry, err)
	}
}

func TestWebDAVEdgeRangeReaderBoundaryAndFailureMatrix(t *testing.T) {
	t.Parallel()
	newReader := func(t *testing.T, handler http.HandlerFunc, size int64) *webDAVRangeReader {
		t.Helper()
		server := httptest.NewTLSServer(handler)
		t.Cleanup(server.Close)
		readerCtx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		return &webDAVRangeReader{client: server.Client(), url: server.URL, size: size, etag: `"one"`, ctx: readerCtx, cancel: cancel}
	}

	t.Run("local boundaries do not issue requests", func(t *testing.T) {
		requests := 0
		reader := newReader(t, func(w http.ResponseWriter, r *http.Request) {
			requests++
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}, 5)
		if n, err := reader.ReadAt(context.Background(), make([]byte, 1), -1); n != 0 || !errors.Is(err, os.ErrInvalid) {
			t.Fatalf("negative ReadAt = %d, %v", n, err)
		}
		if n, err := reader.ReadAt(context.Background(), make([]byte, 1), 5); n != 0 || !errors.Is(err, io.EOF) {
			t.Fatalf("EOF ReadAt = %d, %v", n, err)
		}
		if n, err := reader.ReadAt(context.Background(), nil, 5); n != 0 || err != nil {
			t.Fatalf("empty ReadAt = %d, %v", n, err)
		}
		if requests != 0 {
			t.Fatalf("boundary reads issued %d requests", requests)
		}
	})

	t.Run("malformed Content-Range", func(t *testing.T) {
		reader := newReader(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Range", "not-a-range")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "a")
		}, 5)
		if _, err := reader.ReadAt(context.Background(), make([]byte, 1), 0); err == nil || !strings.Contains(err.Error(), "Content-Range") {
			t.Fatalf("malformed Content-Range = %v", err)
		}
	})

	t.Run("truncated range body", func(t *testing.T) {
		reader := newReader(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Range", "bytes 0-2/5")
			w.Header().Set("ETag", `"one"`)
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "ab")
		}, 5)
		buffer := make([]byte, 3)
		n, err := reader.ReadAt(context.Background(), buffer, 0)
		if n != 2 || !errors.Is(err, io.ErrUnexpectedEOF) || !strings.Contains(err.Error(), "truncated WebDAV range response") || string(buffer[:n]) != "ab" {
			t.Fatalf("truncated ReadAt = %d, %v, %q", n, err, buffer[:n])
		}
	})

	t.Run("precondition changed", func(t *testing.T) {
		reader := newReader(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "changed", http.StatusPreconditionFailed)
		}, 5)
		if _, err := reader.ReadAt(context.Background(), make([]byte, 1), 0); !errors.Is(err, ErrRemoteObjectChanged) {
			t.Fatalf("precondition ReadAt = %v", err)
		}
	})

	t.Run("response ETag changed", func(t *testing.T) {
		reader := newReader(t, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Range", "bytes 0-0/5")
			w.Header().Set("ETag", `"two"`)
			w.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(w, "a")
		}, 5)
		if _, err := reader.ReadAt(context.Background(), make([]byte, 1), 0); !errors.Is(err, ErrRemoteObjectChanged) {
			t.Fatalf("changed ETag ReadAt = %v", err)
		}
	})
}

func TestWebDAVEdgeSameSourceAndDestinationOverwriteSafety(t *testing.T) {
	t.Parallel()
	harness := newEdgeWebDAVHarness(t)
	backend := harness.backend
	ctx := context.Background()

	edgeWriteWebDAVFile(t, backend, "/same.bin", []byte("same-source"))
	if err := backend.Copy(ctx, "/same.bin", "/same.bin"); err == nil {
		t.Fatal("same-source COPY unexpectedly succeeded")
	}
	if got := edgeReadWebDAVFile(t, backend, "/same.bin"); string(got) != "same-source" {
		t.Fatalf("same-source COPY changed content to %q", got)
	}
	if err := backend.Rename(ctx, "/same.bin", "/same.bin"); err == nil {
		t.Fatal("same-source MOVE unexpectedly succeeded")
	}
	if got := edgeReadWebDAVFile(t, backend, "/same.bin"); string(got) != "same-source" {
		t.Fatalf("same-source MOVE changed content to %q", got)
	}

	edgeWriteWebDAVFile(t, backend, "/source.bin", []byte("copy-source"))
	edgeWriteWebDAVFile(t, backend, "/destination.bin", []byte("old-destination"))
	if err := backend.Copy(ctx, "/source.bin", "/destination.bin"); err != nil {
		t.Fatal(err)
	}
	if got := edgeReadWebDAVFile(t, backend, "/destination.bin"); string(got) != "copy-source" {
		t.Fatalf("COPY overwrite content = %q", got)
	}
	if got := edgeReadWebDAVFile(t, backend, "/source.bin"); string(got) != "copy-source" {
		t.Fatalf("COPY removed or changed source: %q", got)
	}

	edgeWriteWebDAVFile(t, backend, "/source.bin", []byte("move-source"))
	if err := backend.Rename(vfs.WithDestinationOverwrite(ctx, false), "/source.bin", "/destination.bin"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("MOVE to occupied destination = %v, want os.ErrExist", err)
	}
	if got := edgeReadWebDAVFile(t, backend, "/destination.bin"); string(got) != "copy-source" {
		t.Fatalf("failed MOVE changed destination to %q", got)
	}
	if got := edgeReadWebDAVFile(t, backend, "/source.bin"); string(got) != "move-source" {
		t.Fatalf("failed MOVE changed source to %q", got)
	}
	if err := backend.Rename(ctx, "/source.bin", "/destination.bin"); err != nil {
		t.Fatalf("replacement MOVE used by atomic editor save = %v", err)
	}
	if got := edgeReadWebDAVFile(t, backend, "/destination.bin"); string(got) != "move-source" {
		t.Fatalf("replacement MOVE content = %q", got)
	}
	if _, err := backend.Stat(ctx, "/source.bin"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement MOVE left source behind: %v", err)
	}
}
