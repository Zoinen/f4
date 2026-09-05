package cloudfox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func yandexShareTestBackend(server *httptest.Server) *yandexDiskBackend {
	return &yandexDiskBackend{
		client:  server.Client(),
		baseURL: server.URL,
		token:   "share-token",
		root:    "disk:/",
	}
}

func TestYandexShareLinkInfoReportsOnlyPersistentViewerLinks(t *testing.T) {
	t.Parallel()

	const publicURL = "https://share.invalid/d/public-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/resources" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "OAuth share-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("path"); got != "disk:/folder/report.txt" {
			t.Errorf("path = %q", got)
		}
		if got := r.URL.Query().Get("fields"); got != "name,public_key,public_url" {
			t.Errorf("fields = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"name":"report.txt","public_key":"opaque-key","public_url":%q}`, publicURL)
	}))
	defer server.Close()

	info, err := yandexShareTestBackend(server).ShareLinkInfo(context.Background(), "disk:/folder/report.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Provider != "Yandex.Disk" || info.ItemName != "report.txt" || !info.CanCreate || !info.CanRevoke {
		t.Fatalf("share info = %#v", info)
	}
	if !reflect.DeepEqual(info.Roles, []vfs.ShareRole{vfs.ShareRoleViewer}) {
		t.Fatalf("roles = %v", info.Roles)
	}
	if !reflect.DeepEqual(info.ExpirationOptions, []time.Duration{0}) || info.DefaultExpiration != 0 {
		t.Fatalf("expiration support = %v, default %v", info.ExpirationOptions, info.DefaultExpiration)
	}
	if info.Notice != yandexShareNotice {
		t.Fatalf("notice = %q", info.Notice)
	}
	wantLink := &vfs.ShareLink{URL: publicURL, Role: vfs.ShareRoleViewer, Revocable: true}
	if !reflect.DeepEqual(info.Link, wantLink) {
		t.Fatalf("link = %#v, want %#v", info.Link, wantLink)
	}
}

func TestYandexShareLinkInfoReportsPrivateResource(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"name":"private folder"}`)
	}))
	defer server.Close()

	info, err := yandexShareTestBackend(server).ShareLinkInfo(context.Background(), "disk:/private folder")
	if err != nil {
		t.Fatal(err)
	}
	if info.Link != nil || info.CanRevoke || !info.CanCreate || info.ItemName != "private folder" {
		t.Fatalf("private share info = %#v", info)
	}
}

func TestYandexCreateShareLinkPublishesThenReadsMetadataFromConfiguredOrigin(t *testing.T) {
	t.Parallel()

	const publicURL = "https://share.invalid/d/public-token"
	var foreignRequests atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		foreignRequests.Add(1)
	}))
	defer foreign.Close()

	var mu sync.Mutex
	var requests []string
	var metadataRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.Method+" "+r.URL.Path)
		mu.Unlock()
		if got := r.Header.Get("Authorization"); got != "OAuth share-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("path"); got != "disk:/folder" {
			t.Errorf("path = %q", got)
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/resources/publish":
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"href":%q,"method":"GET","templated":false}`, foreign.URL+"/must-not-be-requested")
		case r.Method == http.MethodGet && r.URL.Path == "/resources":
			w.Header().Set("Content-Type", "application/json")
			if metadataRequests.Add(1) == 1 {
				_, _ = fmt.Fprint(w, `{"name":"folder"}`)
			} else {
				_, _ = fmt.Fprintf(w, `{"name":"folder","public_key":"opaque-key","public_url":%q}`, publicURL)
			}
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	link, err := yandexShareTestBackend(server).CreateShareLink(context.Background(), "disk:/folder", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	if link != (vfs.ShareLink{URL: publicURL, Role: vfs.ShareRoleViewer, Revocable: true}) {
		t.Fatalf("link = %#v", link)
	}
	mu.Lock()
	gotRequests := append([]string(nil), requests...)
	mu.Unlock()
	if !reflect.DeepEqual(gotRequests, []string{"GET /resources", "PUT /resources/publish", "GET /resources"}) {
		t.Fatalf("requests = %v", gotRequests)
	}
	if got := foreignRequests.Load(); got != 0 {
		t.Fatalf("provider followed response Link to another origin %d time(s)", got)
	}
}

