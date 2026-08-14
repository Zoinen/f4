package cloudfox

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/unxed/f4/vfs"
)

func newS3ShareTestBackend(t *testing.T, auth string) (*s3Backend, *s3CopyFake) {
	t.Helper()
	client := awss3.NewFromConfig(aws.Config{
		Region:      "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider("TESTACCESSKEY", "test-secret-key", ""),
	}, func(options *awss3.Options) {
		options.BaseEndpoint = aws.String("https://objects.example.test")
		options.UsePathStyle = true
	})
	backend := newS3Backend(client, S3Settings{
		Bucket: "share-bucket", Region: "us-east-1", RootPrefix: "root", Auth: auth,
	})
	fake := &s3CopyFake{headOutput: &awss3.HeadObjectOutput{
		ContentLength: aws.Int64(123),
		LastModified:  aws.Time(time.Unix(1_700_000_000, 0)),
	}}
	backend.client = fake
	return backend, fake
}

func TestS3ShareInfoAndPresignedObjectLinks(t *testing.T) {
	t.Parallel()
	backend, _ := newS3ShareTestBackend(t, "static")
	location := encodeS3OpaqueLocation(s3LocationOpaqueFile, "root/folder/a b+% файл.txt")

	info, err := backend.ShareLinkInfo(context.Background(), location)
	if err != nil {
		t.Fatal(err)
	}
	if info.Provider != "Amazon S3 / S3-compatible" || info.ItemName != "a b+% файл.txt" || !info.CanCreate || info.CanRevoke || !info.LinksUnenumerable || info.Link != nil {
		t.Fatalf("ShareLinkInfo = %#v", info)
	}
	if len(info.Roles) != 2 || info.Roles[0] != vfs.ShareRoleViewer || info.Roles[1] != vfs.ShareRoleUploader {
		t.Fatalf("roles = %v", info.Roles)
	}
	wantExpirations := []time.Duration{15 * time.Minute, time.Hour, 24 * time.Hour, 7 * 24 * time.Hour}
	if len(info.ExpirationOptions) != len(wantExpirations) {
		t.Fatalf("expiration options = %v", info.ExpirationOptions)
	}
	for i := range wantExpirations {
		if info.ExpirationOptions[i] != wantExpirations[i] {
			t.Fatalf("expiration options = %v", info.ExpirationOptions)
		}
	}
	if info.DefaultExpiration != 24*time.Hour || !strings.Contains(info.Notice, "cannot be listed") || !strings.Contains(info.Notice, "PutObject permission") {
		t.Fatalf("share info notice/default = %q, %v", info.Notice, info.DefaultExpiration)
	}

	for _, test := range []struct {
		role   vfs.ShareRole
		method string
	}{
		{role: vfs.ShareRoleViewer, method: http.MethodGet},
		{role: vfs.ShareRoleUploader, method: http.MethodPut},
	} {
		t.Run(test.method, func(t *testing.T) {
			before := time.Now().Add(time.Hour)
			link, err := backend.CreateShareLink(context.Background(), location, vfs.ShareLinkRequest{
				Role: test.role, ExpiresIn: time.Hour,
			})
			if err != nil {
				t.Fatal(err)
			}
			if link.Role != test.role || link.Revocable || link.ExpiresAt.Before(before.Add(-time.Second)) || link.ExpiresAt.After(time.Now().Add(time.Hour+time.Second)) {
				t.Fatalf("share link metadata = %#v", link)
			}
			parsed, err := url.Parse(link.URL)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Scheme != "https" || parsed.Host != "objects.example.test" {
				t.Fatalf("share URL origin = %s://%s", parsed.Scheme, parsed.Host)
			}
			if got := parsed.EscapedPath(); got != "/share-bucket/root/folder/a%20b%2B%25%20%D1%84%D0%B0%D0%B9%D0%BB.txt" {
				t.Fatalf("share URL path = %q", got)
			}
			query := parsed.Query()
			if query.Get("X-Amz-Algorithm") != "AWS4-HMAC-SHA256" || query.Get("X-Amz-Expires") != "3600" || query.Get("X-Amz-Signature") == "" {
				t.Fatalf("presign query is incomplete: %v", query)
			}
			if strings.Contains(link.URL, "test-secret-key") {
				t.Fatal("presigned URL contains the secret access key")
			}
		})
	}
}

