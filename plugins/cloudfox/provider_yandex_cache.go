package cloudfox

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

const (
	defaultYandexCacheEntries = 8
	defaultYandexCacheBytes   = int64(2 << 30)
)

type yandexDownloadCacheEntry struct {
	fingerprint string
	path        string
	size        int64
	readers     int
	retired     bool
	lastUsed    uint64
}

// yandexDownloadCache owns complete, private downloads for one authenticated
// backend session. It is intentionally bounded by both count and bytes. An
// active reader is never invalidated underneath the viewer; on Windows its
// retired file is removed when the last descriptor closes.
type yandexDownloadCache struct {
	mu         sync.Mutex
	entries    map[string]*yandexDownloadCacheEntry
	retired    map[*yandexDownloadCacheEntry]struct{}
	closed     bool
	bytes      int64
	clock      uint64
	maxEntries int
	maxBytes   int64
}

func newYandexDownloadCache() *yandexDownloadCache {
	return &yandexDownloadCache{
		entries:    make(map[string]*yandexDownloadCacheEntry),
		retired:    make(map[*yandexDownloadCacheEntry]struct{}),
		maxEntries: defaultYandexCacheEntries,
		maxBytes:   defaultYandexCacheBytes,
	}
}

type yandexCachedReader struct {
	*os.File
	size  int64
	cache *yandexDownloadCache
	entry *yandexDownloadCacheEntry
	once  sync.Once
	err   error
}

func (r *yandexCachedReader) Size() int64 { return r.size }
func (r *yandexCachedReader) LocalPath() (string, bool) {
	if r.File == nil || r.File.Name() == "" {
		return "", false
	}
	return r.File.Name(), true
}
func (r *yandexCachedReader) Read(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.File.Read(p)
}
func (r *yandexCachedReader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.File.ReadAt(p, off)
}
func (r *yandexCachedReader) Close() error {
	r.once.Do(func() {
		r.err = r.File.Close()
		if r.cache != nil {
			r.cache.release(r.entry)
		}
	})
	return r.err
}

func (c *yandexDownloadCache) touchLocked(entry *yandexDownloadCacheEntry) {
	c.clock++
	entry.lastUsed = c.clock
}