func TestYandexCreateShareLinkReturnsExistingLinkWithoutPublishing(t *testing.T) {
	t.Parallel()

	const publicURL = "https://share.invalid/d/already-public"
	var gets, puts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			gets.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"name":"file.txt","public_key":"opaque-key","public_url":%q}`, publicURL)
		case http.MethodPut:
			puts.Add(1)
			http.Error(w, "must not publish", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	link, err := yandexShareTestBackend(server).CreateShareLink(context.Background(), "disk:/file.txt", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
	if err != nil {
		t.Fatal(err)
	}
	if link.URL != publicURL || link.Role != vfs.ShareRoleViewer || !link.Revocable {
		t.Fatalf("link = %#v", link)
	}
	if gets.Load() != 1 || puts.Load() != 0 {
		t.Fatalf("requests: GET=%d PUT=%d", gets.Load(), puts.Load())
	}
}

func TestYandexRevokeShareLinkUsesUnpublish(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPut || r.URL.Path != "/resources/unpublish" || r.URL.Query().Get("path") != "disk:/folder/file.txt" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "OAuth share-token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = fmt.Fprint(w, `{"href":"https://api.invalid/metadata-containing-no-public-url"}`)
	}))
	defer server.Close()

	if err := yandexShareTestBackend(server).RevokeShareLink(context.Background(), "disk:/folder/file.txt"); err != nil {
		t.Fatal(err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d", got)
	}
}

func TestYandexShareRejectsUnsupportedOptionsAndRootBeforeNetwork(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	backend := yandexShareTestBackend(server)

	tests := []vfs.ShareLinkRequest{
		{Role: vfs.ShareRoleCommenter},
		{Role: vfs.ShareRoleEditor},
		{Role: vfs.ShareRoleUploader},
		{Role: vfs.ShareRoleServerControlled},
		{Role: vfs.ShareRoleViewer, ExpiresIn: time.Hour},
	}
	for _, request := range tests {
		if _, err := backend.CreateShareLink(context.Background(), "disk:/file.txt", request); !errors.Is(err, ErrShareLinksUnsupported) {
			t.Errorf("request %#v error = %v", request, err)
		}
	}
	if _, err := backend.ShareLinkInfo(context.Background(), "disk:/"); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Errorf("root info error = %v", err)
	}
	if _, err := backend.CreateShareLink(context.Background(), "disk:/", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer}); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Errorf("root create error = %v", err)
	}
	if err := backend.RevokeShareLink(context.Background(), "disk:/"); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Errorf("root revoke error = %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("unsupported requests reached the network %d time(s)", got)
	}
}

func TestYandexShareErrorsDoNotExposeProviderResponseURLs(t *testing.T) {
	t.Parallel()

	const secretURL = "https://share.invalid/d/must-not-leak" // #nosec G101 -- a synthetic secret-bearing URL verifies provider-error redaction.
	t.Run("metadata", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"description":%q}`, secretURL)
		}))
		defer server.Close()

		_, err := yandexShareTestBackend(server).ShareLinkInfo(context.Background(), "disk:/file.txt")
		if err == nil || strings.Contains(err.Error(), secretURL) {
			t.Fatalf("metadata error = %v", err)
		}
	})

	t.Run("permission rejection", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprintf(w, `{"description":%q}`, secretURL)
		}))
		defer server.Close()

		_, err := yandexShareTestBackend(server).CreateShareLink(context.Background(), "disk:/file.txt", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
		if !errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), secretURL) {
			t.Fatalf("permission error = %v", err)
		}
	})

	t.Run("unknown mutation state", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintf(w, `{"description":%q}`, secretURL)
		}))
		defer server.Close()

		err := yandexShareTestBackend(server).RevokeShareLink(context.Background(), "disk:/file.txt")
		if !errors.Is(err, vfs.ErrOperationStateUnknown) || strings.Contains(err.Error(), secretURL) {
			t.Fatalf("unknown-state error = %v", err)
		}
	})
}

