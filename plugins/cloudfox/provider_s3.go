package cloudfox

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awstypes "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/unxed/f4/vfs"
)

type S3Settings struct {
	Bucket        string `json:"bucket"`
	Region        string `json:"region,omitempty"`
	RootPrefix    string `json:"root_prefix,omitempty"`
	Endpoint      string `json:"endpoint,omitempty"`
	UsePathStyle  bool   `json:"use_path_style,omitempty"`
	AllowInsecure bool   `json:"allow_insecure,omitempty"`
	Auth          string `json:"auth,omitempty"` // default, profile, static, anonymous
	Profile       string `json:"profile,omitempty"`
	CustomCA      string `json:"custom_ca,omitempty"`
}

type S3Factory struct {
	HTTPClient *http.Client
}

func (f *S3Factory) Provider() ProviderType { return ProviderS3 }

func (f *S3Factory) settings(c Connection) (S3Settings, error) {
	var settings S3Settings
	if err := json.Unmarshal(c.Settings, &settings); err != nil {
		return settings, fmt.Errorf("cloudfox: decode S3 settings: %w", err)
	}
	settings.Bucket = strings.TrimSpace(settings.Bucket)
	settings.Region = strings.TrimSpace(settings.Region)
	if settings.Region == "" {
		settings.Region = "us-east-1"
	}
	settings.RootPrefix = strings.Trim(strings.ReplaceAll(settings.RootPrefix, "\\", "/"), "/")
	if strings.Contains(settings.RootPrefix, "../") || settings.RootPrefix == ".." {
		return settings, errors.New("cloudfox: S3 root prefix cannot contain '..'")
	}
	if settings.Bucket == "" && settings.RootPrefix != "" {
		return settings, errors.New("cloudfox: S3 root prefix requires an explicit bucket")
	}
	settings.Auth = strings.ToLower(strings.TrimSpace(settings.Auth))
	if settings.Auth == "" {
		settings.Auth = "default"
	}
	switch settings.Auth {
	case "default", "profile", "static", "anonymous":
	default:
		return settings, fmt.Errorf("cloudfox: unsupported S3 authentication %q", settings.Auth)
	}
	if settings.Auth == "profile" && strings.TrimSpace(settings.Profile) == "" {
		return settings, errors.New("cloudfox: AWS profile name is required")
	}
	if settings.Endpoint != "" {
		u, err := url.Parse(settings.Endpoint)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil {
			return settings, errors.New("cloudfox: invalid S3 endpoint")
		}
		if u.Scheme == "http" && settings.Auth != "anonymous" && !settings.AllowInsecure {
			return settings, errors.New("cloudfox: HTTP S3 endpoints require explicit insecure-credentials confirmation")
		}
		settings.Endpoint = strings.TrimRight(settings.Endpoint, "/")
	}
	return settings, nil
}

func (f *S3Factory) Validate(c Connection) error {
	_, err := f.settings(c)
	return err
}

func (f *S3Factory) Open(ctx context.Context, c Connection, secrets SecretValues) (Backend, error) {
	settings, err := f.settings(c)
	if err != nil {
		return nil, err
	}
	httpClient := f.HTTPClient
	if httpClient == nil {
		httpClient, err = s3HTTPClient(settings.CustomCA)
		if err != nil {
			return nil, err
		}
	}
	httpClient = s3NoRedirectClient(httpClient)
	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(settings.Region),
		awsconfig.WithHTTPClient(httpClient),
	}
	switch settings.Auth {
	case "profile":
		loadOptions = append(loadOptions, awsconfig.WithSharedConfigProfile(settings.Profile))
	case "static":
		accessKey := strings.TrimSpace(secrets["access_key_id"])
		secretKey := secrets["secret_access_key"]
		if accessKey == "" || secretKey == "" {
			return nil, ErrAuthenticationRequired
		}
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, secrets["session_token"])))
	case "anonymous":
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(aws.AnonymousCredentials{}))
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("cloudfox: load AWS configuration: %w", err)
	}
	client := awss3.NewFromConfig(config, func(options *awss3.Options) {
		options.UsePathStyle = settings.UsePathStyle
		options.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		options.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		if settings.Endpoint != "" {
			options.BaseEndpoint = aws.String(settings.Endpoint)
		}
	})
	backend := newS3Backend(client, settings)
	backend.httpClient = httpClient
	return backend, nil
}

func s3HTTPClient(customCA string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(customCA) != "" {
		pem, err := os.ReadFile(customCA)
		if err != nil {
			return nil, fmt.Errorf("cloudfox: read S3 custom CA: %w", err)
		}
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("cloudfox: S3 custom CA does not contain a certificate")
		}
		transport.TLSClientConfig.RootCAs = roots
	}
	return s3NoRedirectClient(&http.Client{Transport: transport}), nil
}

func s3NoRedirectClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	// Never replay signed S3 requests at a Location selected by a remote
	// endpoint. Apart from leaking session tokens, the standard library may
	// rewrite a mutating 301/302 request to GET.
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &clone
}

type s3API interface {
	ListObjectsV2(context.Context, *awss3.ListObjectsV2Input, ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error)
	HeadObject(context.Context, *awss3.HeadObjectInput, ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error)
	GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
	PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error)
	DeleteObject(context.Context, *awss3.DeleteObjectInput, ...func(*awss3.Options)) (*awss3.DeleteObjectOutput, error)
	DeleteObjects(context.Context, *awss3.DeleteObjectsInput, ...func(*awss3.Options)) (*awss3.DeleteObjectsOutput, error)
	CopyObject(context.Context, *awss3.CopyObjectInput, ...func(*awss3.Options)) (*awss3.CopyObjectOutput, error)
	CreateMultipartUpload(context.Context, *awss3.CreateMultipartUploadInput, ...func(*awss3.Options)) (*awss3.CreateMultipartUploadOutput, error)
	UploadPartCopy(context.Context, *awss3.UploadPartCopyInput, ...func(*awss3.Options)) (*awss3.UploadPartCopyOutput, error)
	CompleteMultipartUpload(context.Context, *awss3.CompleteMultipartUploadInput, ...func(*awss3.Options)) (*awss3.CompleteMultipartUploadOutput, error)
	AbortMultipartUpload(context.Context, *awss3.AbortMultipartUploadInput, ...func(*awss3.Options)) (*awss3.AbortMultipartUploadOutput, error)
}

// s3BucketLister remains separate from s3API so operation-focused fakes and
// compatible clients which implement only bucket-scoped calls do not need a
// meaningless ListBuckets stub. Discovery mode requires this interface.
type s3BucketLister interface {
	ListBuckets(context.Context, *awss3.ListBucketsInput, ...func(*awss3.Options)) (*awss3.ListBucketsOutput, error)
}

const (
	s3LocationAuto = "a:"
	s3LocationFile = "f:"
	s3LocationDir  = "d:"
	// Provider-returned keys use an opaque form. S3 keys are not paths: a
	// backslash is data and dot segments must never be cleaned. Raw URL-safe
	// base64 also keeps the identity safe when it is embedded in a cloud:// URI.
	s3LocationOpaqueAuto = "A:"
	s3LocationOpaqueFile = "F:"
	s3LocationOpaqueDir  = "D:"
	// Discovery locations carry the selected bucket and its signing region in
	// addition to the opaque object key. The old A:/F:/D: forms intentionally
	// remain unchanged for profiles pinned to one explicit bucket.
	s3LocationBucket         = "B:"
	s3LocationDiscoveredAuto = "C:"
	s3LocationDiscoveredFile = "E:"
	s3LocationDiscoveredDir  = "G:"

	s3MaxSingleCopySize    int64 = 5 << 30
	s3DefaultCopyPartSize  int64 = 128 << 20
	s3MinMultipartPartSize int64 = 5 << 20
	s3MaxMultipartPartSize int64 = 5 << 30
	s3MaxMultipartParts          = 10_000
)

var errS3MultipartCleanup = errors.New("cloudfox: S3 multipart cleanup failed")

type s3Backend struct {
	client        s3API
	clientRaw     *awss3.Client
	httpClient    *http.Client
	bucket        string
	rootPrefix    string
	region        string
	auth          string
	shareHTTPS    bool
	regionsMu     sync.RWMutex
	bucketRegions map[string]string
}

