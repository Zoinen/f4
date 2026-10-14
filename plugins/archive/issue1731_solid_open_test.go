package archive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/unxed/archives"
	"github.com/unxed/f4/vfs"
	zipperarchive "github.com/unxed/zipper/archive"
)

// issue1731CountingFS counts how often each member is opened. Opening a
// member of a solid 7z block decodes everything stored before it, so the
// count is the number of times the block is decoded up to that member.
type issue1731CountingFS struct {
	zipperarchive.FileSystem
	mu    sync.Mutex
	opens map[string]int
}

func newIssue1731CountingFS(inner zipperarchive.FileSystem) *issue1731CountingFS {
	return &issue1731CountingFS{FileSystem: inner, opens: make(map[string]int)}
}

func (c *issue1731CountingFS) Open(name string) (fs.File, error) {
	c.mu.Lock()
	c.opens[name]++
	c.mu.Unlock()
	return c.FileSystem.Open(name)
}

func (c *issue1731CountingFS) count(name string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opens[name]
}

// issue1731StreamFile hides Seek, as a 7z or deflate member stream does.
type issue1731StreamFile struct{ file fs.File }

func (f issue1731StreamFile) Read(p []byte) (int, error) { return f.file.Read(p) }
func (f issue1731StreamFile) Stat() (fs.FileInfo, error) { return f.file.Stat() }
func (f issue1731StreamFile) Close() error               { return f.file.Close() }

// issue1731StreamFS is an in-memory archive whose members cannot seek.
type issue1731StreamFS struct{ fstest.MapFS }

func (s issue1731StreamFS) Open(name string) (fs.File, error) {
	file, err := s.MapFS.Open(name)
	if err != nil {
		return nil, err
	}
	return issue1731StreamFile{file: file}, nil
}

func (issue1731StreamFS) Close() error { return nil }

// issue1731Payload is distinct per member, so a member read from the wrong
// place in the block cannot pass for the right one.
func issue1731Payload(index, size int) []byte {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789-+*/"
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = alphabet[(i*7+index*13)%len(alphabet)]
	}
	return payload
}

