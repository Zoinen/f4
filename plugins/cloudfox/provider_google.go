package cloudfox

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/unxed/f4/vfs"
)

var (
	DefaultGoogleClientID     string
	DefaultGoogleClientSecret string
)

type GoogleDriveSettings struct {
	ClientID string `json:"client_id,omitempty"`
}

type GoogleDriveFactory struct {
	HTTPClient  *http.Client
	TokenUpdate func(context.Context, Connection, *oauth2.Token) (Connection, error)
}

func (f *GoogleDriveFactory) Provider() ProviderType { return ProviderGoogleDrive }

func (f *GoogleDriveFactory) settings(c Connection) (GoogleDriveSettings, error) {
	var settings GoogleDriveSettings
	if len(c.Settings) != 0 {
		if err := json.Unmarshal(c.Settings, &settings); err != nil {
			return settings, fmt.Errorf("cloudfox: decode Google Drive settings: %w", err)
		}
	}
	if strings.TrimSpace(settings.ClientID) == "" {
		settings.ClientID = strings.TrimSpace(DefaultGoogleClientID)
	}
	if settings.ClientID == "" {
		return settings, errors.New("cloudfox: Google OAuth client ID is required")
	}
	return settings, nil
}

func (f *GoogleDriveFactory) Validate(c Connection) error {
	_, err := f.settings(c)
	return err
}

func googleOAuthConfig(settings GoogleDriveSettings, secrets SecretValues, redirectURL string) *oauth2.Config {
	clientSecret := secrets["client_secret"]
	defaultClientID := strings.TrimSpace(DefaultGoogleClientID)
	// The release-level secret belongs only to the release-level client ID.
	// Applying it to a user-supplied client ID makes Google reject an otherwise
	// valid public-client PKCE flow and leaks one OAuth client's credential into
	// another client's token exchange.
	if clientSecret == "" && defaultClientID != "" &&
		strings.TrimSpace(settings.ClientID) == defaultClientID {
		clientSecret = DefaultGoogleClientSecret
	}
	return &oauth2.Config{
		ClientID:     settings.ClientID,
		ClientSecret: clientSecret,
		Endpoint:     google.Endpoint,
		RedirectURL:  redirectURL,
		Scopes:       []string{drive.DriveScope},
	}
}

// GoogleAuthorizationURL builds the browser URL for the desktop loopback
// flow. The caller owns the loopback listener and must verify state.
func GoogleAuthorizationURL(c Connection, secrets SecretValues, redirectURL, state, verifier string) (string, error) {
	settings, err := (&GoogleDriveFactory{}).settings(c)
	if err != nil {
		return "", err
	}
	if state == "" || verifier == "" || redirectURL == "" {
		return "", errors.New("cloudfox: Google OAuth state, verifier and redirect URL are required")
	}
	config := googleOAuthConfig(settings, secrets, redirectURL)
	return config.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier)), nil
}

func ExchangeGoogleAuthorizationCode(ctx context.Context, c Connection, secrets SecretValues, redirectURL, code, verifier string) (*oauth2.Token, error) {
	settings, err := (&GoogleDriveFactory{}).settings(c)
	if err != nil {
		return nil, err
	}
	config := googleOAuthConfig(settings, secrets, redirectURL)
	ctx = noRedirectOAuthContext(ctx)
	token, err := config.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		return nil, fmt.Errorf("cloudfox: exchange Google authorization code: %w", err)
	}
	return token, nil
}

type notifyingTokenSource struct {
	base       oauth2.TokenSource
	connection Connection
	callback   func(context.Context, Connection, *oauth2.Token) (Connection, error)
	ctx        context.Context
	mu         sync.Mutex
	last       string
}

func googleTokenFingerprint(token *oauth2.Token) string {
	if token == nil {
		return ""
	}
	return token.AccessToken + "\x00" + token.RefreshToken + "\x00" + token.Expiry.UTC().Format(time.RFC3339Nano)
}

func (s *notifyingTokenSource) Token() (*oauth2.Token, error) {
	token, err := s.base.Token()
	if err != nil {
		return nil, err
	}
	if s.callback != nil {
		fingerprint := googleTokenFingerprint(token)
		s.mu.Lock()
		changed := fingerprint != s.last
		if changed {
			updated, err := s.callback(s.ctx, s.connection.Clone(), token)
			if err != nil {
				s.mu.Unlock()
				return nil, err
			}
			if updated.ID != "" {
				s.connection = updated.Clone()
			}
			s.last = fingerprint
		}
		s.mu.Unlock()
	}
	return token, nil
}

func (f *GoogleDriveFactory) Open(ctx context.Context, c Connection, secrets SecretValues) (Backend, error) {
	settings, err := f.settings(c)
	if err != nil {
		return nil, err
	}
	refreshToken := strings.TrimSpace(secrets["refresh_token"])
	accessToken := strings.TrimSpace(secrets["access_token"])
	if refreshToken == "" && accessToken == "" {
		return nil, ErrAuthenticationRequired
	}
	token := &oauth2.Token{AccessToken: accessToken, RefreshToken: refreshToken, TokenType: "Bearer"}
	if expiresAt := strings.TrimSpace(secrets["expires_at"]); expiresAt != "" {
		if expiry, parseErr := time.Parse(time.RFC3339Nano, expiresAt); parseErr == nil {
			token.Expiry = expiry
		}
	}
	// Older profiles did not persist expiry. If a refresh token exists, force
	// one refresh instead of treating an old access token as valid forever.
	if accessToken == "" || (refreshToken != "" && token.Expiry.IsZero()) {
		token.Expiry = time.Now().Add(-time.Hour)
	}
	// The URI activation context ends as soon as the panel is mounted. Token
	// refreshes belong to the backend session and must remain valid until Close,
	// while still retaining transport values such as oauth2.HTTPClient.
	sessionCtx, sessionCancel := context.WithCancel(noRedirectOAuthContext(context.WithoutCancel(ctx)))
	config := googleOAuthConfig(settings, secrets, "")
	source := config.TokenSource(sessionCtx, token)
	if f.TokenUpdate != nil {
		source = &notifyingTokenSource{base: source, connection: c.Clone(), callback: f.TokenUpdate, ctx: sessionCtx, last: googleTokenFingerprint(token)}
	}
	httpClient := f.HTTPClient
	if httpClient == nil {
		httpClient = oauth2.NewClient(sessionCtx, oauth2.ReuseTokenSource(token, source))
	}
	service, err := drive.NewService(sessionCtx, option.WithHTTPClient(httpClient), option.WithUserAgent("f4-cloudfox/1"))
	if err != nil {
		sessionCancel()
		return nil, fmt.Errorf("cloudfox: create Google Drive client: %w", err)
	}
	backend := &googleDriveBackend{
		service:       service,
		client:        httpClient,
		cancel:        sessionCancel,
		items:         make(map[string]*drive.File),
		parents:       make(map[string]string),
		names:         make(map[string]string),
		transferNames: make(map[string]string),
		resolved:      make(map[string]string),
		downloads:     newGoogleDownloadCache(),
	}
	if err := backend.refreshAbout(ctx); err != nil {
		_ = backend.Close()
		return nil, err
	}
	return backend, nil
}

const (
	googleRootLocation   = "g:root"
	googleMyLocation     = "g:my"
	googleSharedLocation = "g:shared"
	googleFolderMime     = "application/vnd.google-apps.folder"
	googleShortcutMime   = "application/vnd.google-apps.shortcut"
	googleDocMime        = "application/vnd.google-apps.document"
	googleSheetMime      = "application/vnd.google-apps.spreadsheet"
	googleSlidesMime     = "application/vnd.google-apps.presentation"
	googleFileFields     = "id,name,mimeType,size,modifiedTime,version,md5Checksum,resourceKey,driveId,parents,shortcutDetails(targetId,targetMimeType,targetResourceKey),capabilities(canCopy,canDelete,canTrash)"
	googleFileListFields = "nextPageToken,files(" + googleFileFields + ")"
)

func googleEncode(value string) string { return base64.RawURLEncoding.EncodeToString([]byte(value)) }
func googleDecode(value string) (string, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	return string(data), err
}

type googleLocation struct {
	kind              string
	driveID           string
	itemID            string
	targetID          string
	resourceKey       string
	targetResourceKey string
	parent            string
	name              string
}