func (c *yandexDownloadCache) retireLocked(entry *yandexDownloadCacheEntry) string {
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

func (c *yandexDownloadCache) release(entry *yandexDownloadCacheEntry) {
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

func (c *yandexDownloadCache) evictLocked(protected *yandexDownloadCacheEntry) []string {
	var removePaths []string
	overBudget := func() bool {
		return (c.maxEntries > 0 && len(c.entries) > c.maxEntries) ||
			(c.maxBytes > 0 && c.bytes > c.maxBytes)
	}
	for overBudget() {
		oldestKey := ""
		var oldest *yandexDownloadCacheEntry
		for key, entry := range c.entries {
			if entry == protected || entry.readers != 0 {
				continue
			}
			if oldest == nil || entry.lastUsed < oldest.lastUsed {
				oldestKey, oldest = key, entry
			}
		}
		if oldest == nil {
			// Do not let a new cache entry exceed the budget merely because all
			// older entries are in use. Keep this response private to its caller.
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

func (c *yandexDownloadCache) open(key, fingerprint string) (vfs.ReadAtCloser, bool) {
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
	return &yandexCachedReader{File: f, size: entry.size, cache: c, entry: entry}, true
}

func (c *yandexDownloadCache) install(key, fingerprint string, temp *providerTempReader) (vfs.ReadAtCloser, error) {
	if c == nil || fingerprint == "" || (c.maxBytes > 0 && temp.size > c.maxBytes) {
		return temp, nil
	}
	tempPath, size, err := temp.detach()
	if err != nil {
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
		return nil, err
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		f, openErr := os.Open(tempPath)
		if openErr != nil {
			_ = os.Remove(tempPath)
			return nil, openErr
		}
		return newProviderTempReader(f, tempPath, size), nil
	}
	var removePaths []string
	if current, ok := c.entries[key]; ok && current.fingerprint == fingerprint {
		if f, openErr := os.Open(current.path); openErr == nil {
			current.readers++
			c.touchLocked(current)
			c.mu.Unlock()
			_ = os.Remove(tempPath)
			return &yandexCachedReader{File: f, size: current.size, cache: c, entry: current}, nil
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
	f, openErr := os.Open(tempPath)
	if openErr != nil {
		c.mu.Unlock()
		_ = os.Remove(tempPath)
		for _, retiredPath := range removePaths {
			_ = os.Remove(retiredPath)
		}
		return nil, openErr
	}
	entry := &yandexDownloadCacheEntry{fingerprint: fingerprint, path: tempPath, size: size, readers: 1}
	c.touchLocked(entry)
	c.entries[key] = entry
	c.bytes += size
	removePaths = append(removePaths, c.evictLocked(entry)...)
	c.mu.Unlock()
	for _, retiredPath := range removePaths {
		_ = os.Remove(retiredPath)
	}
	return &yandexCachedReader{File: f, size: size, cache: c, entry: entry}, nil
}

func (c *yandexDownloadCache) invalidate(location string) {
	if c == nil {
		return
	}
	prefix := strings.TrimSuffix(location, "/") + "/"
	var removePaths []string
	c.mu.Lock()
	for key, entry := range c.entries {
		if key == location || strings.HasPrefix(key, prefix) {
			delete(c.entries, key)
			c.bytes -= entry.size
			if retiredPath := c.retireLocked(entry); retiredPath != "" {
				removePaths = append(removePaths, retiredPath)
			}
		}
	}
	c.mu.Unlock()
	for _, retiredPath := range removePaths {
		_ = os.Remove(retiredPath)
	}
}

func (c *yandexDownloadCache) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	var removePaths []string
	for _, entry := range c.entries {
		if retiredPath := c.retireLocked(entry); retiredPath != "" {
			removePaths = append(removePaths, retiredPath)
		}
	}
	c.entries = make(map[string]*yandexDownloadCacheEntry)
	c.bytes = 0
	c.mu.Unlock()
	for _, cachedPath := range removePaths {
		_ = os.Remove(cachedPath)
	}
}

func (b *yandexDiskBackend) downloadCache() *yandexDownloadCache {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.downloads == nil {
		b.downloads = newYandexDownloadCache()
	}
	return b.downloads
}

func validatedYandexDigest(value string, bytes int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != bytes {
		return "", errors.New("cloudfox: Yandex.Disk returned an invalid content digest")
	}
	return strings.ToLower(value), nil
}

func yandexDownloadFingerprint(resource yandexResource) (string, error) {
	sha, err := validatedYandexDigest(resource.SHA256, sha256.Size)
	if err != nil {
		return "", err
	}
	md5sum, err := validatedYandexDigest(resource.MD5, md5.Size)
	if err != nil {
		return "", err
	}
	if resource.Size < 0 {
		return "", errors.New("cloudfox: Yandex.Disk returned a negative resource size")
	}
	if resource.Revision <= 0 && sha == "" && md5sum == "" {
		return "", nil
	}
	return fmt.Sprintf("%d|%d|%s|%s|%s|%s", resource.Revision, resource.Size, resource.ResourceID, resource.Modified, sha, md5sum), nil
}

func yandexSameSnapshot(before, after yandexResource) bool {
	beforeFingerprint, beforeErr := yandexDownloadFingerprint(before)
	afterFingerprint, afterErr := yandexDownloadFingerprint(after)
	if beforeErr != nil || afterErr != nil {
		return false
	}
	if beforeFingerprint != "" || afterFingerprint != "" {
		return beforeFingerprint != "" && beforeFingerprint == afterFingerprint
	}
	return before.Path == after.Path && before.Type == after.Type && before.Size == after.Size && before.Modified == after.Modified
}

func yandexExpectedHasher(resource yandexResource) (hash.Hash, string, error) {
	sha, err := validatedYandexDigest(resource.SHA256, sha256.Size)
	if err != nil {
		return nil, "", err
	}
	md5sum, err := validatedYandexDigest(resource.MD5, md5.Size)
	if err != nil {
		return nil, "", err
	}
	if sha != "" {
		return sha256.New(), sha, nil
	}
	if md5sum != "" {
		return md5.New(), md5sum, nil
	}
	return nil, "", nil
}

func yandexReportTransfer(ctx context.Context, action, name string, percent int) {
	if reporter, ok := ctx.Value(vfs.ReporterKey).(vfs.TaskReporter); ok && reporter != nil {
		reporter.UpdateTransfer(action, name, percent, "", percent, "")
	}
	if update, ok := ctx.Value(vfs.ProgressKey).(vfs.ProgressCallback); ok && update != nil {
		message := "Downloading file..."
		if action == "Uploading" {
			message = "Uploading file..."
		}
		update(message, percent)
	}
}

func copyYandexResponse(ctx context.Context, dst io.Writer, src io.Reader, expectedSize int64, displayName string, digest hash.Hash) (int64, error) {
	yandexReportTransfer(ctx, "Downloading", displayName, 0)
	buffer := make([]byte, 256*1024)
	var written int64
	lastPercent := 0
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		if reporter, ok := ctx.Value(vfs.ReporterKey).(vfs.TaskReporter); ok && reporter != nil && reporter.IsCancelled() {
			return written, context.Canceled
		}
		n, readErr := src.Read(buffer)
		if n > 0 {
			count, writeErr := dst.Write(buffer[:n])
			written += int64(count)
			if digest != nil && count > 0 {
				_, _ = digest.Write(buffer[:count])
			}
			if writeErr != nil {
				return written, writeErr
			}
			if count != n {
				return written, io.ErrShortWrite
			}
			if expectedSize > 0 {
				percent := int(written * 100 / expectedSize)
				if percent >= 100 {
					percent = 99
				}
				if percent != lastPercent {
					yandexReportTransfer(ctx, "Downloading", displayName, percent)
					lastPercent = percent
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func yandexResponseToTemp(ctx context.Context, resp *http.Response, resource yandexResource, displayName string) (*providerTempReader, error) {
	if resource.Size < 0 {
		return nil, errors.New("cloudfox: Yandex.Disk returned a negative resource size")
	}
	if resp.ContentLength >= 0 && resp.ContentLength != resource.Size {
		return nil, fmt.Errorf("%w: Yandex.Disk response size changed from %d to %d bytes", ErrRemoteObjectChanged, resource.Size, resp.ContentLength)
	}
	digest, expectedDigest, err := yandexExpectedHasher(resource)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp("", "f4-cloudfox-yandex-cache-*")
	if err != nil {
		return nil, err
	}
	tempPath := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tempPath)
	}
	if err := f.Chmod(0o600); err != nil {
		cleanup()
		return nil, err
	}
	var body io.Reader = &contextReader{ctx: ctx, r: resp.Body}
	const maxInt64 = int64(^uint64(0) >> 1)
	if resource.Size < maxInt64 {
		body = io.LimitReader(body, resource.Size+1)
	}
	written, err := copyYandexResponse(ctx, f, body, resource.Size, displayName, digest)
	if err != nil {
		cleanup()
		return nil, err
	}
	if written != resource.Size {
		cleanup()
		return nil, fmt.Errorf("%w: Yandex.Disk response size changed from %d to %d bytes", ErrRemoteObjectChanged, resource.Size, written)
	}
	if digest != nil && !strings.EqualFold(hex.EncodeToString(digest.Sum(nil)), expectedDigest) {
		cleanup()
		return nil, fmt.Errorf("%w: Yandex.Disk response content digest did not match metadata", ErrRemoteObjectChanged)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, err
	}
	return newProviderTempReader(f, tempPath, written), nil
}

type yandexUploadProgressReader struct {
	r       io.Reader
	ctx     context.Context
	name    string
	total   int64
	read    int64
	started bool
	lastPct int
}

func (r *yandexUploadProgressReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if reporter, ok := r.ctx.Value(vfs.ReporterKey).(vfs.TaskReporter); ok && reporter != nil && reporter.IsCancelled() {
		return 0, context.Canceled
	}
	if !r.started {
		yandexReportTransfer(r.ctx, "Uploading", r.name, 0)
		r.started = true
	}
	n, err := r.r.Read(p)
	r.read += int64(n)
	if r.total > 0 {
		percent := int(r.read * 100 / r.total)
		if percent >= 100 {
			percent = 99
		}
		if percent != r.lastPct {
			yandexReportTransfer(r.ctx, "Uploading", r.name, percent)
			r.lastPct = percent
		}
	}
	return n, err
}
