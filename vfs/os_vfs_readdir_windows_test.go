//go:build windows

package vfs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"
)

func TestReadCompleteOSDirectoryBaseHandlesLocalDirectory(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "visible.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, handled, err := readCompleteOSDirectoryBase(
		context.Background(), prepareOSPath(directory))
	if err != nil {
		t.Fatalf("fast directory read failed: %v", err)
	}
	if !handled {
		t.Fatal("fast directory read unexpectedly declined a local directory")
	}
	if len(items) != 1 || items[0].Name != "visible.txt" ||
		items[0].Size != 1 || !items[0].SizeKnown || items[0].IsDir {
		t.Fatalf("unexpected fast directory result: %#v", items)
	}
}

func TestAppendUTF16DirectoryNameMatchesUnicodeDecoder(t *testing.T) {
	tests := []struct {
		name  string
		units []uint16
	}{
		{name: "ascii", units: []uint16{'W', 'i', 'n', 'S', 'x', 'S'}},
		{name: "bmp", units: []uint16{'ф', 'а', 'й', 'л'}},
		{name: "surrogate-pair", units: utf16.Encode([]rune("folder-😀"))},
		{name: "unpaired-high", units: []uint16{'a', 0xd800, 'b'}},
		{name: "unpaired-low", units: []uint16{'a', 0xdc00, 'b'}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := string(appendUTF16DirectoryName(nil, test.units))
			want := string(utf16.Decode(test.units))
			if got != want {
				t.Fatalf("decoded name = %q, want %q", got, want)
			}
		})
	}
}

func TestReadCompleteOSDirectoryBasePublishesOneBoundedPreview(t *testing.T) {
	directory := t.TempDir()
	for index := 0; index < 150; index++ {
		name := filepath.Join(directory, fmt.Sprintf("folder-%03d", index))
		if err := os.Mkdir(name, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(directory, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	var previews [][]VFSItem
	items, handled, err := readCompleteOSDirectoryBasePhased(
		context.Background(), prepareOSPath(directory),
		func(preview []VFSItem) {
			previews = append(previews, append([]VFSItem(nil), preview...))
		})
	if err != nil || !handled {
		t.Fatalf("phased read handled=%v err=%v", handled, err)
	}
	if len(previews) != 1 || len(previews[0]) != directoryPreviewLimit {
		t.Fatalf("preview shape = %d callbacks/%d rows, want 1/%d",
			len(previews), func() int {
				if len(previews) == 0 {
					return 0
				}
				return len(previews[0])
			}(), directoryPreviewLimit)
	}
	if len(items) != 151 {
		t.Fatalf("authoritative base rows = %d, want 151", len(items))
	}
	directories := make([]VFSItem, 0, 150)
	for _, item := range items {
		if item.IsDir {
			directories = append(directories, item)
		}
	}
	for index, preview := range previews[0] {
		if !preview.IsDir || preview.Name != directories[index].Name {
			t.Fatalf("preview[%d] = %+v, want authoritative directory %+v",
				index, preview, directories[index])
		}
	}
}

func TestOSVFSReadDirWindowedPublishesExactBoundedCatalogBeforeBase(t *testing.T) {
	directory := t.TempDir()
	for _, name := range []string{"b-dir", "a-dir"} {
		if err := os.Mkdir(filepath.Join(directory, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"b.txt", "a.txt"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	filesystem := NewOSVFS(directory)
	var window DirectoryWindow
	var eventOrder []string
	err := filesystem.ReadDirWindowed(
		context.Background(), directory,
		DirectoryWindowRequest{Limit: 3, IncludeHidden: true},
		func(value DirectoryWindow) {
			window = value
			eventOrder = append(eventOrder, "window")
		},
		func(phase DirectoryReadPhase, _ []VFSItem) {
			if phase == DirectoryReadBase {
				eventOrder = append(eventOrder, "base")
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if window.TotalCount != 4 || len(window.Entries) != 3 {
		t.Fatalf("window = %#v, want 3/4 rows", window)
	}
	gotNames := []string{
		window.Entries[0].Name,
		window.Entries[1].Name,
		window.Entries[2].Name,
	}
	wantNames := []string{"a-dir", "b-dir", "a.txt"}
	if !slices.Equal(gotNames, wantNames) {
		t.Fatalf("window names = %#v, want %#v", gotNames, wantNames)
	}
	if !slices.Equal(eventOrder, []string{"window", "base"}) {
		t.Fatalf("callback order = %#v, want window before base", eventOrder)
	}
}

func TestReadCompleteOSDirectoryBaseDiagnosticPath(t *testing.T) {
	directory := os.Getenv("F4_TEST_LARGE_DIRECTORY")
	if directory == "" {
		t.Skip("F4_TEST_LARGE_DIRECTORY is not set")
	}
	items, handled, err := readCompleteOSDirectoryBase(
		context.Background(), prepareOSPath(directory))
	if err != nil {
		t.Fatalf("fast directory read failed: %v", err)
	}
	if !handled {
		t.Fatal("fast directory read declined the diagnostic path")
	}
	t.Logf("read %d entries", len(items))
}

func BenchmarkReadCompleteOSDirectoryBase(b *testing.B) {
	directory := os.Getenv("F4_TEST_LARGE_DIRECTORY")
	if directory == "" {
		b.Skip("F4_TEST_LARGE_DIRECTORY is not set")
	}
	path := prepareOSPath(directory)
	for _, bufferSize := range []int{64 << 10, 128 << 10, 256 << 10, 512 << 10, 1 << 20} {
		bufferSize := bufferSize
		b.Run(fmt.Sprintf("Win32/%dKiB", bufferSize>>10), func(b *testing.B) {
			b.ReportAllocs()
			var queryCount int64
			var queryDuration time.Duration
			var parseDuration time.Duration
			var windowDuration time.Duration
			var nameBytes int64
			for iteration := 0; iteration < b.N; iteration++ {
				iterationStarted := time.Now()
				items, handled, err := readCompleteOSDirectoryBaseWithQuery(
					context.Background(), path, bufferSize,
					queryCompleteOSDirectoryWin32,
					DirectoryWindowRequest{Limit: 48, IncludeHidden: true}, nil,
					func(window DirectoryWindow) {
						if window.TotalCount <= 0 || len(window.Entries) > 48 {
							b.Fatalf("invalid bounded window: %#v", window)
						}
						windowDuration += time.Since(iterationStarted)
					},
					func(stats completeOSDirectoryReadStats) {
						queryCount += int64(stats.QueryCount)
						queryDuration += stats.QueryDuration
						parseDuration += stats.ParseDuration
						nameBytes += int64(stats.NameBytes)
					})
				if err != nil || !handled || len(items) == 0 {
					b.Fatalf("fast directory read = %d rows, handled=%v, err=%v",
						len(items), handled, err)
				}
			}
			b.ReportMetric(float64(queryCount)/float64(b.N), "queries/op")
			b.ReportMetric(float64(queryDuration.Nanoseconds())/float64(b.N), "query-ns/op")
			b.ReportMetric(float64(parseDuration.Nanoseconds())/float64(b.N), "parse-ns/op")
			b.ReportMetric(float64(windowDuration.Nanoseconds())/float64(b.N), "window-ns/op")
			b.ReportMetric(float64(unsafe.Sizeof(VFSItem{})), "VFSItem-B")
			b.ReportMetric(float64(nameBytes)/float64(b.N), "name-B/op")
		})
	}
}
