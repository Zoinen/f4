package cloudfox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestWebDAVShareInfoReturnsCredentialFreeServerControlledResourceURL(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != "PROPFIND" || r.URL.Path != "/dav/root/folder name" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if user, password, ok := r.BasicAuth(); !ok || user != "alice" || password != "secret" {
			t.Errorf("BasicAuth = %q, %q, %v", user, password, ok)
		}
		if r.Header.Get("Depth") != "0" {
			t.Errorf("Depth = %q", r.Header.Get("Depth"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/folder%20name/", "folder name", "0", true)))
	}))
	defer server.Close()

	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	info, err := dav.ShareLinkInfo(context.Background(), "/folder name")
	if err != nil {
		t.Fatal(err)
	}
	if info.Provider != "Generic WebDAV" || info.ItemName != "folder name" || info.CanCreate || info.CanRevoke || info.Link == nil {
		t.Fatalf("ShareLinkInfo = %#v", info)
	}
	if len(info.Roles) != 1 || info.Roles[0] != vfs.ShareRoleServerControlled || len(info.ExpirationOptions) != 0 {
		t.Fatalf("roles/expiration = %v / %v", info.Roles, info.ExpirationOptions)
	}
	if info.Link.Role != vfs.ShareRoleServerControlled || info.Link.Revocable || !info.Link.ExpiresAt.IsZero() {
		t.Fatalf("link metadata = %#v", *info.Link)
	}
	parsed, err := url.Parse(info.Link.URL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.EscapedPath() != "/dav/root/folder%20name/" {
		t.Fatalf("direct WebDAV URL = %q", info.Link.URL)
	}
	if strings.Contains(info.Link.URL, "alice") || strings.Contains(info.Link.URL, "secret") {
		t.Fatal("direct WebDAV URL contains credentials")
	}
	if !strings.Contains(info.Notice, "not a public share link") || !strings.Contains(info.Notice, "credentials") || !strings.Contains(info.Notice, "method-specific") {
		t.Fatalf("notice = %q", info.Notice)
	}
	if requests.Load() != 1 {
		t.Fatalf("requests = %d", requests.Load())
	}
}

func TestWebDAVShareCreateAndRevokeAreHonestlyUnsupported(t *testing.T) {
	t.Parallel()
	dav := &webDAVBackend{}
	request := vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer}
	if _, err := dav.CreateShareLink(context.Background(), "/file.txt", request); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Fatalf("CreateShareLink error = %v", err)
	}
	if err := dav.RevokeShareLink(context.Background(), "/file.txt"); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Fatalf("RevokeShareLink error = %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dav.CreateShareLink(cancelled, "/file.txt", request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CreateShareLink error = %v", err)
	}
	if err := dav.RevokeShareLink(cancelled, "/file.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled RevokeShareLink error = %v", err)
	}
}

func TestWebDAVShareInfoKeepsFileURLNonCollectionForm(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" || r.URL.Path != "/dav/root/report #1.txt" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/root/report%20%231.txt", "report #1.txt", "7", false)))
	}))
	defer server.Close()
	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	info, err := dav.ShareLinkInfo(context.Background(), "/report #1.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Link == nil {
		t.Fatal("ShareLinkInfo returned no direct URL")
	}
	parsed, err := url.Parse(info.Link.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.EscapedPath(); got != "/dav/root/report%20%231.txt" || strings.HasSuffix(parsed.Path, "/") {
		t.Fatalf("file share URL = %q", info.Link.URL)
	}
}

func TestWebDAVShareInfoHonorsCancellationBeforeNetwork(t *testing.T) {
	t.Parallel()
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/root", Auth: "basic", Username: "alice",
	}, SecretValues{"password": "secret"})
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dav.ShareLinkInfo(cancelled, "/file.txt"); !errors.Is(err, context.Canceled) {
		t.Fatalf("ShareLinkInfo error = %v", err)
	}
	if requests.Load() != 0 {
		t.Fatalf("cancelled ShareLinkInfo made %d request(s)", requests.Load())
	}
}

func TestWebDAVShareInfoDoesNotInferGETAccessFromConnectionAuth(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("anonymous request contained Authorization")
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = io.WriteString(w, davMultiStatusXML(davResponseXML("/dav/public.txt", "public.txt", "4", false)))
	}))
	defer server.Close()
	dav := openTestWebDAV(t, &WebDAVFactory{HTTPClient: server.Client()}, WebDAVSettings{
		BaseURL: server.URL + "/dav", Root: "/", Auth: "anonymous",
	}, nil)
	info, err := dav.ShareLinkInfo(context.Background(), "/public.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Link == nil || info.Link.Role != vfs.ShareRoleServerControlled || len(info.Roles) != 1 || info.Roles[0] != vfs.ShareRoleServerControlled {
		t.Fatalf("anonymous share info = %#v", info)
	}
	if strings.Contains(strings.ToLower(info.Notice), "needs credentials") || !strings.Contains(strings.ToLower(info.Notice), "method-specific") {
		t.Fatalf("anonymous notice = %q", info.Notice)
	}
}