func newS3Backend(client *awss3.Client, settings S3Settings) *s3Backend {
	shareHTTPS := true
	if settings.Endpoint != "" {
		if endpoint, err := url.Parse(settings.Endpoint); err == nil {
			shareHTTPS = strings.EqualFold(endpoint.Scheme, "https")
		}
	}
	return &s3Backend{
		client: client, clientRaw: client, bucket: settings.Bucket,
		rootPrefix: settings.RootPrefix, region: settings.Region, auth: settings.Auth, shareHTTPS: shareHTTPS,
		bucketRegions: make(map[string]string),
	}
}

func (b *s3Backend) Root() string { return s3LocationDir + "/" }

func isS3OpaqueKind(kind string) bool {
	return kind == s3LocationOpaqueAuto || kind == s3LocationOpaqueFile || kind == s3LocationOpaqueDir
}

func isS3DiscoveredKind(kind string) bool {
	return kind == s3LocationBucket || kind == s3LocationDiscoveredAuto || kind == s3LocationDiscoveredFile || kind == s3LocationDiscoveredDir
}

func encodeS3OpaqueLocation(kind, key string) string {
	return kind + base64.RawURLEncoding.EncodeToString([]byte(key))
}

func splitS3Location(location string) (kind, logical string, err error) {
	kind = s3LocationAuto
	logical = location
	if len(location) >= 2 && location[1] == ':' {
		kind = location[:2]
		logical = location[2:]
	}
	if kind != s3LocationAuto && kind != s3LocationFile && kind != s3LocationDir && !isS3OpaqueKind(kind) && !isS3DiscoveredKind(kind) {
		return "", "", errors.New("cloudfox: invalid S3 location kind")
	}
	if isS3OpaqueKind(kind) || isS3DiscoveredKind(kind) {
		decoded, decodeErr := base64.RawURLEncoding.DecodeString(logical)
		if decodeErr != nil {
			return "", "", errors.New("cloudfox: invalid opaque S3 location")
		}
		logical = string(decoded)
		if isS3DiscoveredKind(kind) {
			// Discovery payloads deliberately use NUL separators internally;
			// decodeS3DiscoveredLocation validates the three fields below.
			return kind, logical, nil
		}
		if strings.ContainsRune(logical, '\x00') {
			return "", "", errors.New("cloudfox: S3 location contains NUL")
		}
		return kind, logical, nil
	}
	logical = strings.ReplaceAll(logical, "\\", "/")
	cleaned := path.Clean("/" + strings.TrimPrefix(logical, "/"))
	if strings.ContainsRune(cleaned, '\x00') {
		return "", "", errors.New("cloudfox: S3 location contains NUL")
	}
	return kind, cleaned, nil
}

type s3Target struct {
	bucket        string
	key           string
	region        string
	kind          string
	discoveryRoot bool
	bucketRoot    bool
}

func encodeS3DiscoveredLocation(kind, bucket, region, key string) string {
	payload := bucket + "\x00" + region + "\x00" + key
	return kind + base64.RawURLEncoding.EncodeToString([]byte(payload))
}

func decodeS3DiscoveredLocation(kind, payload string) (s3Target, error) {
	parts := strings.SplitN(payload, "\x00", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) == "" || strings.ContainsRune(parts[1], '\x00') || strings.ContainsRune(parts[2], '\x00') {
		return s3Target{}, errors.New("cloudfox: invalid discovered S3 location")
	}
	return s3Target{
		bucket: parts[0], region: parts[1], key: parts[2], kind: kind,
		bucketRoot: kind == s3LocationBucket && parts[2] == "",
	}, nil
}

func (b *s3Backend) discoveryMode() bool { return b.bucket == "" }

func (b *s3Backend) rememberBucketRegion(bucket, region string) string {
	region = strings.TrimSpace(region)
	if region == "" {
		return ""
	}
	b.regionsMu.Lock()
	if b.bucketRegions == nil {
		b.bucketRegions = make(map[string]string)
	}
	b.bucketRegions[bucket] = region
	b.regionsMu.Unlock()
	return region
}

func (b *s3Backend) knownBucketRegion(bucket, encoded string) string {
	if encoded = strings.TrimSpace(encoded); encoded != "" {
		return encoded
	}
	b.regionsMu.RLock()
	region := b.bucketRegions[bucket]
	b.regionsMu.RUnlock()
	return region
}

func s3RequestOptions(region string) []func(*awss3.Options) {
	region = strings.TrimSpace(region)
	if region == "" {
		return nil
	}
	return []func(*awss3.Options){func(options *awss3.Options) { options.Region = region }}
}

func (b *s3Backend) validateOpaqueKey(key string) error {
	if strings.ContainsRune(key, '\x00') {
		return errors.New("cloudfox: S3 key contains NUL")
	}
	if b.rootPrefix != "" && key != b.rootPrefix && !strings.HasPrefix(key, b.rootPrefix+"/") {
		return errors.New("cloudfox: opaque S3 location is outside the configured root prefix")
	}
	return nil
}

func (b *s3Backend) targetFor(location string) (s3Target, error) {
	kind, logical, err := splitS3Location(location)
	if err != nil {
		return s3Target{}, err
	}
	if b.discoveryMode() {
		if !isS3DiscoveredKind(kind) {
			if (kind == s3LocationDir || kind == s3LocationAuto) && logical == "/" {
				return s3Target{kind: kind, discoveryRoot: true}, nil
			}
			return s3Target{}, errors.New("cloudfox: S3 discovery location does not select a bucket")
		}
		target, err := decodeS3DiscoveredLocation(kind, logical)
		if err != nil {
			return s3Target{}, err
		}
		// An empty region is deliberate: a visual path may be reconstructed
		// before ListBuckets has populated the alias/region cache. Do not bake
		// the configured fallback region into that opaque location. The first
		// provider request resolves it authoritatively at the API boundary.
		target.region = b.knownBucketRegion(target.bucket, target.region)
		return target, nil
	}
	if isS3DiscoveredKind(kind) {
		return s3Target{}, errors.New("cloudfox: discovered S3 location is invalid for an explicit-bucket profile")
	}
	key, err := b.keyForLegacy(kind, logical, false)
	if err != nil {
		return s3Target{}, err
	}
	return s3Target{bucket: b.bucket, key: key, region: b.region, kind: kind, bucketRoot: key == b.rootPrefix}, nil
}

func (b *s3Backend) targetKey(location string, directory bool) (s3Target, error) {
	target, err := b.targetFor(location)
	if err != nil {
		return s3Target{}, err
	}
	if target.discoveryRoot {
		return target, nil
	}
	target.key, err = b.keyFor(location, directory)
	return target, err
}

func (b *s3Backend) targetKeyForRequest(ctx context.Context, location string, directory bool) (s3Target, error) {
	target, err := b.targetKey(location, directory)
	if err != nil || target.discoveryRoot || !b.discoveryMode() {
		return target, err
	}
	return b.resolveTargetRegion(ctx, target)
}

func (b *s3Backend) probeBucketRegion(ctx context.Context, bucket string) (string, error) {
	resolver, ok := b.client.(manager.HeadBucketAPIClient)
	if !ok {
		// S3-compatible test/minimal clients may not expose HeadBucket and
		// commonly use one configured signing region for all buckets.
		return b.region, nil
	}
	region, err := manager.GetBucketRegion(ctx, resolver, bucket, s3RequestOptions(b.region)...)
	if err != nil {
		return "", mapS3Error(err)
	}
	if region = strings.TrimSpace(region); region == "" {
		return b.region, nil
	}
	return b.rememberBucketRegion(bucket, region), nil
}

