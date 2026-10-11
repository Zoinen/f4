package archive

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// ---- archiveReaderLocalPath fixtures ----

type materializeLocalBackingReader struct {
	path  string
	valid bool
}

func (r *materializeLocalBackingReader) LocalPath() (string, bool)                 { return r.path, r.valid }
func (r *materializeLocalBackingReader) Read(context.Context, []byte) (int, error) { return 0, io.EOF }
func (r *materializeLocalBackingReader) ReadAt(context.Context, []byte, int64) (int, error) {
	return 0, io.EOF
}
func (r *materializeLocalBackingReader) Close() error { return nil }
func (r *materializeLocalBackingReader) Size() int64  { return 0 }

type materializeLegacyTempPathReader struct {
	tempPath string
}

func (r *materializeLegacyTempPathReader) TempPath() string { return r.tempPath }
func (r *materializeLegacyTempPathReader) Read(context.Context, []byte) (int, error) {
	return 0, io.EOF
}
func (r *materializeLegacyTempPathReader) ReadAt(context.Context, []byte, int64) (int, error) {
	return 0, io.EOF
}
func (r *materializeLegacyTempPathReader) Close() error { return nil }
func (r *materializeLegacyTempPathReader) Size() int64  { return 0 }

type materializePlainReader struct{}

func (r *materializePlainReader) Read(context.Context, []byte) (int, error) { return 0, io.EOF }
func (r *materializePlainReader) ReadAt(context.Context, []byte, int64) (int, error) {
	return 0, io.EOF
}
func (r *materializePlainReader) Close() error { return nil }
func (r *materializePlainReader) Size() int64  { return 0 }

// TestArchiveReaderLocalPath exercises every interface archiveReaderLocalPath
// probes for, in priority order: the structural archiveLocalBacking contract,
// the legacy TempPath() method, and vfs.TempFileWrapper's field. Only the
// success case of the first was reachable from the package's existing tests
// (through archiveFixtureLocalReader), leaving the fallback paths untested.
func TestArchiveReaderLocalPath(t *testing.T) {
	for _, test := range []struct {
		name     string
		reader   vfs.ReadAtCloser
		wantPath string
		wantOK   bool
	}{
		{
			name:     "archiveLocalBacking with a valid non-empty path",
			reader:   &materializeLocalBackingReader{path: "/tmp/archive-source", valid: true},
			wantPath: "/tmp/archive-source",
			wantOK:   true,
		},
		{
			name:   "archiveLocalBacking reports invalid",
			reader: &materializeLocalBackingReader{path: "/tmp/archive-source", valid: false},
		},
		{
			name:   "archiveLocalBacking reports valid but an empty path",
			reader: &materializeLocalBackingReader{path: "", valid: true},
		},
		{
			name:     "legacy TempPath interface with a non-empty path",
			reader:   &materializeLegacyTempPathReader{tempPath: "/tmp/legacy-source"},
			wantPath: "/tmp/legacy-source",
			wantOK:   true,
		},
		{
			name:   "legacy TempPath interface returns an empty path",
			reader: &materializeLegacyTempPathReader{tempPath: ""},
		},
		{
			name:     "vfs.TempFileWrapper with a populated TempPath",
			reader:   &vfs.TempFileWrapper{TempPath: "/tmp/wrapper-source"},
			wantPath: "/tmp/wrapper-source",
			wantOK:   true,
		},
		{
			name:   "vfs.TempFileWrapper with an empty TempPath",
			reader: &vfs.TempFileWrapper{},
		},
		{
			name:   "reader implementing none of the local-backing interfaces",
			reader: &materializePlainReader{},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, ok := archiveReaderLocalPath(test.reader)
			if path != test.wantPath || ok != test.wantOK {
				t.Fatalf("archiveReaderLocalPath() = (%q, %v), want (%q, %v)", path, ok, test.wantPath, test.wantOK)
			}
		})
	}
}

// ---- archiveSessionCacheIdentity fixtures ----

type materializeSessionIdentityVFS struct {
	vfs.VFS
	key any
}

func (v *materializeSessionIdentityVFS) SessionKey() any { return v.key }

type materializeComparableVFS struct {
	vfs.VFS
}

// materializeNonComparableVFS deliberately holds a slice so that its value
// type fails reflect.Type.Comparable(); archiveSessionCacheIdentity must
// refuse to use such a value as a map key rather than let the map operation
// panic at runtime.
type materializeNonComparableVFS struct {
	vfs.VFS
	data []string
}

