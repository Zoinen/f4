package cloudfox

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsv4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/unxed/f4/vfs"
)

var s3ShareExpirationOptions = [...]time.Duration{
	15 * time.Minute,
	time.Hour,
	24 * time.Hour,
	7 * 24 * time.Hour,
}

const s3ShareNotice = "Amazon S3 links apply to one object only. Viewer links allow repeated downloads; uploader links allow repeated replacement of this exact object until expiry, but PutObject permission is checked only when the URL is used. Anyone with the URL can use it. Issued URLs cannot be listed or revoked individually, and temporary AWS credentials or bucket policy may make them expire earlier."

func s3ShareUnsupported(detail string) error {
	return fmt.Errorf("%w: %s", ErrShareLinksUnsupported, detail)
}

func (b *s3Backend) shareObject(ctx context.Context, location string) (RemoteEntry, s3Target, error) {
	if err := ctx.Err(); err != nil {
		return RemoteEntry{}, s3Target{}, err
	}
	if strings.EqualFold(b.auth, "anonymous") {
		return RemoteEntry{}, s3Target{}, s3ShareUnsupported("anonymous S3 connections cannot create presigned URLs")
	}
	if !b.shareHTTPS {
		return RemoteEntry{}, s3Target{}, s3ShareUnsupported("presigned share URLs require an HTTPS S3 endpoint")
	}
	if b.clientRaw == nil {
		return RemoteEntry{}, s3Target{}, s3ShareUnsupported("this S3 session has no request signer")
	}
	target, err := b.targetKey(location, false)
	if err != nil {
		return RemoteEntry{}, s3Target{}, err
	}
	if target.discoveryRoot || target.bucketRoot {
		return RemoteEntry{}, s3Target{}, s3ShareUnsupported("an S3 account, bucket, or configured root cannot be represented by one presigned URL")
	}
	kind, _, err := splitS3Location(location)
	if err != nil {
		return RemoteEntry{}, s3Target{}, err
	}
	if kind == s3LocationDir || kind == s3LocationOpaqueDir || kind == s3LocationDiscoveredDir || kind == s3LocationBucket {
		return RemoteEntry{}, s3Target{}, s3ShareUnsupported("S3 folders are key prefixes and cannot be represented by one presigned URL")
	}
	entry, err := b.Stat(ctx, location)
	if err != nil {
		return RemoteEntry{}, s3Target{}, err
	}
	if entry.IsDir {
		return RemoteEntry{}, s3Target{}, s3ShareUnsupported("S3 folders are key prefixes and cannot be represented by one presigned URL")
	}
	target, err = b.targetKeyForRequest(ctx, entry.Location, false)
	if err != nil {
		return RemoteEntry{}, s3Target{}, err
	}
	if target.key == "" {
		return RemoteEntry{}, s3Target{}, s3ShareUnsupported("an S3 bucket or configured root cannot be represented by one presigned URL")
	}
	return entry, target, nil
}

func s3ShareExpirationAllowed(value time.Duration) bool {
	for _, allowed := range s3ShareExpirationOptions {
		if value == allowed {
			return true
		}
	}
	return false
}

func (b *s3Backend) ShareLinkInfo(ctx context.Context, location string) (vfs.ShareLinkInfo, error) {
	entry, _, err := b.shareObject(ctx, location)
	if err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	return vfs.ShareLinkInfo{
		Provider:          "Amazon S3 / S3-compatible",
		ItemName:          entry.Name,
		Roles:             []vfs.ShareRole{vfs.ShareRoleViewer, vfs.ShareRoleUploader},
		ExpirationOptions: append([]time.Duration(nil), s3ShareExpirationOptions[:]...),
		DefaultExpiration: 24 * time.Hour,
		CanCreate:         true,
		CanRevoke:         false,
		LinksUnenumerable: true,
		Notice:            s3ShareNotice,
	}, nil
}

func validateS3PresignedRequest(request *awsv4.PresignedHTTPRequest, method string) error {
	if request == nil || request.URL == "" || request.Method != method {
		return errors.New("cloudfox: S3 returned an invalid presigned request")
	}
	target, err := url.Parse(request.URL)
	if err != nil || !strings.EqualFold(target.Scheme, "https") || target.Host == "" || target.User != nil {
		return errors.New("cloudfox: S3 presigned share URLs require HTTPS")
	}
	// A copied URL cannot carry required headers. With the deliberately bare
	// GetObject and PutObject inputs below, the SDK should sign only Host.
	for name := range request.SignedHeader {
		if !strings.EqualFold(name, "host") {
			return fmt.Errorf("cloudfox: S3 presigned request requires unsupported header %q", name)
		}
	}
	return nil
}

func (b *s3Backend) CreateShareLink(ctx context.Context, location string, request vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	if request.Role != vfs.ShareRoleViewer && request.Role != vfs.ShareRoleUploader {
		return vfs.ShareLink{}, fmt.Errorf("cloudfox: unsupported S3 share role %d", request.Role)
	}
	if !s3ShareExpirationAllowed(request.ExpiresIn) {
		return vfs.ShareLink{}, errors.New("cloudfox: unsupported S3 share-link expiration")
	}
	_, target, err := b.shareObject(ctx, location)
	if err != nil {
		return vfs.ShareLink{}, err
	}
	presigner := awss3.NewPresignClient(b.clientRaw)
	issuedAt := time.Now()
	var signed *awsv4.PresignedHTTPRequest
	switch request.Role {
	case vfs.ShareRoleViewer:
		signed, err = presigner.PresignGetObject(ctx, &awss3.GetObjectInput{
			Bucket: aws.String(target.bucket),
			Key:    aws.String(target.key),
		}, func(options *awss3.PresignOptions) {
			options.Expires = request.ExpiresIn
			options.ClientOptions = append(options.ClientOptions, s3RequestOptions(target.region)...)
		})
		if err == nil {
			err = validateS3PresignedRequest(signed, http.MethodGet)
		}
	case vfs.ShareRoleUploader:
		signed, err = presigner.PresignPutObject(ctx, &awss3.PutObjectInput{
			Bucket: aws.String(target.bucket),
			Key:    aws.String(target.key),
		}, func(options *awss3.PresignOptions) {
			options.Expires = request.ExpiresIn
			options.ClientOptions = append(options.ClientOptions, s3RequestOptions(target.region)...)
		})
		if err == nil {
			err = validateS3PresignedRequest(signed, http.MethodPut)
		}
	}
	if err != nil {
		return vfs.ShareLink{}, fmt.Errorf("cloudfox: create S3 presigned request: %w", err)
	}
	return vfs.ShareLink{
		URL:                signed.URL,
		Role:               request.Role,
		ExpiresAt:          issuedAt.Add(request.ExpiresIn),
		ExpiresAtIsMaximum: true,
		Revocable:          false,
	}, nil
}

func (*s3Backend) RevokeShareLink(ctx context.Context, _ string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s3ShareUnsupported("S3 presigned URLs cannot be revoked individually")
}

var _ BackendShareLinker = (*s3Backend)(nil)