func parseGoogleLocation(raw string) (googleLocation, error) {
	switch raw {
	case googleRootLocation:
		return googleLocation{kind: "root"}, nil
	case googleMyLocation:
		return googleLocation{kind: "my"}, nil
	case googleSharedLocation:
		return googleLocation{kind: "shared"}, nil
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 3 || parts[0] != "g" {
		return googleLocation{}, errors.New("cloudfox: invalid Google Drive location")
	}
	switch parts[1] {
	case "drive":
		if len(parts) != 3 {
			return googleLocation{}, errors.New("cloudfox: invalid shared drive location")
		}
		driveID, err := googleDecode(parts[2])
		return googleLocation{kind: "drive", driveID: driveID}, err
	case "item":
		if len(parts) != 4 && len(parts) != 5 {
			return googleLocation{}, errors.New("cloudfox: invalid Google Drive item location")
		}
		driveID, err := googleDecode(parts[2])
		if err != nil {
			return googleLocation{}, err
		}
		itemID, err := googleDecode(parts[3])
		if err != nil {
			return googleLocation{}, err
		}
		resourceKey := ""
		if len(parts) == 5 {
			resourceKey, err = googleDecode(parts[4])
		}
		return googleLocation{kind: "item", driveID: driveID, itemID: itemID, resourceKey: resourceKey}, err
	case "shortcut":
		if len(parts) != 5 && len(parts) != 7 {
			return googleLocation{}, errors.New("cloudfox: invalid Google Drive shortcut location")
		}
		driveID, err := googleDecode(parts[2])
		if err != nil {
			return googleLocation{}, err
		}
		itemID, err := googleDecode(parts[3])
		if err != nil {
			return googleLocation{}, err
		}
		targetID, err := googleDecode(parts[4])
		if err != nil {
			return googleLocation{}, err
		}
		resourceKey, targetResourceKey := "", ""
		if len(parts) == 7 {
			resourceKey, err = googleDecode(parts[5])
			if err != nil {
				return googleLocation{}, err
			}
			targetResourceKey, err = googleDecode(parts[6])
		}
		return googleLocation{kind: "shortcut", driveID: driveID, itemID: itemID, targetID: targetID, resourceKey: resourceKey, targetResourceKey: targetResourceKey}, err
	case "new":
		if len(parts) != 4 {
			return googleLocation{}, errors.New("cloudfox: invalid Google Drive destination location")
		}
		parent, err := googleDecode(parts[2])
		if err != nil {
			return googleLocation{}, err
		}
		name, err := googleDecode(parts[3])
		return googleLocation{kind: "new", parent: parent, name: name}, err
	default:
		return googleLocation{}, errors.New("cloudfox: unknown Google Drive location kind")
	}
}

func googleDriveLocation(driveID string) string {
	return "g:drive:" + googleEncode(driveID)
}

func googleItemLocation(driveID, itemID string) string {
	return googleItemLocationWithResourceKey(driveID, itemID, "")
}

func googleItemLocationWithResourceKey(driveID, itemID, resourceKey string) string {
	if resourceKey != "" {
		return "g:item:" + googleEncode(driveID) + ":" + googleEncode(itemID) + ":" + googleEncode(resourceKey)
	}
	return "g:item:" + googleEncode(driveID) + ":" + googleEncode(itemID)
}

func googleShortcutLocation(driveID, itemID, targetID string) string {
	return googleShortcutLocationWithResourceKeys(driveID, itemID, targetID, "", "")
}

func googleShortcutLocationWithResourceKeys(driveID, itemID, targetID, resourceKey, targetResourceKey string) string {
	if resourceKey != "" || targetResourceKey != "" {
		return "g:shortcut:" + googleEncode(driveID) + ":" + googleEncode(itemID) + ":" + googleEncode(targetID) + ":" + googleEncode(resourceKey) + ":" + googleEncode(targetResourceKey)
	}
	return "g:shortcut:" + googleEncode(driveID) + ":" + googleEncode(itemID) + ":" + googleEncode(targetID)
}

func googleNewLocation(parent, name string) string {
	return "g:new:" + googleEncode(parent) + ":" + googleEncode(name)
}

type googleDriveBackend struct {
	service   *drive.Service
	client    *http.Client
	cancel    context.CancelFunc
	close     sync.Once
	shareGate googleShareGate

	mu            sync.RWMutex
	items         map[string]*drive.File
	parents       map[string]string
	names         map[string]string
	transferNames map[string]string
	resolved      map[string]string
	about         *drive.About
	aboutAt       time.Time
	downloads     *googleDownloadCache
}

type googleDownloadCacheEntry struct {
	fingerprint string
	path        string
	size        int64
	readers     int
	retired     bool
	lastUsed    uint64
}

const (
	defaultGoogleCacheEntries = 8
	defaultGoogleCacheBytes   = int64(2 << 30)
)

type googleDownloadCache struct {
	mu         sync.Mutex
	entries    map[string]*googleDownloadCacheEntry
	retired    map[*googleDownloadCacheEntry]struct{}
	closed     bool
	bytes      int64
	clock      uint64
	maxEntries int
	maxBytes   int64
}

func newGoogleDownloadCache() *googleDownloadCache {
	return &googleDownloadCache{
		entries: make(map[string]*googleDownloadCacheEntry), retired: make(map[*googleDownloadCacheEntry]struct{}),
		maxEntries: defaultGoogleCacheEntries, maxBytes: defaultGoogleCacheBytes,
	}
}

type googleCachedReader struct {
	*os.File
	size  int64
	cache *googleDownloadCache
	entry *googleDownloadCacheEntry
	once  sync.Once
	err   error
}

func (r *googleCachedReader) Size() int64 { return r.size }
func (r *googleCachedReader) LocalPath() (string, bool) {
	if r == nil || r.File == nil || r.File.Name() == "" {
		return "", false
	}
	return r.File.Name(), true
}
func (r *googleCachedReader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.File.ReadAt(p, off)
}
func (r *googleCachedReader) Read(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.File.Read(p)
}

func (r *googleCachedReader) Close() error {
	r.once.Do(func() {
		r.err = r.File.Close()
		if r.cache != nil {
			r.cache.release(r.entry)
		}
	})
	return r.err
}

func newGoogleCachedReader(file *os.File, cache *googleDownloadCache, entry *googleDownloadCacheEntry) *googleCachedReader {
	return &googleCachedReader{File: file, size: entry.size, cache: cache, entry: entry}
}

func (c *googleDownloadCache) retireLocked(entry *googleDownloadCacheEntry) string {
	if entry == nil || entry.retired {
		return ""
	}
	entry.retired = true
	if entry.readers == 0 {
		return entry.path
	}
	c.retired[entry] = struct{}{}
	return ""
}

func (c *googleDownloadCache) touchLocked(entry *googleDownloadCacheEntry) {
	c.clock++
	entry.lastUsed = c.clock
}

func (c *googleDownloadCache) evictLocked(protected *googleDownloadCacheEntry) []string {
	var removePaths []string
	overBudget := func() bool {
		return (c.maxEntries > 0 && len(c.entries) > c.maxEntries) ||
			(c.maxBytes > 0 && c.bytes > c.maxBytes)
	}
	for overBudget() {
		oldestKey := ""
		var oldest *googleDownloadCacheEntry
		for key, entry := range c.entries {
			if entry == protected || entry.readers != 0 {
				continue
			}
			if oldest == nil || entry.lastUsed < oldest.lastUsed {
				oldestKey, oldest = key, entry
			}
		}
		if oldest == nil {
			for key, entry := range c.entries {
				if entry == protected {
					oldestKey, oldest = key, entry
					break
				}
			}
		}
		if oldest == nil {
			break
		}
		delete(c.entries, oldestKey)
		c.bytes -= oldest.size
		if retiredPath := c.retireLocked(oldest); retiredPath != "" {
			removePaths = append(removePaths, retiredPath)
		}
	}
	return removePaths
}

func (c *googleDownloadCache) release(entry *googleDownloadCacheEntry) {
	if c == nil || entry == nil {
		return
	}
	removePath := ""
	c.mu.Lock()
	if entry.readers > 0 {
		entry.readers--
	}
	if entry.retired && entry.readers == 0 {
		delete(c.retired, entry)
		removePath = entry.path
	}
	c.mu.Unlock()
	if removePath != "" {
		_ = os.Remove(removePath)
	}
}

func (c *googleDownloadCache) open(key, fingerprint string) (vfs.ReadAtCloser, bool) {
	if c == nil || fingerprint == "" {
		return nil, false
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, false
	}
	entry, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		return nil, false
	}
	if entry.fingerprint != fingerprint {
		delete(c.entries, key)
		c.bytes -= entry.size
		removePath := c.retireLocked(entry)
		c.mu.Unlock()
		if removePath != "" {
			_ = os.Remove(removePath)
		}
		return nil, false
	}
	f, err := os.Open(entry.path)
	if err != nil {
		delete(c.entries, key)
		c.bytes -= entry.size
		removePath := c.retireLocked(entry)
		c.mu.Unlock()
		if removePath != "" {
			_ = os.Remove(removePath)
		}
		return nil, false
	}
	entry.readers++
	c.touchLocked(entry)
	c.mu.Unlock()
	return newGoogleCachedReader(f, c, entry), true
}

