package archive

import (
	stdzip "archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type archiveTestContextKey struct{}

type remoteArchiveFixtureVFS struct {
	vfs.VFS

	mu             sync.Mutex
	session        any
	uri            string
	name           string
	data           []byte
	modified       time.Time
	revision       string
	localPath      string
	openCount      int
	readCount      int
	statContext    any
	openContext    any
	blockOpen      bool
	blockRead      bool
	readerChunkMax int
}

func (v *remoteArchiveFixtureVFS) SessionKey() any { return v.session }
func (v *remoteArchiveFixtureVFS) Abs(candidate string) (string, error) {
	if candidate == v.uri {
		return candidate, nil
	}
	return "", os.ErrInvalid
}
func (v *remoteArchiveFixtureVFS) Base(string) string { return v.name }
func (v *remoteArchiveFixtureVFS) Stat(ctx context.Context, candidate string) (vfs.VFSItem, error) {
	v.mu.Lock()
	v.statContext = ctx.Value(archiveTestContextKey{})
	dataSize := int64(len(v.data))
	if v.localPath != "" {
		if info, err := os.Stat(v.localPath); err == nil {
			dataSize = info.Size()
		}
	}
	modified := v.modified
	v.mu.Unlock()
	if candidate != v.uri {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	return vfs.VFSItem{Name: v.name, Size: dataSize, MTime: modified, Revision: v.revision}, ctx.Err()
}
func (v *remoteArchiveFixtureVFS) Open(ctx context.Context, candidate string) (vfs.ReadAtCloser, error) {
	v.mu.Lock()
	v.openCount++
	v.openContext = ctx.Value(archiveTestContextKey{})
	blockOpen := v.blockOpen
	blockRead := v.blockRead
	chunkMax := v.readerChunkMax
	localPath := v.localPath
	data := append([]byte(nil), v.data...)
	v.mu.Unlock()
	if candidate != v.uri {
		return nil, os.ErrNotExist
	}
	if blockOpen {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if localPath != "" {
		file, err := os.Open(localPath)
		if err != nil {
			return nil, err
		}
		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}
		return &archiveFixtureLocalReader{File: file, path: localPath, size: info.Size()}, nil
	}
	return &archiveFixtureMemoryReader{owner: v, data: data, block: blockRead, chunkMax: chunkMax}, nil
}

func (v *remoteArchiveFixtureVFS) counts() (opens, reads int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.openCount, v.readCount
}

type archiveFixtureMemoryReader struct {
	owner    *remoteArchiveFixtureVFS
	data     []byte
	offset   int64
	block    bool
	chunkMax int
}

func (r *archiveFixtureMemoryReader) Size() int64 { return int64(len(r.data)) }
func (r *archiveFixtureMemoryReader) Read(ctx context.Context, buffer []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if r.block {
		<-ctx.Done()
		return 0, ctx.Err()
	}
	r.owner.mu.Lock()
	r.owner.readCount++
	r.owner.mu.Unlock()
	if r.offset >= int64(len(r.data)) {
		return 0, io.EOF
	}
	if r.chunkMax > 0 && len(buffer) > r.chunkMax {
		buffer = buffer[:r.chunkMax]
	}
	n := copy(buffer, r.data[r.offset:])
	r.offset += int64(n)
	return n, nil
}
func (r *archiveFixtureMemoryReader) ReadAt(ctx context.Context, buffer []byte, offset int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if offset >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(buffer, r.data[offset:])
	if n != len(buffer) {
		return n, io.EOF
	}
	return n, nil
}
func (*archiveFixtureMemoryReader) Close() error { return nil }

type archiveFixtureLocalReader struct {
	*os.File
	path string
	size int64
}

func (r *archiveFixtureLocalReader) Size() int64 { return r.size }
func (r *archiveFixtureLocalReader) Read(ctx context.Context, buffer []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.File.Read(buffer)
}
func (r *archiveFixtureLocalReader) ReadAt(ctx context.Context, buffer []byte, offset int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.File.ReadAt(buffer, offset)
}
func (r *archiveFixtureLocalReader) LocalPath() (string, bool) { return r.path, true }

type archiveProgressRecorder struct {
	mu       sync.Mutex
	percents []int
}

func (r *archiveProgressRecorder) callback(_ string, percent int) {
	r.mu.Lock()
	r.percents = append(r.percents, percent)
	r.mu.Unlock()
}
func (*archiveProgressRecorder) UpdateScan(string, int64, int64) {}
func (r *archiveProgressRecorder) UpdateTransfer(_ string, _ string, current int, _ string, total int, _ string) {
	if total >= 0 {
		r.callback("", total)
	} else {
		r.callback("", current)
	}
}
func (*archiveProgressRecorder) IsCancelled() bool { return false }
func (r *archiveProgressRecorder) snapshot() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]int(nil), r.percents...)
}