// resolveTargetRegion is the sole network-assisted translation needed when a
// persisted visual path is restored without first displaying the account
// root. Join intentionally leaves the internal region empty in that case;
// resolving it here keeps histories/bookmarks visual-only while ensuring the
// very first signed bucket request uses the authoritative region.
func (b *s3Backend) resolveTargetRegion(ctx context.Context, target s3Target) (s3Target, error) {
	if target.region != "" || target.discoveryRoot || !b.discoveryMode() {
		return target, nil
	}
	if region := b.knownBucketRegion(target.bucket, ""); region != "" {
		target.region = region
		return target, nil
	}
	lister, ok := b.client.(s3BucketLister)
	if !ok {
		return s3Target{}, errors.New("cloudfox: this S3 client cannot resolve the bucket region; enter a bucket name explicitly")
	}
	var token *string
	for {
		output, err := lister.ListBuckets(ctx, &awss3.ListBucketsInput{
			ContinuationToken: token,
			MaxBuckets:        aws.Int32(1000),
		}, s3RequestOptions(b.region)...)
		if err != nil {
			return s3Target{}, mapS3DiscoveryError(err)
		}
		if output == nil {
			return s3Target{}, errors.New("cloudfox: S3 ListBuckets returned no response while resolving a bucket region")
		}
		for _, bucket := range output.Buckets {
			name := strings.TrimSpace(aws.ToString(bucket.Name))
			if name == "" {
				continue
			}
			region := b.rememberBucketRegion(name, aws.ToString(bucket.BucketRegion))
			if name == target.bucket {
				if region == "" {
					region, err = b.probeBucketRegion(ctx, target.bucket)
					if err != nil {
						return s3Target{}, err
					}
				}
				target.region = region
				return target, nil
			}
		}
		next := strings.TrimSpace(aws.ToString(output.ContinuationToken))
		if next == "" {
			return s3Target{}, fmt.Errorf("cloudfox: S3 bucket %q is no longer available: %w", target.bucket, os.ErrNotExist)
		}
		if token != nil && next == *token {
			return s3Target{}, errors.New("cloudfox: S3 ListBuckets pagination token did not advance while resolving a bucket region")
		}
		token = aws.String(next)
		if err := ctx.Err(); err != nil {
			return s3Target{}, err
		}
	}
}

func (b *s3Backend) Normalize(location string) (string, error) {
	kind, logical, err := splitS3Location(location)
	if err != nil {
		return "", err
	}
	if isS3DiscoveredKind(kind) {
		if !b.discoveryMode() {
			return "", errors.New("cloudfox: discovered S3 location is invalid for an explicit-bucket profile")
		}
		target, err := decodeS3DiscoveredLocation(kind, logical)
		if err != nil {
			return "", err
		}
		target.region = b.knownBucketRegion(target.bucket, target.region)
		if kind == s3LocationBucket && target.key != "" {
			return "", errors.New("cloudfox: invalid S3 bucket-root location")
		}
		return encodeS3DiscoveredLocation(kind, target.bucket, target.region, target.key), nil
	}
	if isS3OpaqueKind(kind) {
		if b.discoveryMode() {
			return "", errors.New("cloudfox: S3 discovery location does not select a bucket")
		}
		if err := b.validateOpaqueKey(logical); err != nil {
			return "", err
		}
		return encodeS3OpaqueLocation(kind, logical), nil
	}
	return kind + logical, nil
}

func (b *s3Backend) Join(base string, elems ...string) string {
	kind, logical, err := splitS3Location(base)
	if err != nil {
		return base
	}
	if isS3DiscoveredKind(kind) {
		target, targetErr := decodeS3DiscoveredLocation(kind, logical)
		if targetErr != nil {
			return base
		}
		key := strings.TrimSuffix(target.key, "/")
		for _, elem := range elems {
			for _, segment := range strings.FieldsFunc(elem, func(r rune) bool { return r == '/' || r == '\\' }) {
				switch segment {
				case "", ".":
					continue
				case "..":
					if key == "" {
						return b.Root()
					}
					key = strings.TrimSuffix(key, "/")
					if slash := strings.LastIndexByte(key, '/'); slash >= 0 {
						key = key[:slash]
					} else {
						key = ""
					}
				default:
					if key != "" {
						key += "/"
					}
					key += segment
				}
			}
		}
		if key == "" {
			return encodeS3DiscoveredLocation(s3LocationBucket, target.bucket, target.region, "")
		}
		return encodeS3DiscoveredLocation(s3LocationDiscoveredAuto, target.bucket, target.region, key)
	}
	if b.discoveryMode() && (kind == s3LocationDir || kind == s3LocationAuto) && logical == "/" {
		parts := make([]string, 0, len(elems))
		for _, elem := range elems {
			parts = applyVisualPathParts(parts, elem)
		}
		if len(parts) == 0 {
			return b.Root()
		}
		bucket := parts[0]
		region := b.knownBucketRegion(bucket, "")
		if len(parts) == 1 {
			return encodeS3DiscoveredLocation(s3LocationBucket, bucket, region, "")
		}
		return encodeS3DiscoveredLocation(s3LocationDiscoveredAuto, bucket, region, strings.Join(parts[1:], "/"))
	}
	if isS3OpaqueKind(kind) {
		key := strings.TrimSuffix(logical, "/")
		for _, elem := range elems {
			for _, segment := range strings.Split(elem, "/") {
				switch segment {
				case "", ".":
					continue
				case "..":
					key = b.parentOpaqueKey(key)
				default:
					if key != "" {
						key += "/"
					}
					key += segment
				}
			}
		}
		if key == b.rootPrefix || (b.rootPrefix == "" && key == "") {
			return b.Root()
		}
		return encodeS3OpaqueLocation(s3LocationOpaqueAuto, key)
	}
	return s3LocationAuto + path.Join(append([]string{logical}, elems...)...)
}

func (b *s3Backend) Base(location string) string {
	kind, logical, err := splitS3Location(location)
	if err != nil {
		return ""
	}
	if isS3DiscoveredKind(kind) {
		target, targetErr := decodeS3DiscoveredLocation(kind, logical)
		if targetErr != nil {
			return ""
		}
		if target.key == "" {
			return target.bucket
		}
		return s3KeyBase(target.key)
	}
	if isS3OpaqueKind(kind) {
		return s3KeyBase(logical)
	}
	return path.Base(logical)
}

func (b *s3Backend) Dir(location string) string {
	kind, logical, err := splitS3Location(location)
	if err != nil || logical == "/" {
		return b.Root()
	}
	if isS3DiscoveredKind(kind) {
		target, targetErr := decodeS3DiscoveredLocation(kind, logical)
		if targetErr != nil || target.key == "" {
			return b.Root()
		}
		key := strings.TrimRight(target.key, "/")
		if slash := strings.LastIndexByte(key, '/'); slash >= 0 {
			key = key[:slash]
		} else {
			key = ""
		}
		if key == "" {
			return encodeS3DiscoveredLocation(s3LocationBucket, target.bucket, target.region, "")
		}
		if !strings.HasSuffix(key, "/") {
			key += "/"
		}
		return encodeS3DiscoveredLocation(s3LocationDiscoveredDir, target.bucket, target.region, key)
	}
	if isS3OpaqueKind(kind) {
		if logical == b.rootPrefix {
			return b.Root()
		}
		parent := b.parentOpaqueKey(logical)
		if parent == b.rootPrefix || (b.rootPrefix == "" && parent == "") {
			return encodeS3OpaqueLocation(s3LocationOpaqueDir, parent)
		}
		if parent != "" && !strings.HasSuffix(parent, "/") {
			parent += "/"
		}
		return encodeS3OpaqueLocation(s3LocationOpaqueDir, parent)
	}
	return s3LocationDir + path.Dir(logical)
}

func s3KeyBase(key string) string {
	key = strings.TrimRight(key, "/")
	if slash := strings.LastIndexByte(key, '/'); slash >= 0 {
		return key[slash+1:]
	}
	return key
}

func (b *s3Backend) parentOpaqueKey(key string) string {
	key = strings.TrimRight(key, "/")
	if slash := strings.LastIndexByte(key, '/'); slash >= 0 {
		key = key[:slash]
	} else {
		key = ""
	}
	if b.rootPrefix != "" && (key == "" || (key != b.rootPrefix && !strings.HasPrefix(key, b.rootPrefix+"/"))) {
		return b.rootPrefix
	}
	return key
}

func (b *s3Backend) IsRoot(location string) bool {
	kind, logical, err := splitS3Location(location)
	if err != nil {
		return false
	}
	if isS3DiscoveredKind(kind) {
		return false
	}
	if isS3OpaqueKind(kind) {
		return logical == b.rootPrefix
	}
	return logical == "/"
}

func (b *s3Backend) keyFor(location string, directory bool) (string, error) {
	kind, logical, err := splitS3Location(location)
	if err != nil {
		return "", err
	}
	if isS3DiscoveredKind(kind) {
		target, targetErr := decodeS3DiscoveredLocation(kind, logical)
		if targetErr != nil {
			return "", targetErr
		}
		key := target.key
		if directory && key != "" && !strings.HasSuffix(key, "/") {
			key += "/"
		}
		return key, nil
	}
	return b.keyForLegacy(kind, logical, directory)
}