func (c *googleDownloadCache) install(key, fingerprint string, temp *providerTempReader) (vfs.ReadAtCloser, error) {
	if c == nil || fingerprint == "" {
		return temp, nil
	}
	if c.maxBytes > 0 && temp.size > c.maxBytes {
		return temp, nil
	}
	path, size, err := temp.detach()
	if err != nil {
		if path != "" {
			_ = os.Remove(path)
		}
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		f, openErr := os.Open(path)
		if openErr != nil {
			_ = os.Remove(path)
			return nil, openErr
		}
		return newProviderTempReader(f, path, size), nil
	}
	var removePaths []string
	if current, ok := c.entries[key]; ok && current.fingerprint == fingerprint {
		if f, openErr := os.Open(current.path); openErr == nil {
			current.readers++
			c.touchLocked(current)
			c.mu.Unlock()
			_ = os.Remove(path)
			return newGoogleCachedReader(f, c, current), nil
		}
		delete(c.entries, key)
		c.bytes -= current.size
		if retiredPath := c.retireLocked(current); retiredPath != "" {
			removePaths = append(removePaths, retiredPath)
		}
	}
	if old, ok := c.entries[key]; ok {
		delete(c.entries, key)
		c.bytes -= old.size
		if retiredPath := c.retireLocked(old); retiredPath != "" {
			removePaths = append(removePaths, retiredPath)
		}
	}
	f, openErr := os.Open(path)
	if openErr != nil {
		c.mu.Unlock()
		_ = os.Remove(path)
		for _, retiredPath := range removePaths {
			_ = os.Remove(retiredPath)
		}
		return nil, openErr
	}
	entry := &googleDownloadCacheEntry{fingerprint: fingerprint, path: path, size: size, readers: 1}
	c.touchLocked(entry)
	c.entries[key] = entry
	c.bytes += size
	removePaths = append(removePaths, c.evictLocked(entry)...)
	c.mu.Unlock()
	for _, retiredPath := range removePaths {
		_ = os.Remove(retiredPath)
	}
	return newGoogleCachedReader(f, c, entry), nil
}

func (c *googleDownloadCache) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	var paths []string
	for _, entry := range c.entries {
		if retiredPath := c.retireLocked(entry); retiredPath != "" {
			paths = append(paths, retiredPath)
		}
	}
	c.entries = make(map[string]*googleDownloadCacheEntry)
	c.bytes = 0
	c.mu.Unlock()
	for _, path := range paths {
		_ = os.Remove(path)
	}
}

func (c *googleDownloadCache) invalidate(fileID string) {
	if c == nil || fileID == "" {
		return
	}
	var removePaths []string
	c.mu.Lock()
	for key, entry := range c.entries {
		if key == fileID || strings.HasPrefix(key, fileID+"|") {
			delete(c.entries, key)
			c.bytes -= entry.size
			if retiredPath := c.retireLocked(entry); retiredPath != "" {
				removePaths = append(removePaths, retiredPath)
			}
		}
	}
	c.mu.Unlock()
	for _, removePath := range removePaths {
		_ = os.Remove(removePath)
	}
}

func googleFileFingerprint(file *drive.File, etag, exportMime string) string {
	if file == nil {
		return ""
	}
	return fmt.Sprintf("%s|%d|%d|%s|%s|%s", etag, file.Version, file.Size, file.Md5Checksum, file.ModifiedTime, exportMime)
}

func googleContentRevision(file *drive.File) string {
	if file == nil {
		return ""
	}
	if file.Version != 0 {
		return fmt.Sprintf("gdrive:v%d:%s:%d", file.Version, file.Md5Checksum, file.Size)
	}
	if file.Md5Checksum != "" {
		return "gdrive:md5:" + file.Md5Checksum
	}
	return ""
}

func googleDownloadFingerprint(file *drive.File, etag, exportMime string) string {
	if etag == "" && googleContentRevision(file) == "" {
		return ""
	}
	return googleFileFingerprint(file, etag, exportMime)
}

func (b *googleDriveBackend) downloadCache() *googleDownloadCache {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.downloads == nil {
		b.downloads = newGoogleDownloadCache()
	}
	return b.downloads
}

func (b *googleDriveBackend) Root() string { return googleRootLocation }

func (b *googleDriveBackend) CanonicalLocation(location string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for range 32 {
		next := b.resolved[location]
		if next == "" || next == location {
			return location
		}
		location = next
	}
	return location
}

func (b *googleDriveBackend) rememberCanonicalLocation(requested, actual string) {
	if requested == "" || actual == "" || requested == actual {
		return
	}
	b.mu.Lock()
	if b.resolved == nil {
		b.resolved = make(map[string]string)
	}
	b.resolved[requested] = actual
	b.mu.Unlock()
}

func (b *googleDriveBackend) Normalize(location string) (string, error) {
	if location == "" {
		return googleRootLocation, nil
	}
	_, err := parseGoogleLocation(location)
	if err != nil {
		return "", err
	}
	return b.CanonicalLocation(location), nil
}

func (b *googleDriveBackend) Join(base string, elems ...string) string {
	current := b.CanonicalLocation(base)
	for _, name := range elems {
		if name == "" || name == "." {
			continue
		}
		if name == ".." {
			current = b.Dir(current)
			continue
		}
		current = googleNewLocation(current, name)
	}
	return current
}

func (b *googleDriveBackend) Base(location string) string {
	location = b.CanonicalLocation(location)
	b.mu.RLock()
	name := b.names[location]
	b.mu.RUnlock()
	if name != "" {
		return name
	}
	parsed, err := parseGoogleLocation(location)
	if err != nil {
		return ""
	}
	switch parsed.kind {
	case "root":
		return "/"
	case "my":
		return "My Drive"
	case "shared":
		return "Shared drives"
	case "new":
		return parsed.name
	case "drive", "item", "shortcut":
		// Opaque Drive and item IDs are API identities, never user-facing path
		// components. Their display names must come from a listing or metadata
		// lookup; returning the ID here can permanently leak it into bookmarks
		// and folder history while that metadata is still being hydrated.
		return ""
	default:
		return ""
	}
}

func (b *googleDriveBackend) Dir(location string) string {
	location = b.CanonicalLocation(location)
	b.mu.RLock()
	parent := b.parents[location]
	b.mu.RUnlock()
	if parent != "" {
		return parent
	}
	parsed, err := parseGoogleLocation(location)
	if err != nil {
		return googleRootLocation
	}
	switch parsed.kind {
	case "my", "shared":
		return googleRootLocation
	case "drive":
		return googleSharedLocation
	case "new":
		return parsed.parent
	default:
		return googleRootLocation
	}
}

func (b *googleDriveBackend) IsRoot(location string) bool { return location == googleRootLocation }

func mapGoogleError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		switch apiErr.Code {
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

func googleMutationError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) {
		if apiErr.Code >= 400 && apiErr.Code < 500 && apiErr.Code != http.StatusRequestTimeout {
			return mapGoogleError(err)
		}
	}
	return &vfs.UnknownOperationStateError{Operation: "Google Drive " + operation, Err: mapGoogleError(err)}
}

func googleExport(mimeType string) (extension, exportMime string, ok bool) {
	switch mimeType {
	case googleDocMime:
		return ".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true
	case googleSheetMime:
		return ".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true
	case googleSlidesMime:
		return ".pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", true
	default:
		return "", "", false
	}
}

func googleIsNative(mimeType string) bool {
	return strings.HasPrefix(mimeType, "application/vnd.google-apps.") && mimeType != googleFolderMime && mimeType != googleShortcutMime
}

func (b *googleDriveBackend) locationForFile(file *drive.File) string {
	if file.MimeType == googleShortcutMime && file.ShortcutDetails != nil {
		return googleShortcutLocationWithResourceKeys(file.DriveId, file.Id, file.ShortcutDetails.TargetId, file.ResourceKey, file.ShortcutDetails.TargetResourceKey)
	}
	return googleItemLocationWithResourceKey(file.DriveId, file.Id, file.ResourceKey)
}