func archiveFixtureZIP(t *testing.T, payloadSize int) []byte {
	return archiveFixtureZIPWithSeed(t, payloadSize, 17)
}

func archiveFixtureZIPWithSeed(t *testing.T, payloadSize int, seed byte) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := stdzip.NewWriter(&output)
	header := &stdzip.FileHeader{Name: "folder/payload.bin", Method: stdzip.Store}
	member, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, payloadSize)
	value := seed
	for index := range payload {
		payload[index] = value
		value += 31
	}
	if _, err := member.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}

func archiveTestContext(parent context.Context, recorder *archiveProgressRecorder) context.Context {
	ctx := context.WithValue(parent, archiveTestContextKey{}, "preserved")
	if recorder != nil {
		ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(recorder.callback))
		ctx = context.WithValue(ctx, vfs.ReporterKey, vfs.TaskReporter(recorder))
	}
	return ctx
}

func closeArchiveFixtureImmediately(archiveVFS *ArchiveVFS) {
	archiveVFS.mu.Lock()
	archiveVFS.isClosed = true
	_ = archiveVFS.performCleanup()
	archiveVFS.mu.Unlock()
}

func prepareArchiveCacheTest(t *testing.T) {
	t.Helper()
	closeSharedArchiveMaterializations()
	t.Cleanup(closeSharedArchiveMaterializations)
}

