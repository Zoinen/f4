package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const defaultYandexDiskAPI = "https://cloud-api.yandex.net/v1/disk"

var errYandexOperationFailed = errors.New("cloudfox: Yandex.Disk operation failed")

// DefaultYandexClientID may be populated by release builds. A profile-level
// ClientID always takes precedence.
var DefaultYandexClientID string

type YandexDiskSettings struct {
	ClientID string `json:"client_id,omitempty"`
	Root     string `json:"root,omitempty"`
}

type YandexDiskFactory struct {
	HTTPClient *http.Client
	BaseURL    string
}

func (f *YandexDiskFactory) Provider() ProviderType { return ProviderYandexDisk }

func (f *YandexDiskFactory) settings(c Connection) (YandexDiskSettings, error) {
	var settings YandexDiskSettings
	if len(c.Settings) != 0 {
		if err := json.Unmarshal(c.Settings, &settings); err != nil {
			return settings, fmt.Errorf("cloudfox: decode Yandex.Disk settings: %w", err)
		}
	}
	if strings.TrimSpace(settings.ClientID) == "" {
		settings.ClientID = strings.TrimSpace(DefaultYandexClientID)
	}
	root, err := normalizeYandexPath(settings.Root)
	if err != nil {
		return settings, err
	}
	settings.Root = root
	return settings, nil
}

func (f *YandexDiskFactory) Validate(c Connection) error {
	_, err := f.settings(c)
	return err
}

func (f *YandexDiskFactory) Open(ctx context.Context, c Connection, secrets SecretValues) (Backend, error) {
	settings, err := f.settings(c)
	if err != nil {
		return nil, err
	}
	token := strings.TrimSpace(secrets["oauth_token"])
	if token == "" {
		token = strings.TrimSpace(secrets["access_token"])
	}
	if token == "" {
		return nil, ErrAuthenticationRequired
	}
	client := f.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	baseURL := strings.TrimRight(f.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultYandexDiskAPI
	}
	backend := &yandexDiskBackend{client: client, baseURL: baseURL, token: token, root: settings.Root, downloads: newYandexDownloadCache()}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return backend, nil
}

type yandexDiskBackend struct {
	client    *http.Client
	baseURL   string
	token     string
	root      string
	shareGate yandexShareGate

	mu        sync.RWMutex
	about     *yandexDiskAbout
	aboutAt   time.Time
	downloads *yandexDownloadCache
	close     sync.Once
}

// do applies one redirect policy to every request issued by a backend,
// including backends constructed directly in tests. Authenticated API/status
// calls and write methods never follow redirects. Unauthenticated temporary
// downloads may follow redirects only within the API or Yandex.Disk transfer
// origins.
func (b *yandexDiskBackend) do(req *http.Request) (*http.Response, error) {
	client := b.client
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	previousRedirectPolicy := clone.CheckRedirect
	clone.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) == 0 {
			return errors.New("cloudfox: Yandex.Disk redirect has no originating request")
		}
		if len(via) >= 5 {
			return errors.New("cloudfox: too many Yandex.Disk redirects")
		}
		initial := via[0]
		previous := via[len(via)-1]
		if initial.Header.Get("Authorization") != "" || (initial.Method != http.MethodGet && initial.Method != http.MethodHead) || next.Method != previous.Method {
			return http.ErrUseLastResponse
		}
		if _, err := b.validateTemporaryURL(next.URL.String()); err != nil {
			return http.ErrUseLastResponse
		}
		if previousRedirectPolicy != nil {
			return previousRedirectPolicy(next, via)
		}
		return nil
	}
	return clone.Do(req)
}

func (b *yandexDiskBackend) validateTemporaryURL(raw string) (string, error) {
	target, err := url.Parse(raw)
	if err != nil || !target.IsAbs() || target.Host == "" || target.User != nil || target.Fragment != "" || (target.Scheme != "https" && target.Scheme != "http") {
		return "", errors.New("cloudfox: Yandex.Disk returned an invalid temporary URL")
	}
	base, err := url.Parse(b.baseURL)
	if err != nil {
		return "", err
	}
	if sameWebDAVOrigin(target, base) {
		return target.String(), nil
	}
	if !strings.EqualFold(target.Scheme, "https") || (target.Port() != "" && target.Port() != "443") || !isYandexDiskTransferHost(target.Hostname()) {
		return "", errors.New("cloudfox: Yandex.Disk returned a temporary URL outside its transfer service")
	}
	return target.String(), nil
}

func isYandexDiskTransferHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	return host == "disk.yandex.net" || strings.HasSuffix(host, ".disk.yandex.net") ||
		host == "disk.yandex.ru" || strings.HasSuffix(host, ".disk.yandex.ru") ||
		strings.HasSuffix(host, ".storage.yandex.net")
}

func sanitizeYandexTransferError(err error) error {
	if err == nil {
		return nil
	}
	var requestErr *url.Error
	if !errors.As(err, &requestErr) {
		return err
	}
	clean := *requestErr
	// Signed CDN URLs can carry credentials in both the query and path. Keep
	// the original cause so errors.Is/errors.As for context and net errors keep
	// working, but never retain the transfer URL in the displayed wrapper.
	clean.URL = "[Yandex.Disk temporary transfer URL redacted]"
	return &clean
}

func sanitizeYandexTransferResponse(resp *http.Response) *http.Response {
	if resp == nil || resp.Request == nil || resp.Request.URL == nil {
		return resp
	}
	cleanResponse := new(http.Response)
	*cleanResponse = *resp
	cleanRequest := new(http.Request)
	*cleanRequest = *resp.Request
	cleanURL := &url.URL{Scheme: resp.Request.URL.Scheme, Host: resp.Request.URL.Host, Path: "/[redacted]"}
	cleanRequest.URL = cleanURL
	cleanResponse.Request = cleanRequest
	return cleanResponse
}