func TestArchiveSessionCacheIdentity(t *testing.T) {
	comparableParent := &materializeComparableVFS{}

	for _, test := range []struct {
		name       string
		parent     vfs.VFS
		wantOK     bool
		wantParent bool // identity must equal the parent value itself
	}{
		{name: "nil parent", parent: nil, wantOK: false},
		{
			name:   "SessionIdentity with a nil key",
			parent: &materializeSessionIdentityVFS{key: nil},
			wantOK: false,
		},
		{
			name:   "SessionIdentity with a non-comparable key",
			parent: &materializeSessionIdentityVFS{key: []string{"a"}},
			wantOK: false,
		},
		{
			name:   "SessionIdentity with a comparable key",
			parent: &materializeSessionIdentityVFS{key: "session-1"},
			wantOK: true,
		},
		{
			name:       "no SessionIdentity, comparable parent falls back to itself",
			parent:     comparableParent,
			wantOK:     true,
			wantParent: true,
		},
		{
			name:   "no SessionIdentity, non-comparable parent value",
			parent: materializeNonComparableVFS{data: []string{"a"}},
			wantOK: false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			identity, ok := archiveSessionCacheIdentity(test.parent)
			if ok != test.wantOK {
				t.Fatalf("archiveSessionCacheIdentity() ok = %v, want %v", ok, test.wantOK)
			}
			if ok && test.wantParent && identity != comparableParent {
				t.Fatalf("identity = %#v, want the parent itself (%#v)", identity, comparableParent)
			}
		})
	}
}

// ---- archiveMaterializationCacheKey fixtures ----

type materializeCacheKeyVFS struct {
	vfs.VFS
	session  any
	absPath  string
	absErr   error
	statItem vfs.VFSItem
	statErr  error
}

func (v *materializeCacheKeyVFS) SessionKey() any            { return v.session }
func (v *materializeCacheKeyVFS) Abs(string) (string, error) { return v.absPath, v.absErr }
func (v *materializeCacheKeyVFS) Stat(context.Context, string) (vfs.VFSItem, error) {
	return v.statItem, v.statErr
}

func TestArchiveMaterializationCacheKey(t *testing.T) {
	validItem := vfs.VFSItem{Revision: "rev-1", Size: 10, MTime: time.Unix(1_700_000_000, 0)}

	for _, test := range []struct {
		name   string
		parent *materializeCacheKeyVFS
		wantOK bool
	}{
		{
			name:   "a nil session key",
			parent: &materializeCacheKeyVFS{session: nil, absPath: "/archive.zip", statItem: validItem},
			wantOK: false,
		},
		{
			name:   "Abs returns an error",
			parent: &materializeCacheKeyVFS{session: "s", absErr: os.ErrInvalid, statItem: validItem},
			wantOK: false,
		},
		{
			name:   "Abs returns an empty path",
			parent: &materializeCacheKeyVFS{session: "s", absPath: "", statItem: validItem},
			wantOK: false,
		},
		{
			name:   "a full identity yields a cache key",
			parent: &materializeCacheKeyVFS{session: "s", absPath: "/archive.zip", statItem: validItem},
			wantOK: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, ok := archiveMaterializationCacheKey(context.Background(), test.parent, "archive.zip")
			if ok != test.wantOK {
				t.Fatalf("archiveMaterializationCacheKey() ok = %v, want %v", ok, test.wantOK)
			}
		})
	}
}

// ---- archiveMaterializationCache.acquire ----

// waitForArchiveMaterializationRefs polls the cache's internal state (under
// its own mutex, the same way the cache itself does) until the entry for key
// has accumulated at least want references. It gives the concurrency tests
// below a deterministic rendezvous point instead of a fixed sleep: once refs
// reaches want, the waiting goroutine has necessarily already joined the
// in-flight entry rather than started a competing load.
func waitForArchiveMaterializationRefs(t *testing.T, cache *archiveMaterializationCache, key archiveMaterializationKey, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		cache.mu.Lock()
		entry := cache.entries[key]
		refs := 0
		if entry != nil {
			refs = entry.refs
		}
		cache.mu.Unlock()
		if refs >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d refs on %+v, last saw %d", want, key, refs)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestArchiveMaterializationCacheAcquireOnClosedCache(t *testing.T) {
	cache := newArchiveMaterializationCache()
	t.Cleanup(cache.close)
	cache.close()

	key := archiveMaterializationKey{path: "closed"}
	_, err := cache.acquire(context.Background(), key, func(context.Context) (string, int64, func() error, error) {
		t.Fatal("load must not run once the cache is closed")
		return "", 0, nil, nil
	})
	if !errors.Is(err, os.ErrClosed) {
		t.Fatalf("acquire() on a closed cache = %v, want os.ErrClosed", err)
	}
}

