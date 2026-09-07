package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/unxed/f4/vfs"
)

type s3DiscoveryHTTPFixture struct {
	mu       sync.Mutex
	requests []string
}

type s3SignedRegionRequest struct {
	bucket string
	prefix string
	region string
}

type s3SignedRegionFixture struct {
	mu              sync.Mutex
	listBucketCalls int
	objectRequests  []s3SignedRegionRequest
}

func s3RequestSigningRegion(request *http.Request) string {
	authorization := request.Header.Get("Authorization")
	const marker = "Credential="
	start := strings.Index(authorization, marker)
	if start < 0 {
		return ""
	}
	credential := authorization[start+len(marker):]
	if end := strings.IndexAny(credential, ", \t"); end >= 0 {
		credential = credential[:end]
	}
	parts := strings.Split(credential, "/")
	if len(parts) < 5 {
		return ""
	}
	return parts[2]
}

func (f *s3SignedRegionFixture) resetObservations() {
	f.mu.Lock()
	f.listBucketCalls = 0
	f.objectRequests = nil
	f.mu.Unlock()
}

func (f *s3SignedRegionFixture) observations() (int, []s3SignedRegionRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listBucketCalls, append([]s3SignedRegionRequest(nil), f.objectRequests...)
}

func (f *s3SignedRegionFixture) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	if request.Method == http.MethodGet && query.Get("list-type") == "2" {
		observed := s3SignedRegionRequest{
			bucket: strings.TrimPrefix(request.URL.Path, "/"),
			prefix: query.Get("prefix"),
			region: s3RequestSigningRegion(request),
		}
		f.mu.Lock()
		f.objectRequests = append(f.objectRequests, observed)
		f.mu.Unlock()
		if observed.bucket != "history-eu" {
			writer.Header().Set("Content-Type", "application/xml")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `<Error><Code>InvalidBucketName</Code><Message>wrong test bucket</Message></Error>`)
			return
		}
		if observed.region != "eu-west-1" {
			// Model the response a real regional S3 endpoint gives. The regression
			// requires the very first object-list request to be signed correctly;
			// following or learning from this redirect would still be a failure.
			writer.Header().Set("Content-Type", "application/xml")
			writer.Header().Set("x-amz-bucket-region", "eu-west-1")
			writer.WriteHeader(http.StatusMovedPermanently)
			_, _ = io.WriteString(writer, `<Error><Code>PermanentRedirect</Code><Message>use eu-west-1</Message><Region>eu-west-1</Region></Error>`)
			return
		}
		switch observed.prefix {
		case "":
			writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>history-eu</Name><Prefix></Prefix><Delimiter>/</Delimiter><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
  <CommonPrefixes><Prefix>folder/</Prefix></CommonPrefixes>
</ListBucketResult>`)
		case "folder/":
			writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>history-eu</Name><Prefix>folder/</Prefix><Delimiter>/</Delimiter><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
</ListBucketResult>`)
		default:
			writer.Header().Set("Content-Type", "application/xml")
			writer.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(writer, `<Error><Code>UnexpectedPrefix</Code><Message>unexpected prefix</Message></Error>`)
		}
		return
	}
	if request.Method == http.MethodGet && request.URL.Path == "/" {
		f.mu.Lock()
		f.listBucketCalls++
		f.mu.Unlock()
		writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Owner><ID>owner</ID><DisplayName>owner</DisplayName></Owner>
  <Buckets>
    <Bucket><Name>history-eu</Name><CreationDate>2026-01-02T03:04:05Z</CreationDate><BucketRegion>eu-west-1</BucketRegion></Bucket>
  </Buckets>
</ListAllMyBucketsResult>`)
		return
	}
	writer.Header().Set("Content-Type", "application/xml")
	writer.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(writer, `<Error><Code>UnexpectedRequest</Code><Message>%s %s</Message></Error>`, request.Method, request.URL.RequestURI()) // #nosec G705 -- this closed httptest server reflects only its controlled regression-test request in an error body.
}

type s3HistoryRegressionFactory struct {
	delegate *S3Factory
	mu       sync.Mutex
	opens    int
}

func (*s3HistoryRegressionFactory) Provider() ProviderType { return ProviderS3 }

func (f *s3HistoryRegressionFactory) Validate(connection Connection) error {
	return f.delegate.Validate(connection)
}

func (f *s3HistoryRegressionFactory) Open(ctx context.Context, connection Connection, secrets SecretValues) (Backend, error) {
	f.mu.Lock()
	f.opens++
	f.mu.Unlock()
	return f.delegate.Open(ctx, connection, secrets)
}

func (f *s3HistoryRegressionFactory) openCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opens
}

func (f *s3DiscoveryHTTPFixture) record(request *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, request.Method+" "+request.URL.RequestURI())
}

func (f *s3DiscoveryHTTPFixture) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *s3DiscoveryHTTPFixture) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func writeS3DiscoveryXML(writer http.ResponseWriter, body string) {
	writer.Header().Set("Content-Type", "application/xml")
	writer.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(writer, body)
}

func (f *s3DiscoveryHTTPFixture) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	f.record(request)
	query := request.URL.Query()
	if request.Method == http.MethodGet && query.Get("list-type") == "2" {
		bucket := strings.TrimPrefix(request.URL.Path, "/")
		prefix := query.Get("prefix")
		switch {
		case bucket == "alpha" && prefix == "":
			writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>alpha</Name><Prefix></Prefix><Delimiter>/</Delimiter><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
  <CommonPrefixes><Prefix>folder/</Prefix></CommonPrefixes>
</ListBucketResult>`)
			return
		case bucket == "alpha" && prefix == "folder/":
			writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>alpha</Name><Prefix>folder/</Prefix><Delimiter>/</Delimiter><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
  <Contents><Key>folder/note.txt</Key><LastModified>2026-03-04T05:06:07Z</LastModified><ETag>&quot;note&quot;</ETag><Size>4</Size><StorageClass>STANDARD</StorageClass></Contents>
