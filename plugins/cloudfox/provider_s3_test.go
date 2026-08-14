package cloudfox

import (
	"bytes"
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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awstypes "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/unxed/f4/vfs"
)

type s3CopyFake struct {
	mu sync.Mutex

	headOutput *awss3.HeadObjectOutput
	headErr    error
	headCalls  int

	copyInput   *awss3.CopyObjectInput
	copyErr     error
	putInput    *awss3.PutObjectInput
	putErr      error
	deleteInput *awss3.DeleteObjectInput
	deleteErr   error

	createInput *awss3.CreateMultipartUploadInput
	createErr   error
	uploadID    string

	partInputs []*awss3.UploadPartCopyInput
	partHook   func(context.Context, *awss3.UploadPartCopyInput) (*awss3.UploadPartCopyOutput, error)

	completeInput *awss3.CompleteMultipartUploadInput
	completeErr   error
	abortInput    *awss3.AbortMultipartUploadInput
	abortCtxErr   error
	abortErr      error
}

func (f *s3CopyFake) HeadObject(_ context.Context, _ *awss3.HeadObjectInput, _ ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.headCalls++
	return f.headOutput, f.headErr
}
func (f *s3CopyFake) CopyObject(_ context.Context, input *awss3.CopyObjectInput, _ ...func(*awss3.Options)) (*awss3.CopyObjectOutput, error) {
	f.mu.Lock()
	f.copyInput = input
	f.mu.Unlock()
	return &awss3.CopyObjectOutput{}, f.copyErr
}
func (f *s3CopyFake) CreateMultipartUpload(_ context.Context, input *awss3.CreateMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.CreateMultipartUploadOutput, error) {
	f.mu.Lock()
	f.createInput = input
	f.mu.Unlock()
	if f.createErr != nil {
		return nil, f.createErr
	}
	return &awss3.CreateMultipartUploadOutput{UploadId: aws.String(f.uploadID)}, nil
}
func (f *s3CopyFake) UploadPartCopy(ctx context.Context, input *awss3.UploadPartCopyInput, _ ...func(*awss3.Options)) (*awss3.UploadPartCopyOutput, error) {
	f.mu.Lock()
	f.partInputs = append(f.partInputs, input)
	hook := f.partHook
	f.mu.Unlock()
	if hook != nil {
		return hook(ctx, input)
	}
	etag := fmt.Sprintf("etag-%d", aws.ToInt32(input.PartNumber))
	return &awss3.UploadPartCopyOutput{CopyPartResult: &awstypes.CopyPartResult{ETag: aws.String(etag)}}, nil
}
func (f *s3CopyFake) CompleteMultipartUpload(_ context.Context, input *awss3.CompleteMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.CompleteMultipartUploadOutput, error) {
	f.mu.Lock()
	f.completeInput = input
	f.mu.Unlock()
	return &awss3.CompleteMultipartUploadOutput{}, f.completeErr
}
func (f *s3CopyFake) AbortMultipartUpload(ctx context.Context, input *awss3.AbortMultipartUploadInput, _ ...func(*awss3.Options)) (*awss3.AbortMultipartUploadOutput, error) {
	f.mu.Lock()
	f.abortInput = input
	f.abortCtxErr = ctx.Err()
	f.mu.Unlock()
	return &awss3.AbortMultipartUploadOutput{}, f.abortErr
}

func (*s3CopyFake) ListObjectsV2(context.Context, *awss3.ListObjectsV2Input, ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	panic("unexpected ListObjectsV2")
}
func (*s3CopyFake) GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	panic("unexpected GetObject")
}
func (f *s3CopyFake) PutObject(_ context.Context, input *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	f.mu.Lock()
	f.putInput = input
	f.mu.Unlock()
	return &awss3.PutObjectOutput{}, f.putErr
}
func (f *s3CopyFake) DeleteObject(_ context.Context, input *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	f.mu.Lock()
	f.deleteInput = input
	f.mu.Unlock()
	return &awss3.DeleteObjectOutput{}, f.deleteErr
}
func (*s3CopyFake) DeleteObjects(context.Context, *awss3.DeleteObjectsInput, ...func(*awss3.Options)) (*awss3.DeleteObjectsOutput, error) {
	panic("unexpected DeleteObjects")
}

func newS3CopyTestBackend(fake s3API) *s3Backend {
	return &s3Backend{client: fake, bucket: "my bucket", rootPrefix: "root"}
}

func TestS3CopyUsesSingleRequestAtFiveGiB(t *testing.T) {
	t.Parallel()
	fake := &s3CopyFake{headOutput: &awss3.HeadObjectOutput{ContentLength: aws.Int64(s3MaxSingleCopySize), ETag: aws.String(`"source-etag"`)}}
	backend := newS3CopyTestBackend(fake)
	if err := backend.Copy(context.Background(), "f:/source/a b+%ф.txt", "a:/destination/copied.txt"); err != nil {
		t.Fatal(err)
	}
	if fake.copyInput == nil {
		t.Fatal("CopyObject was not called")
	}
	if fake.createInput != nil || len(fake.partInputs) != 0 || fake.completeInput != nil {
		t.Fatal("small copy unexpectedly used multipart operations")
	}
	wantSource := url.PathEscape("my bucket/root/source/a b+%ф.txt")
	if got := aws.ToString(fake.copyInput.CopySource); got != wantSource {
		t.Fatalf("CopySource = %q, want %q", got, wantSource)
	}
	if got := aws.ToString(fake.copyInput.CopySourceIfMatch); got != `"source-etag"` {
		t.Fatalf("CopySourceIfMatch = %q", got)
	}
	if fake.copyInput.IfNoneMatch != nil {
		t.Fatalf("unspecified overwrite intent unexpectedly set IfNoneMatch=%q", aws.ToString(fake.copyInput.IfNoneMatch))
	}
	if strings.Contains(aws.ToString(fake.copyInput.CopySource), " ") || !strings.Contains(aws.ToString(fake.copyInput.CopySource), "%2F") || !strings.Contains(aws.ToString(fake.copyInput.CopySource), "%25") {
		t.Fatalf("CopySource is not safely escaped: %q", aws.ToString(fake.copyInput.CopySource))
	}
}