func (b *googleDriveBackend) parentLocation(file *drive.File, fallback string) string {
	// Callers that obtained file through a known directory listing already
	// know its visual parent. Google may return the My Drive root as its opaque
	// folder ID rather than the literal alias "root" in file.parents; preferring
	// the listing parent prevents that internal ID from entering the display
	// hierarchy before ancestor metadata has been hydrated.
	if fallback != "" && fallback != googleRootLocation {
		return fallback
	}
	if len(file.Parents) == 0 {
		return fallback
	}
	parentID := file.Parents[0]
	if file.DriveId == "" && parentID == "root" {
		return googleMyLocation
	}
	if file.DriveId != "" && parentID == file.DriveId {
		return googleDriveLocation(file.DriveId)
	}
	return googleItemLocation(file.DriveId, parentID)
}

func (b *googleDriveBackend) entryForFile(file *drive.File, parent string) RemoteEntry {
	location := b.locationForFile(file)
	name := file.Name
	transferName := name
	transferMime := file.MimeType
	if file.MimeType == googleShortcutMime && file.ShortcutDetails != nil {
		transferMime = file.ShortcutDetails.TargetMimeType
	}
	if extension, _, ok := googleExport(transferMime); ok {
		if !strings.HasSuffix(strings.ToLower(name), extension) {
			name += extension
		}
		transferName = name
	}
	modified, _ := time.Parse(time.RFC3339Nano, file.ModifiedTime)
	isDir := file.MimeType == googleFolderMime
	isSymlink := file.MimeType == googleShortcutMime
	if isSymlink && file.ShortcutDetails != nil && file.ShortcutDetails.TargetMimeType == googleFolderMime {
		isDir = true
	}
	entry := RemoteEntry{
		VFSItem:  vfs.VFSItem{Name: name, Size: file.Size, IsDir: isDir, IsSymlink: isSymlink, MTime: modified, Revision: googleContentRevision(file)},
		Location: location, TransferName: transferName,
		SizeKnown: !isDir && !googleIsNative(transferMime),
		Revision:  googleFileFingerprint(file, "", ""),
	}
	b.mu.Lock()
	b.items[location] = file
	b.parents[location] = b.parentLocation(file, parent)
	b.names[location] = name
	b.transferNames[location] = transferName
	b.mu.Unlock()
	return entry
}

func (b *googleDriveBackend) getFile(ctx context.Context, itemID string, resourceKeys ...string) (*drive.File, error) {
	resourceKey := ""
	if len(resourceKeys) != 0 {
		resourceKey = resourceKeys[0]
	}
	call := b.service.Files.Get(itemID).
		SupportsAllDrives(true).
		Fields(googleFileFields).
		Context(ctx)
	setGoogleShareResourceKey(call.Header(), itemID, resourceKey)
	file, err := call.Do()
	if err != nil {
		return nil, mapGoogleError(err)
	}
	if file != nil && file.ResourceKey == "" {
		file.ResourceKey = resourceKey
	}
	return file, nil
}

type googleResourceKeyPair struct {
	itemID string
	key    string
}

func setGoogleResourceKeys(header http.Header, pairs ...googleResourceKeyPair) {
	seen := make(map[string]struct{})
	values := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		if pair.itemID == "" || pair.key == "" {
			continue
		}
		value := pair.itemID + "/" + pair.key
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		values = append(values, value)
	}
	if len(values) != 0 {
		header.Set("X-Goog-Drive-Resource-Keys", strings.Join(values, ","))
	}
}

func (b *googleDriveBackend) cacheGoogleFile(requested string, file *drive.File, parent string) RemoteEntry {
	entry := b.entryForFile(file, parent)
	b.rememberCanonicalLocation(requested, entry.Location)
	return entry
}

func (b *googleDriveBackend) invalidateGoogleFile(fileID string) {
	if fileID == "" {
		return
	}
	b.mu.Lock()
	for location, file := range b.items {
		if file == nil || file.Id != fileID {
			continue
		}
		delete(b.items, location)
		delete(b.parents, location)
		delete(b.names, location)
		delete(b.transferNames, location)
	}
	for requested, actual := range b.resolved {
		parsed, parsedErr := parseGoogleLocation(actual)
		if parsedErr == nil && (parsed.itemID == fileID || parsed.targetID == fileID) {
			delete(b.resolved, requested)
		}
	}
	downloads := b.downloads
	b.mu.Unlock()
	if downloads != nil {
		downloads.invalidate(fileID)
	}
}

func (b *googleDriveBackend) hydrateGoogleAncestors(ctx context.Context, file *drive.File, fallback string) error {
	if file == nil {
		return os.ErrInvalid
	}
	entry := b.cacheGoogleFile(fallback, file, b.Dir(fallback))
	location := b.Dir(entry.Location)
	seen := make(map[string]struct{})
	for location != "" && location != googleRootLocation && location != googleMyLocation && location != googleSharedLocation {
		if _, duplicate := seen[location]; duplicate {
			return errors.New("cloudfox: Google Drive returned a cyclic parent hierarchy")
		}
		seen[location] = struct{}{}
		parsed, err := parseGoogleLocation(location)
		if err != nil {
			return err
		}
		if parsed.kind == "drive" {
			sharedDrive, err := b.service.Drives.Get(parsed.driveID).Fields("id,name").Context(ctx).Do()
			if err != nil {
				return mapGoogleError(err)
			}
			b.mu.Lock()
			b.names[location] = sharedDrive.Name
			b.parents[location] = googleSharedLocation
			b.mu.Unlock()
			return nil
		}
		if parsed.kind != "item" {
			return nil
		}
		parent, err := b.getFile(ctx, parsed.itemID, parsed.resourceKey)
		if err != nil {
			return err
		}
		parentEntry := b.cacheGoogleFile(location, parent, googleRootLocation)
		location = b.Dir(parentEntry.Location)
	}
	return nil
}

func escapeGoogleQuery(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	return strings.ReplaceAll(value, "'", "\\'")
}

func (b *googleDriveBackend) readParent(ctx context.Context, location string) (parentID, driveID, resourceKey string, err error) {
	location = b.CanonicalLocation(location)
	parsed, err := parseGoogleLocation(location)
	if err != nil {
		return "", "", "", err
	}
	switch parsed.kind {
	case "my":
		return "root", "", "", nil
	case "drive":
		return parsed.driveID, parsed.driveID, "", nil
	case "item", "shortcut":
		itemID := parsed.itemID
		resourceKey := parsed.resourceKey
		if parsed.kind == "shortcut" {
			itemID = parsed.targetID
			resourceKey = parsed.targetResourceKey
			if resourceKey == "" {
				shortcut, getErr := b.getFile(ctx, parsed.itemID, parsed.resourceKey)
				if getErr != nil {
					return "", "", "", getErr
				}
				if shortcut.ShortcutDetails != nil {
					resourceKey = shortcut.ShortcutDetails.TargetResourceKey
				}
			}
		}
		file, err := b.getFile(ctx, itemID, resourceKey)
		if err != nil {
			return "", "", "", err
		}
		if file.MimeType != googleFolderMime {
			return "", "", "", os.ErrInvalid
		}
		return file.Id, file.DriveId, file.ResourceKey, nil
	case "new":
		file, _, resolveErr := b.resolveNew(ctx, parsed)
		if resolveErr != nil {
			return "", "", "", resolveErr
		}
		if file.MimeType != googleFolderMime {
			return "", "", "", os.ErrInvalid
		}
		b.cacheGoogleFile(location, file, parsed.parent)
		return file.Id, file.DriveId, file.ResourceKey, nil
	default:
		return "", "", "", os.ErrInvalid
	}
}

func (b *googleDriveBackend) ReadDir(ctx context.Context, location string, onChunk func([]RemoteEntry)) error {
	if _, err := b.Normalize(location); err != nil {
		return err
	}
	switch location {
	case googleRootLocation:
		onChunk([]RemoteEntry{
			{VFSItem: vfs.VFSItem{Name: "My Drive", IsDir: true}, Location: googleMyLocation, TransferName: "My Drive"},
			{VFSItem: vfs.VFSItem{Name: "Shared drives", IsDir: true}, Location: googleSharedLocation, TransferName: "Shared drives"},
		})
		return nil
	case googleSharedLocation:
		pageToken := ""
		for {
			call := b.service.Drives.List().PageSize(100).Fields("nextPageToken,drives(id,name)").Context(ctx)
			if pageToken != "" {
				call.PageToken(pageToken)
			}
			result, err := call.Do()
			if err != nil {
				return mapGoogleError(err)
			}
			entries := make([]RemoteEntry, 0, len(result.Drives))
			for _, sharedDrive := range result.Drives {
				location := googleDriveLocation(sharedDrive.Id)
				entries = append(entries, RemoteEntry{VFSItem: vfs.VFSItem{Name: sharedDrive.Name, IsDir: true}, Location: location, TransferName: sharedDrive.Name})
				b.mu.Lock()
				b.names[location] = sharedDrive.Name
				b.parents[location] = googleSharedLocation
				b.mu.Unlock()
			}
			if len(entries) != 0 {
				onChunk(entries)
			}
			if result.NextPageToken == "" {
				return nil
			}
			pageToken = result.NextPageToken
		}
	}
	parentID, driveID, parentResourceKey, err := b.readParent(ctx, location)
	if err != nil {
		return err
	}
	pageToken := ""
	for {
		call := b.service.Files.List().
			Q(fmt.Sprintf("'%s' in parents and trashed = false", escapeGoogleQuery(parentID))).
			Spaces("drive").PageSize(1000).SupportsAllDrives(true).IncludeItemsFromAllDrives(true).
			Fields(googleFileListFields).Context(ctx)
		setGoogleShareResourceKey(call.Header(), parentID, parentResourceKey)
		if driveID != "" {
			call = call.Corpora("drive").DriveId(driveID)
		}
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		result, err := call.Do()
		if err != nil {
			return mapGoogleError(err)
		}
		entries := make([]RemoteEntry, 0, len(result.Files))
		for _, file := range result.Files {
			entries = append(entries, b.entryForFile(file, location))
		}
		if len(entries) != 0 {
			onChunk(entries)
		}
		if result.NextPageToken == "" {
			return nil
		}
		pageToken = result.NextPageToken
	}
}