func (b *s3Backend) keyForLegacy(kind, logical string, directory bool) (string, error) {
	if isS3OpaqueKind(kind) {
		if err := b.validateOpaqueKey(logical); err != nil {
			return "", err
		}
		key := logical
		if directory && key != "" && !strings.HasSuffix(key, "/") {
			key += "/"
		}
		return key, nil
	}
	relative := strings.TrimPrefix(logical, "/")
	key := relative
	if b.rootPrefix != "" {
		key = b.rootPrefix
		if relative != "" {
			key += "/" + relative
		}
	}
	if directory && key != "" && !strings.HasSuffix(key, "/") {
		key += "/"
	}
	return key, nil
}

func (b *s3Backend) locationForKey(key, kind string) string {
	if key == b.rootPrefix || (b.rootPrefix == "" && key == "") {
		return s3LocationDir + "/"
	}
	opaqueKind := s3LocationOpaqueAuto
	switch kind {
	case s3LocationFile, s3LocationOpaqueFile:
		opaqueKind = s3LocationOpaqueFile
	case s3LocationDir, s3LocationOpaqueDir:
		opaqueKind = s3LocationOpaqueDir
	}
	return encodeS3OpaqueLocation(opaqueKind, key)
}

func (b *s3Backend) locationForTarget(target s3Target, key, kind string) string {
	if !b.discoveryMode() {
		return b.locationForKey(key, kind)
	}
	if key == "" {
		return encodeS3DiscoveredLocation(s3LocationBucket, target.bucket, target.region, "")
	}
	opaqueKind := s3LocationDiscoveredAuto
	switch kind {
	case s3LocationFile, s3LocationOpaqueFile, s3LocationDiscoveredFile:
		opaqueKind = s3LocationDiscoveredFile
	case s3LocationDir, s3LocationOpaqueDir, s3LocationDiscoveredDir:
		opaqueKind = s3LocationDiscoveredDir
	}
	return encodeS3DiscoveredLocation(opaqueKind, target.bucket, target.region, key)
}

func mapS3Error(err error) error {
	if err == nil {
		return nil
	}
	var responseError *smithyhttp.ResponseError
	if errors.As(err, &responseError) {
		switch responseError.HTTPStatusCode() {
		case http.StatusNotFound:
			return fmt.Errorf("%w: %v", os.ErrNotExist, err)
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("%w: %v", os.ErrPermission, err)
		case http.StatusConflict, http.StatusPreconditionFailed:
			return fmt.Errorf("%w: %v", os.ErrExist, err)
		}
	}
	return err
}

func isS3PreconditionFailed(err error) bool {
	var responseError *smithyhttp.ResponseError
	return errors.As(err, &responseError) && responseError.HTTPStatusCode() == http.StatusPreconditionFailed
}

func isS3DestinationConflict(err error) bool {
	var responseError *smithyhttp.ResponseError
	if !errors.As(err, &responseError) {
		return false
	}
	status := responseError.HTTPStatusCode()
	return status == http.StatusConflict || status == http.StatusPreconditionFailed
}

func s3MutationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var responseError *smithyhttp.ResponseError
	if errors.As(err, &responseError) {
		status := responseError.HTTPStatusCode()
		if status >= 400 && status < 500 && status != http.StatusRequestTimeout {
			return mapS3Error(err)
		}
	}
	return &vfs.UnknownOperationStateError{Operation: "S3 " + operation, Err: mapS3Error(err)}
}

func mapS3DiscoveryError(err error) error {
	if err == nil {
		return nil
	}
	mapped := mapS3Error(err)
	if errors.Is(mapped, os.ErrPermission) {
		return fmt.Errorf("cloudfox: cannot list S3 buckets; grant s3:ListAllMyBuckets or enter a bucket name explicitly: %w", mapped)
	}
	return mapped
}

