package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestYandexReadDirPaginatesAndAuthenticates(t *testing.T) {
	t.Parallel()

	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resources" || r.Method != http.MethodGet {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if got := r.Header.Get("Authorization"); got != "OAuth test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("path"); got != "disk:/folder" {
			t.Errorf("path = %q", got)
		}
		offset := r.URL.Query().Get("offset")
		offsets = append(offsets, offset)
		w.Header().Set("Content-Type", "application/json")
		switch offset {
		case "0":
			_, _ = fmt.Fprint(w, `{"name":"folder","path":"disk:/folder","type":"dir","_embedded":{"items":[{"name":"a.txt","path":"disk:/folder/a","type":"file","size":1},{"name":"b","path":"disk:/folder/b","type":"dir"}],"limit":1000,"offset":0,"total":3}}`)
		case "2":
			_, _ = fmt.Fprint(w, `{"name":"folder","path":"disk:/folder","type":"dir","_embedded":{"items":[{"name":"c.txt","path":"disk:/folder/c","type":"file","size":3}],"limit":1000,"offset":2,"total":3}}`)
		default:
			t.Errorf("unexpected offset %q", offset)
			_, _ = fmt.Fprint(w, `{"_embedded":{"items":[],"total":3}}`)
		}
	}))
	defer server.Close()

	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "test-token", root: "disk:/"}
	var chunks [][]RemoteEntry
	err := backend.ReadDir(context.Background(), "disk:/folder", func(entries []RemoteEntry) {
		chunks = append(chunks, append([]RemoteEntry(nil), entries...))
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(offsets, []string{"0", "2"}) {
		t.Fatalf("offsets = %v", offsets)
	}
	if len(chunks) != 2 || len(chunks[0]) != 2 || len(chunks[1]) != 1 {
		t.Fatalf("chunks = %#v", chunks)
	}
	if chunks[0][0].Location != "disk:/folder/a" || chunks[0][1].Name != "b" || !chunks[0][1].IsDir || chunks[1][0].Size != 3 {
		t.Fatalf("entries = %#v", chunks)
	}
}

func TestYandexTrashAndPermanentDeletePollAcceptedOperation(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var permanent []string
	var operationChecks int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/resources" && r.Method == http.MethodDelete:
			mu.Lock()
			permanent = append(permanent, r.URL.Query().Get("permanently"))
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"href":%q,"method":"GET"}`, "http://"+r.Host+"/operations/1") // #nosec G705 -- this closed httptest server uses its own controlled Host to form a fixture operation URL.
		case r.URL.Path == "/operations/1" && r.Method == http.MethodGet:
			operationChecks++
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprint(w, `{"status":"success"}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/"}
	if err := backend.MoveToTrash(context.Background(), "disk:/trash-me"); err != nil {
		t.Fatal(err)
	}
	if err := backend.Remove(context.Background(), "disk:/delete-me"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	gotPermanent := append([]string(nil), permanent...)
	mu.Unlock()
	if !reflect.DeepEqual(gotPermanent, []string{"false", "true"}) {
		t.Fatalf("permanently values = %v", gotPermanent)
	}
	if operationChecks != 2 {
		t.Fatalf("operation checks = %d, want 2", operationChecks)
	}
}

func TestYandexMalformedAcceptedMutationIsUnknown(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resources" || r.Method != http.MethodDelete {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, "{")
	}))
	defer server.Close()

	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "test-token", root: "disk:/"}
	err := backend.Remove(context.Background(), "disk:/possibly-deleted")
	if !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("malformed accepted mutation error = %v, want unknown operation state", err)
	}
}

func TestYandexAuthorizationCodePKCEExchange(t *testing.T) {
	t.Parallel()
	const verifier = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ-._~"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/token" || r.Method != http.MethodPost {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("code") != "browser-code" ||
			r.Form.Get("client_id") != "client-123" || r.Form.Get("code_verifier") != verifier {
			t.Errorf("token form = %v", r.Form)
		}
		if r.Form.Get("client_secret") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("client secret unexpectedly sent: form=%v authorization=%q", r.Form, r.Header.Get("Authorization"))
		}
		_, _ = fmt.Fprint(w, `{"access_token":"access-xyz","refresh_token":"refresh-xyz","token_type":"bearer"}`)
	}))
	defer server.Close()

	authorizationURL, err := YandexAuthorizationURL("https://oauth.test", " client-123 ", "https://oauth.yandex.ru/verification_code", verifier)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorizationURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Path != "/authorize" || query.Get("response_type") != "code" || query.Get("client_id") != "client-123" ||
		query.Get("redirect_uri") != "https://oauth.yandex.ru/verification_code" || query.Get("code_challenge_method") != "S256" ||
		query.Get("code_challenge") == "" || query.Get("force_confirm") != "yes" {
		t.Fatalf("authorization URL = %s", authorizationURL)
	}
	secrets, err := ExchangeYandexAuthorizationCode(context.Background(), server.Client(), server.URL, "client-123", " browser-code ", verifier)
	if err != nil {
		t.Fatal(err)
	}
	delete(secrets, "expires_at")
	if !reflect.DeepEqual(secrets, SecretValues{"oauth_token": "access-xyz", "refresh_token": "refresh-xyz"}) {
		t.Fatalf("secrets = %#v", secrets)
	}
}