func TestS3DestinationOverwriteIntentIsAtomic(t *testing.T) {
	t.Parallel()

	newFake := func() *s3CopyFake {
		return &s3CopyFake{headOutput: &awss3.HeadObjectOutput{
			ContentLength: aws.Int64(10), ETag: aws.String(`"source-etag"`),
		}}
	}

	noReplace := vfs.WithDestinationOverwrite(context.Background(), false)
	fake := newFake()
	backend := newS3CopyTestBackend(fake)
	if err := backend.Copy(noReplace, "f:/source.txt", "a:/copy.txt"); err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(fake.copyInput.IfNoneMatch); got != "*" {
		t.Fatalf("CopyObject IfNoneMatch = %q, want *", got)
	}

	fake = newFake()
	backend = newS3CopyTestBackend(fake)
	if err := backend.Rename(noReplace, "f:/source.txt", "a:/moved.txt"); err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(fake.copyInput.IfNoneMatch); got != "*" {
		t.Fatalf("Rename CopyObject IfNoneMatch = %q, want *", got)
	}
	if fake.deleteInput == nil {
		t.Fatal("successful conditional rename did not clean up its source")
	}

	fake = newFake()
	backend = newS3CopyTestBackend(fake)
	if err := backend.Copy(vfs.WithDestinationOverwrite(context.Background(), true), "f:/source.txt", "a:/copy.txt"); err != nil {
		t.Fatal(err)
	}
	if fake.copyInput.IfNoneMatch != nil {
		t.Fatalf("replace CopyObject unexpectedly set IfNoneMatch=%q", aws.ToString(fake.copyInput.IfNoneMatch))
	}

	fake = newFake()
	backend = newS3CopyTestBackend(fake)
	if err := backend.MkDir(noReplace, "a:/directory"); err != nil {
		t.Fatal(err)
	}
	if got := aws.ToString(fake.putInput.IfNoneMatch); got != "*" {
		t.Fatalf("MkDir PutObject IfNoneMatch = %q, want *", got)
	}
	fake.putInput = nil
	if err := backend.MkDir(context.Background(), "a:/plain-context-directory"); err != nil {
		t.Fatal(err)
	}
	if fake.putInput == nil || aws.ToString(fake.putInput.IfNoneMatch) != "*" {
		t.Fatalf("plain-context MkDir PutObject = %#v, want IfNoneMatch=*", fake.putInput)
	}
	fake.putInput = nil
	if err := backend.MkDir(vfs.WithDestinationOverwrite(context.Background(), true), "a:/directory"); err != nil {
		t.Fatal(err)
	}
	if fake.putInput == nil || aws.ToString(fake.putInput.IfNoneMatch) != "*" {
		t.Fatalf("overwrite-context MkDir PutObject = %#v, want create-only IfNoneMatch=*", fake.putInput)
	}
}

func TestS3NoOverwriteConflictIsDefinitive(t *testing.T) {
	t.Parallel()
	ctx := vfs.WithDestinationOverwrite(context.Background(), false)
	fake := &s3CopyFake{
		headOutput: &awss3.HeadObjectOutput{ContentLength: aws.Int64(10), ETag: aws.String(`"source-etag"`)},
		copyErr:    s3TestResponseError(http.StatusPreconditionFailed),
	}
	err := newS3CopyTestBackend(fake).Copy(ctx, "f:/source.txt", "a:/occupied.txt")
	if !errors.Is(err, os.ErrExist) || errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("conditional Copy error = %v, want definitive os.ErrExist", err)
	}
}

func s3TestResponseError(status int) error {
	return &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
		Err:      fmt.Errorf("S3 returned HTTP %d", status),
	}
}

func TestS3CopyPinsSourceVersionAndClassifiesMismatch(t *testing.T) {
	versionID := "version +/7"
	fake := &s3CopyFake{
		headOutput: &awss3.HeadObjectOutput{
			ContentLength: aws.Int64(10), ETag: aws.String(`"generation-one"`), VersionId: aws.String(versionID),
		},
		copyErr: s3TestResponseError(http.StatusPreconditionFailed),
	}
	err := newS3CopyTestBackend(fake).Copy(context.Background(), "f:/source.txt", "a:/copy.txt")
	if !errors.Is(err, ErrRemoteObjectChanged) {
		t.Fatalf("Copy error = %v, want ErrRemoteObjectChanged", err)
	}
	if fake.headCalls != 1 {
		t.Fatalf("HeadObject calls = %d, want one generation pin", fake.headCalls)
	}
	wantSource := s3CopySource("my bucket", "root/source.txt", versionID)
	if got := aws.ToString(fake.copyInput.CopySource); got != wantSource {
		t.Fatalf("versioned CopySource = %q, want %q", got, wantSource)
	}
	if got := aws.ToString(fake.copyInput.CopySourceIfMatch); got != `"generation-one"` {
		t.Fatalf("CopySourceIfMatch = %q", got)
	}
}