func issue1731BuildSolidSevenZip(t *testing.T, dir string, members, size int) string {
	t.Helper()
	source := filepath.Join(dir, "source")
	if err := os.MkdirAll(source, 0o750); err != nil {
		t.Fatal(err)
	}
	names := make(map[string]string, members)
	for index := 0; index < members; index++ {
		name := fmt.Sprintf("member%02d.bin", index)
		// #nosec G703 -- the path is inside the test's temporary directory.
		if err := os.WriteFile(filepath.Join(source, name), issue1731Payload(index, size), 0o600); err != nil {
			t.Fatal(err)
		}
		names[filepath.Join(source, name)] = name
	}
	files, err := archives.FilesFromDisk(context.Background(), nil, names)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(dir, "solid.7z")
	output, err := os.Create(archivePath) // #nosec G304 G703 -- the archive is built in the test's temporary directory.
	if err != nil {
		t.Fatal(err)
	}
	if err := (archives.SevenZip{Solid: true}).Archive(context.Background(), output, files); err != nil {
		_ = output.Close()
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	return archivePath
}

// openIssue1731SolidArchive enters a solid 7z and counts member opens.
func openIssue1731SolidArchive(t *testing.T, members, size int) (*ArchiveVFS, *issue1731CountingFS) {
	t.Helper()
	root := t.TempDir()
	archivePath := issue1731BuildSolidSevenZip(t, root, members, size)
	v, err := NewArchiveVFSContext(context.Background(), vfs.NewOSVFS(root), filepath.Base(archivePath))
	if err != nil {
		t.Fatalf("enter solid archive: %v", err)
	}
	t.Cleanup(func() { _ = v.Close() })
	v.mu.Lock()
	counter := newIssue1731CountingFS(v.fsys)
	v.fsys = counter
	v.mu.Unlock()
	return v, counter
}

// Reading a 7z member without a progress dialog (Quick View, checksums,
// search) used to open it twice: once in Open and once more to copy it to a
// private file. For a solid block every open decodes all data stored before
// the member, so a member near the end of a large block cost two full passes.
func TestIssue1731SolidSevenZipReadDecodesMemberOnce(t *testing.T) {
	const members, size = 4, 96 * 1024
	v, counter := openIssue1731SolidArchive(t, members, size)
	ctx := context.Background()

	for index := 0; index < members; index++ {
		name := fmt.Sprintf("member%02d.bin", index)
		file, err := v.Open(ctx, v.Join(v.GetPath(), name))
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		data, err := io.ReadAll(ctxReader{r: file, ctx: ctx})
		_ = file.Close()
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !bytes.Equal(data, issue1731Payload(index, size)) {
			t.Fatalf("%s: content differs from the stored member", name)
		}
		if got := counter.count(name); got != 1 {
			t.Fatalf("%s opened %d times in the archive, want 1", name, got)
		}
	}
}

// Quick View reads the head of the member through ReadAt.
func TestIssue1731SolidSevenZipReadAtDecodesMemberOnce(t *testing.T) {
	const members, size = 3, 64 * 1024
	v, counter := openIssue1731SolidArchive(t, members, size)
	ctx := context.Background()
	name := "member02.bin"
	want := issue1731Payload(2, size)

	file, err := v.Open(ctx, v.Join(v.GetPath(), name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 16*1024)
	if n, err := file.ReadAt(ctx, head, 0); err != nil || n != len(head) {
		t.Fatalf("ReadAt head = %d, %v", n, err)
	}
	if !bytes.Equal(head, want[:len(head)]) {
		t.Fatal("head differs from the stored member")
	}
	tail := make([]byte, 1024)
	if n, err := file.ReadAt(ctx, tail, int64(size-len(tail))); err != nil || n != len(tail) {
		t.Fatalf("ReadAt tail = %d, %v", n, err)
	}
	if !bytes.Equal(tail, want[size-len(tail):]) {
		t.Fatal("tail differs from the stored member")
	}
	if got := counter.count(name); got != 1 {
		t.Fatalf("%s opened %d times in the archive, want 1", name, got)
	}
}

func newIssue1731StreamArchiveVFS(content string) (*ArchiveVFS, *issue1731CountingFS) {
	counter := newIssue1731CountingFS(issue1731StreamFS{MapFS: fstest.MapFS{
		"member.txt": &fstest.MapFile{Data: []byte(content), Mode: 0o644},
	}})
	return &ArchiveVFS{arcPath: "plain.zip", displayName: "plain.zip", innerPath: ".", fsys: counter}, counter
}

// A handle Read has already streamed from no longer starts at the member's
// first byte, so the private copy still has to come from a fresh open.
func TestIssue1731MaterializeReopensStreamedHandle(t *testing.T) {
	const content = "0123456789abcdefghijklmnopqrstuvwxyz"
	v, counter := newIssue1731StreamArchiveVFS(content)
	ctx := context.Background()

	file, err := v.Open(ctx, "member.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	head := make([]byte, 4)
	if n, err := file.Read(ctx, head); err != nil || string(head[:n]) != content[:4] {
		t.Fatalf("Read = %q, %v", head[:n], err)
	}
	all := make([]byte, len(content))
	if n, err := file.ReadAt(ctx, all, 0); (err != nil && err != io.EOF) || string(all[:n]) != content {
		t.Fatalf("ReadAt after Read = %q, %v", all[:n], err)
	}
	rest, err := io.ReadAll(ctxReader{r: file, ctx: ctx})
	if err != nil || string(rest) != content[4:] {
		t.Fatalf("Read after ReadAt = %q, %v; want %q", rest, err, content[4:])
	}
	if got := counter.count("member.txt"); got != 2 {
		t.Fatalf("member opened %d times, want 2 (Open, then a fresh copy)", got)
	}
}

// An unread handle is the member from its first byte: copy it as is.
func TestIssue1731MaterializeReusesFreshHandle(t *testing.T) {
	const content = "fresh handles need no second open"
	v, counter := newIssue1731StreamArchiveVFS(content)
	ctx := context.Background()

	file, err := v.Open(ctx, "member.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	all := make([]byte, len(content))
	if n, err := file.ReadAt(ctx, all, 0); (err != nil && err != io.EOF) || string(all[:n]) != content {
		t.Fatalf("ReadAt = %q, %v", all[:n], err)
	}
	data, err := io.ReadAll(ctxReader{r: file, ctx: ctx})
	if err != nil || string(data) != content {
		t.Fatalf("Read after ReadAt = %q, %v", data, err)
	}
	if got := counter.count("member.txt"); got != 1 {
		t.Fatalf("member opened %d times, want 1", got)
	}
}
