package archive

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	archiveMaterializationMaxEntries = 8
	archiveMaterializationMaxBytes   = int64(4 << 30)
	archiveMaterializationIdleTTL    = 10 * time.Minute
)

func archiveReaderLocalPath(reader vfs.ReadAtCloser) (string, bool) {
	// Keep the reader as the cache cleanup lease: LocalBackingReader promises
	// that its provider-owned path remains valid until Close.
	if local, ok := reader.(vfs.LocalBackingReader); ok {
		if localPath, valid := local.LocalPath(); valid && localPath != "" {
			return localPath, true
		}
	}
	if legacy, ok := reader.(interface{ TempPath() string }); ok {
		if localPath := legacy.TempPath(); localPath != "" {
			return localPath, true
		}
	}
	if temporary, ok := reader.(*vfs.TempFileWrapper); ok && temporary.TempPath != "" {
		return temporary.TempPath, true
	}
	return "", false
}

type archiveMaterializationKey struct {
	session  any
	path     string
	revision string
	size     int64
	modified int64
}

type archiveMaterializationEntry struct {
	ready    chan struct{}
	path     string
	size     int64
	cleanup  func() error
	err      error
	refs     int
	lastUsed time.Time
}

type archiveMaterializationCache struct {
	mu      sync.Mutex
	entries map[archiveMaterializationKey]*archiveMaterializationEntry
	closed  bool
	timer   *time.Timer
}

type archiveMaterializationLease struct {
	cache         *archiveMaterializationCache
	entry         *archiveMaterializationEntry
	directCleanup func() error
	once          sync.Once
	err           error
}

func newArchiveMaterializationCache() *archiveMaterializationCache {
	return &archiveMaterializationCache{entries: make(map[archiveMaterializationKey]*archiveMaterializationEntry)}
}

var (
	sharedArchiveMaterializationsMu sync.Mutex
	sharedArchiveMaterializations   = newArchiveMaterializationCache()
)

func currentArchiveMaterializationCache() *archiveMaterializationCache {
	sharedArchiveMaterializationsMu.Lock()
	defer sharedArchiveMaterializationsMu.Unlock()
	return sharedArchiveMaterializations
}

func closeSharedArchiveMaterializations() {
	sharedArchiveMaterializationsMu.Lock()
	closing := sharedArchiveMaterializations
	sharedArchiveMaterializations = newArchiveMaterializationCache()
	sharedArchiveMaterializationsMu.Unlock()
	closing.close()
}

func (l *archiveMaterializationLease) Path() string {
	if l == nil || l.entry == nil {
		return ""
	}
	return l.entry.path
}

func (l *archiveMaterializationLease) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.cache != nil {
			l.cache.release(l.entry)
		} else if l.directCleanup != nil {
			l.err = l.directCleanup()
		}
	})
	return l.err
}

func archiveSessionCacheIdentity(parent vfs.VFS) (any, bool) {
	if parent == nil {
		return nil, false
	}
	if identified, ok := parent.(vfs.SessionIdentity); ok {
		identity := identified.SessionKey()
		if identity == nil {
			return nil, false
		}
		typ := reflect.TypeOf(identity)
		if typ != nil && typ.Comparable() {
			return identity, true
		}
		return nil, false
	}
	typ := reflect.TypeOf(parent)
	if typ != nil && typ.Comparable() {
		return parent, true
	}
	return nil, false
}

func archiveMaterializationCacheKey(ctx context.Context, parent vfs.VFS, archivePath string) (archiveMaterializationKey, bool) {
	identity, ok := archiveSessionCacheIdentity(parent)
	if !ok {
		return archiveMaterializationKey{}, false
	}
	absPath, err := parent.Abs(archivePath)
	if err != nil || absPath == "" {
		return archiveMaterializationKey{}, false
	}
	item, err := parent.Stat(ctx, archivePath)
	revision := strings.TrimSpace(item.Revision)
	// Size and modified time are deliberately insufficient: Drive and many
	// WebDAV servers allow an overwrite that preserves both. Only the VFS
	// strong-content-identity contract can authorize cross-mount reuse.
	if err != nil || item.IsDir || revision == "" || item.Size < 0 {
		return archiveMaterializationKey{}, false
	}
	return archiveMaterializationKey{
		session: identity, path: absPath, revision: revision, size: item.Size, modified: item.MTime.UnixNano(),
	}, true
}