</ListBucketResult>`)
			return
		case bucket == "beta":
			writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>beta</Name><Prefix></Prefix><Delimiter>/</Delimiter><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
</ListBucketResult>`)
			return
		}
	}
	if request.Method == http.MethodGet && request.URL.Path == "/" {
		writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Owner><ID>owner</ID><DisplayName>owner</DisplayName></Owner>
  <Buckets>
    <Bucket><Name>alpha</Name><CreationDate>2026-01-02T03:04:05Z</CreationDate><BucketRegion>eu-west-1</BucketRegion></Bucket>
    <Bucket><Name>beta</Name><CreationDate>2026-02-03T04:05:06Z</CreationDate><BucketRegion>us-west-2</BucketRegion></Bucket>
  </Buckets>
</ListAllMyBucketsResult>`)
		return
	}
	writer.Header().Set("Content-Type", "application/xml")
	writer.WriteHeader(http.StatusBadRequest)
	_, _ = fmt.Fprintf(writer, `<Error><Code>UnexpectedRequest</Code><Message>%s %s</Message></Error>`, request.Method, request.URL.RequestURI()) // #nosec G705 -- this closed httptest server reflects only its controlled regression-test request in an error body.
}

func openS3DiscoveryRegressionBackend(t *testing.T, server *httptest.Server, bucket string) *s3Backend {
	t.Helper()
	settings := S3Settings{
		Bucket: bucket, Region: "us-east-1", Endpoint: server.URL,
		UsePathStyle: true, Auth: "anonymous",
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := (&S3Factory{HTTPClient: server.Client()}).Open(context.Background(), Connection{
		ID: testConnectionID, Name: "S3 test", Provider: ProviderS3, Settings: raw,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend, ok := opened.(*s3Backend)
	if !ok {
		_ = opened.Close()
		t.Fatalf("opened backend = %T, want *s3Backend", opened)
	}
	return backend
}

func openS3SignedDiscoveryRegressionBackend(t *testing.T, server *httptest.Server) *s3Backend {
	t.Helper()
	settings := S3Settings{
		Region: "us-east-1", Endpoint: server.URL, UsePathStyle: true,
		Auth: "static", AllowInsecure: true,
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := (&S3Factory{HTTPClient: server.Client()}).Open(context.Background(), Connection{
		ID: testConnectionID, Name: "S3 test", Provider: ProviderS3, Settings: raw,
	}, SecretValues{"access_key_id": "test-access", "secret_access_key": "test-secret"})
	if err != nil {
		t.Fatal(err)
	}
	backend, ok := opened.(*s3Backend)
	if !ok {
		_ = opened.Close()
		t.Fatalf("opened backend = %T, want *s3Backend", opened)
	}
	return backend
}

func readS3DiscoveryEntries(t *testing.T, backend Backend, location string) []RemoteEntry {
	t.Helper()
	var entries []RemoteEntry
	if err := backend.ReadDir(context.Background(), location, func(chunk []RemoteEntry) {
		entries = append(entries, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	return entries
}

func findS3DiscoveryEntry(t *testing.T, entries []RemoteEntry, name string) RemoteEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.Name == name {
			return entry
		}
	}
	t.Fatalf("entry %q not found in %#v", name, entries)
	return RemoteEntry{}
}

func newS3DiscoveryRegressionCloud(t *testing.T, backend Backend) *CloudVFS {
	t.Helper()
	pool := newSessionPool()
	ready := make(chan struct{})
	close(ready)
	session := &pooledSession{pool: pool, key: "s3-discovery-regression", ready: ready, backend: backend, refs: 1}
	pool.sessions[session.key] = session
	cloud, err := newCloudVFS(Connection{
		ID: testConnectionID, Name: "S3 test", Provider: ProviderS3,
	}, nil, session, backend.Root())
	if err != nil {
		pool.release(session)
		t.Fatal(err)
	}
	return cloud
}

func TestS3DiscoveryProfileAllowsEmptyBucketAndScopesItSeparately(t *testing.T) {
	factory := &S3Factory{}
	discovery := scopedTestConnection(t, "S3", ProviderS3, S3Settings{
		Bucket: "", Region: "us-east-1", Auth: "static",
	})
	if err := factory.Validate(discovery); err != nil {
		t.Fatalf("empty bucket should enable bucket discovery: %v", err)
	}

	explicit := scopedTestConnection(t, "S3", ProviderS3, S3Settings{
		Bucket: "alpha", Region: "us-east-1", Auth: "static",
	})
	discoveryScope, required, err := credentialScope(discovery)
	if err != nil || !required || discoveryScope == "" {
		t.Fatalf("discovery credential scope = %q, required=%v, err=%v", discoveryScope, required, err)
	}
	explicitScope, explicitRequired, err := credentialScope(explicit)
	if err != nil || !explicitRequired || explicitScope == "" {
		t.Fatalf("explicit credential scope = %q, required=%v, err=%v", explicitScope, explicitRequired, err)
	}
	if discoveryScope == explicitScope {
		t.Fatal("account-wide discovery and an explicit bucket share a credential scope")
	}

	whitespace := scopedTestConnection(t, "S3", ProviderS3, S3Settings{
		Bucket: "  ", Region: " us-east-1 ", Auth: " STATIC ",
	})
	whitespaceScope, _, err := credentialScope(whitespace)
	if err != nil {
		t.Fatal(err)
	}
	if whitespaceScope != discoveryScope {
		t.Fatal("equivalent empty-bucket discovery settings produced a different credential scope")
	}
}

func TestS3ExplicitBucketNeverRequiresListBuckets(t *testing.T) {
	var mu sync.Mutex
	var accountCalls, bucketCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/":
			accountCalls++
			writer.Header().Set("Content-Type", "application/xml")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(writer, `<Error><Code>AccessDenied</Code><Message>s3:ListAllMyBuckets denied</Message></Error>`)
		case request.Method == http.MethodGet && request.URL.Path == "/manual" && request.URL.Query().Get("list-type") == "2":
			bucketCalls++
			writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>manual</Name><Prefix></Prefix><Delimiter>/</Delimiter><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
</ListBucketResult>`)
		default:
			writer.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	backend := openS3DiscoveryRegressionBackend(t, server, "manual")
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if entries := readS3DiscoveryEntries(t, backend, backend.Root()); len(entries) != 0 {
		t.Fatalf("manual bucket root entries = %#v, want empty", entries)
	}
	mu.Lock()
	defer mu.Unlock()
	if accountCalls != 0 {
		t.Fatalf("explicit bucket made %d ListBuckets request(s)", accountCalls)
	}
	if bucketCalls != 1 {
		t.Fatalf("explicit bucket made %d ListObjectsV2 request(s), want 1", bucketCalls)
	}
}