func TestOAuthClientDoesNotReplayPOSTBodyAcrossRedirect(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		mu.Lock()
		targetRequests++
		mu.Unlock()
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/token-capture", http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	if _, err := ExchangeYandexAuthorizationCode(context.Background(), source.Client(), source.URL, "client-id", "code", "verifier"); err == nil {
		t.Fatal("redirected OAuth POST unexpectedly succeeded")
	}
	mu.Lock()
	got := targetRequests
	mu.Unlock()
	if got != 0 {
		t.Fatalf("OAuth redirect target received %d request(s)", got)
	}
}

func yandexTestConnection(t *testing.T, settings YandexDiskSettings) Connection {
	t.Helper()
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	return Connection{ID: testConnectionID, Name: "Yandex", Provider: ProviderYandexDisk, Settings: raw}
}

func TestYandexFactoryRequiresToken(t *testing.T) {
	t.Parallel()
	_, err := (&YandexDiskFactory{}).Open(context.Background(), yandexTestConnection(t, YandexDiskSettings{}), nil)
	if err != ErrAuthenticationRequired {
		t.Fatalf("Open error = %v", err)
	}
}

func TestNormalizeYandexPath(t *testing.T) {
	t.Parallel()
	tests := map[string]string{"": "disk:/", "/a/../b": "disk:/b", `disk:\\folder\\file`: "disk:/folder/file", "app:/folder/../file": "app:/file"}
	for input, want := range tests {
		got, err := normalizeYandexPath(input)
		if err != nil || got != want {
			t.Errorf("normalizeYandexPath(%q) = %q, %v; want %q", input, got, err, want)
		}
	}
}

func TestYandexDoesNotFollowAuthenticatedStatusRedirect(t *testing.T) {
	t.Parallel()

	var targetMu sync.Mutex
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		targetMu.Lock()
		targetRequests++
		targetMu.Unlock()
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("OAuth credential leaked to redirect target: %q", got)
		}
		_, _ = fmt.Fprint(w, `{"status":"success"}`)
	}))
	defer target.Close()

	var api *httptest.Server
	api = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "OAuth test-token" {
			t.Errorf("Authorization = %q", got)
		}
		switch {
		case r.URL.Path == "/resources" && r.Method == http.MethodDelete:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"href":%q,"method":"GET"}`, api.URL+"/operations/1")
		case r.URL.Path == "/operations/1" && r.Method == http.MethodGet:
			http.Redirect(w, r, target.URL+"/pretend-success", http.StatusFound)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer api.Close()

	backend := &yandexDiskBackend{client: api.Client(), baseURL: api.URL, token: "test-token", root: "disk:/"}
	if err := backend.Remove(context.Background(), "disk:/delete-me"); err == nil {
		t.Fatal("redirected status poll unexpectedly reported a successful delete")
	}
	targetMu.Lock()
	gotTargetRequests := targetRequests
	targetMu.Unlock()
	if gotTargetRequests != 0 {
		t.Fatalf("authenticated redirect target was requested %d time(s)", gotTargetRequests)
	}
}

func TestYandexTemporaryURLsStayWithinTransferOrigins(t *testing.T) {
	t.Parallel()
	backend := &yandexDiskBackend{baseURL: "https://cloud-api.yandex.net/v1/disk"}

	for _, raw := range []string{
		"https://downloader.disk.yandex.net/signed",
		"https://uploader.disk.yandex.net/signed",
		"https://node.storage.yandex.net/signed",
		"https://cloud-api.yandex.net/v1/disk/signed",
	} {
		if _, err := backend.validateTemporaryURL(raw); err != nil {
			t.Errorf("validateTemporaryURL(%q): %v", raw, err)
		}
	}
	for _, raw := range []string{
		"https://127.0.0.1/metadata",
		"https://disk.yandex.net.attacker.example/signed",
		"https://uploader.disk.yandex.net:8443/signed",
		"http://downloader.disk.yandex.net/signed",
	} {
		if _, err := backend.validateTemporaryURL(raw); err == nil {
			t.Errorf("validateTemporaryURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestYandexDoesNotFollowTemporaryURLRedirectOutsideTransferOrigins(t *testing.T) {
	t.Parallel()

	var targetMu sync.Mutex
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetMu.Lock()
		targetRequests++
		targetMu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/internal", http.StatusFound)
	}))
	defer source.Close()

	backend := &yandexDiskBackend{client: source.Client(), baseURL: source.URL}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, source.URL+"/signed", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := backend.do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("temporary redirect response status = %d, want %d", resp.StatusCode, http.StatusFound)
	}
	targetMu.Lock()
	gotTargetRequests := targetRequests
	targetMu.Unlock()
	if gotTargetRequests != 0 {
		t.Fatalf("temporary redirect target was requested %d time(s)", gotTargetRequests)
	}
}

func TestYandexDoesNotFollowUploadRedirectAsGET(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	redirectTargets := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/resources/upload" && r.Method == http.MethodGet:
			if got := r.Header.Get("Authorization"); got != "OAuth test-token" {
				t.Errorf("Authorization = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"href":%q,"method":"PUT"}`, server.URL+"/upload-target")
		case r.URL.Path == "/upload-target" && r.Method == http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			http.Redirect(w, r, "/pretend-success", http.StatusFound)
		case r.URL.Path == "/pretend-success":
			mu.Lock()
			redirectTargets++
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "test-token", root: "disk:/"}
	w, err := backend.Create(context.Background(), "disk:/upload.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "payload"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err == nil {
		t.Fatal("redirected upload unexpectedly succeeded")
	}
	mu.Lock()
	gotRedirectTargets := redirectTargets
	mu.Unlock()
	if gotRedirectTargets != 0 {
		t.Fatalf("upload redirect target was requested %d time(s)", gotRedirectTargets)
	}
}

