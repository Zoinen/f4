package archive

import (
	stdtar "archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestArchiveVFSF3FallsBackAfterCorruptGzipRandomAccess(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	archivePath := filepath.Join(root, "test.tar.gz")
	writeIssue815TarGZ(t, archivePath)

	for _, readMethod := range []string{"read-at", "read"} {
		t.Run(readMethod, func(t *testing.T) {
			v, err := NewArchiveVFS(vfs.NewOSVFS(root), filepath.Base(archivePath))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = v.Close() })

			opened, err := v.Open(context.Background(), v.Join(v.GetPath(), "help/en.hlf"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = opened.Close() })
			wrapper, ok := opened.(*archiveReadWrapper)
			if !ok {
				t.Fatalf("Open returned %T, want *archiveReadWrapper", opened)
			}

			wrapper.mu.Lock()
			_ = wrapper.f.Close()
			wrapper.f = corruptArchiveMember{}
			wrapper.mu.Unlock()

			want := []byte("help from a sequential fallback\n")
			if readMethod == "read-at" {
				got := make([]byte, len(want))
				n, err := opened.ReadAt(context.Background(), got, 0)
				if err != nil && !errors.Is(err, io.EOF) {
					t.Fatal(err)
				}
				if string(got[:n]) != string(want) {
					t.Fatalf("ReadAt returned %q, want %q", got[:n], want)
				}
			} else {
				got := make([]byte, len(want))
				n, err := opened.Read(context.Background(), got)
				if err != nil && !errors.Is(err, io.EOF) {
					t.Fatal(err)
				}
				if string(got[:n]) != string(want) {
					t.Fatalf("Read returned %q, want %q", got[:n], want)
				}
			}

			if path, ok := wrapper.LocalPath(); !ok || path == "" {
				t.Fatalf("fallback did not publish a local member path: %q, %v", path, ok)
			}
		})
	}
}

func writeIssue815TarGZ(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := stdtar.NewWriter(gzipWriter)
	contents := []byte("help from a sequential fallback\n")
	if err := tarWriter.WriteHeader(&stdtar.Header{Name: "help/en.hlf", Mode: 0o644, Size: int64(len(contents))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

type corruptArchiveMember struct{}

func (corruptArchiveMember) Read([]byte) (int, error) {
	return 0, errors.New("flate: corrupt input before offset 310846")
}

func (corruptArchiveMember) Close() error { return nil }

func (corruptArchiveMember) Stat() (fs.FileInfo, error) {
	return corruptArchiveMemberInfo{}, nil
}

func (corruptArchiveMember) Seek(int64, int) (int64, error) { return 0, nil }

type corruptArchiveMemberInfo struct{}

func (corruptArchiveMemberInfo) Name() string       { return "en.hlf" }
func (corruptArchiveMemberInfo) Size() int64        { return int64(len("help from a sequential fallback\n")) }
func (corruptArchiveMemberInfo) Mode() fs.FileMode  { return 0o644 }
func (corruptArchiveMemberInfo) ModTime() time.Time { return time.Time{} }
func (corruptArchiveMemberInfo) IsDir() bool        { return false }
func (corruptArchiveMemberInfo) Sys() any           { return nil }