func TestArchiveMaterializationCacheAcquireReusesCachedEntry(t *testing.T) {
	cache := newArchiveMaterializationCache()
	t.Cleanup(cache.close)

	key := archiveMaterializationKey{path: "shared"}
	var calls int32
	load := func(context.Context) (string, int64, func() error, error) {
		atomic.AddInt32(&calls, 1)
		return "/tmp/shared-path", 42, func() error { return nil }, nil
	}

	first, err := cache.acquire(context.Background(), key, load)
	if err != nil {
		t.Fatalf("first acquire() = %v", err)
	}
	second, err := cache.acquire(context.Background(), key, load)
	if err != nil {
		t.Fatalf("second acquire() = %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("load() ran %d times, want exactly 1 (the second acquire should reuse the entry)", got)
	}
	if first.Path() != second.Path() || first.Path() != "/tmp/shared-path" {
		t.Fatalf("leases disagree on the materialized path: %q vs %q", first.Path(), second.Path())
	}
	if err := first.Close(); err != nil {
		t.Errorf("first lease Close() = %v", err)
	}
	if err := second.Close(); err != nil {
		t.Errorf("second lease Close() = %v", err)
	}
}

func TestArchiveMaterializationCacheAcquirePropagatesLoadErrorToWaiters(t *testing.T) {
	cache := newArchiveMaterializationCache()
	t.Cleanup(cache.close)

	key := archiveMaterializationKey{path: "failing"}
	wantErr := errors.New("materialize failed")
	loadStarted := make(chan struct{})
	releaseLoad := make(chan struct{})
	load := func(context.Context) (string, int64, func() error, error) {
		close(loadStarted)
		<-releaseLoad
		return "", 0, nil, wantErr
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := cache.acquire(context.Background(), key, load)
		firstDone <- err
	}()
	<-loadStarted // the entry is registered in the map before load() runs

	secondDone := make(chan error, 1)
	go func() {
		_, err := cache.acquire(context.Background(), key, func(context.Context) (string, int64, func() error, error) {
			t.Error("a caller joining an in-flight load must not run its own loader")
			return "", 0, nil, nil
		})
		secondDone <- err
	}()

	// Once refs reaches 2, the second goroutine has necessarily already
	// joined the existing entry (refs++ happens before it starts waiting),
	// so releasing the load now cannot race with it taking the other path.
	waitForArchiveMaterializationRefs(t, cache, key, 2, 5*time.Second)
	close(releaseLoad)

	firstErr := <-firstDone
	secondErr := <-secondDone
	if !errors.Is(firstErr, wantErr) {
		t.Fatalf("first acquire() error = %v, want %v", firstErr, wantErr)
	}
	if !errors.Is(secondErr, wantErr) {
		t.Fatalf("second acquire() error = %v, want %v", secondErr, wantErr)
	}
}

func TestArchiveMaterializationCacheAcquireContextCancelWhileWaiting(t *testing.T) {
	cache := newArchiveMaterializationCache()
	t.Cleanup(cache.close)

	key := archiveMaterializationKey{path: "slow"}
	loadStarted := make(chan struct{})
	releaseLoad := make(chan struct{})
	load := func(context.Context) (string, int64, func() error, error) {
		close(loadStarted)
		<-releaseLoad
		return "/tmp/slow-path", 1, func() error { return nil }, nil
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := cache.acquire(context.Background(), key, load)
		firstDone <- err
	}()
	<-loadStarted

	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := cache.acquire(ctx, key, func(context.Context) (string, int64, func() error, error) {
			t.Error("a caller cancelled while waiting must not run its own loader")
			return "", 0, nil, nil
		})
		secondDone <- err
	}()

	waitForArchiveMaterializationRefs(t, cache, key, 2, 5*time.Second)
	cancel()

	secondErr := <-secondDone
	if !errors.Is(secondErr, context.Canceled) {
		t.Fatalf("acquire() with a cancelled context = %v, want context.Canceled", secondErr)
	}

	close(releaseLoad)
	firstErr := <-firstDone
	if firstErr != nil {
		t.Fatalf("first acquire() = %v, want nil", firstErr)
	}
}

func TestArchiveMaterializationCacheReleaseNilEntryIsNoop(t *testing.T) {
	cache := newArchiveMaterializationCache()
	t.Cleanup(cache.close)

	cache.release(nil) // must not panic
}

