//go:build linux || darwin || freebsd

package fusefs

import (
	"context"
	"errors"
	"fmt"
	"github.com/hanwen/go-fuse/v2/fuse"
	"github.com/unxed/f4/vfs"
	"math"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestFuseWriteCount(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   int
		want uint32
	}{
		{name: "zero", in: 0, want: 0},
		{name: "one", in: 1, want: 1},
		{name: "negative", in: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := fuseWriteCount(tc.in)
			if tc.name == "negative" {
				if ok {
					t.Fatalf("negative count accepted as %d", got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("fuseWriteCount(%d) = (%d, %v), want (%d, true)", tc.in, got, ok, tc.want)
			}
		})
	}

	maxInt := int(^uint(0) >> 1)
	maxUint32 := uint64(math.MaxUint32)
	got, ok := fuseWriteCount(maxInt)
	if uint64(maxInt) > maxUint32 {
		if ok {
			t.Fatalf("count above MaxUint32 was accepted: %d", maxInt)
		}
	} else if !ok || got != uint32(maxInt) {
		t.Fatalf("fuseWriteCount(maxInt) = (%d, %v), want (%d, true)", got, ok, maxInt)
	}

	// Keep the overflow case portable to 32-bit Unix, where no int can be
	// larger than MaxUint32. The non-constant conversion compiles on both
	// widths; on 32-bit it is unreachable and on 64-bit it is exact.
	if uint64(maxInt) > maxUint32 {
		tooLarge := int(maxUint32)
		tooLarge++
		if _, ok := fuseWriteCount(tooLarge); ok {
			t.Fatalf("count above MaxUint32 was accepted: %d", tooLarge)
		}
	}
}

func TestTypeBitsAndFuseID(t *testing.T) {
	for _, tc := range []struct {
		name string
		item vfs.VFSItem
		want uint32
	}{
		{name: "directory", item: vfs.VFSItem{IsDir: true}, want: fuse.S_IFDIR},
		{name: "symlink", item: vfs.VFSItem{IsSymlink: true}, want: fuse.S_IFLNK},
		{name: "regular", item: vfs.VFSItem{}, want: fuse.S_IFREG},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := typeBits(tc.item); got != tc.want {
				t.Fatalf("typeBits(%+v) = %#x, want %#x", tc.item, got, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		value int
		want  uint32
	}{
		{value: -1},
		{value: 0, want: 0},
		{value: 1, want: 1},
	} {
		got, ok := fuseID(tc.value)
		if tc.value < 0 {
			if ok {
				t.Fatalf("fuseID(%d) accepted a negative id", tc.value)
			}
			continue
		}
		if !ok || got != tc.want {
			t.Fatalf("fuseID(%d) = (%d, %v)", tc.value, got, ok)
		}
	}
	maxInt := int(^uint(0) >> 1)
	maxUint32 := uint64(math.MaxUint32)
	got, ok := fuseID(maxInt)
	if uint64(maxInt) > maxUint32 {
		if ok {
			t.Fatalf("fuseID(%d) accepted an out-of-range id", maxInt)
		}
	} else if !ok || got != uint32(maxInt) {
		t.Fatalf("fuseID(maxInt) = (%d, %v), want (%d, true)", got, ok, maxInt)
	}
	if uint64(maxInt) > maxUint32 {
		tooLarge := int(maxUint32)
		tooLarge++
		if _, ok := fuseID(tooLarge); ok {
			t.Fatalf("fuseID(%d) accepted an out-of-range id", tooLarge)
		}
	}
}

func TestErrnoOf(t *testing.T) {
	wrappedErrno := fmt.Errorf("wrapped: %w", syscall.EBADF)
	cases := []struct {
		name string
		err  error
		want syscall.Errno
	}{
		{name: "nil", want: 0},
		{name: "errno", err: wrappedErrno, want: syscall.EBADF},
		{name: "cancelled", err: context.Canceled, want: syscall.EINTR},
		{name: "deadline", err: context.DeadlineExceeded, want: syscall.ETIMEDOUT},
		{name: "closed", err: errClosed, want: syscall.ENODEV},
		{name: "missing", err: os.ErrNotExist, want: syscall.ENOENT},
		{name: "permission", err: os.ErrPermission, want: syscall.EACCES},
		{name: "exists", err: os.ErrExist, want: syscall.EEXIST},
		{name: "invalid", err: os.ErrInvalid, want: syscall.EINVAL},
		{name: "unknown", err: errors.New("backend exploded"), want: syscall.EIO},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errnoOf(tc.err); got != tc.want {
				t.Fatalf("errnoOf(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestFillAttrForFileDirectoryAndSymlink(t *testing.T) {
	mtime := time.Unix(1234, 5678)
	atime := time.Unix(2345, 6789)
	ctime := time.Unix(3456, 7890)

	var file fuse.Attr
	fillAttr(&file, vfs.VFSItem{
		Size:     1025,
		UnixMode: 0o754,
		MTime:    mtime,
		ATime:    atime,
		CTime:    ctime,
		Uid:      42,
		Gid:      43,
	}, "/root/a.txt")
	if file.Mode != fuse.S_IFREG|0o754 || file.Nlink != 1 {
		t.Fatalf("file mode/nlink = %#o/%d", file.Mode, file.Nlink)
	}
	if file.Size != 1025 || file.Blocks != 3 {
		t.Fatalf("file size/blocks = %d/%d", file.Size, file.Blocks)
	}
	if !file.ModTime().Equal(mtime) || !file.AccessTime().Equal(atime) || !file.ChangeTime().Equal(ctime) {
		t.Fatalf("file times = %v/%v/%v, want %v/%v/%v", file.AccessTime(), file.ModTime(), file.ChangeTime(), atime, mtime, ctime)
	}
	if file.Uid != 42 || file.Gid != 43 {
		t.Fatalf("file owner = %d:%d, want 42:43", file.Uid, file.Gid)
	}

	var dir fuse.Attr
	fillAttr(&dir, vfs.VFSItem{IsDir: true, Size: 999}, "/root/sub")
	if dir.Mode != fuse.S_IFDIR|0o555 || dir.Nlink != 2 || dir.Size != 0 || dir.Blocks != 0 {
		t.Fatalf("directory attr = mode %#o nlink %d size %d blocks %d", dir.Mode, dir.Nlink, dir.Size, dir.Blocks)
	}

	var link fuse.Attr
	fillAttr(&link, vfs.VFSItem{IsSymlink: true, Size: 7, UnixMode: 0o777}, "/root/link")
	if link.Mode != fuse.S_IFLNK|0o777 || link.Nlink != 1 || link.Size != 7 || link.Blocks != 0 {
		t.Fatalf("symlink attr = mode %#o nlink %d size %d blocks %d", link.Mode, link.Nlink, link.Size, link.Blocks)
	}
}

func TestNodeGetattrUsesStagedSizeAndReaddirFilters(t *testing.T) {
	b, _ := newTestBridge(t, true)

	n := &node{b: b, path: "/root"}
	var root fuse.AttrOut
	if errno := n.Getattr(context.Background(), nil, &root); errno != 0 {
		t.Fatalf("Getattr(root) = %v", errno)
	}
	if !root.IsDir() || root.Nlink != 2 {
		t.Fatalf("root attr = mode %#o nlink %d", root.Mode, root.Nlink)
	}

	stream, errno := n.Readdir(context.Background())
	if errno != 0 {
		t.Fatalf("Readdir = %v", errno)
	}
	defer stream.Close()
	names := make(map[string]bool)
	for stream.HasNext() {
		entry, nextErrno := stream.Next()
		if nextErrno != 0 {
			t.Fatalf("Readdir.Next = %v", nextErrno)
		}
		names[entry.Name] = true
		if entry.Name == "." || entry.Name == ".." || entry.Name == "" {
			t.Fatalf("dot/empty entry reached FUSE: %+v", entry)
		}
	}
	for _, name := range []string{"a.txt", "b.bin", "sub"} {
		if !names[name] {
			t.Fatalf("Readdir missing %q: %v", name, names)
		}
	}

	staged, err := newStagedFile()
	if err != nil {
		t.Fatalf("newStagedFile: %v", err)
	}
	defer func() { _ = staged.Close() }()
	if _, err := staged.WriteAt([]byte("staged"), 0); err != nil {
		t.Fatalf("stage data: %v", err)
	}
	b.writeMu.Lock()
	b.writers["/root/new.txt"] = &writeHandle{path: "/root/new.txt", staged: staged}
	b.writeMu.Unlock()

	fake := b.v.(*fakeVFS)
	fake.statErr = true
	var stagedOut fuse.AttrOut
	if errno := (&node{b: b, path: "/root/new.txt"}).Getattr(context.Background(), nil, &stagedOut); errno != 0 {
		t.Fatalf("Getattr(staged) = %v", errno)
	}
	if stagedOut.Size != 6 || !stagedOut.IsRegular() {
		t.Fatalf("staged attr = mode %#o size %d", stagedOut.Mode, stagedOut.Size)
	}
}

func TestNodeOpenReadDirectoryAndClosedHandle(t *testing.T) {
	b, _ := newTestBridge(t, true)
	n := &node{b: b, path: "/root/a.txt"}

	handle, _, errno := n.Open(context.Background(), uint32(syscall.O_RDONLY))
	if errno != 0 {
		t.Fatalf("Open(file) = %v", errno)
	}
	file := handle.(*fileHandle)
	result, errno := file.Read(context.Background(), make([]byte, 3), 2)
	if errno != 0 {
		t.Fatalf("Read(file) = %v", errno)
	}
	data, _ := result.Bytes(nil)
	if string(data) != "llo" {
		t.Fatalf("Read(file) = %q, want %q", data, "llo")
	}
	if errno := file.Release(context.Background()); errno != 0 {
		t.Fatalf("Release(file) = %v", errno)
	}
	if result, errno := file.Read(context.Background(), make([]byte, 1), 0); result != nil || errno != syscall.ENODEV {
		t.Fatalf("Read(after Release) = (%v, %v), want (nil, ENODEV)", result, errno)
	}

	if _, _, errno := (&node{b: b, path: "/root"}).Open(context.Background(), uint32(syscall.O_RDONLY)); errno != syscall.EISDIR {
		t.Fatalf("Open(directory) = %v, want EISDIR", errno)
	}
}

func TestNodeWriteLifecycleAndStatfs(t *testing.T) {
	v := newLifecycleVFS()
	b := newBridge(v, "/root", Options{})
	t.Cleanup(b.close)

	n := &node{b: b, path: "/root/out.txt"}
	handle, _, errno := n.Open(context.Background(), uint32(syscall.O_WRONLY|syscall.O_TRUNC))
	if errno != 0 {
		t.Fatalf("Open(write) = %v", errno)
	}
	writer := handle.(*writeFileHandle)
	if count, errno := writer.Write(context.Background(), []byte("ok"), 0); errno != 0 || count != 2 {
		t.Fatalf("Write = (%d, %v), want (2, nil)", count, errno)
	}
	if errno := writer.Fsync(context.Background(), 0); errno != 0 {
		t.Fatalf("Fsync = %v", errno)
	}
	if errno := writer.Flush(context.Background()); errno != 0 {
		t.Fatalf("Flush = %v", errno)
	}
	if errno := writer.Release(context.Background()); errno != 0 {
		t.Fatalf("Release(write) = %v", errno)
	}

	var statfs fuse.StatfsOut
	if errno := n.Statfs(context.Background(), &statfs); errno != 0 {
		t.Fatalf("Statfs = %v", errno)
	}
	if statfs.Blocks != statfsTotalBlocks || statfs.Bfree != statfsTotalBlocks || statfs.Bsize != statfsBlockSize || statfs.NameLen != 255 {
		t.Fatalf("Statfs = %+v", statfs)
	}
}

func TestNodeWriteRefusalAndUnsupportedReadlink(t *testing.T) {
	b, _ := newTestBridge(t, true)
	n := &node{b: b, path: "/root/a.txt"}

	b.readOnly = true
	if errno := n.writeRefusal(); errno != syscall.EROFS {
		t.Fatalf("read-only writeRefusal = %v, want EROFS", errno)
	}
	b.readOnly = false
	b.writeOK = false
	if errno := n.writeRefusal(); errno != syscall.EROFS {
		t.Fatalf("backend write refusal = %v, want EROFS", errno)
	}
	if _, _, errno := n.Open(context.Background(), uint32(syscall.O_WRONLY)); errno != syscall.EROFS {
		t.Fatalf("Open(write) refusal = %v, want EROFS", errno)
	}

	if target, errno := n.Readlink(context.Background()); target != nil || errno != syscall.EIO {
		t.Fatalf("Readlink(no symlink backend) = (%q, %v), want (nil, EIO)", target, errno)
	}
}
