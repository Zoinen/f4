package cloudfox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awstypes "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/unxed/f4/vfs"
)

type s3DiscoveryCoreFake struct {
	mu          sync.Mutex
	regions     map[string][]string
	listBuckets []*awss3.ListBucketsOutput
	listErr     error
	getBody     string
	listInputs  []*awss3.ListObjectsV2Input
}

func (f *s3DiscoveryCoreFake) record(operation string, options []func(*awss3.Options)) {
	configured := awss3.Options{Region: "configured-region"}
	for _, option := range options {
		option(&configured)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.regions == nil {
		f.regions = make(map[string][]string)
	}
	f.regions[operation] = append(f.regions[operation], configured.Region)
}

func (f *s3DiscoveryCoreFake) lastRegion(operation string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	values := f.regions[operation]
	if len(values) == 0 {
		return ""
	}
	return values[len(values)-1]
}

func (f *s3DiscoveryCoreFake) ListBuckets(_ context.Context, _ *awss3.ListBucketsInput, options ...func(*awss3.Options)) (*awss3.ListBucketsOutput, error) {
	f.record("ListBuckets", options)
	if f.listErr != nil {
		return nil, f.listErr
	}
	if len(f.listBuckets) == 0 {
		return nil, nil
	}
	output := f.listBuckets[0]
	f.listBuckets = f.listBuckets[1:]
	return output, nil
}

func (f *s3DiscoveryCoreFake) ListObjectsV2(_ context.Context, input *awss3.ListObjectsV2Input, options ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	f.record("ListObjectsV2", options)
	f.mu.Lock()
	clone := *input
	f.listInputs = append(f.listInputs, &clone)
	f.mu.Unlock()
	return &awss3.ListObjectsV2Output{}, nil
}

func (f *s3DiscoveryCoreFake) HeadObject(_ context.Context, _ *awss3.HeadObjectInput, options ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error) {
	f.record("HeadObject", options)
	return &awss3.HeadObjectOutput{ContentLength: aws.Int64(4), ETag: aws.String(`"generation"`)}, nil
}

func (f *s3DiscoveryCoreFake) GetObject(_ context.Context, input *awss3.GetObjectInput, options ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	f.record("GetObject", options)
	body := f.getBody
	if body == "" {
		body = "data"
	}
	return &awss3.GetObjectOutput{
		Body: io.NopCloser(strings.NewReader(body)), ContentLength: aws.Int64(int64(len(body))),
		ContentRange: aws.String("bytes 0-3/4"), ETag: aws.String(`"generation"`),
	}, nil
}

func (f *s3DiscoveryCoreFake) PutObject(_ context.Context, _ *awss3.PutObjectInput, options ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	f.record("PutObject", options)
	return &awss3.PutObjectOutput{}, nil
}

func (f *s3DiscoveryCoreFake) DeleteObject(_ context.Context, _ *awss3.DeleteObjectInput, options ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	f.record("DeleteObject", options)
	return &awss3.DeleteObjectOutput{}, nil
}

func (f *s3DiscoveryCoreFake) DeleteObjects(_ context.Context, _ *awss3.DeleteObjectsInput, options ...func(*awss3.Options)) (*awss3.DeleteObjectsOutput, error) {
	f.record("DeleteObjects", options)
	return &awss3.DeleteObjectsOutput{}, nil
}

func (f *s3DiscoveryCoreFake) CopyObject(_ context.Context, _ *awss3.CopyObjectInput, options ...func(*awss3.Options)) (*awss3.CopyObjectOutput, error) {
	f.record("CopyObject", options)
	return &awss3.CopyObjectOutput{}, nil
}

func (f *s3DiscoveryCoreFake) CreateMultipartUpload(_ context.Context, _ *awss3.CreateMultipartUploadInput, options ...func(*awss3.Options)) (*awss3.CreateMultipartUploadOutput, error) {
	f.record("CreateMultipartUpload", options)
	return &awss3.CreateMultipartUploadOutput{UploadId: aws.String("upload")}, nil
}

func (f *s3DiscoveryCoreFake) UploadPartCopy(_ context.Context, _ *awss3.UploadPartCopyInput, options ...func(*awss3.Options)) (*awss3.UploadPartCopyOutput, error) {
	f.record("UploadPartCopy", options)
	return &awss3.UploadPartCopyOutput{CopyPartResult: &awstypes.CopyPartResult{ETag: aws.String(`"part"`)}}, nil
}

func (f *s3DiscoveryCoreFake) CompleteMultipartUpload(_ context.Context, _ *awss3.CompleteMultipartUploadInput, options ...func(*awss3.Options)) (*awss3.CompleteMultipartUploadOutput, error) {
	f.record("CompleteMultipartUpload", options)
	return &awss3.CompleteMultipartUploadOutput{}, nil
}

func (f *s3DiscoveryCoreFake) AbortMultipartUpload(_ context.Context, _ *awss3.AbortMultipartUploadInput, options ...func(*awss3.Options)) (*awss3.AbortMultipartUploadOutput, error) {
	f.record("AbortMultipartUpload", options)
	return &awss3.AbortMultipartUploadOutput{}, nil
}

func newS3DiscoveryCoreBackend(fake *s3DiscoveryCoreFake) *s3Backend {
	return &s3Backend{
		client: fake, region: "us-east-1", auth: "static", shareHTTPS: true,
		bucketRegions: make(map[string]string),
	}
}

func s3DiscoveryCoreLocation(kind, bucket, region, key string) string {
	return encodeS3DiscoveredLocation(kind, bucket, region, key)
}

func TestS3DiscoveryListBucketsPaginationAndLocationRegion(t *testing.T) {
	fake := &s3DiscoveryCoreFake{listBuckets: []*awss3.ListBucketsOutput{
		{Buckets: []awstypes.Bucket{{Name: aws.String("alpha"), BucketRegion: aws.String("eu-west-1")}}, ContinuationToken: aws.String("next")},
		{Buckets: []awstypes.Bucket{{Name: aws.String("beta")}}},
	}}
	backend := newS3DiscoveryCoreBackend(fake)
	var entries []RemoteEntry
	if err := backend.ReadDir(context.Background(), backend.Root(), func(chunk []RemoteEntry) { entries = append(entries, chunk...) }); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "alpha" || entries[1].Name != "beta" {
		t.Fatalf("discovered entries = %#v", entries)
	}
	alpha, err := backend.targetFor(entries[0].Location)
	if err != nil || alpha.bucket != "alpha" || alpha.region != "eu-west-1" || !alpha.bucketRoot {
		t.Fatalf("alpha target = %#v, err=%v", alpha, err)
	}
	beta, err := backend.targetFor(entries[1].Location)
	if err != nil || beta.region != "" {
		t.Fatalf("beta target = %#v, err=%v", beta, err)
	}
	if got := fake.lastRegion("ListBuckets"); got != "us-east-1" {
		t.Fatalf("ListBuckets region = %q", got)
	}
}

func TestS3DiscoveryListBucketsRejectsNilAndRepeatedContinuation(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		backend := newS3DiscoveryCoreBackend(&s3DiscoveryCoreFake{})
		if err := backend.ReadDir(context.Background(), backend.Root(), nil); err == nil || !strings.Contains(err.Error(), "no response") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("repeated token", func(t *testing.T) {
		backend := newS3DiscoveryCoreBackend(&s3DiscoveryCoreFake{listBuckets: []*awss3.ListBucketsOutput{
			{ContinuationToken: aws.String("same")}, {ContinuationToken: aws.String("same")},
		}})
		if err := backend.ReadDir(context.Background(), backend.Root(), nil); err == nil || !strings.Contains(err.Error(), "did not advance") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestS3DiscoveryHistoryVisualPathResolvesRegionBeforeBucketRequest(t *testing.T) {
	fake := &s3DiscoveryCoreFake{listBuckets: []*awss3.ListBucketsOutput{{
		Buckets: []awstypes.Bucket{{Name: aws.String("history-bucket"), BucketRegion: aws.String("eu-west-3")}},
	}}}
	backend := newS3DiscoveryCoreBackend(fake)

	// This is the history/bookmark path: no root ReadDir and therefore no
	// provider alias or bucket-region cache has been populated yet.
	bucket := backend.Join(backend.Root(), "history-bucket")
	folder := backend.Join(bucket, "visited/folder")
	preRequest, err := backend.targetFor(folder)
	if err != nil {
		t.Fatal(err)
	}
	if preRequest.region != "" {
		t.Fatalf("visual Join baked fallback region %q into location", preRequest.region)
	}
	if err := backend.ReadDir(context.Background(), folder, nil); err != nil {
		t.Fatal(err)
	}
	if got := fake.lastRegion("ListObjectsV2"); got != "eu-west-3" {
		t.Fatalf("first restored-folder ListObjectsV2 region = %q", got)
	}
	if fake.lastRegion("ListBuckets") != "us-east-1" {
		t.Fatal("restored visual path did not resolve its bucket before access")
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if len(fake.listInputs) != 1 || aws.ToString(fake.listInputs[0].Bucket) != "history-bucket" || aws.ToString(fake.listInputs[0].Prefix) != "visited/folder/" {
		t.Fatalf("restored-folder request = %#v", fake.listInputs)
	}
}

func TestS3DiscoveryHistoryBucketRootResolvesRegionBeforeFirstRequest(t *testing.T) {
	fake := &s3DiscoveryCoreFake{listBuckets: []*awss3.ListBucketsOutput{{
		Buckets: []awstypes.Bucket{{Name: aws.String("history-bucket"), BucketRegion: aws.String("eu-west-3")}},
	}}}
	backend := newS3DiscoveryCoreBackend(fake)
	bucket := backend.Join(backend.Root(), "history-bucket")
	if err := backend.ReadDir(context.Background(), bucket, nil); err != nil {
		t.Fatal(err)
	}
	if got := fake.lastRegion("ListObjectsV2"); got != "eu-west-3" {
		t.Fatalf("first restored bucket-root ListObjectsV2 region = %q", got)
	}
}

func TestS3DiscoveryHistoryMissingListRegionUsesHeadBucket(t *testing.T) {
	var listAuthorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/":
			writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListAllMyBucketsResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Buckets><Bucket><Name>history-bucket</Name></Bucket></Buckets>
</ListAllMyBucketsResult>`)
		case request.Method == http.MethodHead && request.URL.Path == "/history-bucket":
			writer.Header().Set("X-Amz-Bucket-Region", "eu-north-1")
			writer.WriteHeader(http.StatusMovedPermanently)
		case request.Method == http.MethodGet && request.URL.Path == "/history-bucket" && request.URL.Query().Get("list-type") == "2":
			listAuthorization = request.Header.Get("Authorization")
			writeS3DiscoveryXML(writer, `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>history-bucket</Name><Prefix></Prefix><Delimiter>/</Delimiter><MaxKeys>1000</MaxKeys><IsTruncated>false</IsTruncated>
</ListBucketResult>`)
		default:
			http.Error(writer, request.Method+" "+request.URL.RequestURI(), http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := awss3.NewFromConfig(aws.Config{
		Region: "us-east-1", HTTPClient: server.Client(),
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "secret", ""),
	}, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(server.URL)
		options.UsePathStyle = true
	})
	backend := newS3Backend(client, S3Settings{
		Region: "us-east-1", Auth: "static", Endpoint: server.URL, UsePathStyle: true,
	})
	bucket := backend.Join(backend.Root(), "history-bucket")
	if err := backend.ReadDir(context.Background(), bucket, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listAuthorization, "/eu-north-1/s3/aws4_request") {
		t.Fatalf("restored bucket request was signed for the wrong region: %q", listAuthorization)
	}
}

func TestS3DiscoveryRoutesCRUDCopyAndRangeByLocationRegion(t *testing.T) {
	fake := &s3DiscoveryCoreFake{}
	backend := newS3DiscoveryCoreBackend(fake)
	dir := s3DiscoveryCoreLocation(s3LocationDiscoveredDir, "alpha", "eu-west-1", "folder/")
	file := s3DiscoveryCoreLocation(s3LocationDiscoveredFile, "alpha", "eu-west-1", "source.txt")
	destination := s3DiscoveryCoreLocation(s3LocationDiscoveredAuto, "beta", "ap-southeast-2", "destination.txt")

	if err := backend.ReadDir(context.Background(), dir, nil); err != nil {
		t.Fatal(err)
	}
	if err := backend.MkDir(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if err := backend.Copy(context.Background(), file, destination); err != nil {
		t.Fatal(err)
	}
	if err := backend.Remove(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	reader, err := backend.Open(context.Background(), file)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	buffer := make([]byte, 4)
	if n, err := reader.ReadAt(context.Background(), buffer, 0); n != 4 || err != nil || !bytes.Equal(buffer, []byte("data")) {
		t.Fatalf("ReadAt = %d, %v, %q", n, err, buffer)
	}

	for _, operation := range []string{"ListObjectsV2", "PutObject", "HeadObject", "DeleteObject", "GetObject"} {
		if got := fake.lastRegion(operation); got != "eu-west-1" {
			t.Errorf("%s region = %q", operation, got)
		}
	}
	if got := fake.lastRegion("CopyObject"); got != "ap-southeast-2" {
		t.Errorf("CopyObject destination region = %q", got)
	}
}

func TestS3DiscoveryUploadAndShareSignForSelectedRegion(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		authorization = request.Header.Get("Authorization")
		writer.Header().Set("ETag", `"uploaded"`)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	client := awss3.NewFromConfig(aws.Config{
		Region: "us-east-1", HTTPClient: server.Client(),
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "secret", ""),
	}, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String(server.URL)
		options.UsePathStyle = true
	})
	backend := newS3Backend(client, S3Settings{Region: "us-east-1", Auth: "static", Endpoint: server.URL})
	upload := s3DiscoveryCoreLocation(s3LocationDiscoveredAuto, "alpha", "eu-west-1", "upload.txt")
	writer, err := backend.Create(context.Background(), upload)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("content")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(authorization, "/eu-west-1/s3/aws4_request") {
		t.Fatalf("upload authorization did not use selected region: %q", authorization)
	}

	fake := &s3DiscoveryCoreFake{}
	presignClient := awss3.NewFromConfig(aws.Config{
		Region: "us-east-1", Credentials: credentials.NewStaticCredentialsProvider("AKID", "secret", ""),
	}, func(options *awss3.Options) { options.BaseEndpoint = aws.String("https://objects.example.test") })
	shareBackend := newS3Backend(presignClient, S3Settings{Region: "us-east-1", Auth: "static", Endpoint: "https://objects.example.test"})
	shareBackend.client = fake
	file := s3DiscoveryCoreLocation(s3LocationDiscoveredFile, "alpha", "eu-west-1", "share.txt")
	link, err := shareBackend.CreateShareLink(context.Background(), file, vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer, ExpiresIn: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	target, err := url.Parse(link.URL)
	if err != nil {
		t.Fatal(err)
	}
	if credential := target.Query().Get("X-Amz-Credential"); !strings.Contains(credential, "/eu-west-1/s3/aws4_request") {
		t.Fatalf("presign credential = %q", credential)
	}
	if got := fake.lastRegion("HeadObject"); got != "eu-west-1" {
		t.Fatalf("share HeadObject region = %q", got)
	}
}

func TestS3TestConnectionUsesBoundedModeSpecificProbe(t *testing.T) {
	t.Run("discovery", func(t *testing.T) {
		fake := &s3DiscoveryCoreFake{listBuckets: []*awss3.ListBucketsOutput{{}}}
		backend := newS3DiscoveryCoreBackend(fake)
		if err := backend.TestConnection(context.Background()); err != nil {
			t.Fatal(err)
		}
		if fake.lastRegion("ListBuckets") != "us-east-1" {
			t.Fatal("missing discovery probe")
		}
		if fake.lastRegion("ListObjectsV2") != "" {
			t.Fatal("discovery probe listed objects")
		}
	})
	t.Run("explicit bucket", func(t *testing.T) {
		fake := &s3DiscoveryCoreFake{}
		backend := &s3Backend{client: fake, bucket: "manual", rootPrefix: "root", region: "eu-central-1"}
		if err := backend.TestConnection(context.Background()); err != nil {
			t.Fatal(err)
		}
		if fake.lastRegion("ListObjectsV2") != "eu-central-1" {
			t.Fatal("missing explicit-bucket probe")
		}
		if fake.lastRegion("ListBuckets") != "" {
			t.Fatal("explicit probe listed buckets")
		}
	})
	t.Run("discovery error remains actionable", func(t *testing.T) {
		backend := newS3DiscoveryCoreBackend(&s3DiscoveryCoreFake{listErr: errors.New("denied")})
		if err := backend.TestConnection(context.Background()); err == nil {
			t.Fatal("expected probe failure")
		}
	})
}

var _ s3API = (*s3DiscoveryCoreFake)(nil)
var _ s3BucketLister = (*s3DiscoveryCoreFake)(nil)