func TestS3ShareRejectsUnsupportedTargetsRequestsAndRevocation(t *testing.T) {
	t.Parallel()
	backend, fake := newS3ShareTestBackend(t, "static")
	file := encodeS3OpaqueLocation(s3LocationOpaqueFile, "root/file.txt")
	directory := encodeS3OpaqueLocation(s3LocationOpaqueDir, "root/folder/")

	for name, location := range map[string]string{"root": backend.Root(), "directory": directory} {
		t.Run(name, func(t *testing.T) {
			_, err := backend.ShareLinkInfo(context.Background(), location)
			if !errors.Is(err, ErrShareLinksUnsupported) {
				t.Fatalf("ShareLinkInfo error = %v", err)
			}
		})
	}
	if fake.headCalls != 0 {
		t.Fatalf("directory rejection made %d HeadObject calls", fake.headCalls)
	}

	for _, request := range []vfs.ShareLinkRequest{
		{Role: vfs.ShareRoleEditor, ExpiresIn: time.Hour},
		{Role: vfs.ShareRoleViewer, ExpiresIn: 0},
		{Role: vfs.ShareRoleViewer, ExpiresIn: 2 * time.Hour},
		{Role: vfs.ShareRoleViewer, ExpiresIn: 8 * 24 * time.Hour},
	} {
		if _, err := backend.CreateShareLink(context.Background(), file, request); err == nil {
			t.Fatalf("CreateShareLink(%#v) succeeded", request)
		}
	}
	if fake.headCalls != 0 {
		t.Fatalf("invalid requests made %d HeadObject calls", fake.headCalls)
	}
	if err := backend.RevokeShareLink(context.Background(), file); !errors.Is(err, ErrShareLinksUnsupported) {
		t.Fatalf("RevokeShareLink error = %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backend.ShareLinkInfo(cancelled, file); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled ShareLinkInfo error = %v", err)
	}
	if _, err := backend.CreateShareLink(cancelled, file, vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer, ExpiresIn: time.Hour}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CreateShareLink error = %v", err)
	}
	if err := backend.RevokeShareLink(cancelled, file); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled RevokeShareLink error = %v", err)
	}
}

func TestS3ShareRejectsAnonymousConnectionsBeforeSigning(t *testing.T) {
	t.Parallel()
	backend, fake := newS3ShareTestBackend(t, "anonymous")
	file := encodeS3OpaqueLocation(s3LocationOpaqueFile, "root/file.txt")
	if _, err := backend.ShareLinkInfo(context.Background(), file); !errors.Is(err, ErrShareLinksUnsupported) || !strings.Contains(err.Error(), "anonymous") {
		t.Fatalf("ShareLinkInfo error = %v", err)
	}
	if _, err := backend.CreateShareLink(context.Background(), file, vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer, ExpiresIn: time.Hour}); !errors.Is(err, ErrShareLinksUnsupported) || !strings.Contains(err.Error(), "anonymous") {
		t.Fatalf("CreateShareLink error = %v", err)
	}
	if fake.headCalls != 0 {
		t.Fatalf("anonymous rejection made %d HeadObject calls", fake.headCalls)
	}
}

func TestS3ShareRejectsPlainHTTPPresignedBearerURLs(t *testing.T) {
	t.Parallel()
	backend, fake := newS3ShareTestBackend(t, "static")
	backend.shareHTTPS = false
	file := encodeS3OpaqueLocation(s3LocationOpaqueFile, "root/file.txt")
	if _, err := backend.ShareLinkInfo(context.Background(), file); !errors.Is(err, ErrShareLinksUnsupported) || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("ShareLinkInfo error = %v", err)
	}
	if _, err := backend.CreateShareLink(context.Background(), file, vfs.ShareLinkRequest{Role: vfs.ShareRoleViewer, ExpiresIn: time.Hour}); !errors.Is(err, ErrShareLinksUnsupported) || !strings.Contains(err.Error(), "HTTPS") {
		t.Fatalf("CreateShareLink error = %v", err)
	}
	if fake.headCalls != 0 {
		t.Fatalf("HTTP rejection made %d HeadObject calls", fake.headCalls)
	}
}
