package cloudfox

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func webDAVConnection(t *testing.T, settings WebDAVSettings) Connection {
	t.Helper()
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	return Connection{ID: testConnectionID, Name: "DAV", Provider: ProviderWebDAV, Settings: raw}
}

func openTestWebDAV(t *testing.T, factory *WebDAVFactory, settings WebDAVSettings, secrets SecretValues) *webDAVBackend {
	t.Helper()
	backend, err := factory.Open(context.Background(), webDAVConnection(t, settings), secrets)
	if err != nil {
		t.Fatal(err)
	}
	dav, ok := backend.(*webDAVBackend)
	if !ok {
		t.Fatalf("backend = %T", backend)
	}
	t.Cleanup(func() { _ = dav.Close() })
	return dav
}

func davResponseXML(href, displayName, length string, directory bool) string {
	resourceType := ""
	if directory {
		resourceType = "<D:collection/>"
	}
	return `<D:response><D:href>` + href + `</D:href><D:propstat><D:prop>` +
		`<D:displayname>` + displayName + `</D:displayname>` +
		`<D:resourcetype>` + resourceType + `</D:resourcetype>` +
		`<D:getcontentlength>` + length + `</D:getcontentlength>` +
		`<D:getlastmodified>Wed, 21 Oct 2015 07:28:00 GMT</D:getlastmodified>` +
		`<D:getetag>&quot;version-one&quot;</D:getetag>` +
		`</D:prop><D:status>HTTP/1.1 200 OK</D:status></D:propstat></D:response>`
}

func davMultiStatusXML(responses ...string) string {
	return `<?xml version="1.0" encoding="utf-8"?><D:multistatus xmlns:D="DAV:">` + strings.Join(responses, "") + `</D:multistatus>`
}

func TestWebDAVReadDirStreamsMultiStatusAndSkipsSelf(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" || r.URL.Path != "/dav/root/" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Errorf("BasicAuth = %q, %q, %v", user, password, ok)
		}
		if depth := r.Header.Get("Depth"); depth != "1" {
			t.Errorf("Depth = %q", depth)
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(
			davResponseXML("/dav/root/", "root", "0", true),
			davResponseXML("/dav/root/docs/", "docs", "0", true),
			davResponseXML("/dav/root/report.txt", "report.txt", "42", false),
		))
	}))
	defer server.Close()

	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	var entries []RemoteEntry
	if err := dav.ReadDir(context.Background(), "/", func(chunk []RemoteEntry) { entries = append(entries, chunk...) }); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %#v", entries)
	}
	if entries[0].Name != "docs" || entries[0].Location != "/docs" || !entries[0].IsDir {
		t.Errorf("directory = %#v", entries[0])
	}
	if entries[1].Name != "report.txt" || entries[1].Location != "/report.txt" || entries[1].Size != 42 || entries[1].IsDir {
		t.Errorf("file = %#v", entries[1])
	}
}