func (b *googleDriveBackend) resolveNew(ctx context.Context, parsed googleLocation) (*drive.File, string, error) {
	parentID, driveID, parentResourceKey, err := b.readParent(ctx, parsed.parent)
	if err != nil {
		return nil, "", err
	}
	call := b.service.Files.List().
		Q(fmt.Sprintf("'%s' in parents and name = '%s' and trashed = false", escapeGoogleQuery(parentID), escapeGoogleQuery(parsed.name))).
		Spaces("drive").PageSize(2).SupportsAllDrives(true).IncludeItemsFromAllDrives(true).
		Fields("files(" + googleFileFields + ")").Context(ctx)
	setGoogleShareResourceKey(call.Header(), parentID, parentResourceKey)
	if driveID != "" {
		call = call.Corpora("drive").DriveId(driveID)
	}
	result, err := call.Do()
	if err != nil {
		return nil, "", mapGoogleError(err)
	}
	if len(result.Files) == 0 {
		return nil, parentID, os.ErrNotExist
	}
	if len(result.Files) > 1 {
		return nil, parentID, fmt.Errorf("cloudfox: Google Drive destination %q is ambiguous", parsed.name)
	}
	b.cacheGoogleFile(googleNewLocation(parsed.parent, parsed.name), result.Files[0], parsed.parent)
	return result.Files[0], parentID, nil
}

func (b *googleDriveBackend) fileForLocation(ctx context.Context, location string) (*drive.File, error) {
	location = b.CanonicalLocation(location)
	parsed, err := parseGoogleLocation(location)
	if err != nil {
		return nil, err
	}
	switch parsed.kind {
	case "item", "shortcut":
		b.mu.RLock()
		cached := b.items[location]
		b.mu.RUnlock()
		if cached != nil {
			return cached, nil
		}
		return b.getFile(ctx, parsed.itemID, parsed.resourceKey)
	case "new":
		file, _, err := b.resolveNew(ctx, parsed)
		return file, err
	default:
		return nil, os.ErrInvalid
	}
}

func (b *googleDriveBackend) Stat(ctx context.Context, location string) (RemoteEntry, error) {
	location = b.CanonicalLocation(location)
	switch location {
	case googleRootLocation:
		return RemoteEntry{VFSItem: vfs.VFSItem{Name: "/", IsDir: true}, Location: location, TransferName: "/"}, nil
	case googleMyLocation:
		return RemoteEntry{VFSItem: vfs.VFSItem{Name: "My Drive", IsDir: true}, Location: location, TransferName: "My Drive"}, nil
	case googleSharedLocation:
		return RemoteEntry{VFSItem: vfs.VFSItem{Name: "Shared drives", IsDir: true}, Location: location, TransferName: "Shared drives"}, nil
	}
	parsed, err := parseGoogleLocation(location)
	if err != nil {
		return RemoteEntry{}, err
	}
	if parsed.kind == "drive" {
		sharedDrive, err := b.service.Drives.Get(parsed.driveID).Fields("id,name").Context(ctx).Do()
		if err != nil {
			return RemoteEntry{}, mapGoogleError(err)
		}
		return RemoteEntry{VFSItem: vfs.VFSItem{Name: sharedDrive.Name, IsDir: true}, Location: location, TransferName: sharedDrive.Name}, nil
	}
	var file *drive.File
	switch parsed.kind {
	case "item", "shortcut":
		// Stat is the authoritative content-identity boundary used by higher
		// level caches. A row remembered from ReadDir may be arbitrarily stale
		// after another client edits the same immutable Drive ID.
		file, err = b.getFile(ctx, parsed.itemID, parsed.resourceKey)
	case "new":
		file, _, err = b.resolveNew(ctx, parsed)
	default:
		err = os.ErrInvalid
	}
	if err != nil {
		return RemoteEntry{}, err
	}
	if err := b.hydrateGoogleAncestors(ctx, file, location); err != nil && ctx.Err() != nil {
		return RemoteEntry{}, ctx.Err()
	}
	entry := b.entryForFile(file, b.Dir(location))
	if file.MimeType == googleShortcutMime && file.ShortcutDetails != nil && file.ShortcutDetails.TargetId != "" {
		target, targetErr := b.getFile(ctx, file.ShortcutDetails.TargetId, file.ShortcutDetails.TargetResourceKey)
		if targetErr != nil {
			return RemoteEntry{}, targetErr
		}
		targetLocation := b.locationForFile(target)
		b.cacheGoogleFile(targetLocation, target, b.Dir(targetLocation))
		entry.Size = target.Size
		entry.SizeKnown = target.MimeType != googleFolderMime && !googleIsNative(target.MimeType)
		entry.VFSItem.Revision = googleContentRevision(target)
		entry.Revision = googleFileFingerprint(target, "", "")
		if modified, parseErr := time.Parse(time.RFC3339Nano, target.ModifiedTime); parseErr == nil {
			entry.MTime = modified
		}
	}
	b.rememberCanonicalLocation(location, entry.Location)
	return entry, nil
}

func (b *googleDriveBackend) destination(ctx context.Context, location string) (parentID, parentResourceKey, name string, existing *drive.File, err error) {
	location = b.CanonicalLocation(location)
	parsed, err := parseGoogleLocation(location)
	if err != nil {
		return "", "", "", nil, err
	}
	if parsed.kind == "new" {
		parentID, _, parentResourceKey, err = b.readParent(ctx, parsed.parent)
		if err != nil {
			return "", "", "", nil, err
		}
		existing, _, findErr := b.resolveNew(ctx, parsed)
		if findErr != nil && !errors.Is(findErr, os.ErrNotExist) {
			return "", "", "", nil, findErr
		}
		return parentID, parentResourceKey, parsed.name, existing, nil
	}
	existing, err = b.fileForLocation(ctx, location)
	if err != nil {
		return "", "", "", nil, err
	}
	if len(existing.Parents) == 0 {
		return "", "", "", nil, os.ErrPermission
	}
	return existing.Parents[0], "", existing.Name, existing, nil
}

func (b *googleDriveBackend) MkDir(ctx context.Context, location string) error {
	parentID, parentResourceKey, name, existing, err := b.destination(ctx, location)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.MimeType == googleFolderMime {
			return nil
		}
		return os.ErrExist
	}
	call := b.service.Files.Create(&drive.File{Name: name, MimeType: googleFolderMime, Parents: []string{parentID}}).
		SupportsAllDrives(true).Fields(googleFileFields).Context(ctx)
	setGoogleShareResourceKey(call.Header(), parentID, parentResourceKey)
	created, err := call.Do()
	if err == nil && (created == nil || created.Id == "") {
		return &vfs.UnknownOperationStateError{Operation: "Google Drive create directory", Err: errors.New("successful response did not include the created folder id")}
	}
	if err == nil {
		b.cacheGoogleFile(location, created, b.Dir(location))
	}
	return googleMutationError("create directory", err)
}

func (b *googleDriveBackend) Remove(ctx context.Context, location string) error {
	file, err := b.fileForLocation(ctx, location)
	if err != nil {
		return err
	}
	if file.Capabilities != nil && !file.Capabilities.CanDelete {
		return os.ErrPermission
	}
	call := b.service.Files.Delete(file.Id).SupportsAllDrives(true).Context(ctx)
	setGoogleShareResourceKey(call.Header(), file.Id, file.ResourceKey)
	mutationErr := googleMutationError("delete", call.Do())
	if mutationErr == nil || errors.Is(mutationErr, vfs.ErrOperationStateUnknown) {
		b.invalidateGoogleFile(file.Id)
	}
	return mutationErr
}