func TestYandexCreateNoOverwriteSurvivesConcurrentRace(t *testing.T) {
	t.Parallel()

	var (
		mu               sync.Mutex
		stored           []byte
		uploadURLQueries []string
	)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/resources/upload" && r.Method == http.MethodGet:
			overwrite := r.URL.Query().Get("overwrite")
			mu.Lock()
			uploadURLQueries = append(uploadURLQueries, overwrite)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"href":%q,"method":"PUT"}`, server.URL+"/upload-target?overwrite="+url.QueryEscape(overwrite))
		case r.URL.Path == "/upload-target" && r.Method == http.MethodPut:
			payload, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read upload: %v", err)
			}
			arrived <- struct{}{}
			<-release
			mu.Lock()
			defer mu.Unlock()
			if r.URL.Query().Get("overwrite") == "false" && stored != nil {
				w.WriteHeader(http.StatusConflict)
				_, _ = io.WriteString(w, `{"error":"DiskResourceAlreadyExistsError"}`)
				return
			}
			stored = append([]byte(nil), payload...)
			w.WriteHeader(http.StatusCreated)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/"}
	writers := make([]io.WriteCloser, 2)
	for i, payload := range []string{"first-writer", "second-writer"} {
		writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), false), "disk:/race.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(writer, payload); err != nil {
			t.Fatal(err)
		}
		writers[i] = writer
	}
	errorsByWriter := make(chan error, len(writers))
	for _, writer := range writers {
		go func(writer io.WriteCloser) { errorsByWriter <- writer.Close() }(writer)
	}
	<-arrived
	<-arrived
	close(release)

	var success, conflict int
	for range writers {
		err := <-errorsByWriter
		switch {
		case err == nil:
			success++
		case errors.Is(err, os.ErrExist):
			if errors.Is(err, vfs.ErrOperationStateUnknown) {
				t.Fatalf("definitive conflict reported as unknown: %v", err)
			}
			conflict++
		default:
			t.Fatalf("Close error = %v, want success or os.ErrExist", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if success != 1 || conflict != 1 {
		t.Fatalf("successes=%d conflicts=%d, want one of each", success, conflict)
	}
	if !reflect.DeepEqual(uploadURLQueries, []string{"false", "false"}) {
		t.Fatalf("upload URL overwrite values = %v", uploadURLQueries)
	}
	if got := string(stored); got != "first-writer" && got != "second-writer" {
		t.Fatalf("stored payload = %q", got)
	}
}

func TestYandexCreateNoOverwriteURLConflictIsDefinitive(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/resources/upload" || r.Method != http.MethodGet {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		if got := r.URL.Query().Get("overwrite"); got != "false" {
			t.Errorf("overwrite = %q, want false", got)
		}
		w.WriteHeader(http.StatusConflict)
		_, _ = io.WriteString(w, `{"error":"DiskResourceAlreadyExistsError"}`)
	}))
	defer server.Close()

	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/"}
	writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), false), "disk:/occupied.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(writer, "must-not-upload")
	err = writer.Close()
	if !errors.Is(err, os.ErrExist) || errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("Close error = %v, want definitive os.ErrExist", err)
	}
}

func TestYandexMutationOverwriteIntentIsExplicit(t *testing.T) {
	t.Parallel()

	type requestRecord struct {
		method    string
		path      string
		overwrite string
	}
	var (
		mu      sync.Mutex
		records []requestRecord
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		records = append(records, requestRecord{method: r.Method, path: r.URL.Path, overwrite: r.URL.Query().Get("overwrite")})
		mu.Unlock()
		if r.URL.Path == "/resources" && r.Method == http.MethodPut {
			w.WriteHeader(http.StatusConflict)
			_, _ = io.WriteString(w, `{"error":"DiskResourceAlreadyExistsError"}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/"}
	for _, overwrite := range []bool{false, true} {
		ctx := vfs.WithDestinationOverwrite(context.Background(), overwrite)
		if err := backend.Copy(ctx, "disk:/source", "disk:/copy"); err != nil {
			t.Fatal(err)
		}
		if err := backend.Rename(ctx, "disk:/source", "disk:/moved"); err != nil {
			t.Fatal(err)
		}
	}
	if err := backend.MkDir(vfs.WithDestinationOverwrite(context.Background(), false), "disk:/existing"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("MkDir error = %v, want os.ErrExist", err)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []requestRecord{
		{method: http.MethodPost, path: "/resources/copy", overwrite: "false"},
		{method: http.MethodPost, path: "/resources/move", overwrite: "false"},
		{method: http.MethodPost, path: "/resources/copy", overwrite: "true"},
		{method: http.MethodPost, path: "/resources/move", overwrite: "true"},
		// Yandex MkDir is intrinsically create-only and exposes no overwrite
		// parameter; an occupied name is a definitive conflict.
		{method: http.MethodPut, path: "/resources"},
	}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("requests = %#v, want %#v", records, want)
	}
}