func (c *archiveMaterializationCache) acquire(
	ctx context.Context,
	key archiveMaterializationKey,
	load func(context.Context) (path string, size int64, cleanup func() error, err error),
) (*archiveMaterializationLease, error) {
	now := time.Now()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, os.ErrClosed
	}
	if entry := c.entries[key]; entry != nil {
		entry.refs++
		entry.lastUsed = now
		ready := entry.ready
		c.mu.Unlock()
		select {
		case <-ready:
			if entry.err != nil {
				c.release(entry)
				return nil, entry.err
			}
			return &archiveMaterializationLease{cache: c, entry: entry}, nil
		case <-ctx.Done():
			c.release(entry)
			return nil, ctx.Err()
		}
	}
	entry := &archiveMaterializationEntry{ready: make(chan struct{}), refs: 1, lastUsed: now}
	c.entries[key] = entry
	c.mu.Unlock()

	localPath, size, cleanup, err := load(ctx)
	c.mu.Lock()
	entry.path, entry.size, entry.cleanup, entry.err = localPath, size, cleanup, err
	if err != nil {
		delete(c.entries, key)
	}
	close(entry.ready)
	cleanups := c.pruneLocked(time.Now())
	c.schedulePruneLocked()
	c.mu.Unlock()
	runArchiveCleanups(cleanups)

	if err != nil {
		c.release(entry)
		return nil, err
	}
	return &archiveMaterializationLease{cache: c, entry: entry}, nil
}

func (c *archiveMaterializationCache) release(entry *archiveMaterializationEntry) {
	if entry == nil {
		return
	}
	c.mu.Lock()
	if entry.refs > 0 {
		entry.refs--
	}
	entry.lastUsed = time.Now()
	cleanups := c.pruneLocked(entry.lastUsed)
	c.schedulePruneLocked()
	c.mu.Unlock()
	runArchiveCleanups(cleanups)
}

func (c *archiveMaterializationCache) pruneLocked(now time.Time) []func() error {
	var cleanups []func() error
	type candidate struct {
		key   archiveMaterializationKey
		entry *archiveMaterializationEntry
	}
	var idle []candidate
	var completed int
	var totalBytes int64
	for key, entry := range c.entries {
		select {
		case <-entry.ready:
			if entry.err == nil {
				completed++
				totalBytes += entry.size
			}
		default:
			continue
		}
		if entry.refs == 0 {
			idle = append(idle, candidate{key: key, entry: entry})
		}
	}
	sort.Slice(idle, func(i, j int) bool { return idle[i].entry.lastUsed.Before(idle[j].entry.lastUsed) })
	for _, candidate := range idle {
		entry := candidate.entry
		expired := now.Sub(entry.lastUsed) >= archiveMaterializationIdleTTL
		overLimit := completed > archiveMaterializationMaxEntries || totalBytes > archiveMaterializationMaxBytes
		if !c.closed && !expired && !overLimit {
			continue
		}
		delete(c.entries, candidate.key)
		completed--
		totalBytes -= entry.size
		if entry.cleanup != nil {
			cleanups = append(cleanups, entry.cleanup)
			entry.cleanup = nil
		}
	}
	return cleanups
}

func (c *archiveMaterializationCache) schedulePruneLocked() {
	if c.closed || len(c.entries) == 0 {
		if c.timer != nil {
			c.timer.Stop()
			c.timer = nil
		}
		return
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(archiveMaterializationIdleTTL, func() {
		c.mu.Lock()
		cleanups := c.pruneLocked(time.Now())
		c.schedulePruneLocked()
		c.mu.Unlock()
		runArchiveCleanups(cleanups)
	})
}

func (c *archiveMaterializationCache) close() {
	c.mu.Lock()
	c.closed = true
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	cleanups := c.pruneLocked(time.Now())
	c.mu.Unlock()
	runArchiveCleanups(cleanups)
}

func runArchiveCleanups(cleanups []func() error) {
	for _, cleanup := range cleanups {
		_ = cleanup()
	}
}

func archiveOperationCancelled(ctx context.Context, reporter vfs.TaskReporter) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if reporter != nil && reporter.IsCancelled() {
		return context.Canceled
	}
	return nil
}

func archiveProgressTargets(ctx context.Context) (vfs.ProgressCallback, vfs.TaskReporter) {
	update, _ := ctx.Value(vfs.ProgressKey).(vfs.ProgressCallback)
	reporter, _ := ctx.Value(vfs.ReporterKey).(vfs.TaskReporter)
	return update, reporter
}

func reportArchiveMaterializationProgress(ctx context.Context, name string, copied, total int64) error {
	update, reporter := archiveProgressTargets(ctx)
	if err := archiveOperationCancelled(ctx, reporter); err != nil {
		return err
	}
	percent := -1
	if total >= 0 {
		if total == 0 {
			percent = 100
		} else {
			percent = int(copied * 100 / total)
			if percent > 100 {
				percent = 100
			}
		}
	}
	totalText := "unknown"
	if total >= 0 {
		totalText = formatSize(total)
	}
	message := fmt.Sprintf("Downloading archive: %s / %s", formatSize(copied), totalText)
	if update != nil {
		update("Downloading archive...", percent)
	}
	if reporter != nil {
		reporter.UpdateTransfer("Downloading", name, percent, message, percent, "")
	}
	return nil
}