func TestS3RenameConditionsCleanupOnCopiedGeneration(t *testing.T) {
	versionID := "version-1"
	fake := &s3CopyFake{
		headOutput: &awss3.HeadObjectOutput{
			ContentLength: aws.Int64(10), ETag: aws.String(`"generation-one"`), VersionId: aws.String(versionID),
		},
		deleteErr: s3TestResponseError(http.StatusPreconditionFailed),
	}
	err := newS3CopyTestBackend(fake).Rename(context.Background(), "f:/source.txt", "a:/renamed.txt")
	var partial *vfs.PartialOperationError
	if !errors.As(err, &partial) || !errors.Is(err, vfs.ErrOperationPartial) || !errors.Is(err, ErrRemoteObjectChanged) {
		t.Fatalf("Rename error = %v, want partial remote-object-changed", err)
	}
	if errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("definitive precondition mismatch was classified as unknown: %v", err)
	}
	if fake.deleteInput == nil {
		t.Fatal("DeleteObject was not called")
	}
	if got := aws.ToString(fake.deleteInput.Key); got != "root/source.txt" {
		t.Fatalf("DeleteObject key = %q", got)
	}
	if got := aws.ToString(fake.deleteInput.IfMatch); got != `"generation-one"` {
		t.Fatalf("DeleteObject IfMatch = %q", got)
	}
	if fake.deleteInput.VersionId != nil {
		t.Fatalf("DeleteObject must be unqualified so versioning creates a delete marker, got VersionId %q", aws.ToString(fake.deleteInput.VersionId))
	}
}

func TestS3RenameCleanupAmbiguityIsPartialUnknownState(t *testing.T) {
	fake := &s3CopyFake{
		headOutput: &awss3.HeadObjectOutput{ContentLength: aws.Int64(10), ETag: aws.String(`"generation-one"`)},
		deleteErr:  errors.New("connection reset after request"),
	}
	err := newS3CopyTestBackend(fake).Rename(context.Background(), "f:/source.txt", "a:/renamed.txt")
	if !errors.Is(err, vfs.ErrOperationPartial) || !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("Rename error = %v, want partial unknown state", err)
	}
	if got := aws.ToString(fake.deleteInput.IfMatch); got != `"generation-one"` {
		t.Fatalf("DeleteObject IfMatch = %q", got)
	}
}

func TestS3RenameWithoutETagRefusesBeforeCopy(t *testing.T) {
	fake := &s3CopyFake{
		headOutput: &awss3.HeadObjectOutput{ContentLength: aws.Int64(10), VersionId: aws.String("version-only")},
	}
	err := newS3CopyTestBackend(fake).Rename(context.Background(), "f:/source.txt", "a:/renamed.txt")
	if err == nil || !strings.Contains(err.Error(), "ETag") {
		t.Fatalf("Rename error = %v, want missing ETag refusal", err)
	}
	if fake.copyInput != nil || fake.deleteInput != nil {
		t.Fatalf("unsafe rename performed copy/delete: copy=%#v delete=%#v", fake.copyInput, fake.deleteInput)
	}
}

func TestS3CopyUsesMultipartAboveFiveGiB(t *testing.T) {
	t.Parallel()
	size := s3MaxSingleCopySize + 1
	etag := "source-etag"
	fake := &s3CopyFake{
		headOutput: &awss3.HeadObjectOutput{
			ContentLength: aws.Int64(size), ETag: aws.String(etag), ContentType: aws.String("application/test"),
			CacheControl: aws.String("max-age=60"), Metadata: map[string]string{"custom": "value"},
		},
		uploadID: "upload-123",
	}
	backend := newS3CopyTestBackend(fake)
	if err := backend.Copy(vfs.WithDestinationOverwrite(context.Background(), false), "f:/source.bin", "a:/copy.bin"); err != nil {
		t.Fatal(err)
	}
	if fake.copyInput != nil {
		t.Fatal("large copy unexpectedly called CopyObject")
	}
	if fake.headCalls != 1 {
		t.Fatalf("HeadObject calls = %d, want one pinned metadata read", fake.headCalls)
	}
	if fake.createInput == nil || aws.ToString(fake.createInput.ContentType) != "application/test" || fake.createInput.Metadata["custom"] != "value" {
		t.Fatalf("CreateMultipartUpload did not preserve source metadata: %#v", fake.createInput)
	}
	partSize, err := s3MultipartCopyPartSize(size)
	if err != nil {
		t.Fatal(err)
	}
	wantParts := int((size + partSize - 1) / partSize)
	if len(fake.partInputs) != wantParts {
		t.Fatalf("UploadPartCopy calls = %d, want %d", len(fake.partInputs), wantParts)
	}
	var nextStart int64
	for i, input := range fake.partInputs {
		if aws.ToInt32(input.PartNumber) != int32(i+1) {
			t.Fatalf("part %d number = %d", i, aws.ToInt32(input.PartNumber))
		}
		if aws.ToString(input.CopySourceIfMatch) != etag {
			t.Fatalf("part %d missing source ETag condition", i+1)
		}
		start, end, err := parseCopyRange(aws.ToString(input.CopySourceRange))
		if err != nil {
			t.Fatal(err)
		}
		if start != nextStart || end < start || end-start+1 > s3MaxMultipartPartSize {
			t.Fatalf("part %d invalid range %d-%d after %d", i+1, start, end, nextStart)
		}
		nextStart = end + 1
	}
	if nextStart != size {
		t.Fatalf("multipart ranges cover %d bytes, want %d", nextStart, size)
	}
	if fake.completeInput == nil || fake.completeInput.MultipartUpload == nil || len(fake.completeInput.MultipartUpload.Parts) != wantParts {
		t.Fatalf("CompleteMultipartUpload = %#v", fake.completeInput)
	}
	if got := aws.ToString(fake.completeInput.IfNoneMatch); got != "*" {
		t.Fatalf("CompleteMultipartUpload IfNoneMatch = %q, want *", got)
	}
	for i, part := range fake.completeInput.MultipartUpload.Parts {
		if aws.ToInt32(part.PartNumber) != int32(i+1) || aws.ToString(part.ETag) != fmt.Sprintf("etag-%d", i+1) {
			t.Fatalf("completed part %d = %#v", i, part)
		}
	}
	if fake.abortInput != nil {
		t.Fatal("successful multipart copy was aborted")
	}
}