func (b *s3Backend) listBuckets(ctx context.Context, onChunk func([]RemoteEntry)) error {
	lister, ok := b.client.(s3BucketLister)
	if !ok {
		return errors.New("cloudfox: this S3 client cannot list buckets; enter a bucket name explicitly")
	}
	var token *string
	for {
		output, err := lister.ListBuckets(ctx, &awss3.ListBucketsInput{
			ContinuationToken: token,
			MaxBuckets:        aws.Int32(1000),
		}, s3RequestOptions(b.region)...)
		if err != nil {
			return mapS3DiscoveryError(err)
		}
		if output == nil {
			return errors.New("cloudfox: S3 ListBuckets returned no response")
		}
		items := make([]RemoteEntry, 0, len(output.Buckets))
		for _, bucket := range output.Buckets {
			name := strings.TrimSpace(aws.ToString(bucket.Name))
			if name == "" {
				continue
			}
			region := b.rememberBucketRegion(name, aws.ToString(bucket.BucketRegion))
			modified := time.Time{}
			if bucket.CreationDate != nil {
				modified = *bucket.CreationDate
			}
			items = append(items, RemoteEntry{
				VFSItem:      vfs.VFSItem{Name: name, IsDir: true, MTime: modified},
				Location:     encodeS3DiscoveredLocation(s3LocationBucket, name, region, ""),
				TransferName: name,
			})
		}
		if len(items) != 0 && onChunk != nil {
			onChunk(items)
		}
		next := strings.TrimSpace(aws.ToString(output.ContinuationToken))
		if next == "" {
			return nil
		}
		if token != nil && next == *token {
			return errors.New("cloudfox: S3 ListBuckets pagination token did not advance")
		}
		token = aws.String(next)
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (b *s3Backend) ReadDir(ctx context.Context, location string, onChunk func([]RemoteEntry)) error {
	kind, _, err := splitS3Location(location)
	if err != nil {
		return err
	}
	if kind == s3LocationFile || kind == s3LocationOpaqueFile || kind == s3LocationDiscoveredFile {
		return fmt.Errorf("%w: not a directory", os.ErrInvalid)
	}
	target, err := b.targetKeyForRequest(ctx, location, true)
	if err != nil {
		return err
	}
	if target.discoveryRoot {
		return b.listBuckets(ctx, onChunk)
	}
	prefix := target.key
	var token *string
	for {
		output, err := b.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
			Bucket:            aws.String(target.bucket),
			Prefix:            aws.String(prefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: token,
			MaxKeys:           aws.Int32(1000),
		}, s3RequestOptions(target.region)...)
		if err != nil {
			return mapS3Error(err)
		}
		items := make([]RemoteEntry, 0, len(output.CommonPrefixes)+len(output.Contents))
		for _, commonPrefix := range output.CommonPrefixes {
			if commonPrefix.Prefix == nil || *commonPrefix.Prefix == prefix {
				continue
			}
			name := s3KeyBase(*commonPrefix.Prefix)
			items = append(items, RemoteEntry{
				VFSItem:      vfs.VFSItem{Name: name, IsDir: true},
				Location:     b.locationForTarget(target, *commonPrefix.Prefix, s3LocationDir),
				TransferName: name,
			})
		}
		for _, object := range output.Contents {
			if object.Key == nil || *object.Key == prefix || strings.HasSuffix(*object.Key, "/") {
				continue
			}
			name := s3KeyBase(*object.Key)
			modified := time.Time{}
			if object.LastModified != nil {
				modified = *object.LastModified
			}
			items = append(items, RemoteEntry{
				VFSItem: vfs.VFSItem{
					Name:     name,
					Size:     aws.ToInt64(object.Size),
					MTime:    modified,
					Revision: s3ContentRevision("", aws.ToString(object.ETag), aws.ToInt64(object.Size)),
				},
				Location:     b.locationForTarget(target, *object.Key, s3LocationFile),
				TransferName: name,
				SizeKnown:    true,
			})
		}
		if len(items) != 0 {
			onChunk(items)
		}
		if !aws.ToBool(output.IsTruncated) || output.NextContinuationToken == nil {
			return nil
		}
		token = output.NextContinuationToken
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (b *s3Backend) headFile(ctx context.Context, location string) (RemoteEntry, *awss3.HeadObjectOutput, error) {
	target, err := b.targetKeyForRequest(ctx, location, false)
	if err != nil {
		return RemoteEntry{}, nil, err
	}
	if target.discoveryRoot || target.bucketRoot {
		return RemoteEntry{}, nil, os.ErrNotExist
	}
	key := target.key
	if key == "" {
		return RemoteEntry{}, nil, os.ErrNotExist
	}
	output, err := b.client.HeadObject(ctx, &awss3.HeadObjectInput{Bucket: aws.String(target.bucket), Key: aws.String(key)}, s3RequestOptions(target.region)...)
	if err != nil {
		return RemoteEntry{}, nil, mapS3Error(err)
	}
	if output == nil {
		return RemoteEntry{}, nil, errors.New("cloudfox: S3 HeadObject returned no response")
	}
	modified := time.Time{}
	if output.LastModified != nil {
		modified = *output.LastModified
	}
	name := s3KeyBase(key)
	return RemoteEntry{
		VFSItem: vfs.VFSItem{
			Name:     name,
			Size:     aws.ToInt64(output.ContentLength),
			MTime:    modified,
			Revision: s3ContentRevision(aws.ToString(output.VersionId), aws.ToString(output.ETag), aws.ToInt64(output.ContentLength)),
		},
		Location:     b.locationForTarget(target, key, s3LocationFile),
		TransferName: name,
		SizeKnown:    true,
	}, output, nil

}

func s3ContentRevision(versionID, etag string, size int64) string {
	if versionID = strings.TrimSpace(versionID); versionID != "" && versionID != "null" {
		return "version:" + versionID
	}
	// S3 changes an object's ETag when its stored representation changes. It
	// need not be a checksum (multipart/SSE), so retain it only as an opaque
	// provider revision and bind it to the exact size.
	if etag = strings.TrimSpace(etag); etag != "" {
		return fmt.Sprintf("etag:%s|size:%d", etag, size)
	}
	return ""
}

func (b *s3Backend) statFile(ctx context.Context, location string) (RemoteEntry, error) {
	entry, _, err := b.headFile(ctx, location)
	return entry, err
}

func (b *s3Backend) statDir(ctx context.Context, location string) (RemoteEntry, error) {
	if b.IsRoot(location) {
		return RemoteEntry{VFSItem: vfs.VFSItem{Name: "/", IsDir: true}, Location: b.Root(), TransferName: "/"}, nil
	}
	target, err := b.targetKey(location, true)
	if err != nil {
		return RemoteEntry{}, err
	}
	if target.bucketRoot {
		return RemoteEntry{VFSItem: vfs.VFSItem{Name: target.bucket, IsDir: true}, Location: b.locationForTarget(target, "", s3LocationDir), TransferName: target.bucket}, nil
	}
	target, err = b.resolveTargetRegion(ctx, target)
	if err != nil {
		return RemoteEntry{}, err
	}
	prefix := target.key
	output, err := b.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{Bucket: aws.String(target.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(1)}, s3RequestOptions(target.region)...)
	if err != nil {
		return RemoteEntry{}, mapS3Error(err)
	}
	if len(output.Contents) == 0 && len(output.CommonPrefixes) == 0 {
		return RemoteEntry{}, os.ErrNotExist
	}
	name := s3KeyBase(prefix)
	return RemoteEntry{VFSItem: vfs.VFSItem{Name: name, IsDir: true}, Location: b.locationForTarget(target, prefix, s3LocationDir), TransferName: name}, nil
}

func (b *s3Backend) Stat(ctx context.Context, location string) (RemoteEntry, error) {
	kind, _, err := splitS3Location(location)
	if err != nil {
		return RemoteEntry{}, err
	}
	switch kind {
	case s3LocationFile, s3LocationOpaqueFile, s3LocationDiscoveredFile:
		return b.statFile(ctx, location)
	case s3LocationDir, s3LocationOpaqueDir, s3LocationDiscoveredDir, s3LocationBucket:
		return b.statDir(ctx, location)
	default:
		entry, fileErr := b.statFile(ctx, location)
		if fileErr == nil {
			return entry, nil
		}
		entry, dirErr := b.statDir(ctx, location)
		if dirErr == nil {
			return entry, nil
		}
		if !errors.Is(fileErr, os.ErrNotExist) {
			return RemoteEntry{}, fileErr
		}
		return RemoteEntry{}, dirErr
	}
}

func (b *s3Backend) MkDir(ctx context.Context, location string) error {
	target, err := b.targetKey(location, true)
	if err != nil {
		return err
	}
	if target.discoveryRoot || target.bucketRoot {
		return os.ErrPermission
	}
	target, err = b.resolveTargetRegion(ctx, target)
	if err != nil {
		return err
	}
	key := target.key
	_, err = b.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket: aws.String(target.bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(""),
		// MkDir is create-only: replacing an existing/concurrently-created
		// marker has no useful meaning and can destroy its metadata. Keep this
		// conditional even when the caller did not supply a file overwrite
		// decision (or supplied true for a surrounding operation).
		IfNoneMatch: aws.String("*"),
	}, s3RequestOptions(target.region)...)
	return s3MutationError("create directory", err)
}

func (b *s3Backend) deleteDirectory(ctx context.Context, location string) error {
	target, err := b.targetKey(location, true)
	if err != nil {
		return err
	}
	if target.discoveryRoot || target.bucketRoot {
		return os.ErrPermission
	}
	target, err = b.resolveTargetRegion(ctx, target)
	if err != nil {
		return err
	}
	prefix := target.key
	var completed []string
	var failed []string
	var failureDetails []string
	partialOr := func(operationErr error, uncertain []string) error {
		if len(completed) == 0 && len(failed) == 0 {
			return operationErr
		}
		return &vfs.PartialOperationError{
			Operation: "S3 directory delete",
			Completed: append([]string(nil), completed...),
			Failed:    append(append([]string(nil), failed...), uncertain...),
			Err:       operationErr,
		}
	}
	var token *string
	for {
		output, err := b.client.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{Bucket: aws.String(target.bucket), Prefix: aws.String(prefix), ContinuationToken: token, MaxKeys: aws.Int32(1000)}, s3RequestOptions(target.region)...)
		if err != nil {
			return partialOr(mapS3Error(err), []string{prefix})
		}
		if output == nil {
			return partialOr(errors.New("cloudfox: S3 ListObjectsV2 returned no response"), []string{prefix})
		}
		for start := 0; start < len(output.Contents); start += 1000 {
			end := start + 1000
			if end > len(output.Contents) {
				end = len(output.Contents)
			}
			objects := make([]awstypes.ObjectIdentifier, 0, end-start)
			for _, object := range output.Contents[start:end] {
				if object.Key != nil {
					objects = append(objects, awstypes.ObjectIdentifier{Key: object.Key})
				}
			}
			if len(objects) != 0 {
				deleteOutput, err := b.client.DeleteObjects(ctx, &awss3.DeleteObjectsInput{Bucket: aws.String(target.bucket), Delete: &awstypes.Delete{Objects: objects, Quiet: aws.Bool(true)}}, s3RequestOptions(target.region)...)
				if err != nil {
					uncertain := make([]string, 0, len(objects))
					for _, object := range objects {
						uncertain = append(uncertain, aws.ToString(object.Key))
					}
					return partialOr(s3MutationError("directory delete", err), uncertain)
				}
				if deleteOutput == nil {
					uncertain := make([]string, 0, len(objects))
					for _, object := range objects {
						uncertain = append(uncertain, aws.ToString(object.Key))
					}
					return partialOr(&vfs.UnknownOperationStateError{Operation: "S3 directory delete", Err: errors.New("DeleteObjects returned no response")}, uncertain)
				}
				failedInBatch := make(map[string]struct{}, len(deleteOutput.Errors))
				unknownFailureKey := false
				for _, deleteErr := range deleteOutput.Errors {
					key := aws.ToString(deleteErr.Key)
					if key == "" {
						unknownFailureKey = true
					} else {
						failedInBatch[key] = struct{}{}
						failed = append(failed, key)
					}
					detail := strings.TrimSpace(strings.Join([]string{aws.ToString(deleteErr.Code), aws.ToString(deleteErr.Message)}, ": "))
					if detail != "" {
						failureDetails = append(failureDetails, detail)
					}
				}
				if unknownFailureKey {
					uncertain := make([]string, 0, len(objects))
					for _, object := range objects {
						uncertain = append(uncertain, aws.ToString(object.Key))
					}
					return partialOr(&vfs.UnknownOperationStateError{
						Operation: "S3 directory delete",
						Err:       errors.New("DeleteObjects returned an error without an object key"),
					}, uncertain)
				}
				for _, object := range objects {
					key := aws.ToString(object.Key)
					if _, didFail := failedInBatch[key]; !didFail {
						completed = append(completed, key)
					}
				}
				if len(failedInBatch) != 0 && len(failureDetails) == 0 {
					failureDetails = append(failureDetails, "one or more objects were rejected")
				}
			}
		}
		if !aws.ToBool(output.IsTruncated) || output.NextContinuationToken == nil {
			if len(failed) != 0 {
				detail := fmt.Sprintf("cloudfox: S3 rejected %d object deletion(s) below %s", len(failed), b.Base(location))
				if len(failureDetails) != 0 {
					detail += ": " + strings.Join(failureDetails, "; ")
				}
				return &vfs.PartialOperationError{
					Operation: "S3 directory delete",
					Completed: completed,
					Failed:    failed,
					Err:       errors.New(detail),
				}
			}
			return nil
		}
		token = output.NextContinuationToken
		if err := ctx.Err(); err != nil {
			return partialOr(err, []string{prefix})
		}
	}
}

func (b *s3Backend) Remove(ctx context.Context, location string) error {
	target, targetErr := b.targetKey(location, false)
	if targetErr != nil {
		return targetErr
	}
	if target.discoveryRoot || target.bucketRoot {
		return os.ErrPermission
	}
	entry, err := b.Stat(ctx, location)
	if err != nil {
		return err
	}
	if entry.IsDir {
		if b.IsRoot(location) {
			return os.ErrPermission
		}
		return b.deleteDirectory(ctx, entry.Location)
	}
	target, err = b.targetKeyForRequest(ctx, entry.Location, false)
	if err != nil {
		return err
	}
	_, err = b.client.DeleteObject(ctx, &awss3.DeleteObjectInput{Bucket: aws.String(target.bucket), Key: aws.String(target.key)}, s3RequestOptions(target.region)...)
	return s3MutationError("delete", err)
}

func (b *s3Backend) Rename(ctx context.Context, oldLocation, newLocation string) error {
	oldTarget, err := b.targetKey(oldLocation, false)
	if err != nil {
		return err
	}
	newTarget, err := b.targetKey(newLocation, false)
	if err != nil {
		return err
	}
	if oldTarget.discoveryRoot || oldTarget.bucketRoot || newTarget.discoveryRoot || newTarget.bucketRoot {
		return os.ErrPermission
	}
	source, err := b.pinCopySource(ctx, oldLocation)
	if err != nil {
		return err
	}
	if source.etag == "" {
		return errors.New("cloudfox: S3 source has no ETag; refusing a rename without atomic conditional cleanup")
	}
	if err := b.copyPinned(ctx, source, newLocation); err != nil {
		return err
	}
	deleteInput := &awss3.DeleteObjectInput{
		Bucket:  aws.String(source.bucket),
		Key:     aws.String(source.key),
		IfMatch: aws.String(source.etag),
	}
	_, err = b.client.DeleteObject(ctx, deleteInput, s3RequestOptions(source.region)...)
	if err != nil {
		cleanupErr := s3MutationError("rename cleanup", err)
		if isS3PreconditionFailed(err) {
			cleanupErr = ErrRemoteObjectChanged
		}
		return &vfs.PartialOperationError{
			Operation: "S3 rename cleanup",
			Completed: []string{newLocation},
			Failed:    []string{oldLocation},
			Err:       cleanupErr,
		}
	}
	return nil
}

func (b *s3Backend) Copy(ctx context.Context, oldLocation, newLocation string) error {
	oldTarget, err := b.targetKey(oldLocation, false)
	if err != nil {
		return err
	}
	newTarget, err := b.targetKey(newLocation, false)
	if err != nil {
		return err
	}
	if oldTarget.discoveryRoot || oldTarget.bucketRoot || newTarget.discoveryRoot || newTarget.bucketRoot {
		return os.ErrPermission
	}
	source, err := b.pinCopySource(ctx, oldLocation)
	if err != nil {
		return err
	}
	return b.copyPinned(ctx, source, newLocation)
}

type s3CopySourcePin struct {
	bucket    string
	region    string
	key       string
	etag      string
	versionID string
	head      *awss3.HeadObjectOutput
}

func (b *s3Backend) pinCopySource(ctx context.Context, location string) (s3CopySourcePin, error) {
	kind, _, err := splitS3Location(location)
	if err != nil {
		return s3CopySourcePin{}, err
	}
	if kind == s3LocationDir || kind == s3LocationOpaqueDir || kind == s3LocationDiscoveredDir || kind == s3LocationBucket {
		return s3CopySourcePin{}, ErrUnsupportedOperation
	}
	entry, head, err := b.headFile(ctx, location)
	if err != nil {
		if (kind == s3LocationAuto || kind == s3LocationOpaqueAuto) && errors.Is(err, os.ErrNotExist) {
			if _, dirErr := b.statDir(ctx, location); dirErr == nil {
				return s3CopySourcePin{}, ErrUnsupportedOperation
			}
		}
		return s3CopySourcePin{}, err
	}
	target, err := b.targetKeyForRequest(ctx, entry.Location, false)
	if err != nil {
		return s3CopySourcePin{}, err
	}
	key := target.key
	etag := strings.TrimSpace(aws.ToString(head.ETag))
	versionID := strings.TrimSpace(aws.ToString(head.VersionId))
	if etag == "" && versionID == "" {
		return s3CopySourcePin{}, errors.New("cloudfox: S3 source has neither an ETag nor a version ID; refusing an unpinned copy")
	}
	pinnedHead := *head
	pinnedHead.ETag = nil
	pinnedHead.VersionId = nil
	if etag != "" {
		pinnedHead.ETag = aws.String(etag)
	}
	if versionID != "" {
		pinnedHead.VersionId = aws.String(versionID)
	}
	return s3CopySourcePin{bucket: target.bucket, region: target.region, key: key, etag: etag, versionID: versionID, head: &pinnedHead}, nil
}

func (b *s3Backend) copyPinned(ctx context.Context, source s3CopySourcePin, newLocation string) error {
	destination, err := b.targetKeyForRequest(ctx, newLocation, false)
	if err != nil {
		return err
	}
	if destination.discoveryRoot || destination.bucketRoot {
		return os.ErrPermission
	}
	newKey := destination.key
	copySource := s3CopySource(source.bucket, source.key, source.versionID)
	sourceSize := aws.ToInt64(source.head.ContentLength)
	if sourceSize > s3MaxSingleCopySize {
		return b.copyMultipart(ctx, destination, copySource, sourceSize, source.head)
	}
	_, err = b.client.CopyObject(ctx, &awss3.CopyObjectInput{
		Bucket:            aws.String(destination.bucket),
		Key:               aws.String(newKey),
		CopySource:        aws.String(copySource),
		CopySourceIfMatch: source.head.ETag,
		IfNoneMatch:       s3DestinationIfNoneMatch(ctx),
	}, s3RequestOptions(destination.region)...)
	if isS3PreconditionFailed(err) && s3DestinationIfNoneMatch(ctx) == nil {
		return ErrRemoteObjectChanged
	}
	return s3MutationError("copy", err)
}

func s3CopySource(bucket, key string, versionID ...string) string {
	// PathEscape encodes separators, percent signs, whitespace and non-ASCII
	// bytes exactly once for the x-amz-copy-source header expected by the SDK.
	source := url.PathEscape(bucket + "/" + key)
	if len(versionID) != 0 && strings.TrimSpace(versionID[0]) != "" {
		source += "?versionId=" + url.QueryEscape(versionID[0])
	}
	return source
}

func s3MultipartCopyPartSize(size int64) (int64, error) {
	if size <= 0 {
		return 0, errors.New("cloudfox: S3 multipart copy source has no content")
	}
	partSize := s3DefaultCopyPartSize
	minimumForPartLimit := size / s3MaxMultipartParts
	if size%s3MaxMultipartParts != 0 {
		minimumForPartLimit++
	}
	if minimumForPartLimit > partSize {
		const alignment = int64(1 << 20)
		partSize = minimumForPartLimit
		if remainder := partSize % alignment; remainder != 0 {
			partSize += alignment - remainder
		}
	}
	if partSize < s3MinMultipartPartSize {
		partSize = s3MinMultipartPartSize
	}
	if partSize > s3MaxMultipartPartSize || (size/partSize)+(boolToInt64(size%partSize != 0)) > s3MaxMultipartParts {
		return 0, fmt.Errorf("cloudfox: S3 object size %d exceeds multipart copy limits", size)
	}
	return partSize, nil
}

func boolToInt64(value bool) int64 {
	if value {
		return 1
	}
	return 0
}

func (b *s3Backend) copyMultipart(ctx context.Context, destination s3Target, copySource string, size int64, source *awss3.HeadObjectOutput) (retErr error) {
	destinationKey := destination.key
	partSize, err := s3MultipartCopyPartSize(size)
	if err != nil {
		return err
	}
	createOutput, err := b.client.CreateMultipartUpload(ctx, &awss3.CreateMultipartUploadInput{
		Bucket:                  aws.String(destination.bucket),
		Key:                     aws.String(destinationKey),
		BucketKeyEnabled:        source.BucketKeyEnabled,
		CacheControl:            source.CacheControl,
		ContentDisposition:      source.ContentDisposition,
		ContentEncoding:         source.ContentEncoding,
		ContentLanguage:         source.ContentLanguage,
		ContentType:             source.ContentType,
		Expires:                 source.Expires,
		Metadata:                source.Metadata,
		SSEKMSKeyId:             source.SSEKMSKeyId,
		ServerSideEncryption:    source.ServerSideEncryption,
		StorageClass:            source.StorageClass,
		WebsiteRedirectLocation: source.WebsiteRedirectLocation,
	}, s3RequestOptions(destination.region)...)
	if err != nil {
		return s3MutationError("multipart copy start", err)
	}
	uploadID := strings.TrimSpace(aws.ToString(createOutput.UploadId))
	if uploadID == "" {
		return errors.New("cloudfox: S3 did not return a multipart upload ID")
	}

	completed := false
	defer func() {
		if completed {
			return
		}
		abortCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, abortErr := b.client.AbortMultipartUpload(abortCtx, &awss3.AbortMultipartUploadInput{
			Bucket: aws.String(destination.bucket), Key: aws.String(destinationKey), UploadId: aws.String(uploadID),
		}, s3RequestOptions(destination.region)...)
		if abortErr != nil {
			abortErr = fmt.Errorf("cloudfox: abort S3 multipart copy: %w", mapS3Error(abortErr))
			retErr = errors.Join(retErr, abortErr)
		}
	}()

	partCount := size / partSize
	if size%partSize != 0 {
		partCount++
	}
	parts := make([]awstypes.CompletedPart, 0, int(partCount))
	for partNumber, start := int32(1), int64(0); start < size; partNumber, start = partNumber+1, start+partSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		end := start + partSize - 1
		if end >= size {
			end = size - 1
		}
		output, err := b.client.UploadPartCopy(ctx, &awss3.UploadPartCopyInput{
			Bucket:            aws.String(destination.bucket),
			Key:               aws.String(destinationKey),
			UploadId:          aws.String(uploadID),
			PartNumber:        aws.Int32(partNumber),
			CopySource:        aws.String(copySource),
			CopySourceIfMatch: source.ETag,
			CopySourceRange:   aws.String(fmt.Sprintf("bytes=%d-%d", start, end)),
		}, s3RequestOptions(destination.region)...)
		if err != nil {
			if isS3PreconditionFailed(err) {
				return ErrRemoteObjectChanged
			}
			return mapS3Error(err)
		}
		if output == nil || output.CopyPartResult == nil || strings.TrimSpace(aws.ToString(output.CopyPartResult.ETag)) == "" {
			return fmt.Errorf("cloudfox: S3 multipart copy part %d returned no ETag", partNumber)
		}
		parts = append(parts, awstypes.CompletedPart{ETag: output.CopyPartResult.ETag, PartNumber: aws.Int32(partNumber)})
	}
	_, err = b.client.CompleteMultipartUpload(ctx, &awss3.CompleteMultipartUploadInput{
		Bucket:          aws.String(destination.bucket),
		Key:             aws.String(destinationKey),
		UploadId:        aws.String(uploadID),
		MultipartUpload: &awstypes.CompletedMultipartUpload{Parts: parts},
		IfNoneMatch:     s3DestinationIfNoneMatch(ctx),
	}, s3RequestOptions(destination.region)...)
	if err != nil {
		// A server response rejecting the destination condition is definitive:
		// CompleteMultipartUpload did not replace the existing object. Transport
		// failures remain unknown because completion may already have committed.
		if s3DestinationIfNoneMatch(ctx) != nil && isS3DestinationConflict(err) {
			return mapS3Error(err)
		}
		return &vfs.UnknownOperationStateError{Operation: "S3 multipart copy completion", Err: mapS3Error(err)}
	}
	completed = true
	return nil
}