func TestArchiveProviderRemoteURIContextProgressAndSessionCache(t *testing.T) {
	prepareArchiveCacheTest(t)
	data := archiveFixtureZIP(t, 3<<20)
	session := new(int)
	remote := &remoteArchiveFixtureVFS{
		session: session,
		uri:     "cloud://yandex/11111111-1111-1111-1111-111111111111/opaque-item",
		name:    "large.7z", data: data, modified: time.Unix(1_700_000_000, 123), revision: "revision-1", readerChunkMax: 192 << 10,
	}
	provider := &ArchiveProvider{}
	if !provider.CanOpen(context.Background(), remote, remote.uri) {
		t.Fatal("opaque cloud URI lost the display-name archive extension")
	}

	openingProgress := &archiveProgressRecorder{}
	ctx := archiveTestContext(context.Background(), openingProgress)
	firstVFS, err := provider.Open(ctx, remote, remote.uri)
	if err != nil {
		t.Fatalf("first remote open: %v", err)
	}
	first := firstVFS.(*ArchiveVFS)
	defer closeArchiveFixtureImmediately(first)
	if got := first.GetPath(); got != remote.uri {
		t.Fatalf("remote archive root = %q, want canonical cloud URI", got)
	}
	if strings.Contains(first.GetPath(), "cloud:/") && !strings.Contains(first.GetPath(), "cloud://") {
		t.Fatalf("remote archive path was rewritten as a Windows local path: %q", first.GetPath())
	}
	memberPath := first.Join(first.GetPath(), "folder", "payload.bin")
	if want := remote.uri + "/folder/payload.bin"; memberPath != want {
		t.Fatalf("archive member URI = %q, want %q", memberPath, want)
	}
	if err := first.SetPath(first.Join(first.GetPath(), "folder")); err != nil {
		t.Fatalf("enter folder through URI-safe path: %v", err)
	}
	if got := first.GetPath(); got != remote.uri+"/folder" {
		t.Fatalf("nested GetPath = %q", got)
	}
	if err := first.SetPath(remote.uri); err != nil {
		t.Fatalf("return to archive URI root: %v", err)
	}

	remote.mu.Lock()
	statContext, openContext := remote.statContext, remote.openContext
	remote.mu.Unlock()
	if statContext != "preserved" || openContext != "preserved" {
		t.Fatalf("caller context values were lost: Stat=%v Open=%v", statContext, openContext)
	}
	if !containsArchiveProgress(openingProgress.snapshot(), 100) {
		t.Fatal("remote archive opening never reported completion")
	}
	if !containsIntermediateArchiveProgress(openingProgress.snapshot()) {
		t.Fatal("multi-megabyte remote archive opening never advanced through an intermediate percentage")
	}

	secondVFS, err := provider.Open(ctx, remote, remote.uri)
	if err != nil {
		t.Fatalf("same-session reopen: %v", err)
	}
	second := secondVFS.(*ArchiveVFS)
	defer closeArchiveFixtureImmediately(second)
	if opens, _ := remote.counts(); opens != 1 {
		t.Fatalf("unchanged same-session archive opened remotely %d times, want 1", opens)
	}
	if first.backingPath != second.backingPath {
		t.Fatalf("same-session reopen used a second materialization")
	}

	memberProgress := &archiveProgressRecorder{}
	memberCtx := archiveTestContext(context.Background(), memberProgress)
	member, err := second.Open(memberCtx, second.Join(second.GetPath(), "folder", "payload.bin"))
	if err != nil {
		t.Fatalf("F3-style member open: %v", err)
	}
	_ = member.Close()
	if !containsArchiveProgress(memberProgress.snapshot(), 100) {
		t.Fatal("archive member extraction never emitted a final 100% progress sample")
	}
	backingPath := first.backingPath
	closeArchiveFixtureImmediately(first)
	if _, err := os.Stat(backingPath); err != nil {
		t.Fatalf("shared backing disappeared while second archive still retained it: %v", err)
	}
	closeArchiveFixtureImmediately(second)
	if _, err := os.Stat(backingPath); err != nil {
		t.Fatalf("idle backing disappeared before bounded cache cleanup: %v", err)
	}
	closeSharedArchiveMaterializations()
	if _, err := os.Stat(backingPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive cache cleanup left backing behind: %v", err)
	}
}

func TestArchiveProviderCancellationReachesRemoteOpenAndRead(t *testing.T) {
	prepareArchiveCacheTest(t)
	t.Run("open", func(t *testing.T) {
		remote := &remoteArchiveFixtureVFS{
			session: new(int), uri: "cloud://yandex/22222222-2222-2222-2222-222222222222/item",
			name: "cancel.zip", data: archiveFixtureZIP(t, 1024), modified: time.Now(), revision: "revision-open", blockOpen: true,
		}
		ctx, cancel := context.WithTimeout(archiveTestContext(context.Background(), nil), 40*time.Millisecond)
		defer cancel()
		_, err := (&ArchiveProvider{}).Open(ctx, remote, remote.uri)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked parent Open error = %v", err)
		}
		remote.mu.Lock()
		gotContext := remote.openContext
		remote.mu.Unlock()
		if gotContext != "preserved" {
			t.Fatal("caller context did not reach parent Open")
		}
	})

	t.Run("read", func(t *testing.T) {
		remote := &remoteArchiveFixtureVFS{
			session: new(int), uri: "cloud://yandex/33333333-3333-3333-3333-333333333333/item",
			name: "cancel.zip", data: archiveFixtureZIP(t, 1024), modified: time.Now(), revision: "revision-read", blockRead: true,
		}
		ctx, cancel := context.WithTimeout(archiveTestContext(context.Background(), nil), 40*time.Millisecond)
		defer cancel()
		_, err := (&ArchiveProvider{}).Open(ctx, remote, remote.uri)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("blocked parent Read error = %v", err)
		}
	})
}