func TestS3MultipartCopyCancellationAbortsWithIndependentContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	fake := &s3CopyFake{
		headOutput: &awss3.HeadObjectOutput{ContentLength: aws.Int64(s3MaxSingleCopySize + 1), ETag: aws.String(`"source-etag"`)},
		uploadID:   "cancel-upload",
	}
	fake.partHook = func(ctx context.Context, _ *awss3.UploadPartCopyInput) (*awss3.UploadPartCopyOutput, error) {
		cancel()
		return nil, ctx.Err()
	}
	err := newS3CopyTestBackend(fake).Copy(ctx, "f:/source", "a:/destination")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Copy error = %v, want context.Canceled", err)
	}
	if fake.abortInput == nil || aws.ToString(fake.abortInput.UploadId) != "cancel-upload" {
		t.Fatalf("AbortMultipartUpload = %#v", fake.abortInput)
	}
	if fake.abortCtxErr != nil {
		t.Fatalf("abort inherited canceled context: %v", fake.abortCtxErr)
	}
	if fake.completeInput != nil {
		t.Fatal("canceled multipart copy was completed")
	}
}

func TestS3MultipartCopyCompletionFailureAborts(t *testing.T) {
	t.Parallel()
	completeErr := errors.New("complete failed")
	fake := &s3CopyFake{
		headOutput:  &awss3.HeadObjectOutput{ContentLength: aws.Int64(s3MaxSingleCopySize + 1), ETag: aws.String(`"source-etag"`)},
		uploadID:    "failed-complete",
		completeErr: completeErr,
	}
	err := newS3CopyTestBackend(fake).Copy(context.Background(), "f:/source", "a:/destination")
	if !errors.Is(err, completeErr) {
		t.Fatalf("Copy error = %v", err)
	}
	if fake.abortInput == nil {
		t.Fatal("failed completion did not abort multipart upload")
	}
}

func TestS3MultipartPartSizingStaysWithinServiceLimits(t *testing.T) {
	t.Parallel()
	const maxS3ObjectSize = int64(5 << 40)
	partSize, err := s3MultipartCopyPartSize(maxS3ObjectSize)
	if err != nil {
		t.Fatal(err)
	}
	parts := (maxS3ObjectSize + partSize - 1) / partSize
	if partSize < s3MinMultipartPartSize || partSize > s3MaxMultipartPartSize || parts > s3MaxMultipartParts {
		t.Fatalf("part size %d yields %d parts", partSize, parts)
	}
	if _, err := s3MultipartCopyPartSize(int64(51 << 40)); err == nil {
		t.Fatal("oversized object unexpectedly received a multipart part size")
	}
}

func parseCopyRange(value string) (int64, int64, error) {
	if !strings.HasPrefix(value, "bytes=") {
		return 0, 0, fmt.Errorf("invalid copy range %q", value)
	}
	parts := strings.Split(strings.TrimPrefix(value, "bytes="), "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid copy range %q", value)
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	return start, end, err
}

var _ s3API = (*s3CopyFake)(nil)

func TestS3HTTPSEndpointPolicy(t *testing.T) {
	factory := &S3Factory{}
	connection := Connection{Name: "S3", Provider: ProviderS3}
	settings := S3Settings{Bucket: "bucket", Endpoint: "http://storage.example", Auth: "static"}
	connection.Settings, _ = json.Marshal(settings)
	if err := factory.Validate(connection); err == nil {
		t.Fatal("credentialed HTTP endpoint was accepted without explicit confirmation")
	}
	settings.AllowInsecure = true
	connection.Settings, _ = json.Marshal(settings)
	if err := factory.Validate(connection); err != nil {
		t.Fatalf("explicit insecure endpoint was rejected: %v", err)
	}
	settings.Auth = "anonymous"
	settings.AllowInsecure = false
	connection.Settings, _ = json.Marshal(settings)
	if err := factory.Validate(connection); err != nil {
		t.Fatalf("anonymous HTTP endpoint was rejected: %v", err)
	}
}

func TestS3HTTPClientNeverFollowsRedirects(t *testing.T) {
	for _, status := range []int{http.StatusMovedPermanently, http.StatusFound} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			targetHit := false
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				targetHit = true
				if token := r.Header.Get("X-Amz-Security-Token"); token != "" {
					t.Errorf("redirect target received session token %q", token)
				}
				if r.Method == http.MethodGet {
					t.Error("mutating request was rewritten to GET")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer target.Close()

			redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", target.URL)
				w.WriteHeader(status)
			}))
			defer redirector.Close()

			injected := &http.Client{}
			client := s3NoRedirectClient(injected)
			if client == injected || injected.CheckRedirect != nil {
				t.Fatal("redirect policy mutated the injected HTTP client")
			}
			req, err := http.NewRequest(http.MethodPut, redirector.URL, strings.NewReader("payload"))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-Amz-Security-Token", "top-secret-session-token")
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != status {
				t.Fatalf("status = %d, want redirect %d", resp.StatusCode, status)
			}
			if targetHit {
				t.Fatal("S3 client followed redirect")
			}
		})
	}
}