type s3RangeReader struct {
	ctx         context.Context
	cancel      context.CancelFunc
	client      s3API
	bucket      string
	region      string
	key         string
	etag        string
	versionID   string
	displayName string
	size        int64
	mu          sync.Mutex
	offset      int64
}

func (r *s3RangeReader) Size() int64 { return r.size }
func (r *s3RangeReader) ReadAccessProfile() vfs.ReadAccessProfile {
	return vfs.ReadAccessNativeRange
}

func (r *s3RangeReader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 {
		return 0, fmt.Errorf("%w: negative S3 read offset", os.ErrInvalid)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off >= r.size {
		return 0, io.EOF
	}
	want := int64(len(p))
	if remaining := r.size - off; want > remaining {
		want = remaining
	}
	end := off + want - 1
	requestCtx, done := providerOperationContext(ctx, r.ctx)
	defer done()
	input := &awss3.GetObjectInput{
		Bucket: aws.String(r.bucket),
		Key:    aws.String(r.key),
		Range:  aws.String(fmt.Sprintf("bytes=%d-%d", off, end)),
	}
	if r.etag != "" {
		input.IfMatch = aws.String(r.etag)
	}
	if r.versionID != "" {
		input.VersionId = aws.String(r.versionID)
	}
	output, err := r.client.GetObject(requestCtx, input, s3RequestOptions(r.region)...)
	if err != nil {
		if isS3PreconditionFailed(err) {
			return 0, ErrRemoteObjectChanged
		}
		return 0, mapS3Error(err)
	}
	if output == nil || output.Body == nil {
		return 0, errors.New("cloudfox: S3 GetObject returned no response body")
	}
	defer output.Body.Close()
	if r.etag != "" && strings.TrimSpace(aws.ToString(output.ETag)) != r.etag {
		return 0, ErrRemoteObjectChanged
	}
	if r.versionID != "" && strings.TrimSpace(aws.ToString(output.VersionId)) != r.versionID {
		return 0, ErrRemoteObjectChanged
	}
	expectedContentRange := fmt.Sprintf("bytes %d-%d/%d", off, end, r.size)
	if got := strings.TrimSpace(aws.ToString(output.ContentRange)); got != expectedContentRange {
		return 0, fmt.Errorf("cloudfox: S3 returned Content-Range %q, want %q", got, expectedContentRange)
	}
	n, readErr := io.ReadFull(output.Body, p[:want])
	if readErr == io.ErrUnexpectedEOF {
		readErr = io.EOF
	}
	if n < len(p) && readErr == nil {
		readErr = io.EOF
	}
	return n, readErr
}

func (r *s3RangeReader) Read(ctx context.Context, p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.ReadAt(ctx, p, r.offset)
	r.offset += int64(n)
	if reporter, ok := ctx.Value(vfs.ReporterKey).(vfs.TaskReporter); ok && r.size > 0 && n > 0 {
		percent := int(r.offset * 100 / r.size)
		if percent > 100 {
			percent = 100
		}
		reporter.UpdateTransfer("Downloading", r.displayName, percent, "", percent, "")
	}
	return n, err
}

func (r *s3RangeReader) Close() error {
	r.cancel()
	return nil
}

func (b *s3Backend) Open(ctx context.Context, location string) (vfs.ReadAtCloser, error) {
	kind, _, err := splitS3Location(location)
	if err != nil {
		return nil, err
	}
	if kind == s3LocationDir || kind == s3LocationOpaqueDir || kind == s3LocationDiscoveredDir || kind == s3LocationBucket {
		return nil, os.ErrInvalid
	}
	if kind == s3LocationAuto || kind == s3LocationOpaqueAuto || kind == s3LocationDiscoveredAuto {
		resolved, statErr := b.Stat(ctx, location)
		if statErr != nil {
			return nil, statErr
		}
		if resolved.IsDir {
			return nil, os.ErrInvalid
		}
		location = resolved.Location
	}
	entry, head, err := b.headFile(ctx, location)
	if err != nil {
		return nil, err
	}
	target, err := b.targetKeyForRequest(ctx, entry.Location, false)
	if err != nil {
		return nil, err
	}
	key := target.key
	etag := strings.TrimSpace(aws.ToString(head.ETag))
	versionID := strings.TrimSpace(aws.ToString(head.VersionId))
	if etag == "" && versionID == "" {
		return b.openS3Snapshot(ctx, target)
	}
	readerCtx, cancel := context.WithCancel(context.Background())
	return &s3RangeReader{
		ctx: readerCtx, cancel: cancel, client: b.client, bucket: target.bucket, region: target.region, key: key,
		etag: etag, versionID: versionID, size: entry.Size, displayName: b.Base(location),
	}, nil
}

func (b *s3Backend) openS3Snapshot(ctx context.Context, target s3Target) (_ vfs.ReadAtCloser, retErr error) {
	key := target.key
	output, err := b.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(target.bucket), Key: aws.String(key)}, s3RequestOptions(target.region)...)
	if err != nil {
		return nil, mapS3Error(err)
	}
	if output == nil || output.Body == nil {
		return nil, errors.New("cloudfox: S3 GetObject returned no response body")
	}
	defer output.Body.Close()

	file, err := os.CreateTemp("", "f4-cloudfox-s3-read-*")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	defer func() {
		if retErr != nil {
			_ = file.Close()
			_ = os.Remove(name)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return nil, err
	}
	var source io.Reader = output.Body
	reporter, hasReporter := ctx.Value(vfs.ReporterKey).(vfs.TaskReporter)
	if hasReporter {
		source = &providerProgressReader{r: output.Body, ctx: ctx, reporter: reporter, action: "Downloading", name: s3KeyBase(key), total: aws.ToInt64(output.ContentLength)}
	}
	written, err := io.Copy(file, source)
	if err != nil {
		return nil, err
	}
	if output.ContentLength != nil && aws.ToInt64(output.ContentLength) >= 0 && written != aws.ToInt64(output.ContentLength) {
		return nil, io.ErrUnexpectedEOF
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if hasReporter {
		reporter.UpdateTransfer("Downloading", s3KeyBase(key), 100, "", 100, "")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return newProviderTempReader(file, name, written), nil
}

type s3UploadWriter struct {
	ctx    context.Context
	cancel context.CancelFunc
	pipe   *io.PipeWriter
	done   chan error
	once   sync.Once
	err    error
}

func (w *s3UploadWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.pipe.Write(p)
}

func (w *s3UploadWriter) Close() error {
	w.once.Do(func() {
		closeErr := w.pipe.Close()
		uploadErr := <-w.done
		w.cancel()
		if uploadErr != nil {
			w.err = uploadErr
		} else {
			w.err = closeErr
		}
	})
	return w.err
}

func (w *s3UploadWriter) Abort() error {
	w.once.Do(func() {
		// PutObject publishes only after the complete request body, and
		// multipart uploads publish only at CompleteMultipartUpload. Canceling
		// before closing the pipe with EOF therefore remains pre-commit.
		w.cancel()
		pipeErr := w.pipe.CloseWithError(context.Canceled)
		uploadErr := <-w.done
		if uploadErr != nil && (errors.Is(uploadErr, errS3MultipartCleanup) || !errors.Is(uploadErr, context.Canceled)) {
			w.err = uploadErr
		} else if pipeErr != nil && !errors.Is(pipeErr, io.ErrClosedPipe) {
			w.err = pipeErr
		}
	})
	return w.err
}

func s3DestinationIfNoneMatch(ctx context.Context) *string {
	overwrite, known := vfs.DestinationOverwrite(ctx)
	if !known || overwrite {
		return nil
	}
	return aws.String("*")
}

func (b *s3Backend) Create(ctx context.Context, location string) (io.WriteCloser, error) {
	target, err := b.targetKey(location, false)
	if err != nil {
		return nil, err
	}
	if target.discoveryRoot || target.bucketRoot {
		return nil, os.ErrPermission
	}
	target, err = b.resolveTargetRegion(ctx, target)
	if err != nil {
		return nil, err
	}
	key := target.key
	uploadCtx, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	uploader := manager.NewUploader(b.clientRaw, func(u *manager.Uploader) {
		u.PartSize = 8 << 20
		u.Concurrency = 2
		// Manager aborts with the upload context. On user cancellation that
		// context is already dead, so its cleanup request never leaves the
		// process and uploaded parts remain billable. Keep the ID and abort below
		// with a detached, bounded cleanup context instead.
		u.LeavePartsOnError = true
		u.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		u.ClientOptions = append(u.ClientOptions, s3RequestOptions(target.region)...)
	})
	go func() {
		// If-None-Match is the only race-free S3 no-replace primitive. The
		// transfer manager propagates it both to PutObject and, for large files,
		// to CompleteMultipartUpload. Never fall back to HeadObject + PutObject:
		// another writer could create the key between those requests.
		_, uploadErr := uploader.Upload(uploadCtx, &awss3.PutObjectInput{
			Bucket:      aws.String(target.bucket),
			Key:         aws.String(key),
			Body:        reader,
			IfNoneMatch: s3DestinationIfNoneMatch(uploadCtx),
		})
		var multipartFailure manager.MultiUploadFailure
		if errors.As(uploadErr, &multipartFailure) && strings.TrimSpace(multipartFailure.UploadID()) != "" {
			cleanupCtx, cleanupDone := providerDetachedCleanupContext(uploadCtx, 15*time.Second)
			_, abortErr := b.client.AbortMultipartUpload(cleanupCtx, &awss3.AbortMultipartUploadInput{
				Bucket: aws.String(target.bucket), Key: aws.String(key), UploadId: aws.String(multipartFailure.UploadID()),
			}, s3RequestOptions(target.region)...)
			cleanupDone()
			if abortErr != nil {
				uploadErr = errors.Join(uploadErr, fmt.Errorf("%w: %v", errS3MultipartCleanup, mapS3Error(abortErr)))
			}
		}
		_ = reader.CloseWithError(uploadErr)
		done <- s3MutationError("upload", uploadErr)
	}()
	return &s3UploadWriter{ctx: uploadCtx, cancel: cancel, pipe: writer, done: done}, nil
}

func (b *s3Backend) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return ErrUnsupportedOperation
}

func (b *s3Backend) Capabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasServerSideCopy: true, HasServerSideMove: false, HasRandomAccess: true, HasAtomicNoReplaceRename: true, ReadAccess: vfs.ReadAccessNativeRange, StorageClass: vfs.StorageClassNetwork}
}