func (b *googleDriveBackend) MoveToTrash(ctx context.Context, location string) error {
	file, err := b.fileForLocation(ctx, location)
	if err != nil {
		return err
	}
	if file.Capabilities != nil && !file.Capabilities.CanTrash {
		return os.ErrPermission
	}
	call := b.service.Files.Update(file.Id, &drive.File{Trashed: true, ForceSendFields: []string{"Trashed"}}).
		SupportsAllDrives(true).Fields("id,trashed").Context(ctx)
	setGoogleShareResourceKey(call.Header(), file.Id, file.ResourceKey)
	_, err = call.Do()
	mutationErr := googleMutationError("trash", err)
	if mutationErr == nil || errors.Is(mutationErr, vfs.ErrOperationStateUnknown) {
		b.invalidateGoogleFile(file.Id)
	}
	return mutationErr
}

func stripGoogleExportExtension(name, mimeType string) string {
	if extension, _, ok := googleExport(mimeType); ok && strings.HasSuffix(strings.ToLower(name), extension) {
		return name[:len(name)-len(extension)]
	}
	return name
}

func (b *googleDriveBackend) moveMetadata(ctx context.Context, file *drive.File, parentID, parentResourceKey, name string) error {
	resourceKey := file.ResourceKey
	shortcutDetails := file.ShortcutDetails
	targetResourceKey := ""
	if file.ShortcutDetails != nil {
		targetResourceKey = file.ShortcutDetails.TargetResourceKey
	}
	call := b.service.Files.Update(file.Id, &drive.File{Name: stripGoogleExportExtension(name, file.MimeType)}).
		SupportsAllDrives(true).Fields(googleFileFields).Context(ctx)
	if len(file.Parents) == 0 || file.Parents[0] != parentID {
		call = call.AddParents(parentID)
		if len(file.Parents) != 0 {
			call = call.RemoveParents(strings.Join(file.Parents, ","))
		}
	}
	setGoogleResourceKeys(call.Header(), googleResourceKeyPair{file.Id, file.ResourceKey}, googleResourceKeyPair{parentID, parentResourceKey})
	updated, err := call.Do()
	mutationErr := googleMutationError("move", err)
	if mutationErr != nil {
		if errors.Is(mutationErr, vfs.ErrOperationStateUnknown) {
			b.invalidateGoogleFile(file.Id)
		}
		return mutationErr
	}
	if updated == nil || updated.Id == "" {
		b.invalidateGoogleFile(file.Id)
		return &vfs.UnknownOperationStateError{Operation: "Google Drive move", Err: errors.New("successful response did not include the moved file id")}
	}
	if updated.ResourceKey == "" {
		updated.ResourceKey = resourceKey
	}
	if updated.ShortcutDetails == nil {
		updated.ShortcutDetails = shortcutDetails
	} else if updated.ShortcutDetails.TargetResourceKey == "" {
		updated.ShortcutDetails.TargetResourceKey = targetResourceKey
	}
	*file = *updated
	b.cacheGoogleFile("", updated, googleRootLocation)
	return nil
}

func (b *googleDriveBackend) Rename(ctx context.Context, oldLocation, newLocation string) error {
	oldFile, err := b.fileForLocation(ctx, oldLocation)
	if err != nil {
		return err
	}
	parentID, parentResourceKey, name, existing, err := b.destination(ctx, newLocation)
	if err != nil {
		return err
	}
	if overwrite, known := vfs.DestinationOverwrite(ctx); known && !overwrite && existing != nil && existing.Id != oldFile.Id {
		return os.ErrExist
	}
	if existing == nil || existing.Id == oldFile.Id {
		err := b.moveMetadata(ctx, oldFile, parentID, parentResourceKey, name)
		if err == nil {
			b.rememberCanonicalLocation(newLocation, b.locationForFile(oldFile))
		}
		return err
	}
	if googleIsNative(existing.MimeType) && !googleIsNative(oldFile.MimeType) {
		return ErrReadOnlyObject
	}
	backupName := existing.Name + ".f4bak-" + existing.Id[:min(8, len(existing.Id))]
	// destination already resolved the parent that contains existing. Using it
	// also avoids assuming that every Drive response includes a Parents entry.
	if err := b.moveMetadata(ctx, existing, parentID, parentResourceKey, backupName); err != nil {
		return err
	}
	if err := b.moveMetadata(ctx, oldFile, parentID, parentResourceKey, name); err != nil {
		rollbackCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		rollbackErr := b.moveMetadata(rollbackCtx, existing, parentID, parentResourceKey, existing.Name)
		cancel()
		if rollbackErr != nil {
			return &vfs.UnknownOperationStateError{
				Operation: "Google Drive replacement rollback",
				Err:       errors.Join(err, rollbackErr),
			}
		}
		return err
	}
	deleteCall := b.service.Files.Delete(existing.Id).SupportsAllDrives(true).Context(ctx)
	setGoogleShareResourceKey(deleteCall.Header(), existing.Id, existing.ResourceKey)
	if err := deleteCall.Do(); err != nil {
		return &vfs.PartialOperationError{
			Operation: "Google Drive replacement cleanup",
			Completed: []string{oldLocation, newLocation},
			Failed:    []string{backupName},
			Err:       googleMutationError("replacement cleanup", err),
		}
	}
	b.invalidateGoogleFile(existing.Id)
	b.rememberCanonicalLocation(newLocation, b.locationForFile(oldFile))
	return nil
}

func (b *googleDriveBackend) Copy(ctx context.Context, oldLocation, newLocation string) error {
	oldFile, err := b.fileForLocation(ctx, oldLocation)
	if err != nil {
		return err
	}
	if oldFile.Capabilities != nil && !oldFile.Capabilities.CanCopy {
		return os.ErrPermission
	}
	parentID, parentResourceKey, name, existing, err := b.destination(ctx, newLocation)
	if err != nil {
		return err
	}
	if overwrite, known := vfs.DestinationOverwrite(ctx); known && !overwrite && existing != nil {
		return os.ErrExist
	}
	if existing != nil && googleIsNative(existing.MimeType) && !googleIsNative(oldFile.MimeType) {
		return ErrReadOnlyObject
	}
	copyCall := b.service.Files.Copy(oldFile.Id, &drive.File{Name: stripGoogleExportExtension(name, oldFile.MimeType), Parents: []string{parentID}}).
		SupportsAllDrives(true).Fields(googleFileFields).Context(ctx)
	setGoogleResourceKeys(copyCall.Header(), googleResourceKeyPair{oldFile.Id, oldFile.ResourceKey}, googleResourceKeyPair{parentID, parentResourceKey})
	created, err := copyCall.Do()
	if err != nil {
		return googleMutationError("copy", err)
	}
	if created == nil || created.Id == "" {
		// The copy request may already have committed remotely, but without an
		// ID we cannot distinguish the new object from the old destination.
		// Never delete the destination in this indeterminate state.
		return &vfs.UnknownOperationStateError{
			Operation: "Google Drive copy",
			Err:       errors.New("successful response did not include the copied file id"),
		}
	}
	if existing != nil && existing.Id != created.Id {
		deleteCall := b.service.Files.Delete(existing.Id).SupportsAllDrives(true).Context(ctx)
		setGoogleShareResourceKey(deleteCall.Header(), existing.Id, existing.ResourceKey)
		if err := deleteCall.Do(); err != nil {
			return &vfs.PartialOperationError{
				Operation: "Google Drive copy replacement cleanup",
				Completed: []string{newLocation},
				Failed:    []string{existing.Name},
				Err:       googleMutationError("copy replacement cleanup", err),
			}
		}
		b.invalidateGoogleFile(existing.Id)
	}
	b.cacheGoogleFile(newLocation, created, b.Dir(newLocation))
	return nil
}

type googleRangeReader struct {
	service     *drive.Service
	fileID      string
	resourceKey string
	size        int64
	etag        string
	ctx         context.Context
	cancel      context.CancelFunc
	once        sync.Once
	mu          sync.Mutex
	offset      int64

	// Drive normally honors Range for blob files. Keep a local fallback on
	// the handle when an intermediary unexpectedly answers a range request
	// with the complete object instead of HTTP 206.
	fallbackMu  sync.RWMutex
	fallback    vfs.ReadAtCloser
	cache       *googleDownloadCache
	cacheKey    string
	fingerprint string
	displayName string
}