// ---- reportArchiveMaterializationProgress ----

type materializeCancelledReporter struct {
	cancelled bool
}

func (r *materializeCancelledReporter) UpdateScan(string, int64, int64)                         {}
func (r *materializeCancelledReporter) UpdateTransfer(string, string, int, string, int, string) {}
func (r *materializeCancelledReporter) IsCancelled() bool                                       { return r.cancelled }

func TestReportArchiveMaterializationProgress(t *testing.T) {
	t.Run("propagates a cancelled context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := reportArchiveMaterializationProgress(ctx, "a.zip", 0, 10); !errors.Is(err, context.Canceled) {
			t.Fatalf("reportArchiveMaterializationProgress() = %v, want context.Canceled", err)
		}
	})

	t.Run("propagates a cancelled reporter", func(t *testing.T) {
		reporter := &materializeCancelledReporter{cancelled: true}
		ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(reporter))
		if err := reportArchiveMaterializationProgress(ctx, "a.zip", 0, 10); !errors.Is(err, context.Canceled) {
			t.Fatalf("reportArchiveMaterializationProgress() = %v, want context.Canceled", err)
		}
	})

	t.Run("reports 100%% for a zero-length total", func(t *testing.T) {
		recorder := &archiveProgressRecorder{}
		ctx := archiveTestContext(context.Background(), recorder)
		if err := reportArchiveMaterializationProgress(ctx, "a.zip", 0, 0); err != nil {
			t.Fatalf("reportArchiveMaterializationProgress() = %v", err)
		}
		percents := recorder.snapshot()
		if len(percents) == 0 || percents[len(percents)-1] != 100 {
			t.Fatalf("percents = %v, want the last one to be 100 for a zero-length total", percents)
		}
	})

	t.Run("clamps a copied count above the reported total", func(t *testing.T) {
		recorder := &archiveProgressRecorder{}
		ctx := archiveTestContext(context.Background(), recorder)
		if err := reportArchiveMaterializationProgress(ctx, "a.zip", 150, 100); err != nil {
			t.Fatalf("reportArchiveMaterializationProgress() = %v", err)
		}
		percents := recorder.snapshot()
		if len(percents) == 0 || percents[len(percents)-1] != 100 {
			t.Fatalf("percents = %v, want the clamped value 100", percents)
		}
	})
}

// ---- materializeArchiveSource ----

func TestMaterializeArchiveSourcePulsesProgressWhileOpenBlocks(t *testing.T) {
	previous := ProgressTickerInterval
	ProgressTickerInterval = 5 * time.Millisecond
	t.Cleanup(func() { ProgressTickerInterval = previous })

	remote := &remoteArchiveFixtureVFS{
		uri:       "memory://archive/slow.zip",
		name:      "slow.zip",
		data:      []byte("remote archive bytes"),
		blockOpen: true,
	}
	recorder := &archiveProgressRecorder{}
	ctx, cancel := context.WithTimeout(archiveTestContext(context.Background(), recorder), 150*time.Millisecond)
	defer cancel()

	_, _, _, err := materializeArchiveSource(ctx, remote, remote.uri, remote.name)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("materializeArchiveSource() with a blocked Open = %v, want context.DeadlineExceeded", err)
	}
	if percents := recorder.snapshot(); len(percents) == 0 {
		t.Fatal("no progress pulses were reported while Open() was blocked")
	}
}

func TestMaterializeArchiveSourceCancelledReporterAfterOpen(t *testing.T) {
	remote := &remoteArchiveFixtureVFS{
		uri:  "memory://archive/remote.zip",
		name: "remote.zip",
		data: []byte("remote archive bytes"),
	}
	reporter := &materializeCancelledReporter{cancelled: true}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(reporter))

	if _, _, _, err := materializeArchiveSource(ctx, remote, remote.uri, remote.name); !errors.Is(err, context.Canceled) {
		t.Fatalf("materializeArchiveSource() with a cancelled reporter = %v, want context.Canceled", err)
	}
}

// materializeCancelAfterNReporter only reports cancellation starting from its
// cancelAtCall'th IsCancelled() call, letting a test get past an earlier
// cancellation check and fail a later one deterministically.
type materializeCancelAfterNReporter struct {
	calls        int32
	cancelAtCall int32
}

func (r *materializeCancelAfterNReporter) UpdateScan(string, int64, int64)                         {}
func (r *materializeCancelAfterNReporter) UpdateTransfer(string, string, int, string, int, string) {}
func (r *materializeCancelAfterNReporter) IsCancelled() bool {
	return atomic.AddInt32(&r.calls, 1) >= r.cancelAtCall
}