func TestYandexMkDirTransportFailureIsUnknownState(t *testing.T) {
	t.Parallel()
	transportErr := errors.New("connection reset after request write")
	backend := &yandexDiskBackend{
		client: &http.Client{Transport: mutationRoundTripper(func(*http.Request) (*http.Response, error) {
			return nil, transportErr
		})},
		baseURL: "https://cloud-api.test/v1/disk",
		token:   "token",
		root:    "disk:/",
	}
	err := backend.MkDir(context.Background(), "disk:/possibly-created")
	if !errors.Is(err, vfs.ErrOperationStateUnknown) || !errors.Is(err, transportErr) {
		t.Fatalf("MkDir error = %v, want unknown state wrapping transport failure", err)
	}
}

func TestYandexSignedTransferFailuresNeverExposeTemporaryCredentials(t *testing.T) {
	const (
		pathSecret  = "PATH-SIGNATURE-DO-NOT-LOG"
		querySecret = "QUERY-SIGNATURE-DO-NOT-LOG"
	)
	transportErr := errors.New("temporary storage connection failed")

	for _, operation := range []string{"download", "upload"} {
		for _, failure := range []string{"transport", "status"} {
			t.Run(operation+"_"+failure, func(t *testing.T) {
				client := &http.Client{Transport: mutationRoundTripper(func(request *http.Request) (*http.Response, error) {
					if request.URL.Host == "api.test" {
						var body string
						switch request.URL.Path {
						case "/resources":
							body = `{"name":"file.bin","path":"disk:/file.bin","type":"file","size":1,"revision":1,"resource_id":"resource-1"}`
						case "/resources/download", "/resources/upload":
							body = fmt.Sprintf(`{"href":%q,"method":%q}`, "https://downloader.disk.yandex.net/"+pathSecret+"?signature="+querySecret, map[bool]string{true: "PUT", false: "GET"}[request.URL.Path == "/resources/upload"])
						default:
							return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found")), Request: request}, nil
						}
						return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: request, Header: make(http.Header)}, nil
					}
					if failure == "transport" {
						return nil, transportErr
					}
					body := "gateway echoed https://downloader.disk.yandex.net/" + pathSecret + "?signature=" + querySecret
					return &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader(body)), Request: request, Header: make(http.Header)}, nil
				})}
				backend := &yandexDiskBackend{client: client, baseURL: "https://api.test", token: "token", root: "disk:/"}
				var err error
				if operation == "download" {
					_, err = backend.Open(context.Background(), "disk:/file.bin")
				} else {
					var writer io.WriteCloser
					writer, err = backend.Create(context.Background(), "disk:/file.bin")
					if err == nil {
						_, _ = io.WriteString(writer, "x")
						err = writer.Close()
					}
				}
				if err == nil {
					t.Fatal("transfer failure unexpectedly succeeded")
				}
				if failure == "transport" && !errors.Is(err, transportErr) {
					t.Fatalf("transport identity was lost: %v", err)
				}
				if message := err.Error(); strings.Contains(message, pathSecret) || strings.Contains(message, querySecret) {
					t.Fatalf("signed transfer credential leaked in error: %s", message)
				}
			})
		}
	}
}