func (r *googleRangeReader) Size() int64 { return r.size }
func (r *googleRangeReader) LocalPath() (string, bool) {
	local := r.localFallback()
	if local == nil {
		return "", false
	}
	backing, ok := local.(interface{ LocalPath() (string, bool) })
	if !ok {
		return "", false
	}
	return backing.LocalPath()
}
func (r *googleRangeReader) Close() error {
	var closeErr error
	r.once.Do(func() {
		r.cancel()
		r.fallbackMu.Lock()
		if r.fallback != nil {
			closeErr = r.fallback.Close()
			r.fallback = nil
		}
		r.fallbackMu.Unlock()
	})
	return closeErr
}

func (r *googleRangeReader) localFallback() vfs.ReadAtCloser {
	r.fallbackMu.RLock()
	defer r.fallbackMu.RUnlock()
	return r.fallback
}

func (r *googleRangeReader) installFallback(ctx context.Context, resp *http.Response) (vfs.ReadAtCloser, error) {
	local, err := googleResponseToTemp(ctx, resp, r.displayName)
	if err != nil {
		return nil, err
	}
	if local.Size() != r.size {
		_ = local.Close()
		return nil, fmt.Errorf("cloudfox: Google Drive returned %d bytes for a %d-byte file", local.Size(), r.size)
	}
	var installed vfs.ReadAtCloser = local
	if r.cache != nil && r.fingerprint != "" {
		installed, err = r.cache.install(r.cacheKey, r.fingerprint, local)
		if err != nil {
			return nil, err
		}
	}
	r.fallbackMu.Lock()
	if r.fallback == nil {
		r.fallback = installed
	} else {
		_ = installed.Close()
	}
	result := r.fallback
	r.fallbackMu.Unlock()
	return result, nil
}

func (r *googleRangeReader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if local := r.localFallback(); local != nil {
		return local.ReadAt(ctx, p, off)
	}
	end, count, err := requestedByteRange(r.size, len(p), off)
	if err != nil || count == 0 {
		return 0, err
	}
	requestCtx, done := providerOperationContext(ctx, r.ctx)
	defer done()
	call := r.service.Files.Get(r.fileID).SupportsAllDrives(true).Context(requestCtx)
	setGoogleShareResourceKey(call.Header(), r.fileID, r.resourceKey)
	call.Header().Set("Range", fmt.Sprintf("bytes=%d-%d", off, end))
	call.Header().Set("If-Match", r.etag)
	resp, err := call.Download()
	if err != nil {
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusPreconditionFailed {
			return 0, ErrRemoteObjectChanged
		}
		return 0, mapGoogleError(err)
	}
	if resp.StatusCode == http.StatusOK {
		local, fallbackErr := r.installFallback(requestCtx, resp)
		if fallbackErr != nil {
			return 0, fallbackErr
		}
		return local.ReadAt(ctx, p, off)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		return 0, errors.New("cloudfox: Google Drive stopped honoring byte ranges")
	}
	if err := validateContentRange(resp.Header.Get("Content-Range"), off, end, r.size); err != nil {
		return 0, err
	}
	if responseETag := strongETag(resp.Header.Get("ETag")); responseETag != "" && responseETag != r.etag {
		return 0, ErrRemoteObjectChanged
	}
	n, readErr := io.ReadFull(resp.Body, p[:count])
	if readErr == io.ErrUnexpectedEOF {
		readErr = io.EOF
	}
	if n < len(p) && readErr == nil {
		readErr = io.EOF
	}
	return n, readErr
}

func (r *googleRangeReader) Read(ctx context.Context, p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.ReadAt(ctx, p, r.offset)
	r.offset += int64(n)
	return n, err
}

func googleResponseToTemp(ctx context.Context, resp *http.Response, displayName string) (*providerTempReader, error) {
	defer resp.Body.Close()
	f, err := os.CreateTemp("", "f4-cloudfox-google-export-*")
	if err != nil {
		return nil, err
	}
	name := f.Name()
	cleanup := func() {
		f.Close()
		os.Remove(name)
	}
	if err := f.Chmod(0o600); err != nil {
		cleanup()
		return nil, err
	}
	written, err := copyGoogleResponse(ctx, f, resp.Body, resp.ContentLength, displayName)
	if err != nil {
		cleanup()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, err
	}
	return newProviderTempReader(f, name, written), nil
}

func copyGoogleResponse(ctx context.Context, dst io.Writer, src io.Reader, total int64, displayName string) (int64, error) {
	update, _ := ctx.Value(vfs.ProgressKey).(vfs.ProgressCallback)
	reporter, _ := ctx.Value(vfs.ReporterKey).(vfs.TaskReporter)
	buffer := make([]byte, 256*1024)
	var written int64
	lastPercent := -1
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			count, writeErr := dst.Write(buffer[:n])
			written += int64(count)
			if writeErr != nil {
				return written, writeErr
			}
			if count != n {
				return written, io.ErrShortWrite
			}
			if (update != nil || reporter != nil) && total > 0 {
				percent := int(written * 100 / total)
				if percent > 100 {
					percent = 100
				}
				if percent != lastPercent {
					if update != nil {
						update("Downloading file...", percent)
					}
					if reporter != nil {
						reporter.UpdateTransfer("Downloading", displayName, percent, "", percent, "")
					}
					lastPercent = percent
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				if lastPercent != 100 {
					if update != nil {
						update("Downloading file...", 100)
					}
					if reporter != nil {
						reporter.UpdateTransfer("Downloading", displayName, 100, "", 100, "")
					}
				}
				return written, nil
			}
			return written, readErr
		}
	}
}

func (b *googleDriveBackend) Open(ctx context.Context, location string) (vfs.ReadAtCloser, error) {
	file, err := b.fileForLocation(ctx, location)
	if err != nil {
		return nil, err
	}
	if file.MimeType == googleShortcutMime && file.ShortcutDetails != nil {
		file, err = b.getFile(ctx, file.ShortcutDetails.TargetId, file.ShortcutDetails.TargetResourceKey)
		if err != nil {
			return nil, err
		}
	}
	if file.MimeType == googleFolderMime {
		return nil, os.ErrInvalid
	}
	downloads := b.downloadCache()
	if _, exportMime, ok := googleExport(file.MimeType); ok {
		file, err = b.getFile(ctx, file.Id, file.ResourceKey)
		if err != nil {
			return nil, err
		}
		b.cacheGoogleFile(location, file, b.Dir(location))
		fingerprint := googleDownloadFingerprint(file, "", exportMime)
		cacheKey := file.Id + "|" + exportMime
		if cached, ok := downloads.open(cacheKey, fingerprint); ok {
			return cached, nil
		}
		exportCall := b.service.Files.Export(file.Id, exportMime).Context(ctx)
		setGoogleShareResourceKey(exportCall.Header(), file.Id, file.ResourceKey)
		resp, err := exportCall.Download()
		if err != nil {
			return nil, mapGoogleError(err)
		}
		temp, err := googleResponseToTemp(ctx, resp, b.Base(location))
		if err != nil {
			return nil, err
		}
		return downloads.install(cacheKey, fingerprint, temp)
	}
	if googleIsNative(file.MimeType) {
		return nil, fmt.Errorf("%w: unsupported Google Workspace type %s", ErrReadOnlyObject, file.MimeType)
	}
	// Refresh metadata once at Open so every range can be conditioned on the
	// same object generation instead of trusting a potentially stale list row.
	file, err = b.getFile(ctx, file.Id, file.ResourceKey)
	if err != nil {
		return nil, err
	}
	b.cacheGoogleFile(location, file, b.Dir(location))
	etag := strongETag(file.ServerResponse.Header.Get("ETag"))
	fingerprint := googleDownloadFingerprint(file, etag, "")
	if cached, ok := downloads.open(file.Id, fingerprint); ok {
		return cached, nil
	}
	if etag == "" {
		downloadCall := b.service.Files.Get(file.Id).SupportsAllDrives(true).Context(ctx)
		setGoogleShareResourceKey(downloadCall.Header(), file.Id, file.ResourceKey)
		resp, err := downloadCall.Download()
		if err != nil {
			return nil, mapGoogleError(err)
		}
		temp, err := googleResponseToTemp(ctx, resp, b.Base(location))
		if err != nil {
			return nil, err
		}
		return downloads.install(file.Id, fingerprint, temp)
	}
	readerCtx, cancel := context.WithCancel(context.Background())
	return &googleRangeReader{service: b.service, fileID: file.Id, resourceKey: file.ResourceKey, size: file.Size, etag: etag, ctx: readerCtx, cancel: cancel, cache: downloads, cacheKey: file.Id, fingerprint: fingerprint, displayName: b.Base(location)}, nil
}

type googleUploadWriter struct {
	ctx    context.Context
	cancel context.CancelFunc
	pipe   *io.PipeWriter
	done   chan error
	once   sync.Once
	err    error
}