func openS3HTTPTestBackend(t *testing.T, server *httptest.Server) *s3Backend {
	t.Helper()
	settings := S3Settings{
		Bucket: "test-bucket", Region: "us-east-1", Endpoint: server.URL,
		UsePathStyle: true, Auth: "anonymous",
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := (&S3Factory{HTTPClient: server.Client()}).Open(context.Background(), Connection{
		Name: "test", Provider: ProviderS3, Settings: raw,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	backend, ok := opened.(*s3Backend)
	if !ok {
		t.Fatalf("opened backend = %T", opened)
	}
	t.Cleanup(func() { _ = backend.Close() })
	return backend
}

func TestS3CreateNoOverwriteSurvivesConcurrentRace(t *testing.T) {
	t.Parallel()

	var (
		mu          sync.Mutex
		stored      []byte
		conditions  []string
		putRequests int
	)
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/test-bucket/race.txt" {
			http.Error(w, "unexpected request", http.StatusNotFound)
			return
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read S3 body: %v", err)
		}
		mu.Lock()
		putRequests++
		conditions = append(conditions, r.Header.Get("If-None-Match"))
		mu.Unlock()
		arrived <- struct{}{}
		<-release

		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("If-None-Match") == "*" && stored != nil {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = io.WriteString(w, `<Error><Code>PreconditionFailed</Code><Message>occupied</Message></Error>`)
			return
		}
		stored = append([]byte(nil), payload...)
		w.Header().Set("ETag", `"new-etag"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	backend := openS3HTTPTestBackend(t, server)
	writers := make([]io.WriteCloser, 2)
	for i, payload := range []string{"first-writer", "second-writer"} {
		writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), false), "f:/race.txt")
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
	for i := 0; i < len(writers); i++ {
		select {
		case <-arrived:
		case <-time.After(10 * time.Second):
			close(release)
			t.Fatal("timed out waiting for concurrent S3 PUT requests")
		}
	}
	close(release)

	var success, conflict int
	for range writers {
		err := <-errorsByWriter
		switch {
		case err == nil:
			success++
		case errors.Is(err, os.ErrExist):
			if errors.Is(err, vfs.ErrOperationStateUnknown) {
				t.Fatalf("definitive conditional failure reported as unknown: %v", err)
			}
			conflict++
		default:
			t.Fatalf("Close error = %v, want success or os.ErrExist", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if putRequests != 2 || success != 1 || conflict != 1 {
		t.Fatalf("PUTs=%d successes=%d conflicts=%d", putRequests, success, conflict)
	}
	if !reflect.DeepEqual(conditions, []string{"*", "*"}) {
		t.Fatalf("If-None-Match headers = %v", conditions)
	}
	if got := string(stored); got != "first-writer" && got != "second-writer" {
		t.Fatalf("stored payload = %q", got)
	}
}

func TestS3CreateNoOverwriteFailsClosedWhenConditionIsRejected(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	requests := 0
	stored := "pre-existing"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		if got := r.Header.Get("If-None-Match"); got != "*" {
			t.Errorf("If-None-Match = %q, want *", got)
			mu.Lock()
			stored = "unsafe-overwrite"
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `<Error><Code>NotImplemented</Code><Message>conditional writes unsupported</Message></Error>`)
	}))
	defer server.Close()

	backend := openS3HTTPTestBackend(t, server)
	writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), false), "f:/occupied.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(writer, "replacement")
	if err := writer.Close(); err == nil {
		t.Fatal("conditional-write rejection unexpectedly fell back to an unsafe upload")
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Fatalf("requests = %d, want one conditional request and no fallback", requests)
	}
	if stored != "pre-existing" {
		t.Fatalf("stored object changed to %q", stored)
	}
}

func TestS3CreateOverwriteTrueOmitsNoReplaceCondition(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		condition string
		stored    []byte
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		condition = r.Header.Get("If-None-Match")
		stored, _ = io.ReadAll(r.Body)
		mu.Unlock()
		w.Header().Set("ETag", `"replacement"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	backend := openS3HTTPTestBackend(t, server)
	writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), true), "f:/replace.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(writer, "replacement")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if condition != "" {
		t.Fatalf("overwrite=true sent If-None-Match=%q", condition)
	}
	if string(stored) != "replacement" {
		t.Fatalf("stored payload = %q", stored)
	}
}

func TestS3MultipartCreateCarriesNoOverwriteToAtomicCompletion(t *testing.T) {
	t.Parallel()

	var (
		mu                sync.Mutex
		completeCondition string
		abortRequests     int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case r.Method == http.MethodPost && query.Has("uploads"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><Bucket>test-bucket</Bucket><Key>large.bin</Key><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && query.Get("uploadId") == "upload-1":
			_, _ = io.Copy(io.Discard, r.Body)
			part := query.Get("partNumber")
			w.Header().Set("ETag", `"part-`+part+`"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && query.Get("uploadId") == "upload-1":
			mu.Lock()
			completeCondition = r.Header.Get("If-None-Match")
			mu.Unlock()
			if r.Header.Get("If-None-Match") != "*" {
				w.Header().Set("Content-Type", "application/xml")
				_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><ETag>"overwrote-existing"</ETag></CompleteMultipartUploadResult>`)
				return
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusPreconditionFailed)
			_, _ = io.WriteString(w, `<Error><Code>PreconditionFailed</Code><Message>occupied</Message></Error>`)
		case r.Method == http.MethodDelete && query.Get("uploadId") == "upload-1":
			mu.Lock()
			abortRequests++
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected S3 multipart request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	backend := openS3HTTPTestBackend(t, server)
	writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), false), "f:/large.bin")
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{'x'}, (8<<20)+1)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	err = writer.Close()
	if !errors.Is(err, os.ErrExist) || errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("multipart Close error = %v, want definitive os.ErrExist", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if completeCondition != "*" {
		t.Fatalf("CompleteMultipartUpload If-None-Match = %q, want *", completeCondition)
	}
	if abortRequests != 1 {
		t.Fatalf("AbortMultipartUpload requests = %d, want 1", abortRequests)
	}
}