func TestYandexShareRejectsIncompletePublishedMetadataWithoutLeakingValues(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodPut:
			_, _ = fmt.Fprint(w, `{"href":"https://api.invalid/metadata"}`)
		case http.MethodGet:
			_, _ = fmt.Fprint(w, `{"name":"file.txt","public_key":"sensitive-key"}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	backend := yandexShareTestBackend(server)

	if _, err := backend.ShareLinkInfo(context.Background(), "disk:/file.txt"); err == nil || strings.Contains(err.Error(), "sensitive-key") {
		t.Fatalf("incomplete info error = %v", err)
	}
	if _, err := backend.CreateShareLink(context.Background(), "disk:/file.txt", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer}); err == nil || strings.Contains(err.Error(), "sensitive-key") {
		t.Fatalf("incomplete create error = %v", err)
	}
}

func TestYandexCreateShareLinkRollsBackPostPublishFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		metadata string
		wantText string
	}{
		{
			name:     "decode error",
			metadata: `{"name":"file.txt","public_url":`,
			wantText: "decode Yandex.Disk share metadata",
		},
		{
			name:     "missing URL",
			metadata: `{"name":"file.txt"}`,
			wantText: "without returning a public URL",
		},
		{
			name:     "incomplete metadata",
			metadata: `{"name":"file.txt","public_key":"sensitive-public-key"}`,
			wantText: "incomplete share metadata",
		},
		{
			name:     "invalid URL",
			metadata: `{"name":"file.txt","public_key":"sensitive-public-key","public_url":"javascript:sensitive-public-url"}`,
			wantText: "invalid share URL",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var metadataRequests, publishes, rollbacks atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/resources":
					if metadataRequests.Add(1) == 1 {
						_, _ = fmt.Fprint(w, `{"name":"file.txt"}`)
					} else {
						_, _ = fmt.Fprint(w, test.metadata)
					}
				case r.Method == http.MethodPut && r.URL.Path == "/resources/publish":
					publishes.Add(1)
					_, _ = fmt.Fprint(w, `{"href":"https://api.invalid/metadata"}`)
				case r.Method == http.MethodPut && r.URL.Path == "/resources/unpublish":
					rollbacks.Add(1)
					_, _ = fmt.Fprint(w, `{"href":"https://api.invalid/metadata"}`)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			_, err := yandexShareTestBackend(server).CreateShareLink(context.Background(), "disk:/file.txt", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("error = %v, want text %q", err, test.wantText)
			}
			if errors.Is(err, vfs.ErrOperationStateUnknown) {
				t.Fatalf("successful rollback reported unknown state: %v", err)
			}
			for _, secret := range []string{"sensitive-public-key", "sensitive-public-url"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("error leaked %q: %v", secret, err)
				}
			}
			if publishes.Load() != 1 || rollbacks.Load() != 1 {
				t.Fatalf("mutations: publish=%d rollback=%d", publishes.Load(), rollbacks.Load())
			}
		})
	}
}

func TestYandexCreateShareLinkReportsUnknownStateWhenRollbackCannotBeConfirmed(t *testing.T) {
	t.Parallel()

	const (
		secretURL  = "https://user:sensitive-token@share.invalid/file" // #nosec G101 -- synthetic credential-bearing URL verifies rollback error redaction.
		secretBody = "rollback-sensitive-response"
	)
	var metadataRequests, publishes, rollbacks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resources":
			if metadataRequests.Add(1) == 1 {
				_, _ = fmt.Fprint(w, `{"name":"file.txt"}`)
			} else {
				_, _ = fmt.Fprintf(w, `{"name":"file.txt","public_key":"sensitive-key","public_url":%q}`, secretURL)
			}
		case r.Method == http.MethodPut && r.URL.Path == "/resources/publish":
			publishes.Add(1)
			_, _ = fmt.Fprint(w, `{"href":"https://api.invalid/metadata"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/resources/unpublish":
			rollbacks.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprint(w, secretBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	_, err := yandexShareTestBackend(server).CreateShareLink(context.Background(), "disk:/file.txt", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
	if !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("error = %v, want unknown operation state", err)
	}
	for _, secret := range []string{secretURL, "sensitive-token", "sensitive-key", secretBody, "share-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("unknown-state error leaked %q: %v", secret, err)
		}
	}
	if publishes.Load() != 1 || rollbacks.Load() != 1 {
		t.Fatalf("mutations: publish=%d rollback=%d", publishes.Load(), rollbacks.Load())
	}
}

func TestYandexCreateShareLinkRollsBackAfterCallerCancellation(t *testing.T) {
	t.Parallel()

	var cancel context.CancelFunc
	var metadataRequests, rollbacks atomic.Int32
	postMetadataStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resources":
			if metadataRequests.Add(1) == 1 {
				_, _ = fmt.Fprint(w, `{"name":"file.txt"}`)
				return
			}
			close(postMetadataStarted)
			cancel()
			select {
			case <-r.Context().Done():
			case <-time.After(5 * time.Second):
				t.Error("post-publish metadata request was not canceled")
			}
		case r.Method == http.MethodPut && r.URL.Path == "/resources/publish":
			_, _ = fmt.Fprint(w, `{"href":"https://api.invalid/metadata"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/resources/unpublish":
			rollbacks.Add(1)
			_, _ = fmt.Fprint(w, `{"href":"https://api.invalid/metadata"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancelContext := context.WithCancel(context.Background())
	cancel = cancelContext
	defer cancelContext()
	_, err := yandexShareTestBackend(server).CreateShareLink(ctx, "disk:/file.txt", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("successful detached rollback reported unknown state: %v", err)
	}
	select {
	case <-postMetadataStarted:
	default:
		t.Fatal("post-publish metadata request did not start")
	}
	if rollbacks.Load() != 1 {
		t.Fatalf("rollbacks = %d, want 1 despite canceled caller context", rollbacks.Load())
	}
}

func TestYandexShareCyclesAreSerializedAndQueueCancellationIsPrompt(t *testing.T) {
	t.Parallel()

	publishStarted := make(chan struct{})
	releasePublish := make(chan struct{})
	secondMetadata := make(chan struct{}, 1)
	var metadataRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/resources":
			number := metadataRequests.Add(1)
			if number == 1 {
				_, _ = fmt.Fprint(w, `{"name":"file.txt"}`)
				return
			}
			if number >= 3 {
				select {
				case secondMetadata <- struct{}{}:
				default:
				}
			}
			_, _ = fmt.Fprint(w, `{"name":"file.txt","public_key":"opaque-key","public_url":"https://share.invalid/public"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/resources/publish":
			close(publishStarted)
			<-releasePublish
			_, _ = fmt.Fprint(w, `{"href":"https://api.invalid/metadata"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	backend := yandexShareTestBackend(server)

	createResult := make(chan error, 1)
	go func() {
		_, err := backend.CreateShareLink(context.Background(), "disk:/file.txt", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer})
		createResult <- err
	}()
	select {
	case <-publishStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("publish did not start")
	}

	infoResult := make(chan error, 1)
	go func() {
		_, err := backend.ShareLinkInfo(context.Background(), "disk:/file.txt")
		infoResult <- err
	}()
	select {
	case <-secondMetadata:
		t.Fatal("metadata inspection overlapped the publish cycle")
	case <-time.After(100 * time.Millisecond):
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	canceledResult := make(chan error, 1)
	go func() {
		_, err := backend.ShareLinkInfo(canceledCtx, "disk:/file.txt")
		canceledResult <- err
	}()
	select {
	case err := <-canceledResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued cancellation error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled sharing request remained blocked behind the gate")
	}

	close(releasePublish)
	for name, result := range map[string]<-chan error{"create": createResult, "info": infoResult} {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("%s error = %v", name, err)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s did not finish", name)
		}
	}
	select {
	case <-secondMetadata:
	case <-time.After(time.Second):
		t.Fatal("queued metadata inspection never ran")
	}
	if metadataRequests.Load() != 3 {
		t.Fatalf("metadata requests = %d, want 3", metadataRequests.Load())
	}
}

func TestYandexShareRejectsInvalidPublicURLBeforeMutationWithoutLeakingIt(t *testing.T) {
	t.Parallel()

	const invalidURL = "https://user:sensitive-password@share.invalid/file" // #nosec G101 -- invalid synthetic userinfo verifies rejection without secret leakage.
	var puts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			puts.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"name":"file.txt","public_key":"sensitive-key","public_url":%q}`, invalidURL)
	}))
	defer server.Close()
	backend := yandexShareTestBackend(server)

	if _, err := backend.ShareLinkInfo(context.Background(), "disk:/file.txt"); err == nil || strings.Contains(err.Error(), invalidURL) || strings.Contains(err.Error(), "sensitive-password") {
		t.Fatalf("info error = %v", err)
	}
	if _, err := backend.CreateShareLink(context.Background(), "disk:/file.txt", vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer}); err == nil || strings.Contains(err.Error(), invalidURL) || strings.Contains(err.Error(), "sensitive-password") {
		t.Fatalf("create error = %v", err)
	}
	if puts.Load() != 0 {
		t.Fatalf("invalid preflight metadata caused %d mutation(s)", puts.Load())
	}
}

func TestProviderDetachedCleanupContextIgnoresCallerButHonorsSessionLifetime(t *testing.T) {
	caller, cancelCaller := context.WithCancel(context.Background())
	lifetime, cancelLifetime := context.WithCancel(context.Background())
	merged, stopMerged := providerOperationContext(caller, lifetime)
	defer stopMerged()
	merged = context.WithValue(merged, providerSessionLifetimeContextKey{}, lifetime)
	cancelCaller()

	cleanup, stopCleanup := providerDetachedCleanupContext(merged, time.Minute)
	defer stopCleanup()
	select {
	case <-cleanup.Done():
		t.Fatalf("caller cancellation leaked into detached cleanup: %v", cleanup.Err())
	default:
	}
	cancelLifetime()
	select {
	case <-cleanup.Done():
		if !errors.Is(cleanup.Err(), context.Canceled) {
			t.Fatalf("cleanup error = %v", cleanup.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("session lifetime cancellation did not stop detached cleanup")
	}
}

func TestYandexShareRejectsPlainHTTPPublicURL(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, `{"name":"file.txt","public_key":"key","public_url":"http://share.invalid/public"}`)
	}))
	defer server.Close()
	backend := yandexShareTestBackend(server)
	if _, err := backend.ShareLinkInfo(context.Background(), "disk:/file.txt"); err == nil || strings.Contains(err.Error(), "http://") {
		t.Fatalf("plain HTTP public URL error = %v", err)
	}
}