func TestArchiveProviderUsesExistingLocalBackingWithoutSecondTemp(t *testing.T) {
	prepareArchiveCacheTest(t)
	backing := filepath.Join(t.TempDir(), "download.zip")
	if err := os.WriteFile(backing, archiveFixtureZIP(t, 128<<10), 0o600); err != nil {
		t.Fatal(err)
	}
	remote := &remoteArchiveFixtureVFS{
		session: new(int), uri: "cloud://gdrive/44444444-4444-4444-4444-444444444444/item",
		name: "download.zip", localPath: backing, modified: time.Now(), revision: "revision-local",
	}
	opened, err := NewArchiveVFSContext(context.Background(), remote, remote.uri)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeArchiveFixtureImmediately(opened)
		closeSharedArchiveMaterializations()
	}()
	if opened.backingPath != backing {
		t.Fatalf("archive copied an existing local backing to %q", opened.backingPath)
	}
	if opens, reads := remote.counts(); opens != 1 || reads != 0 {
		t.Fatalf("local backing use: opens=%d reads=%d, want 1/0", opens, reads)
	}
	if _, err := opened.Create(context.Background(), opened.Join(opened.GetPath(), "new.txt")); err == nil {
		t.Fatal("remote archive allowed a mutation of its provider-owned cached backing")
	}
}

func TestArchiveProviderCacheInvalidatesWhenRemoteMetadataChanges(t *testing.T) {
	prepareArchiveCacheTest(t)
	remote := &remoteArchiveFixtureVFS{
		session: new(int), uri: "cloud://yandex/55555555-5555-5555-5555-555555555555/item",
		name: "changed.zip", data: archiveFixtureZIP(t, 1024), modified: time.Unix(1_700_000_000, 0), revision: "revision-before",
	}
	first, err := NewArchiveVFSContext(context.Background(), remote, remote.uri)
	if err != nil {
		t.Fatal(err)
	}
	closeArchiveFixtureImmediately(first)
	remote.mu.Lock()
	remote.data = archiveFixtureZIPWithSeed(t, 1024, 91)
	remote.revision = "revision-after"
	remote.mu.Unlock()
	second, err := NewArchiveVFSContext(context.Background(), remote, remote.uri)
	if err != nil {
		t.Fatal(err)
	}
	defer closeArchiveFixtureImmediately(second)
	if opens, _ := remote.counts(); opens != 2 {
		t.Fatalf("changed remote archive used stale cache; opens=%d, want 2", opens)
	}
}

func TestArchiveProviderDoesNotReuseWeakMetadata(t *testing.T) {
	prepareArchiveCacheTest(t)
	remote := &remoteArchiveFixtureVFS{
		session: new(int), uri: "cloud://webdav/66666666-6666-6666-6666-666666666666/item",
		name: "weak.zip", data: archiveFixtureZIP(t, 1024), modified: time.Unix(1_700_000_000, 0),
		// Deliberately no Revision: size and modified time alone are not a
		// strong content identity and must never authorize reuse.
	}
	first, err := NewArchiveVFSContext(context.Background(), remote, remote.uri)
	if err != nil {
		t.Fatal(err)
	}
	firstBacking := first.backingPath
	closeArchiveFixtureImmediately(first)
	second, err := NewArchiveVFSContext(context.Background(), remote, remote.uri)
	if err != nil {
		t.Fatal(err)
	}
	defer closeArchiveFixtureImmediately(second)
	if opens, _ := remote.counts(); opens != 2 {
		t.Fatalf("weak size/MTime metadata authorized cache reuse; opens=%d", opens)
	}
	if second.backingPath == firstBacking {
		t.Fatal("weak metadata reused the old archive materialization")
	}
}

func containsArchiveProgress(samples []int, wanted int) bool {
	for _, sample := range samples {
		if sample == wanted {
			return true
		}
	}
	return false
}

func containsIntermediateArchiveProgress(samples []int) bool {
	for _, sample := range samples {
		if sample > 0 && sample < 100 {
			return true
		}
	}
	return false
}