func TestWebDAVOpenUsesByteRangeReader(t *testing.T) {
	t.Parallel()

	content := "abcdef"
	var mu sync.Mutex
	var ranges []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/file.txt", "file.txt", "6", false)))
		case http.MethodGet:
			if got := r.Header.Get("If-Match"); got != `"version-one"` {
				t.Errorf("If-Match = %q", got)
			}
			requested := r.Header.Get("Range")
			mu.Lock()
			ranges = append(ranges, requested)
			mu.Unlock()
			switch requested {
			case "bytes=0-0":
				w.Header().Set("Content-Range", "bytes 0-0/6")
				w.Header().Set("ETag", `"version-one"`)
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.WriteString(w, content[:1])
			case "bytes=2-4":
				w.Header().Set("Content-Range", "bytes 2-4/6")
				w.Header().Set("ETag", `"version-one"`)
				w.WriteHeader(http.StatusPartialContent)
				_, _ = io.WriteString(w, content[2:5])
			default:
				http.Error(w, "unexpected range", http.StatusRequestedRangeNotSatisfiable)
			}
		default:
			http.Error(w, "unexpected request", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	reader, err := dav.Open(context.Background(), "/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.Size() != 6 {
		t.Fatalf("Size = %d", reader.Size())
	}
	buffer := make([]byte, 3)
	n, err := reader.ReadAt(context.Background(), buffer, 2)
	if err != nil || n != 3 || string(buffer) != "cde" {
		t.Fatalf("ReadAt = %d, %v, %q", n, err, buffer)
	}
	mu.Lock()
	gotRanges := append([]string(nil), ranges...)
	mu.Unlock()
	if strings.Join(gotRanges, ",") != "bytes=0-0,bytes=2-4" {
		t.Fatalf("ranges = %v", gotRanges)
	}
}

func TestWebDAVOpenFallsBackWhenRangesAreIgnored(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/file.txt", "file.txt", "6", false)))
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "abcdef")
		default:
			http.Error(w, "unexpected request", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	reader, err := dav.Open(context.Background(), "/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	buffer := make([]byte, 3)
	n, err := reader.ReadAt(context.Background(), buffer, 1)
	if err != nil || n != 3 || string(buffer) != "bcd" {
		t.Fatalf("ReadAt = %d, %v, %q", n, err, buffer)
	}
}

func TestWebDAVDigestAuthenticationReplaysPropfind(t *testing.T) {
	t.Parallel()

	const (
		username = "digest-user"
		password = "digest-password"
		realm    = "test-dav"
		nonce    = "0123456789abcdef"
	)
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		authorization := r.Header.Get("Authorization")
		if authorization == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="test-dav", nonce="0123456789abcdef", algorithm=SHA-256, qop="auth"`)
			http.Error(w, "authenticate", http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(authorization, "Digest ") {
			t.Errorf("Authorization = %q", authorization)
		}
		params := parseAuthParams(strings.TrimPrefix(authorization, "Digest "))
		ha1, _ := digestHash("SHA-256", username+":"+realm+":"+password)
		ha2, _ := digestHash("SHA-256", r.Method+":"+r.URL.RequestURI())
		want, _ := digestHash("SHA-256", ha1+":"+nonce+":"+params["nc"]+":"+params["cnonce"]+":auth:"+ha2)
		if params["username"] != username || params["response"] != want || params["nc"] != "00000001" {
			t.Errorf("Digest params = %#v; response want %q", params, want)
		}
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(
			davResponseXML("/", "/", "0", true),
			davResponseXML("/child.txt", "child.txt", "5", false),
		))
	}))
	defer server.Close()

	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL, Root: "/", Auth: "digest", Username: username, AllowInsecureDigest: true,
	}, SecretValues{"password": password})
	var entries []RemoteEntry
	if err := dav.ReadDir(context.Background(), "/", func(chunk []RemoteEntry) { entries = append(entries, chunk...) }); err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(entries) != 1 || entries[0].Name != "child.txt" {
		t.Fatalf("requests = %d, entries = %#v", requests, entries)
	}
}

func TestWebDAVRejectsBasicAuthOverHTTP(t *testing.T) {
	t.Parallel()
	err := (&WebDAVFactory{}).Validate(webDAVConnection(t, WebDAVSettings{
		BaseURL: "http://example.test/dav", Auth: "basic", Username: "alice",
	}))
	if err == nil || !strings.Contains(err.Error(), "HTTPS is required") {
		t.Fatalf("Validate error = %v", err)
	}
}

func TestWebDAVDoesNotFollowMutationRedirectsAsGET(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	redirectTargets := 0
	deleteRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Errorf("BasicAuth = %q, %q, %v", user, password, ok)
		}
		switch {
		case r.URL.Path == "/dav/root/delete-me" && r.Method == "PROPFIND":
			w.WriteHeader(http.StatusMultiStatus)
			_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/delete-me", "delete-me", "1", false)))
		case r.URL.Path == "/dav/root/delete-me" && r.Method == http.MethodDelete:
			mu.Lock()
			deleteRequests++
			mu.Unlock()
			http.Redirect(w, r, "/dav/root/pretend-success", http.StatusFound)
		case r.URL.Path == "/dav/root/upload.bin" && r.Method == http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			http.Redirect(w, r, "/dav/root/pretend-success", http.StatusFound)
		case r.URL.Path == "/dav/root/pretend-success":
			mu.Lock()
			redirectTargets++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})

	if err := dav.Remove(context.Background(), "/delete-me"); err == nil {
		t.Fatal("redirected DELETE unexpectedly succeeded")
	}
	w, err := dav.Create(context.Background(), "/upload.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "payload"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err == nil {
		t.Fatal("redirected PUT unexpectedly succeeded")
	}

	mu.Lock()
	gotRedirectTargets := redirectTargets
	gotDeleteRequests := deleteRequests
	mu.Unlock()
	if gotDeleteRequests != 1 {
		t.Fatalf("DELETE requests = %d, want 1", gotDeleteRequests)
	}
	if gotRedirectTargets != 0 {
		t.Fatalf("mutation redirect target was requested %d time(s)", gotRedirectTargets)
	}
}