func TestMaterializeArchiveSourceLocalFastPathPropagatesProgressError(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "already-local.zip")
	if err := os.WriteFile(localPath, []byte("local archive bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	remote := &remoteArchiveFixtureVFS{
		uri:       "memory://archive/local.zip",
		name:      "local.zip",
		localPath: localPath,
	}
	// The first IsCancelled() call is the post-Open check (must pass so the
	// fast path is reached); the second is reportArchiveMaterializationProgress
	// inside that fast path, which this test wants to fail.
	reporter := &materializeCancelAfterNReporter{cancelAtCall: 2}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, vfs.TaskReporter(reporter))

	if _, _, _, err := materializeArchiveSource(ctx, remote, remote.uri, remote.name); !errors.Is(err, context.Canceled) {
		t.Fatalf("materializeArchiveSource() on the local fast path with a late cancellation = %v, want context.Canceled", err)
	}
}

// materializeControlledReader gives fine-grained control over what Read
// reports, independently of what Size claims, so tests can drive
// materializeArchiveSource's download-loop error paths precisely.
type materializeControlledReader struct {
	data      []byte
	offset    int
	size      int64
	zeroReads int
}

func (r *materializeControlledReader) Size() int64 { return r.size }
func (r *materializeControlledReader) Read(_ context.Context, p []byte) (int, error) {
	if r.zeroReads > 0 {
		r.zeroReads--
		return 0, nil
	}
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}
func (r *materializeControlledReader) ReadAt(context.Context, []byte, int64) (int, error) {
	return 0, io.EOF
}
func (r *materializeControlledReader) Close() error { return nil }

type materializeControlledVFS struct {
	vfs.VFS
	reader *materializeControlledReader
}

func (v *materializeControlledVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return v.reader, nil
}

func TestMaterializeArchiveSourcePropagatesTempFileCreationFailure(t *testing.T) {
	home := t.TempDir()
	bad := filepath.Join(home, "no-such-tmp-dir")
	t.Setenv("TMPDIR", bad)
	t.Setenv("TMP", bad)
	t.Setenv("TEMP", bad)

	remote := &materializeControlledVFS{reader: &materializeControlledReader{
		data: []byte("payload that needs a temp file"),
		size: int64(len("payload that needs a temp file")),
	}}
	if _, _, _, err := materializeArchiveSource(context.Background(), remote, "archive.zip", "archive.zip"); err == nil {
		t.Fatal("a broken temp directory must surface as an error, not silently succeed")
	}
}

func TestMaterializeArchiveSourcePropagatesNoProgressRead(t *testing.T) {
	remote := &materializeControlledVFS{reader: &materializeControlledReader{
		size:      -1,
		zeroReads: 1,
	}}
	if _, _, _, err := materializeArchiveSource(context.Background(), remote, "archive.zip", "archive.zip"); !errors.Is(err, io.ErrNoProgress) {
		t.Fatalf("materializeArchiveSource() with a Read that reports no bytes and no error = %v, want %v", err, io.ErrNoProgress)
	}
}

func TestMaterializeArchiveSourcePropagatesSizeMismatch(t *testing.T) {
	payload := []byte("archive bytes shorter than advertised")
	remote := &materializeControlledVFS{reader: &materializeControlledReader{
		data: payload,
		size: int64(len(payload)) + 1, // lies about the true length
	}}
	if _, _, _, err := materializeArchiveSource(context.Background(), remote, "archive.zip", "archive.zip"); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("materializeArchiveSource() with a reader that under-delivers its advertised size = %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

// ---- acquireArchiveMaterialization ----

// materializeUncachedVFS holds a slice so that, used by value, its type is
// not comparable: archiveMaterializationCacheKey then refuses to derive a
// cache key from it, and acquireArchiveMaterialization must fall back to
// calling the loader directly and propagating its error.
type materializeUncachedVFS struct {
	vfs.VFS
	openErr error
	marker  []string
}

func (v materializeUncachedVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return nil, v.openErr
}

func TestAcquireArchiveMaterializationPropagatesLoadErrorWithoutCaching(t *testing.T) {
	wantErr := errors.New("open failed")
	parent := materializeUncachedVFS{openErr: wantErr, marker: []string{"non-comparable"}}

	if _, err := acquireArchiveMaterialization(context.Background(), parent, "archive.zip", "archive.zip"); !errors.Is(err, wantErr) {
		t.Fatalf("acquireArchiveMaterialization() error = %v, want %v", err, wantErr)
	}
}