func materializeArchiveSource(ctx context.Context, parent vfs.VFS, archivePath, displayName string) (string, int64, func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	update, reporter := archiveProgressTargets(ctx)
	pulseDone := make(chan struct{})
	if update != nil || reporter != nil {
		go func() {
			ticker := time.NewTicker(ProgressTickerInterval)
			defer ticker.Stop()
			for {
				select {
				case <-pulseDone:
					return
				case <-ctx.Done():
					return
				case <-ticker.C:
					if update != nil {
						update("Opening remote archive...", -1)
					}
					if reporter != nil {
						reporter.UpdateTransfer("Opening", displayName, -1, "Waiting for remote data...", -1, "")
					}
				}
			}
		}()
	}
	reader, err := parent.Open(ctx, archivePath)
	close(pulseDone)
	if err != nil {
		return "", 0, nil, err
	}
	if err := archiveOperationCancelled(ctx, reporter); err != nil {
		_ = reader.Close()
		return "", 0, nil, err
	}

	if localPath, valid := archiveReaderLocalPath(reader); valid {
		info, statErr := os.Stat(localPath)
		if statErr == nil && !info.IsDir() && (reader.Size() < 0 || info.Size() == reader.Size()) {
			if progressErr := reportArchiveMaterializationProgress(ctx, displayName, info.Size(), info.Size()); progressErr != nil {
				_ = reader.Close()
				return "", 0, nil, progressErr
			}
			return localPath, info.Size(), reader.Close, nil
		}
	}

	tmp, err := os.CreateTemp("", "f4-archive-source-*")
	if err != nil {
		_ = reader.Close()
		return "", 0, nil, err
	}
	tmpPath := tmp.Name()
	cleanupTemp := func() error {
		closeErr := tmp.Close()
		removeErr := os.Remove(tmpPath)
		if closeErr != nil {
			return closeErr
		}
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		return nil
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = reader.Close()
		_ = cleanupTemp()
		return "", 0, nil, err
	}

	total := reader.Size()
	if err := reportArchiveMaterializationProgress(ctx, displayName, 0, total); err != nil {
		_ = reader.Close()
		_ = cleanupTemp()
		return "", 0, nil, err
	}
	buffer := make([]byte, 1<<20)
	var copied int64
	for {
		if err := archiveOperationCancelled(ctx, reporter); err != nil {
			_ = reader.Close()
			_ = cleanupTemp()
			return "", 0, nil, err
		}
		n, readErr := reader.Read(ctx, buffer)
		if n > 0 {
			written, writeErr := tmp.Write(buffer[:n])
			copied += int64(written)
			if writeErr != nil {
				_ = reader.Close()
				_ = cleanupTemp()
				return "", 0, nil, writeErr
			}
			if written != n {
				_ = reader.Close()
				_ = cleanupTemp()
				return "", 0, nil, io.ErrShortWrite
			}
			if err := reportArchiveMaterializationProgress(ctx, displayName, copied, total); err != nil {
				_ = reader.Close()
				_ = cleanupTemp()
				return "", 0, nil, err
			}
		}
		if readErr != nil {
			if readErr != io.EOF {
				_ = reader.Close()
				_ = cleanupTemp()
				return "", 0, nil, readErr
			}
			break
		}
		if n == 0 {
			_ = reader.Close()
			_ = cleanupTemp()
			return "", 0, nil, io.ErrNoProgress
		}
	}
	if err := reader.Close(); err != nil {
		_ = cleanupTemp()
		return "", 0, nil, err
	}
	if total >= 0 && copied != total {
		_ = cleanupTemp()
		return "", 0, nil, io.ErrUnexpectedEOF
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath) // The failed private materialization cannot be reused.
		return "", 0, nil, err
	}
	if err := reportArchiveMaterializationProgress(ctx, displayName, copied, copied); err != nil {
		_ = os.Remove(tmpPath) // The canceled private materialization cannot be reused.
		return "", 0, nil, err
	}
	cleanupTemp = func() error {
		removeErr := os.Remove(tmpPath)
		if removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
		return nil
	}
	return tmpPath, copied, cleanupTemp, nil
}

func acquireArchiveMaterialization(ctx context.Context, parent vfs.VFS, archivePath, displayName string) (*archiveMaterializationLease, error) {
	loader := func(loadCtx context.Context) (string, int64, func() error, error) {
		return materializeArchiveSource(loadCtx, parent, archivePath, displayName)
	}
	if key, ok := archiveMaterializationCacheKey(ctx, parent, archivePath); ok {
		return currentArchiveMaterializationCache().acquire(ctx, key, loader)
	}
	path, size, cleanup, err := loader(ctx)
	if err != nil {
		return nil, err
	}
	entry := &archiveMaterializationEntry{path: path, size: size}
	return &archiveMaterializationLease{entry: entry, directCleanup: cleanup}, nil
}