type yandexResource struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Modified   string `json:"modified"`
	Revision   int64  `json:"revision"`
	ResourceID string `json:"resource_id"`
	MD5        string `json:"md5"`
	SHA256     string `json:"sha256"`
	Embedded   struct {
		Items  []yandexResource `json:"items"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
		Total  int              `json:"total"`
	} `json:"_embedded"`
}

type yandexDiskAbout struct {
	TotalSpace  uint64 `json:"total_space"`
	UsedSpace   uint64 `json:"used_space"`
	TrashSize   uint64 `json:"trash_size"`
	MaxFileSize uint64 `json:"max_file_size"`
	User        struct {
		UID         json.RawMessage `json:"uid"`
		Login       string          `json:"login"`
		DisplayName string          `json:"display_name"`
	} `json:"user"`
}

type yandexLink struct {
	Href      string `json:"href"`
	Method    string `json:"method"`
	Templated bool   `json:"templated"`
}

func normalizeYandexPath(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || raw == "/" || raw == "disk:" || raw == "disk:/" {
		return "disk:/", nil
	}
	if strings.HasPrefix(raw, "app:/") {
		cleaned := path.Clean("/" + strings.TrimPrefix(raw, "app:/"))
		return "app:" + cleaned, nil
	}
	raw = strings.TrimPrefix(raw, "disk:/")
	cleaned := path.Clean("/" + strings.TrimPrefix(raw, "/"))
	if cleaned == "/" {
		return "disk:/", nil
	}
	return "disk:" + cleaned, nil
}

func (b *yandexDiskBackend) Root() string { return b.root }

func (b *yandexDiskBackend) Normalize(raw string) (string, error) {
	normalized, err := normalizeYandexPath(raw)
	if err != nil {
		return "", err
	}
	if !yandexWithinRoot(b.root, normalized) {
		return "", os.ErrPermission
	}
	return normalized, nil
}

func yandexWithinRoot(root, candidate string) bool {
	if root == "disk:/" || root == "app:/" {
		return strings.HasPrefix(candidate, strings.TrimSuffix(root, "/"))
	}
	return candidate == root || strings.HasPrefix(candidate, strings.TrimSuffix(root, "/")+"/")
}

func yandexParts(p string) (prefix, rest string) {
	if strings.HasPrefix(p, "app:") {
		return "app:", strings.TrimPrefix(p, "app:")
	}
	return "disk:", strings.TrimPrefix(p, "disk:")
}

func (b *yandexDiskBackend) Join(base string, elems ...string) string {
	prefix, rest := yandexParts(base)
	parts := append([]string{rest}, elems...)
	joined := path.Clean(path.Join(parts...))
	if !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	result, err := b.Normalize(prefix + joined)
	if err != nil {
		return base
	}
	return result
}

func (b *yandexDiskBackend) Base(p string) string {
	_, rest := yandexParts(p)
	return path.Base(rest)
}

func (b *yandexDiskBackend) Dir(p string) string {
	if b.IsRoot(p) {
		return b.root
	}
	prefix, rest := yandexParts(p)
	parent, err := b.Normalize(prefix + path.Dir(rest))
	if err != nil {
		return b.root
	}
	return parent
}

func (b *yandexDiskBackend) IsRoot(p string) bool {
	normalized, err := b.Normalize(p)
	return err == nil && normalized == b.root
}

func (b *yandexDiskBackend) apiRequest(ctx context.Context, method, endpoint string, query url.Values, body io.Reader) (*http.Response, error) {
	requestURL := b.baseURL + endpoint
	if len(query) != 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "OAuth "+b.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := b.do(req)
	if err != nil {
		return nil, err
	}
	if resp.Request != nil && resp.Request.Method != method {
		_ = resp.Body.Close() // Response-body cleanup is best effort.
		return nil, fmt.Errorf("cloudfox: Yandex.Disk %s completed as %s after a redirect", method, resp.Request.Method)
	}
	return resp, nil
}

func readSmallResponse(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	return strings.TrimSpace(string(data))
}

func (b *yandexDiskBackend) getResource(ctx context.Context, location string, limit, offset int) (yandexResource, error) {
	query := url.Values{"path": {location}}
	if limit > 0 {
		query.Set("limit", fmt.Sprint(limit))
		query.Set("offset", fmt.Sprint(offset))
	}
	resp, err := b.apiRequest(ctx, http.MethodGet, "/resources", query, nil)
	if err != nil {
		return yandexResource{}, err
	}
	defer func() { _ = resp.Body.Close() }() // Response-body cleanup is best effort.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return yandexResource{}, mapProviderHTTPError(resp, readSmallResponse(resp))
	}
	var resource yandexResource
	if err := json.NewDecoder(resp.Body).Decode(&resource); err != nil {
		return yandexResource{}, fmt.Errorf("cloudfox: decode Yandex.Disk resource: %w", err)
	}
	return resource, nil
}

func yandexEntry(resource yandexResource) RemoteEntry {
	modified, _ := time.Parse(time.RFC3339Nano, resource.Modified)
	contentRevision := ""
	if resource.Type != "dir" {
		// The download fingerprint is built from Yandex's content revision,
		// hashes, resource identity and size. Invalid provider digests must not
		// become cache identities; Open will surface them during validation.
		contentRevision, _ = yandexDownloadFingerprint(resource)
	}
	return RemoteEntry{
		VFSItem: vfs.VFSItem{
			Name:     resource.Name,
			Size:     resource.Size,
			IsDir:    resource.Type == "dir",
			MTime:    modified,
			Revision: contentRevision,
		},
		Location:     resource.Path,
		TransferName: resource.Name,
		SizeKnown:    resource.Type != "dir",
		Revision:     yandexRevisionString(resource.Revision),
	}
}

func yandexRevisionString(revision int64) string {
	if revision <= 0 {
		return ""
	}
	return strconv.FormatInt(revision, 10)
}

func (b *yandexDiskBackend) ReadDir(ctx context.Context, location string, onChunk func([]RemoteEntry)) error {
	location, err := b.Normalize(location)
	if err != nil {
		return err
	}
	const pageSize = 1000
	offset := 0
	seen := make(map[string]struct{})
	for {
		resource, err := b.getResource(ctx, location, pageSize, offset)
		if err != nil {
			return err
		}
		items := make([]RemoteEntry, 0, len(resource.Embedded.Items))
		for _, item := range resource.Embedded.Items {
			if _, duplicate := seen[item.Path]; duplicate {
				continue
			}
			seen[item.Path] = struct{}{}
			items = append(items, yandexEntry(item))
		}
		if len(items) != 0 {
			onChunk(items)
		}
		offset += len(resource.Embedded.Items)
		if len(resource.Embedded.Items) == 0 || offset >= resource.Embedded.Total {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

func (b *yandexDiskBackend) Stat(ctx context.Context, location string) (RemoteEntry, error) {
	location, err := b.Normalize(location)
	if err != nil {
		return RemoteEntry{}, err
	}
	resource, err := b.getResource(ctx, location, 0, 0)
	if err != nil {
		return RemoteEntry{}, err
	}
	entry := yandexEntry(resource)
	if entry.Name == "" {
		entry.Name = b.Base(location)
	}
	return entry, nil
}

func (b *yandexDiskBackend) MkDir(ctx context.Context, location string) error {
	location, err := b.Normalize(location)
	if err != nil {
		return err
	}
	resp, err := b.apiRequest(ctx, http.MethodPut, "/resources", url.Values{"path": {location}}, nil)
	if err != nil {
		// The request may have reached Yandex before the transport failed. A
		// retry could race or turn a committed create into a misleading conflict.
		return &vfs.UnknownOperationStateError{Operation: "Yandex.Disk create directory", Err: err}
	}
	defer func() { _ = resp.Body.Close() }() // Response-body cleanup is best effort.
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return providerHTTPMutationError("Yandex.Disk create directory", resp, readSmallResponse(resp))
	}
	return nil
}

func (b *yandexDiskBackend) mutation(ctx context.Context, method, endpoint string, query url.Values) error {
	resp, err := b.apiRequest(ctx, method, endpoint, query, nil)
	if err != nil {
		return &vfs.UnknownOperationStateError{Operation: "Yandex.Disk " + method, Err: err}
	}
	defer func() { _ = resp.Body.Close() }() // Response-body cleanup is best effort.
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.StatusCode != http.StatusAccepted {
		return nil
	}
	if resp.StatusCode != http.StatusAccepted {
		return providerHTTPMutationError("Yandex.Disk "+method, resp, readSmallResponse(resp))
	}
	var link yandexLink
	if err := json.NewDecoder(resp.Body).Decode(&link); err != nil && resp.Header.Get("Location") == "" {
		return &vfs.UnknownOperationStateError{
			Operation: "Yandex.Disk asynchronous operation",
			Err:       fmt.Errorf("cloudfox: decode accepted operation response: %w", err),
		}
	}
	if link.Href == "" {
		link.Href = resp.Header.Get("Location")
	}
	if err := b.waitOperation(ctx, link.Href); err != nil {
		if errors.Is(err, errYandexOperationFailed) {
			return err
		}
		return &vfs.UnknownOperationStateError{Operation: "Yandex.Disk asynchronous operation", Err: err}
	}
	return nil
}

func (b *yandexDiskBackend) waitOperation(ctx context.Context, href string) error {
	if href == "" {
		return errors.New("cloudfox: Yandex.Disk accepted an operation without a status URL")
	}
	base, err := url.Parse(b.baseURL)
	if err != nil {
		return err
	}
	target, err := url.Parse(href)
	if err != nil {
		return err
	}
	if !target.IsAbs() {
		target = base.ResolveReference(target)
	}
	if target.User != nil || target.Fragment != "" || !strings.EqualFold(target.Scheme, base.Scheme) || !strings.EqualFold(target.Host, base.Host) {
		return errors.New("cloudfox: Yandex.Disk operation URL changed origin")
	}
	href = target.String()
	delay := 250 * time.Millisecond
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, href, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "OAuth "+b.token)
		resp, err := b.do(req)
		if err != nil {
			return err
		}
		if resp.Request != nil && resp.Request.Method != http.MethodGet {
			_ = resp.Body.Close() // Response-body cleanup is best effort.
			return fmt.Errorf("cloudfox: Yandex.Disk status GET completed as %s after a redirect", resp.Request.Method)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			message := readSmallResponse(resp)
			_ = resp.Body.Close() // Response-body cleanup is best effort.
			return mapProviderHTTPError(resp, message)
		}
		var status struct {
			Status string `json:"status"`
		}
		err = json.NewDecoder(resp.Body).Decode(&status)
		_ = resp.Body.Close() // Response-body cleanup is best effort.
		if err != nil {
			return fmt.Errorf("cloudfox: decode Yandex.Disk operation status: %w", err)
		}
		switch strings.ToLower(status.Status) {
		case "success":
			return nil
		case "failed":
			return errYandexOperationFailed
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		if delay < 4*time.Second {
			delay *= 2
		}
	}
}

func (b *yandexDiskBackend) Remove(ctx context.Context, location string) error {
	location, err := b.Normalize(location)
	if err != nil {
		return err
	}
	if b.IsRoot(location) {
		return os.ErrPermission
	}
	err = b.mutation(ctx, http.MethodDelete, "/resources", url.Values{"path": {location}, "permanently": {"true"}})
	// A transport/status-poll failure may be returned after the server has
	// committed the delete. Never keep serving a pre-mutation session copy.
	b.downloadCache().invalidate(location)
	return err
}

func (b *yandexDiskBackend) MoveToTrash(ctx context.Context, location string) error {
	location, err := b.Normalize(location)
	if err != nil {
		return err
	}
	if b.IsRoot(location) {
		return os.ErrPermission
	}
	err = b.mutation(ctx, http.MethodDelete, "/resources", url.Values{"path": {location}, "permanently": {"false"}})
	b.downloadCache().invalidate(location)
	return err
}

func (b *yandexDiskBackend) Rename(ctx context.Context, oldLocation, newLocation string) error {
	oldLocation, err := b.Normalize(oldLocation)
	if err != nil {
		return err
	}
	newLocation, err = b.Normalize(newLocation)
	if err != nil {
		return err
	}
	err = b.mutation(ctx, http.MethodPost, "/resources/move", url.Values{
		"from":      {oldLocation},
		"path":      {newLocation},
		"overwrite": {strconv.FormatBool(yandexDestinationOverwrite(ctx))},
	})
	b.downloadCache().invalidate(oldLocation)
	b.downloadCache().invalidate(newLocation)
	return err
}

func (b *yandexDiskBackend) Copy(ctx context.Context, oldLocation, newLocation string) error {
	oldLocation, err := b.Normalize(oldLocation)
	if err != nil {
		return err
	}
	newLocation, err = b.Normalize(newLocation)
	if err != nil {
		return err
	}
	err = b.mutation(ctx, http.MethodPost, "/resources/copy", url.Values{
		"from":      {oldLocation},
		"path":      {newLocation},
		"overwrite": {strconv.FormatBool(yandexDestinationOverwrite(ctx))},
	})
	b.downloadCache().invalidate(newLocation)
	return err
}

func yandexDestinationOverwrite(ctx context.Context) bool {
	overwrite, known := vfs.DestinationOverwrite(ctx)
	return !known || overwrite
}

func (b *yandexDiskBackend) getLink(ctx context.Context, endpoint, location, overwrite string) (yandexLink, error) {
	query := url.Values{"path": {location}}
	if overwrite != "" {
		query.Set("overwrite", overwrite)
	}
	resp, err := b.apiRequest(ctx, http.MethodGet, endpoint, query, nil)
	if err != nil {
		return yandexLink{}, err
	}
	defer func() { _ = resp.Body.Close() }() // Response-body cleanup is best effort.
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return yandexLink{}, mapProviderHTTPError(resp, readSmallResponse(resp))
	}
	var link yandexLink
	if err := json.NewDecoder(resp.Body).Decode(&link); err != nil {
		return yandexLink{}, fmt.Errorf("cloudfox: decode Yandex.Disk temporary URL: %w", err)
	}
	if link.Href == "" {
		return yandexLink{}, errors.New("cloudfox: Yandex.Disk returned an empty temporary URL")
	}
	link.Href, err = b.validateTemporaryURL(link.Href)
	if err != nil {
		return yandexLink{}, err
	}
	return link, nil
}

func (b *yandexDiskBackend) Open(ctx context.Context, location string) (vfs.ReadAtCloser, error) {
	location, err := b.Normalize(location)
	if err != nil {
		return nil, err
	}
	resource, err := b.getResource(ctx, location, 0, 0)
	if err != nil {
		return nil, err
	}
	if resource.Type == "dir" {
		return nil, os.ErrInvalid
	}
	fingerprint, err := yandexDownloadFingerprint(resource)
	if err != nil {
		return nil, err
	}
	if cached, ok := b.downloadCache().open(location, fingerprint); ok {
		return cached, nil
	}
	link, err := b.getLink(ctx, "/resources/download", location, "")
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link.Href, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.do(req)
	if err != nil {
		return nil, sanitizeYandexTransferError(err)
	}
	defer func() { _ = resp.Body.Close() }() // Response-body cleanup is best effort.
	if resp.Request != nil && resp.Request.Method != http.MethodGet {
		return nil, fmt.Errorf("cloudfox: Yandex.Disk download GET completed as %s after a redirect", resp.Request.Method)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// A CDN error document is untrusted and may echo its signed request URL.
		return nil, mapProviderHTTPError(sanitizeYandexTransferResponse(resp), "")
	}
	temp, err := yandexResponseToTemp(ctx, resp, resource, b.Base(location))
	if err != nil {
		return nil, err
	}
	latest, err := b.getResource(ctx, location, 0, 0)
	if err != nil {
		_ = temp.Close()
		return nil, fmt.Errorf("%w: Yandex.Disk could not validate the downloaded revision: %v", ErrRemoteObjectChanged, err)
	}
	if !yandexSameSnapshot(resource, latest) {
		_ = temp.Close()
		return nil, fmt.Errorf("%w: Yandex.Disk resource revision changed during download", ErrRemoteObjectChanged)
	}
	reader, err := b.downloadCache().install(location, fingerprint, temp)
	if err != nil {
		return nil, err
	}
	yandexReportTransfer(ctx, "Downloading", b.Base(location), 100)
	return reader, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (b *yandexDiskBackend) Create(ctx context.Context, location string) (io.WriteCloser, error) {
	location, err := b.Normalize(location)
	if err != nil {
		return nil, err
	}
	return newProviderSpoolWriter(ctx, b.Base(location), func(uploadCtx context.Context, file *os.File, size int64) error {
		// The overwrite decision has to be part of the upload-URL request. A
		// Stat-before-PUT check would race another client and could still replace
		// an object created after the check.
		link, err := b.getLink(uploadCtx, "/resources/upload", location, strconv.FormatBool(yandexDestinationOverwrite(uploadCtx)))
		if err != nil {
			return err
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		var body io.Reader = &yandexUploadProgressReader{r: file, ctx: uploadCtx, name: b.Base(location), total: size}
		req, err := http.NewRequestWithContext(uploadCtx, http.MethodPut, link.Href, body)
		if err != nil {
			return err
		}
		req.ContentLength = size
		resp, err := b.do(req)
		// Once Do is called the request body may have reached the storage host
		// even when no response is available. A stale cached predecessor would
		// be more dangerous than the cost of a conservative re-download.
		b.downloadCache().invalidate(location)
		if err != nil {
			return &vfs.UnknownOperationStateError{Operation: "Yandex.Disk upload", Err: sanitizeYandexTransferError(err)}
		}
		defer func() { _ = resp.Body.Close() }() // Response-body cleanup is best effort.
		if resp.Request != nil && resp.Request.Method != http.MethodPut {
			return &vfs.UnknownOperationStateError{
				Operation: "Yandex.Disk upload",
				Err:       fmt.Errorf("PUT completed as %s after a redirect", resp.Request.Method),
			}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// Do not surface a CDN body: some gateways echo the complete signed
			// URL, which is itself a temporary credential.
			return providerHTTPMutationError("Yandex.Disk upload", sanitizeYandexTransferResponse(resp), "")
		}
		yandexReportTransfer(uploadCtx, "Uploading", b.Base(location), 100)
		return nil
	})
}

func (b *yandexDiskBackend) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return ErrUnsupportedOperation
}

func (b *yandexDiskBackend) Capabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasServerSideCopy: true, HasServerSideMove: true, HasRandomAccess: false, HasAtomicNoReplaceRename: true, ReadAccess: vfs.ReadAccessMaterializeOnce, StorageClass: vfs.StorageClassNetwork}
}

func (b *yandexDiskBackend) Close() error {
	b.close.Do(func() {
		b.downloadCache().close()
		if b.client != nil {
			b.client.CloseIdleConnections()
		}
	})
	return nil
}

func (b *yandexDiskBackend) TransferName(location string) string { return b.Base(location) }

var _ Backend = (*yandexDiskBackend)(nil)
var _ BackendCopier = (*yandexDiskBackend)(nil)
var _ BackendTrasher = (*yandexDiskBackend)(nil)
var _ BackendTransferNamer = (*yandexDiskBackend)(nil)
var _ BackendPanelInfo = (*yandexDiskBackend)(nil)