func (b *s3Backend) TransferName(location string) string { return b.Base(location) }

// TestConnection performs one bounded, authoritative request without walking
// an account or bucket. Discovery checks account-level enumeration permission;
// explicit-bucket profiles remain usable without s3:ListAllMyBuckets.
func (b *s3Backend) TestConnection(ctx context.Context) error {
	probeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if b.discoveryMode() {
		lister, ok := b.client.(s3BucketLister)
		if !ok {
			return errors.New("cloudfox: this S3 client cannot list buckets; enter a bucket name explicitly")
		}
		output, err := lister.ListBuckets(probeCtx, &awss3.ListBucketsInput{MaxBuckets: aws.Int32(1)}, s3RequestOptions(b.region)...)
		if err != nil {
			return mapS3DiscoveryError(err)
		}
		if output == nil {
			return errors.New("cloudfox: S3 ListBuckets returned no response")
		}
		return nil
	}
	prefix := b.rootPrefix
	if prefix != "" {
		prefix += "/"
	}
	output, err := b.client.ListObjectsV2(probeCtx, &awss3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(1),
	}, s3RequestOptions(b.region)...)
	if err != nil {
		return mapS3Error(err)
	}
	if output == nil {
		return errors.New("cloudfox: S3 ListObjectsV2 returned no response")
	}
	return nil
}

func (b *s3Backend) Close() error {
	if b.httpClient != nil {
		b.httpClient.CloseIdleConnections()
	}
	return nil
}

var _ Backend = (*s3Backend)(nil)
var _ BackendCopier = (*s3Backend)(nil)
var _ BackendTransferNamer = (*s3Backend)(nil)