func (w *googleUploadWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.pipe.Write(p)
}

func (w *googleUploadWriter) Close() error {
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

func (w *googleUploadWriter) Abort() error {
	w.once.Do(func() {
		// A resumable upload cannot publish the replacement until it receives
		// the final EOF/chunk. Cancel first, then break the pipe without EOF.
		w.cancel()
		pipeErr := w.pipe.CloseWithError(context.Canceled)
		uploadErr := <-w.done
		if uploadErr != nil && !errors.Is(uploadErr, context.Canceled) {
			w.err = uploadErr
		} else if pipeErr != nil && !errors.Is(pipeErr, io.ErrClosedPipe) {
			w.err = pipeErr
		}
	})
	return w.err
}

func (b *googleDriveBackend) Create(ctx context.Context, location string) (io.WriteCloser, error) {
	parentID, parentResourceKey, name, existing, err := b.destination(ctx, location)
	if err != nil {
		return nil, err
	}
	if overwrite, known := vfs.DestinationOverwrite(ctx); known && !overwrite && existing != nil {
		return nil, os.ErrExist
	}
	if existing != nil && googleIsNative(existing.MimeType) {
		return nil, ErrReadOnlyObject
	}
	uploadTarget := existing
	shortcut := (*drive.File)(nil)
	if existing != nil && existing.MimeType == googleShortcutMime {
		if existing.ShortcutDetails == nil || existing.ShortcutDetails.TargetId == "" {
			return nil, fmt.Errorf("%w: Google Drive shortcut has no writable target", ErrReadOnlyObject)
		}
		shortcut = existing
		uploadTarget, err = b.getFile(ctx, existing.ShortcutDetails.TargetId, existing.ShortcutDetails.TargetResourceKey)
		if err != nil {
			return nil, err
		}
		if uploadTarget.MimeType == googleFolderMime || googleIsNative(uploadTarget.MimeType) || uploadTarget.MimeType == googleShortcutMime {
			return nil, ErrReadOnlyObject
		}
	}
	uploadCtx, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		var uploadErr error
		var uploaded *drive.File
		if uploadTarget != nil {
			call := b.service.Files.Update(uploadTarget.Id, &drive.File{}).SupportsAllDrives(true).
				Media(reader, googleapi.ChunkSize(8<<20)).Fields(googleFileFields).Context(uploadCtx)
			setGoogleShareResourceKey(call.Header(), uploadTarget.Id, uploadTarget.ResourceKey)
			uploaded, uploadErr = call.Do()
		} else {
			call := b.service.Files.Create(&drive.File{Name: name, Parents: []string{parentID}}).SupportsAllDrives(true).
				Media(reader, googleapi.ChunkSize(8<<20)).Fields(googleFileFields).Context(uploadCtx)
			setGoogleShareResourceKey(call.Header(), parentID, parentResourceKey)
			uploaded, uploadErr = call.Do()
		}
		if uploadErr == nil && (uploaded == nil || uploaded.Id == "") {
			uploadErr = &vfs.UnknownOperationStateError{Operation: "Google Drive upload", Err: errors.New("successful response did not include the uploaded file id")}
		}
		if uploadErr == nil && uploadTarget != nil && uploaded.Id != uploadTarget.Id {
			uploadErr = &vfs.UnknownOperationStateError{Operation: "Google Drive in-place upload", Err: errors.New("successful response changed the destination file id")}
		}
		mutationErr := uploadErr
		if mutationErr != nil && !errors.Is(mutationErr, vfs.ErrOperationStateUnknown) {
			mutationErr = googleMutationError("upload", mutationErr)
		}
		if uploadTarget != nil && (mutationErr == nil || errors.Is(mutationErr, vfs.ErrOperationStateUnknown)) {
			b.invalidateGoogleFile(uploadTarget.Id)
		}
		if mutationErr == nil {
			if uploadTarget != nil {
				if uploaded.Name == "" {
					uploaded.Name = uploadTarget.Name
				}
				if uploaded.MimeType == "" {
					uploaded.MimeType = uploadTarget.MimeType
				}
				if uploaded.ResourceKey == "" {
					uploaded.ResourceKey = uploadTarget.ResourceKey
				}
				if len(uploaded.Parents) == 0 {
					uploaded.Parents = append([]string(nil), uploadTarget.Parents...)
				}
			}
			if shortcut != nil {
				targetLocation := b.locationForFile(uploaded)
				b.cacheGoogleFile(targetLocation, uploaded, b.Dir(targetLocation))
				// The editor path remains the shortcut's identity; only its target
				// content changed. Reinstall the shortcut metadata invalidation of the
				// target cannot accidentally turn the panel row into the target item.
				b.entryForFile(shortcut, b.Dir(location))
			} else {
				b.cacheGoogleFile(location, uploaded, b.Dir(location))
			}
		}
		_ = reader.CloseWithError(mutationErr)
		done <- mutationErr
	}()
	return &googleUploadWriter{ctx: uploadCtx, cancel: cancel, pipe: writer, done: done}, nil
}

func (b *googleDriveBackend) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return ErrUnsupportedOperation
}

func (b *googleDriveBackend) Capabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{
		HasServerSideCopy:          true,
		HasServerSideMove:          true,
		HasRandomAccess:            true,
		HasIdentityPreservingWrite: true,
		HasAtomicNoReplaceRename:   true,
	}
}

func (b *googleDriveBackend) TransferName(location string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if name := b.transferNames[location]; name != "" {
		return name
	}
	return b.names[location]
}

func (b *googleDriveBackend) IntraSessionTransferName(location string) string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if file := b.items[location]; file != nil && file.Name != "" {
		return file.Name
	}
	return b.names[location]
}

func (b *googleDriveBackend) refreshAbout(ctx context.Context) error {
	about, err := b.service.About.Get().Fields("user(displayName,emailAddress),storageQuota(limit,usage)").Context(ctx).Do()
	if err != nil {
		return mapGoogleError(err)
	}
	b.mu.Lock()
	b.about = about
	b.aboutAt = time.Now()
	b.mu.Unlock()
	return nil
}

func (b *googleDriveBackend) PanelInfoKey(req vfs.PanelInfoRequest) string {
	return "gdrive:" + req.Path
}

func (b *googleDriveBackend) CachedPanelInfo(vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	b.mu.RLock()
	about, refreshed := b.about, b.aboutAt
	b.mu.RUnlock()
	return googlePanelSnapshot(about, refreshed), about != nil && time.Since(refreshed) < 5*time.Minute
}

func googlePanelSnapshot(about *drive.About, refreshed time.Time) vfs.PanelInfoSnapshot {
	snapshot := vfs.PanelInfoSnapshot{Authoritative: true, RefreshedAt: refreshed}
	if about == nil {
		return snapshot
	}
	account := vfs.PanelInfoSection{ID: "account", Title: "Google Drive"}
	if about.User != nil {
		account.Fields = append(account.Fields, vfs.PanelInfoField{ID: "user", Label: "Account", Value: about.User.DisplayName + " <" + about.User.EmailAddress + ">"})
	}
	if about.StorageQuota != nil && about.StorageQuota.Limit > 0 {
		available := uint64(0)
		if about.StorageQuota.Usage < about.StorageQuota.Limit {
			available = uint64(about.StorageQuota.Limit - about.StorageQuota.Usage)
		}
		account.Fields = append(account.Fields, vfs.PanelInfoField{ID: "quota", Label: "Storage", Kind: vfs.PanelInfoUsage, TotalBytes: uint64(about.StorageQuota.Limit), AvailableBytes: available})
	}
	snapshot.Sections = []vfs.PanelInfoSection{account}
	return snapshot
}

func (b *googleDriveBackend) RefreshPanelInfo(ctx context.Context, _ vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	if err := b.refreshAbout(ctx); err != nil {
		return vfs.PanelInfoSnapshot{}, err
	}
	snapshot, _ := b.CachedPanelInfo(vfs.PanelInfoRequest{})
	return snapshot, nil
}

func (b *googleDriveBackend) Close() error {
	b.close.Do(func() {
		if b.cancel != nil {
			b.cancel()
		}
		if b.client != nil {
			b.client.CloseIdleConnections()
		}
		b.downloads.close()
	})
	return nil
}

var _ Backend = (*googleDriveBackend)(nil)
var _ BackendCopier = (*googleDriveBackend)(nil)
var _ BackendTrasher = (*googleDriveBackend)(nil)
var _ BackendTransferNamer = (*googleDriveBackend)(nil)
var _ BackendIntraSessionNamer = (*googleDriveBackend)(nil)
var _ BackendPanelInfo = (*googleDriveBackend)(nil)
