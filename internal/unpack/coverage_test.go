package unpack

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestUnpackSanitizePathCleansSafeNames(t *testing.T) {
	destDir := t.TempDir()
	for _, tt := range []struct {
		name string
		want string
	}{
		{name: "folder/../file", want: "file"},
		{name: "./folder/file", want: filepath.Join("folder", "file")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SanitizePath(tt.name, destDir)
			if err != nil {
				t.Fatalf("SanitizePath(%q) error: %v", tt.name, err)
			}
			want := filepath.Join(destDir, tt.want)
			if got != want {
				t.Fatalf("SanitizePath(%q) = %q, want %q", tt.name, got, want)
			}
		})
	}
}

func TestUnpackExtractEntryDirectoryDefaultModeAndErrors(t *testing.T) {
	destDir := t.TempDir()
	if err := extractEntry(archiveEntry{name: "nested/path", isDir: true}, destDir); err != nil {
		t.Fatalf("extractEntry(directory) error: %v", err)
	}
	if info, err := os.Stat(filepath.Join(destDir, "nested", "path")); err != nil || !info.IsDir() {
		t.Fatalf("directory entry was not created: info=%v err=%v", info, err)
	}

	filePath := filepath.Join(destDir, "nested", "file")
	if err := extractEntry(archiveEntry{
		name: "nested/file",
		open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("content")), nil },
	}, destDir); err != nil {
		t.Fatalf("extractEntry(file) error: %v", err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	wantMode := os.FileMode(0644)
	if runtime.GOOS == "windows" {
		// Windows does not retain Unix permission bits on regular files.
		wantMode = 0666
	}
	if got := info.Mode().Perm(); got != wantMode {
		t.Errorf("default file mode = %o, want %o", got, wantMode)
	}

	openErr := errors.New("open failed")
	if err := extractEntry(archiveEntry{
		name: "broken",
		open: func() (io.ReadCloser, error) { return nil, openErr },
	}, destDir); !errors.Is(err, openErr) {
		t.Fatalf("extractEntry(open error) = %v, want %v", err, openErr)
	}

	opened := false
	if err := extractEntry(archiveEntry{
		name: "../outside",
		open: func() (io.ReadCloser, error) {
			opened = true
			return io.NopCloser(strings.NewReader("unsafe")), nil
		},
	}, destDir); err != nil {
		t.Fatalf("extractEntry(unsafe path) error: %v", err)
	}
	if opened {
		t.Fatal("extractEntry opened an unsafe archive member")
	}
}

func TestUnpackZipParallelFallbackAndWorkerLimit(t *testing.T) {
	zipData := makeZipForCoverage(t)
	for _, workers := range []int{0, 1, 8} {
		t.Run("workers="+strconv.Itoa(workers), func(t *testing.T) {
			destDir := t.TempDir()
			if err := ZipParallel(zipData, destDir, workers); err != nil {
				t.Fatalf("ZipParallel(%d) error: %v", workers, err)
			}
			for name, want := range map[string]string{"a.txt": "a", "nested/b.txt": "b"} {
				got, err := os.ReadFile(filepath.Join(destDir, name))
				if err != nil || string(got) != want {
					t.Fatalf("extracted %s = %q, %v; want %q", name, got, err, want)
				}
			}
		})
	}
}

func makeZipForCoverage(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create("nested/"); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"a.txt": "a", "nested/b.txt": "b"} {
		f, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