func TestS3DiscoveryCloudVFSUsesVisualNativePathsAndRestoresThem(t *testing.T) {
	fixture := &s3DiscoveryHTTPFixture{}
	server := httptest.NewServer(fixture)
	defer server.Close()

	backend := openS3DiscoveryRegressionBackend(t, server, "")
	cloud := newS3DiscoveryRegressionCloud(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})

	separator := string(os.PathSeparator)
	root := "S3 test:" + separator
	if got := cloud.GetPath(); got != root {
		t.Fatalf("discovery root = %q, want %q", got, root)
	}
	if err := cloud.ReadDir(context.Background(), root, func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	bucketPath := cloud.Join(root, "alpha")
	if err := cloud.SetPath(bucketPath); err != nil {
		t.Fatal(err)
	}
	if err := cloud.ReadDir(context.Background(), bucketPath, func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	folderPath := cloud.Join(bucketPath, "folder")
	if err := cloud.SetPath(folderPath); err != nil {
		t.Fatal(err)
	}
	wantFolder := root + "alpha" + separator + "folder"
	if got := cloud.GetPath(); got != wantFolder {
		t.Fatalf("nested discovery path = %q, want %q", got, wantFolder)
	}
	if got, want := cloud.Dir(folderPath), root+"alpha"; got != want {
		t.Fatalf("Dir(folder) = %q, want %q", got, want)
	}
	if got, want := cloud.Join(folderPath, ".."), root+"alpha"; got != want {
		t.Fatalf("Join(folder, ..) = %q, want %q", got, want)
	}
	foreignSeparator := "/"
	if os.PathSeparator == '/' {
		foreignSeparator = "\\"
	}
	for _, publicPath := range []string{root, bucketPath, folderPath, cloud.GetPath(), cloud.Dir(folderPath)} {
		if strings.Contains(publicPath, foreignSeparator) || strings.Contains(publicPath, "cloud://") || strings.Contains(publicPath, "%") {
			t.Fatalf("public discovery path leaked an internal/non-native representation: %q", publicPath)
		}
	}

	freshBackend := openS3DiscoveryRegressionBackend(t, server, "")
	fresh := newS3DiscoveryRegressionCloud(t, freshBackend)
	t.Cleanup(func() {
		if err := fresh.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	if err := fresh.restoreVisualPath(context.Background(), []string{"alpha", "folder"}); err != nil {
		t.Fatal(err)
	}
	if got := fresh.GetPath(); got != wantFolder {
		t.Fatalf("restored discovery path = %q, want %q", got, wantFolder)
	}
}

func TestS3DiscoveryGuardsAccountAndBucketRootsWithoutAPIRequests(t *testing.T) {
	fixture := &s3DiscoveryHTTPFixture{}
	server := httptest.NewServer(fixture)
	defer server.Close()
	backend := openS3DiscoveryRegressionBackend(t, server, "")
	t.Cleanup(func() {
		if err := backend.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})

	bucket := findS3DiscoveryEntry(t, readS3DiscoveryEntries(t, backend, backend.Root()), "alpha")
	newBucket := backend.Join(backend.Root(), "new-bucket")
	operations := []struct {
		name string
		call func() error
	}{
		{"mkdir account root", func() error { return backend.MkDir(context.Background(), backend.Root()) }},
		{"remove account root", func() error { return backend.Remove(context.Background(), backend.Root()) }},
		{"mkdir bucket root", func() error { return backend.MkDir(context.Background(), bucket.Location) }},
		{"remove bucket root", func() error { return backend.Remove(context.Background(), bucket.Location) }},
		{"mkdir implicit bucket", func() error { return backend.MkDir(context.Background(), newBucket) }},
		{"rename bucket root", func() error { return backend.Rename(context.Background(), bucket.Location, newBucket) }},
		{"copy bucket root", func() error {
			copier, ok := any(backend).(BackendCopier)
			if !ok {
				return ErrUnsupportedOperation
			}
			return copier.Copy(context.Background(), bucket.Location, newBucket)
		}},
		{"open bucket root", func() error {
			reader, err := backend.Open(context.Background(), bucket.Location)
			if reader != nil {
				_ = reader.Close()
				return errors.New("bucket root was opened as a file")
			}
			return err
		}},
		{"create implicit bucket", func() error {
			writer, err := backend.Create(context.Background(), newBucket)
			if writer != nil {
				if aborter, ok := writer.(vfs.AbortableWriter); ok {
					_ = aborter.Abort()
				} else {
					_ = writer.Close()
				}
				return errors.New("account-root child was created as an object")
			}
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			before := fixture.requestCount()
			if err := operation.call(); err == nil {
				t.Fatal("root operation unexpectedly succeeded")
			}
			if after := fixture.requestCount(); after != before {
				t.Fatalf("guarded root operation issued API requests: before=%d after=%d all=%v", before, after, fixture.snapshot())
			}
		})
	}
}

func TestS3ExplicitBucketRejectsDiscoveryLocationForAnotherBucket(t *testing.T) {
	fixture := &s3DiscoveryHTTPFixture{}
	server := httptest.NewServer(fixture)
	defer server.Close()
	discovery := openS3DiscoveryRegressionBackend(t, server, "")
	t.Cleanup(func() {
		if err := discovery.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	alpha := findS3DiscoveryEntry(t, readS3DiscoveryEntries(t, discovery, discovery.Root()), "alpha")

	explicit := openS3DiscoveryRegressionBackend(t, server, "beta")
	t.Cleanup(func() {
		if err := explicit.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})
	before := fixture.requestCount()
	if normalized, err := explicit.Normalize(alpha.Location); err == nil {
		t.Fatalf("manual beta backend accepted alpha discovery location as %q", normalized)
	}
	if after := fixture.requestCount(); after != before {
		t.Fatalf("rejecting a cross-bucket location performed API I/O: before=%d after=%d", before, after)
	}
}

func assertFirstS3ObjectRequestUsesRegion(t *testing.T, fixture *s3SignedRegionFixture, wantBucket, wantPrefix, wantRegion string) {
	t.Helper()
	_, requests := fixture.observations()
	if len(requests) == 0 {
		t.Fatal("navigation made no ListObjectsV2 request")
	}
	first := requests[0]
	if first.bucket != wantBucket || first.prefix != wantPrefix || first.region != wantRegion {
		t.Fatalf("first ListObjectsV2 = bucket %q prefix %q signing region %q; want %q, %q, %q (all requests: %#v)",
			first.bucket, first.prefix, first.region, wantBucket, wantPrefix, wantRegion, requests)
	}
}

func TestS3DiscoverySameSessionVisualHistoryUsesCachedBucketRegionOnFirstRequest(t *testing.T) {
	fixture := &s3SignedRegionFixture{}
	server := httptest.NewServer(fixture)
	defer server.Close()
	backend := openS3SignedDiscoveryRegressionBackend(t, server)
	cloud := newS3DiscoveryRegressionCloud(t, backend)
	t.Cleanup(func() {
		if err := cloud.Close(); err != nil {
			t.Errorf("close test resource: %v", err)
		}
	})

	separator := string(os.PathSeparator)
	root := "S3 test:" + separator
	bucketPath := root + "history-eu"
	folderPath := bucketPath + separator + "folder"
	if err := cloud.ReadDir(context.Background(), root, func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	if err := cloud.SetPath(bucketPath); err != nil {
		t.Fatal(err)
	}
	if err := cloud.ReadDir(context.Background(), bucketPath, func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	if err := cloud.SetPath(folderPath); err != nil {
		t.Fatal(err)
	}
	if got := cloud.GetPath(); got != folderPath {
		t.Fatalf("same-session history path = %q, want %q", got, folderPath)
	}

	// Simulate Alt+Left/Alt+Right against visual history while keeping the
	// mounted CloudVFS and its aliases. The first provider call after returning
	// to the bucket must already use the region learned by ListBuckets.
	if err := cloud.SetPath(root); err != nil {
		t.Fatal(err)
	}
	fixture.resetObservations()
	if err := cloud.SetPath(bucketPath); err != nil {
		t.Fatal(err)
	}
	if err := cloud.ReadDir(context.Background(), bucketPath, func([]vfs.VFSItem) {}); err != nil {
		t.Fatal(err)
	}
	assertFirstS3ObjectRequestUsesRegion(t, fixture, "history-eu", "", "eu-west-1")
	if listedBuckets, _ := fixture.observations(); listedBuckets != 0 {
		t.Fatalf("same-session alias navigation unnecessarily called ListBuckets %d time(s)", listedBuckets)
	}
}

func TestS3DiscoveryFreshSessionRestoresVisualHistoryBeforeRegionalObjectList(t *testing.T) {
	fixture := &s3SignedRegionFixture{}
	server := httptest.NewServer(fixture)
	defer server.Close()
	factory := &s3HistoryRegressionFactory{delegate: &S3Factory{HTTPClient: server.Client()}}
	secretStore := &memorySecretStore{}
	plugin := NewPlugin(Options{
		ConfigDir: t.TempDir(), Portable: true, Vault: secretStore,
		Factories: []BackendFactory{factory},
	})
	separator := string(os.PathSeparator)
	settings, err := json.Marshal(S3Settings{
		Region: "us-east-1", Endpoint: server.URL, UsePathStyle: true, Auth: "static", AllowInsecure: true,
	})
	if err != nil {
		if closeErr := plugin.Close(); closeErr != nil {
			t.Errorf("close plugin after settings failure: %v", closeErr)
		}
		t.Fatal(err)
	}
	credentials := SecretValues{"access_key_id": "test-access", "secret_access_key": "test-secret"}
	connection, err := plugin.repo.Save(context.Background(), Connection{
		Name: "History S3", Provider: ProviderS3, Settings: settings,
	}, &credentials, SecretStorageVault)
	if err != nil {
		if closeErr := plugin.Close(); closeErr != nil {
			t.Errorf("close plugin after save failure: %v", closeErr)
		}
		t.Fatal(err)
	}
	historyPath := connection.Name + ":" + separator + "history-eu" + separator + "folder"
	provider := &connectionProvider{plugin: plugin}
	if !provider.CanOpen(context.Background(), vfs.NewOSVFS(t.TempDir()), historyPath) {
		if closeErr := plugin.Close(); closeErr != nil {
			t.Errorf("close plugin after rejected path: %v", closeErr)
		}
		t.Fatalf("provider rejected S3 visual history path %q", historyPath)
	}
	opened, err := provider.Open(context.Background(), vfs.NewOSVFS(t.TempDir()), historyPath)
	if err != nil {
		if closeErr := plugin.Close(); closeErr != nil {
			t.Errorf("close plugin after open failure: %v", closeErr)
		}
		t.Fatal(err)
	}
	if got := opened.GetPath(); got != historyPath {
		_ = opened.Close()
		if closeErr := plugin.Close(); closeErr != nil {
			t.Errorf("close plugin after path mismatch: %v", closeErr)
		}
		t.Fatalf("fresh-session restored path = %q, want %q", got, historyPath)
	}
	if err := opened.ReadDir(context.Background(), historyPath, func([]vfs.VFSItem) {}); err != nil {
		_ = opened.Close()
		if closeErr := plugin.Close(); closeErr != nil {
			t.Errorf("close plugin after read failure: %v", closeErr)
		}
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		if closeErr := plugin.Close(); closeErr != nil {
			t.Errorf("close plugin after VFS close failure: %v", closeErr)
		}
		t.Fatal(err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if opens := factory.openCount(); opens != 1 {
		t.Fatalf("fresh visual-history restore opened %d S3 backends, want 1", opens)
	}
	listBucketCalls, requests := fixture.observations()
	if listBucketCalls != 1 {
		t.Fatalf("fresh visual-history restore called ListBuckets %d time(s), want 1", listBucketCalls)
	}
	if len(requests) < 2 {
		t.Fatalf("fresh restore object-list requests = %#v, want bucket and folder resolution", requests)
	}
	if first := requests[0]; first.bucket != "history-eu" || first.prefix != "" || first.region != "eu-west-1" {
		t.Fatalf("fresh restore first ListObjectsV2 = %#v, want history-eu root signed for eu-west-1 (all %#v)", first, requests)
	}
	if second := requests[1]; second.bucket != "history-eu" || second.prefix != "folder/" || second.region != "eu-west-1" {
		t.Fatalf("fresh restore second ListObjectsV2 = %#v, want history-eu/folder signed for eu-west-1 (all %#v)", second, requests)
	}
}