func TestS3MultipartWriterAbortUsesDetachedCleanupContext(t *testing.T) {
	t.Parallel()

	var (
		mu               sync.Mutex
		abortRequests    int
		completeRequests int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		switch {
		case r.Method == http.MethodPost && query.Has("uploads"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><Bucket>test-bucket</Bucket><Key>abort.bin</Key><UploadId>abort-upload-1</UploadId></InitiateMultipartUploadResult>`)
		case r.Method == http.MethodPut && query.Get("uploadId") == "abort-upload-1":
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("ETag", `"uploaded-part"`)
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && query.Get("uploadId") == "abort-upload-1":
			mu.Lock()
			completeRequests++
			mu.Unlock()
			w.WriteHeader(http.StatusInternalServerError)
		case r.Method == http.MethodDelete && query.Get("uploadId") == "abort-upload-1":
			mu.Lock()
			abortRequests++
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	backend := openS3HTTPTestBackend(t, server)
	writer, err := backend.Create(vfs.WithDestinationOverwrite(context.Background(), false), "f:/abort.bin")
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{'x'}, (8<<20)+1)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	aborter, ok := writer.(vfs.AbortableWriter)
	if !ok {
		t.Fatalf("S3 writer %T is not abortable", writer)
	}
	if err := aborter.Abort(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if abortRequests != 1 {
		t.Fatalf("detached AbortMultipartUpload requests = %d, want 1", abortRequests)
	}
	if completeRequests != 0 {
		t.Fatalf("aborted writer sent %d CompleteMultipartUpload request(s)", completeRequests)
	}
}

type s3IdentityFake struct {
	s3API
	keys               []string
	headKeys           []string
	getInputs          []*awss3.GetObjectInput
	deletedKeys        []string
	contentRange       string
	omitHeadValidators bool
	responseETag       string
	responseVersionID  string
}

func (f *s3IdentityFake) ListObjectsV2(_ context.Context, input *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	if got := aws.ToString(input.Prefix); got != "root/" {
		return nil, fmt.Errorf("ListObjectsV2 prefix = %q", got)
	}
	contents := make([]awstypes.Object, 0, len(f.keys))
	for _, key := range f.keys {
		contents = append(contents, awstypes.Object{Key: aws.String(key), Size: aws.Int64(int64(len(key)))})
	}
	return &awss3.ListObjectsV2Output{Contents: contents}, nil
}

func (f *s3IdentityFake) HeadObject(_ context.Context, input *awss3.HeadObjectInput, _ ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error) {
	key := aws.ToString(input.Key)
	f.headKeys = append(f.headKeys, key)
	output := &awss3.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(key))),
	}
	if !f.omitHeadValidators {
		output.ETag = aws.String(`"etag-for-listed-object"`)
		output.VersionId = aws.String("version-7")
	}
	return output, nil
}

func (f *s3IdentityFake) GetObject(_ context.Context, input *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	copyInput := *input
	f.getInputs = append(f.getInputs, &copyInput)
	key := aws.ToString(input.Key)
	if aws.ToString(input.Range) == "" {
		return &awss3.GetObjectOutput{
			Body:          io.NopCloser(bytes.NewReader([]byte(key))),
			ContentLength: aws.Int64(int64(len(key))),
		}, nil
	}
	var start, end int
	if _, err := fmt.Sscanf(aws.ToString(input.Range), "bytes=%d-%d", &start, &end); err != nil {
		return nil, err
	}
	if start < 0 || end < start || end >= len(key) {
		return nil, fmt.Errorf("invalid test range %d-%d for %q", start, end, key)
	}
	contentRange := fmt.Sprintf("bytes %d-%d/%d", start, end, len(key))
	if f.contentRange != "" {
		contentRange = f.contentRange
	}
	etag := f.responseETag
	if etag == "" {
		etag = `"etag-for-listed-object"`
	}
	versionID := f.responseVersionID
	if versionID == "" {
		versionID = "version-7"
	}
	return &awss3.GetObjectOutput{
		Body:         io.NopCloser(bytes.NewReader([]byte(key[start : end+1]))),
		ContentRange: aws.String(contentRange),
		ETag:         aws.String(etag),
		VersionId:    aws.String(versionID),
	}, nil
}

func (f *s3IdentityFake) DeleteObject(_ context.Context, input *awss3.DeleteObjectInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error) {
	f.deletedKeys = append(f.deletedKeys, aws.ToString(input.Key))
	return &awss3.DeleteObjectOutput{}, nil
}

func TestS3ListedOpaqueKeysRoundTripExactly(t *testing.T) {
	keys := []string{
		`root/back\slash.txt`,
		"root/dot/../segment.txt",
		"root/./literal.txt",
	}
	fake := &s3IdentityFake{keys: keys}
	backend := newS3CopyTestBackend(fake)

	var entries []RemoteEntry
	if err := backend.ReadDir(context.Background(), backend.Root(), func(chunk []RemoteEntry) {
		entries = append(entries, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	if len(entries) != len(keys) {
		t.Fatalf("ReadDir returned %d entries, want %d", len(entries), len(keys))
	}

	for i, entry := range entries {
		key := keys[i]
		if !strings.HasPrefix(entry.Location, s3LocationOpaqueFile) {
			t.Fatalf("location for %q = %q, want opaque file identity", key, entry.Location)
		}
		normalized, err := backend.Normalize(entry.Location)
		if err != nil || normalized != entry.Location {
			t.Fatalf("Normalize(%q) = %q, %v", entry.Location, normalized, err)
		}
		resolved, err := backend.keyFor(entry.Location, false)
		if err != nil || resolved != key {
			t.Fatalf("keyFor(%q) = %q, %v; want %q", entry.Location, resolved, err, key)
		}
		parent := backend.Dir(entry.Location)
		rejoined := backend.Join(parent, backend.Base(entry.Location))
		resolved, err = backend.keyFor(rejoined, false)
		if err != nil || resolved != key {
			t.Fatalf("Dir/Join round trip for %q = %q, %v", key, resolved, err)
		}

		if _, err := backend.Stat(context.Background(), entry.Location); err != nil {
			t.Fatalf("Stat(%q): %v", key, err)
		}
		reader, err := backend.Open(context.Background(), entry.Location)
		if err != nil {
			t.Fatalf("Open(%q): %v", key, err)
		}
		buf := make([]byte, 3)
		if n, err := reader.ReadAt(context.Background(), buf, 1); n != len(buf) || err != nil {
			t.Fatalf("ReadAt(%q) = %d, %v", key, n, err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		if err := backend.Remove(context.Background(), entry.Location); err != nil {
			t.Fatalf("Remove(%q): %v", key, err)
		}
	}

	if len(fake.getInputs) != len(keys) || len(fake.deletedKeys) != len(keys) {
		t.Fatalf("Get/Delete calls = %d/%d, want %d/%d", len(fake.getInputs), len(fake.deletedKeys), len(keys), len(keys))
	}
	for i, key := range keys {
		input := fake.getInputs[i]
		if got := aws.ToString(input.Key); got != key {
			t.Fatalf("GetObject key = %q, want %q", got, key)
		}
		if got := aws.ToString(input.IfMatch); got != `"etag-for-listed-object"` {
			t.Fatalf("GetObject IfMatch = %q", got)
		}
		if got := aws.ToString(input.VersionId); got != "version-7" {
			t.Fatalf("GetObject VersionId = %q", got)
		}
		if fake.deletedKeys[i] != key {
			t.Fatalf("DeleteObject key = %q, want %q", fake.deletedKeys[i], key)
		}
	}
	// Stat, Open and Remove each issue an exact HeadObject request.
	if len(fake.headKeys) != len(keys)*3 {
		t.Fatalf("HeadObject calls = %d, want %d", len(fake.headKeys), len(keys)*3)
	}
	for i, got := range fake.headKeys {
		if want := keys[i/3]; got != want {
			t.Fatalf("HeadObject key %d = %q, want %q", i, got, want)
		}
	}
}

func TestS3OpaqueLocationCannotEscapeRootAndLegacyNavigationStillWorks(t *testing.T) {
	backend := newS3CopyTestBackend(&s3IdentityFake{})
	legacy, err := backend.Normalize(`f:/one/../two\file.txt`)
	if err != nil {
		t.Fatal(err)
	}
	if key, err := backend.keyFor(legacy, false); err != nil || key != "root/two/file.txt" {
		t.Fatalf("legacy key = %q, %v", key, err)
	}
	escape := encodeS3OpaqueLocation(s3LocationOpaqueFile, "outside/secret")
	if _, err := backend.Normalize(escape); err == nil {
		t.Fatal("opaque key outside root prefix was accepted")
	}
}

func TestS3RangeReaderRejectsInvalidInputsAndContentRange(t *testing.T) {
	key := "root/file.txt"
	fake := &s3IdentityFake{keys: []string{key}, contentRange: "bytes 0-9/999"}
	backend := newS3CopyTestBackend(fake)
	location := backend.locationForKey(key, s3LocationFile)
	reader, err := backend.Open(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if n, err := reader.ReadAt(context.Background(), nil, 0); n != 0 || err != nil {
		t.Fatalf("zero-length ReadAt = %d, %v", n, err)
	}
	if n, err := reader.ReadAt(context.Background(), make([]byte, 1), -1); n != 0 || !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("negative ReadAt = %d, %v", n, err)
	}
	if n, err := reader.ReadAt(context.Background(), make([]byte, 2), 0); n != 0 || err == nil || !strings.Contains(err.Error(), "Content-Range") {
		t.Fatalf("mismatched Content-Range ReadAt = %d, %v", n, err)
	}
}

func TestS3SequentialReadReportsSourceDownloadThroughTaskReporter(t *testing.T) {
	key := "root/source-progress.bin"
	fake := &s3IdentityFake{keys: []string{key}}
	backend := newS3CopyTestBackend(fake)
	reader, err := backend.Open(context.Background(), backend.locationForKey(key, s3LocationFile))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	capture := &productionProgressCapture{}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, capture)
	buffer := make([]byte, len(key))
	n, err := reader.Read(ctx, buffer)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(key) || string(buffer) != key {
		t.Fatalf("Read = %d bytes %q", n, buffer)
	}
	if capture.action != "Downloading" || capture.name != "source-progress.bin" || capture.pct != 100 {
		t.Fatalf("ReporterKey progress = %#v", capture)
	}
}

func TestS3OpenWithoutValidatorUsesSingleSnapshotDownload(t *testing.T) {
	key := "root/no-validator.txt"
	fake := &s3IdentityFake{keys: []string{key}, omitHeadValidators: true}
	backend := newS3CopyTestBackend(fake)
	reader, err := backend.Open(context.Background(), backend.locationForKey(key, s3LocationFile))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	buf := make([]byte, 5)
	if n, err := reader.ReadAt(context.Background(), buf, 5); n != len(buf) || err != nil {
		t.Fatalf("snapshot ReadAt = %d, %v", n, err)
	}
	if got, want := string(buf), key[5:10]; got != want {
		t.Fatalf("snapshot bytes = %q, want %q", got, want)
	}
	if len(fake.getInputs) != 1 || aws.ToString(fake.getInputs[0].Range) != "" || fake.getInputs[0].IfMatch != nil || fake.getInputs[0].VersionId != nil {
		t.Fatalf("snapshot GetObject input = %#v", fake.getInputs)
	}
}

func TestS3RangeReaderRejectsChangedResponseGeneration(t *testing.T) {
	key := "root/file.txt"
	fake := &s3IdentityFake{keys: []string{key}, responseETag: `"replacement-etag"`}
	backend := newS3CopyTestBackend(fake)
	reader, err := backend.Open(context.Background(), backend.locationForKey(key, s3LocationFile))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if n, err := reader.ReadAt(context.Background(), make([]byte, 2), 0); n != 0 || !errors.Is(err, ErrRemoteObjectChanged) {
		t.Fatalf("changed-generation ReadAt = %d, %v", n, err)
	}
}

type s3DeleteFake struct {
	s3API
	listOutputs  []*awss3.ListObjectsV2Output
	listCalls    int
	deleteOutput []*awss3.DeleteObjectsOutput
	deleteErr    map[int]error
	deleteCalls  int
	prefixes     []string
}

func (f *s3DeleteFake) ListObjectsV2(_ context.Context, input *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	f.prefixes = append(f.prefixes, aws.ToString(input.Prefix))
	output := f.listOutputs[f.listCalls]
	f.listCalls++
	return output, nil
}

func (f *s3DeleteFake) DeleteObjects(_ context.Context, _ *awss3.DeleteObjectsInput, _ ...func(*awss3.Options)) (*awss3.DeleteObjectsOutput, error) {
	call := f.deleteCalls
	f.deleteCalls++
	if err := f.deleteErr[call]; err != nil {
		return nil, err
	}
	return f.deleteOutput[call], nil
}

func s3Objects(keys ...string) []awstypes.Object {
	objects := make([]awstypes.Object, 0, len(keys))
	for _, key := range keys {
		objects = append(objects, awstypes.Object{Key: aws.String(key)})
	}
	return objects
}

func TestS3DeleteDirectoryReportsDefinitivePartialResult(t *testing.T) {
	good, bad := "root/dot/../good", "root/dot/../bad"
	fake := &s3DeleteFake{
		listOutputs: []*awss3.ListObjectsV2Output{{Contents: s3Objects(good, bad)}},
		deleteOutput: []*awss3.DeleteObjectsOutput{{Errors: []awstypes.Error{{
			Key: aws.String(bad), Code: aws.String("AccessDenied"), Message: aws.String("denied"),
		}}}},
	}
	backend := newS3CopyTestBackend(fake)
	location := encodeS3OpaqueLocation(s3LocationOpaqueDir, "root/dot/../")
	err := backend.deleteDirectory(context.Background(), location)
	var partial *vfs.PartialOperationError
	if !errors.As(err, &partial) || !errors.Is(err, vfs.ErrOperationPartial) {
		t.Fatalf("deleteDirectory error = %v, want PartialOperationError", err)
	}
	if fmt.Sprint(partial.Completed) != fmt.Sprint([]string{good}) || fmt.Sprint(partial.Failed) != fmt.Sprint([]string{bad}) {
		t.Fatalf("partial result = completed %v, failed %v", partial.Completed, partial.Failed)
	}
	if len(fake.prefixes) != 1 || fake.prefixes[0] != "root/dot/../" {
		t.Fatalf("delete prefix = %v", fake.prefixes)
	}
}

func TestS3DeleteDirectoryReportsUnknownStateAfterProgress(t *testing.T) {
	first, uncertain := "root/dir/first", "root/dir/second"
	fake := &s3DeleteFake{
		listOutputs: []*awss3.ListObjectsV2Output{
			{Contents: s3Objects(first), IsTruncated: aws.Bool(true), NextContinuationToken: aws.String("next")},
			{Contents: s3Objects(uncertain)},
		},
		deleteOutput: []*awss3.DeleteObjectsOutput{{}, nil},
		deleteErr:    map[int]error{1: errors.New("connection reset")},
	}
	backend := newS3CopyTestBackend(fake)
	err := backend.deleteDirectory(context.Background(), "d:/dir")
	var partial *vfs.PartialOperationError
	if !errors.As(err, &partial) || !errors.Is(err, vfs.ErrOperationPartial) || !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("deleteDirectory error = %v, want partial unknown state", err)
	}
	if fmt.Sprint(partial.Completed) != fmt.Sprint([]string{first}) || fmt.Sprint(partial.Failed) != fmt.Sprint([]string{uncertain}) {
		t.Fatalf("partial result = completed %v, failed %v", partial.Completed, partial.Failed)
	}
}
