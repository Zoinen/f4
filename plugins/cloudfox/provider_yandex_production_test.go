package cloudfox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type yandexProgressCapture struct {
	mu       sync.Mutex
	reporter []int
	callback []int
	cancel   bool
}

func (*yandexProgressCapture) UpdateScan(string, int64, int64) {}
func (p *yandexProgressCapture) UpdateTransfer(_ string, _ string, currentPct int, _ string, _ int, _ string) {
	p.mu.Lock()
	p.reporter = append(p.reporter, currentPct)
	p.mu.Unlock()
}
func (p *yandexProgressCapture) IsCancelled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cancel
}
func (p *yandexProgressCapture) update(_ string, percent int) {
	p.mu.Lock()
	p.callback = append(p.callback, percent)
	p.mu.Unlock()
}
func (p *yandexProgressCapture) samples() (reporter, callback []int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]int(nil), p.reporter...), append([]int(nil), p.callback...)
}

func yandexHasProgress(samples []int, want int) bool {
	for _, sample := range samples {
		if sample == want {
			return true
		}
	}
	return false
}

func yandexHasIntermediateProgress(samples []int) bool {
	for _, sample := range samples {
		if sample > 0 && sample < 100 {
			return true
		}
	}
	return false
}

func yandexReadAll(t *testing.T, reader vfs.ReadAtCloser) string {
	t.Helper()
	defer reader.Close()
	data, err := io.ReadAll(vfsReaderAdapter{ctx: context.Background(), reader: reader})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func yandexPrimeDownloadCache(t *testing.T, cache *yandexDownloadCache, location, fingerprint, value string) string {
	t.Helper()
	f, err := os.CreateTemp("", "f4-yandex-cache-prime-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(f, value); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		t.Fatal(err)
	}
	reader, err := cache.install(location, fingerprint, newProviderTempReader(f, f.Name(), int64(len(value))))
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	entry := cache.entries[location]
	cache.mu.Unlock()
	if entry == nil {
		t.Fatal("primed Yandex cache entry was not retained")
	}
	return entry.path
}

type vfsReaderAdapter struct {
	ctx    context.Context
	reader vfs.ReadAtCloser
}

func (r vfsReaderAdapter) Read(p []byte) (int, error) { return r.reader.Read(r.ctx, p) }

func TestYandexOpenReportsProgressCachesRevisionAndInvalidatesChangedContent(t *testing.T) {
	t.Parallel()

	content := strings.Repeat("first-revision-", 1<<15)
	revision := int64(41)
	modified := "2026-08-08T12:00:00Z"
	var mu sync.Mutex
	downloads := 0
	metadata := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		currentContent, currentRevision, currentModified := content, revision, modified
		mu.Unlock()
		switch {
		case r.URL.Path == "/resources" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "OAuth token" || r.URL.Query().Get("path") != "disk:/archive.7z" {
				t.Errorf("metadata auth/path = %q, %q", r.Header.Get("Authorization"), r.URL.Query().Get("path"))
			}
			sum := sha256.Sum256([]byte(currentContent))
			mu.Lock()
			metadata++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(yandexResource{
				Name: "archive.7z", Path: "disk:/archive.7z", Type: "file", Size: int64(len(currentContent)),
				Modified: currentModified, Revision: currentRevision, ResourceID: "resource-id", SHA256: hex.EncodeToString(sum[:]),
			})
		case r.URL.Path == "/resources/download" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"href":%q,"method":"GET"}`, server.URL+"/content")
		case r.URL.Path == "/content" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "" {
				t.Errorf("temporary download leaked OAuth header %q", r.Header.Get("Authorization"))
			}
			mu.Lock()
			downloads++
			mu.Unlock()
			w.Header().Set("Content-Length", fmt.Sprint(len(currentContent)))
			_, _ = io.WriteString(w, currentContent)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/", downloads: newYandexDownloadCache()}
	progress := &yandexProgressCapture{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(progress))
	ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(progress.update))
	first, err := backend.Open(ctx, "disk:/archive.7z")
	if err != nil {
		t.Fatal(err)
	}
	if local, ok := first.(interface{ LocalPath() (string, bool) }); !ok {
		t.Fatal("Yandex cached reader does not expose its local backing file to the archive layer")
	} else if localPath, valid := local.LocalPath(); !valid || localPath == "" {
		t.Fatalf("Yandex cached reader LocalPath = %q, %t", localPath, valid)
	}
	if got := yandexReadAll(t, first); got != content {
		t.Fatalf("first content length=%d, want %d", len(got), len(content))
	}
	reporter, callback := progress.samples()
	for name, samples := range map[string][]int{"ReporterKey": reporter, "ProgressKey": callback} {
		if !yandexHasProgress(samples, 0) || !yandexHasIntermediateProgress(samples) || !yandexHasProgress(samples, 100) {
			t.Errorf("%s progress = %v; want 0, intermediate, 100", name, samples)
		}
	}

	backend.downloads.mu.Lock()
	firstCachePath := backend.downloads.entries["disk:/archive.7z"].path
	backend.downloads.mu.Unlock()
	if _, err := os.Stat(firstCachePath); err != nil {
		t.Fatalf("session cache file missing after reader close: %v", err)
	}
	second, err := backend.Open(context.Background(), "disk:/archive.7z")
	if err != nil {
		t.Fatal(err)
	}
	if got := yandexReadAll(t, second); got != content {
		t.Fatal("cached reopen returned different content")
	}
	mu.Lock()
	if downloads != 1 || metadata != 3 {
		t.Fatalf("after cached reopen downloads=%d metadata=%d, want 1 and 3", downloads, metadata)
	}
	content = strings.Repeat("second-revision", 1<<15)
	revision++
	modified = "2026-08-08T12:00:01Z"
	changedContent := content
	mu.Unlock()

	third, err := backend.Open(context.Background(), "disk:/archive.7z")
	if err != nil {
		t.Fatal(err)
	}
	if got := yandexReadAll(t, third); got != changedContent {
		t.Fatalf("changed revision returned stale cache, length=%d want=%d", len(got), len(changedContent))
	}
	if _, err := os.Stat(firstCachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale revision cache file remains: %v", err)
	}
	mu.Lock()
	if downloads != 2 || metadata != 5 {
		t.Fatalf("after revision change downloads=%d metadata=%d, want 2 and 5", downloads, metadata)
	}
	mu.Unlock()
	backend.downloads.mu.Lock()
	currentCachePath := backend.downloads.entries["disk:/archive.7z"].path
	backend.downloads.mu.Unlock()
	if err := backend.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(currentCachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("session cache file remains after backend Close: %v", err)
	}
}

func TestYandexOpenRejectsRevisionChangeAndDoesNotReportCompletion(t *testing.T) {
	payload := strings.Repeat("payload", 1024)
	sum := sha256.Sum256([]byte(payload))
	metadataCalls := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/resources":
			metadataCalls++
			_ = json.NewEncoder(w).Encode(yandexResource{
				Name: "file.bin", Path: "disk:/file.bin", Type: "file", Size: int64(len(payload)),
				Modified: time.Unix(int64(metadataCalls), 0).UTC().Format(time.RFC3339), Revision: int64(metadataCalls), SHA256: hex.EncodeToString(sum[:]),
			})
		case "/resources/download":
			_, _ = fmt.Fprintf(w, `{"href":%q}`, server.URL+"/content")
		case "/content":
			w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			_, _ = io.WriteString(w, payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "f4-cloudfox-yandex-cache-*"))
	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/", downloads: newYandexDownloadCache()}
	progress := &yandexProgressCapture{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(progress))
	ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(progress.update))
	_, err := backend.Open(ctx, "disk:/file.bin")
	if !errors.Is(err, ErrRemoteObjectChanged) {
		t.Fatalf("Open error = %v, want ErrRemoteObjectChanged", err)
	}
	reporter, callback := progress.samples()
	if yandexHasProgress(reporter, 100) || yandexHasProgress(callback, 100) {
		t.Fatalf("failed revision validation reported completion: reporter=%v callback=%v", reporter, callback)
	}
	backend.downloads.mu.Lock()
	entries := len(backend.downloads.entries)
	backend.downloads.mu.Unlock()
	if entries != 0 {
		t.Fatalf("revision-mismatched download entered cache: %d entries", entries)
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "f4-cloudfox-yandex-cache-*"))
	if len(after) != len(before) {
		t.Fatalf("temporary files changed from %d to %d after rejected download", len(before), len(after))
	}
}

func TestYandexDownloadCacheIsBoundedAndCleansActiveRetirements(t *testing.T) {
	t.Parallel()
	cache := newYandexDownloadCache()
	cache.maxEntries = 1
	cache.maxBytes = 8

	newTemp := func(value string) *providerTempReader {
		f, err := os.CreateTemp("", "f4-yandex-cache-test-*")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(f, value); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			t.Fatal(err)
		}
		return newProviderTempReader(f, f.Name(), int64(len(value)))
	}

	active, err := cache.install("one", "revision-one", newTemp("aaaa"))
	if err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	activePath := cache.entries["one"].path
	cache.mu.Unlock()
	private, err := cache.install("two", "revision-two", newTemp("bbbb"))
	if err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	entries, bytes := len(cache.entries), cache.bytes
	_, firstCached := cache.entries["one"]
	_, secondCached := cache.entries["two"]
	cache.mu.Unlock()
	if entries != 1 || bytes != 4 || !firstCached || secondCached {
		t.Fatalf("active-pressure cache state: entries=%d bytes=%d first=%t second=%t", entries, bytes, firstCached, secondCached)
	}
	if err := private.Close(); err != nil {
		t.Fatal(err)
	}
	cache.invalidate("one")
	if _, err := os.Stat(activePath); err != nil {
		t.Fatalf("active retired file removed before reader close: %v", err)
	}
	if err := active.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(activePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("retired file remains after last reader close: %v", err)
	}
	cache.close()
}

func TestYandexPanelInfoUsesAuthoritativeDiskEndpointAndCache(t *testing.T) {
	t.Parallel()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/" || r.Method != http.MethodGet || r.Header.Get("Authorization") != "OAuth token" {
			t.Errorf("about request = %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		_, _ = io.WriteString(w, `{"total_space":1000,"used_space":350,"trash_size":25,"max_file_size":200,"user":{"login":"tester@yandex.ru","display_name":"Tester"}}`)
	}))
	defer server.Close()
	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/"}
	req := vfs.PanelInfoRequest{Path: "disk:/folder"}
	if backend.PanelInfoKey(req) != "yandex:disk:/folder" {
		t.Fatalf("PanelInfoKey = %q", backend.PanelInfoKey(req))
	}
	if snapshot, fresh := backend.CachedPanelInfo(req); fresh || !snapshot.Authoritative || len(snapshot.Sections) != 0 {
		t.Fatalf("empty cached snapshot = %#v, fresh=%t", snapshot, fresh)
	}
	snapshot, err := backend.RefreshPanelInfo(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Authoritative || snapshot.RefreshedAt.IsZero() || len(snapshot.Sections) != 1 {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	fields := make(map[string]vfs.PanelInfoField)
	for _, field := range snapshot.Sections[0].Fields {
		fields[field.ID] = field
	}
	if fields["user"].Value != "Tester <tester@yandex.ru>" || fields["user"].Kind != vfs.PanelInfoText {
		t.Fatalf("user field = %#v", fields["user"])
	}
	if fields["quota"].Kind != vfs.PanelInfoUsage || fields["quota"].TotalBytes != 1000 || fields["quota"].AvailableBytes != 650 {
		t.Fatalf("quota field = %#v", fields["quota"])
	}
	if fields["trash"].Bytes != 25 || fields["max_file_size"].Bytes != 200 {
		t.Fatalf("byte fields = trash %#v max %#v", fields["trash"], fields["max_file_size"])
	}
	cached, fresh := backend.CachedPanelInfo(req)
	if !fresh || len(cached.Sections) != 1 || requests != 1 {
		t.Fatalf("cached panel info fresh=%t sections=%d requests=%d", fresh, len(cached.Sections), requests)
	}
}

func TestYandexUploadReportsBothProgressChannelsAndReservesCompletionForCommit(t *testing.T) {
	t.Parallel()
	payload := strings.Repeat("upload-payload-", 1<<15)
	var uploadStatus = http.StatusCreated
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/resources/upload":
			_, _ = fmt.Fprintf(w, `{"href":%q,"method":"PUT"}`, server.URL+"/upload")
		case "/upload":
			_, _ = io.Copy(io.Discard, r.Body)
			w.WriteHeader(uploadStatus)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	run := func(status int) ([]int, []int, error) {
		uploadStatus = status
		backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/"}
		progress := &yandexProgressCapture{}
		ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(progress))
		ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(progress.update))
		writer, err := backend.Create(ctx, "disk:/upload.bin")
		if err != nil {
			return nil, nil, err
		}
		if _, err := io.WriteString(writer, payload); err != nil {
			return nil, nil, err
		}
		err = writer.Close()
		reporter, callback := progress.samples()
		return reporter, callback, err
	}
	reporter, callback, err := run(http.StatusCreated)
	if err != nil {
		t.Fatal(err)
	}
	for name, samples := range map[string][]int{"ReporterKey": reporter, "ProgressKey": callback} {
		if !yandexHasProgress(samples, 0) || !yandexHasIntermediateProgress(samples) || !yandexHasProgress(samples, 100) {
			t.Errorf("successful %s progress = %v", name, samples)
		}
	}
	reporter, callback, err = run(http.StatusInternalServerError)
	if err == nil {
		t.Fatal("failed upload returned success")
	}
	if yandexHasProgress(reporter, 100) || yandexHasProgress(callback, 100) {
		t.Fatalf("failed upload reported completion: reporter=%v callback=%v", reporter, callback)
	}
}

func TestYandexUnknownUploadInvalidatesPredecessorCache(t *testing.T) {
	t.Parallel()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/resources/upload":
			_, _ = fmt.Fprintf(w, `{"href":%q,"method":"PUT"}`, server.URL+"/upload")
		case "/upload":
			_, _ = io.Copy(io.Discard, r.Body)
			hijacker, ok := w.(http.Hijacker)
			if !ok {
				t.Error("test server does not support response hijacking")
				return
			}
			connection, _, err := hijacker.Hijack()
			if err != nil {
				t.Errorf("hijack upload response: %v", err)
				return
			}
			_ = connection.Close()
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cache := newYandexDownloadCache()
	backend := &yandexDiskBackend{client: server.Client(), baseURL: server.URL, token: "token", root: "disk:/", downloads: cache}
	oldPath := yandexPrimeDownloadCache(t, cache, "disk:/replace.bin", "old-revision", "old body")
	writer, err := backend.Create(context.Background(), "disk:/replace.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "replacement body"); err != nil {
		t.Fatal(err)
	}
	err = writer.Close()
	if !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("upload error = %v, want unknown operation state", err)
	}
	cache.mu.Lock()
	_, cached := cache.entries["disk:/replace.bin"]
	cache.mu.Unlock()
	if cached {
		t.Fatal("unknown upload retained the predecessor cache entry")
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unknown upload retained predecessor cache file: %v", err)
	}
}

func TestYandexUnknownMetadataMutationsConservativelyInvalidateCache(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		locations   []string
		mutate      func(*yandexDiskBackend) error
		invalidated []string
	}{
		{
			name:        "permanent delete",
			locations:   []string{"disk:/remove", "disk:/remove/child"},
			mutate:      func(b *yandexDiskBackend) error { return b.Remove(context.Background(), "disk:/remove") },
			invalidated: []string{"disk:/remove", "disk:/remove/child"},
		},
		{
			name:        "trash",
			locations:   []string{"disk:/trash", "disk:/trash/child"},
			mutate:      func(b *yandexDiskBackend) error { return b.MoveToTrash(context.Background(), "disk:/trash") },
			invalidated: []string{"disk:/trash", "disk:/trash/child"},
		},
		{
			name:        "move",
			locations:   []string{"disk:/old", "disk:/old/child", "disk:/new", "disk:/new/child"},
			mutate:      func(b *yandexDiskBackend) error { return b.Rename(context.Background(), "disk:/old", "disk:/new") },
			invalidated: []string{"disk:/old", "disk:/old/child", "disk:/new", "disk:/new/child"},
		},
		{
			name:      "copy destination",
			locations: []string{"disk:/source", "disk:/destination", "disk:/destination/child"},
			mutate: func(b *yandexDiskBackend) error {
				return b.Copy(context.Background(), "disk:/source", "disk:/destination")
			},
			invalidated: []string{"disk:/destination", "disk:/destination/child"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cache := newYandexDownloadCache()
			for index, location := range test.locations {
				yandexPrimeDownloadCache(t, cache, location, fmt.Sprintf("revision-%d", index), location)
			}
			yandexPrimeDownloadCache(t, cache, "disk:/unrelated", "unrelated-revision", "keep")
			client := &http.Client{Transport: mutationRoundTripper(func(*http.Request) (*http.Response, error) {
				return nil, io.ErrUnexpectedEOF
			})}
			backend := &yandexDiskBackend{client: client, baseURL: "https://cloud-api.yandex.test/v1/disk", token: "token", root: "disk:/", downloads: cache}
			if err := test.mutate(backend); !errors.Is(err, vfs.ErrOperationStateUnknown) {
				t.Fatalf("mutation error = %v, want unknown operation state", err)
			}
			cache.mu.Lock()
			for _, location := range test.invalidated {
				if _, ok := cache.entries[location]; ok {
					t.Errorf("affected cache entry %q remains", location)
				}
			}
			_, unrelated := cache.entries["disk:/unrelated"]
			_, source := cache.entries["disk:/source"]
			cache.mu.Unlock()
			if !unrelated {
				t.Error("unrelated cache entry was invalidated")
			}
			if test.name == "copy destination" && !source {
				t.Error("copy invalidated its unchanged source cache")
			}
			cache.close()
		})
	}
}
