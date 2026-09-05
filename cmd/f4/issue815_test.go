package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	archiveplugin "github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestIssue815QueuedF5CopiesCompressedTarContents(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	root := t.TempDir()
	archivePath := filepath.Join(root, "f4-linux-amd64.tar.gz")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(archiveFile)
	tarWriter := tar.NewWriter(gzipWriter)

	payload := make([]byte, 32*1024*1024)
	var state uint32 = 1
	for i := range payload {
		state = state*1664525 + 1013904223
		payload[i] = byte(state >> 24)
	}
	entries := []struct {
		name string
		data []byte
	}{
		{name: "f4", data: payload},
		{name: "LICENSE", data: []byte("license\n")},
		{name: "README.md", data: []byte("readme\n")},
		{name: "README.ru.md", data: []byte("readme ru\n")},
		{name: "README.zh_CN.md", data: []byte("readme zh\n")},
		{name: "README.ja.md", data: []byte("readme ja\n")},
	}
	for _, entry := range entries {
		if err := tarWriter.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o644, Size: int64(len(entry.data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.WriteHeader(&tar.Header{Name: "other/", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.WriteHeader(&tar.Header{Name: "other/ignored.txt", Mode: 0o644, Size: 8}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write([]byte("ignored\n")); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}

	source, err := archiveplugin.NewArchiveVFS(vfs.NewOSVFS(root), archivePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })

	destination := t.TempDir()
	destinationVFS := vfs.NewOSVFS(destination)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.name)
	}

	previousQueue := GlobalQueueManager
	queue := &OpQueueManager{activeKeys: make(map[string]bool)}
	GlobalQueueManager = queue
	t.Cleanup(func() { GlobalQueueManager = previousQueue })

	sourceBasePath := source.GetPath()
	ExecuteFileOpAt(nil, source, destinationVFS, sourceBasePath, names, destination+string(os.PathSeparator), false, 0, nil)
	if err := source.SetPath(source.Join(sourceBasePath, "other")); err != nil {
		t.Fatal(err)
	}
	queue.mu.Lock()
	if len(queue.tasks) != 1 {
		queue.mu.Unlock()
		t.Fatalf("queued tasks = %d, want 1", len(queue.tasks))
	}
	task := queue.tasks[0]
	queue.mu.Unlock()

	if err := task.Run(context.Background(), &DummyReporter{}, nil); err != nil {
		t.Fatalf("queued F5 from compressed tar failed: %v", err)
	}
	for _, entry := range entries {
		got, err := os.ReadFile(filepath.Join(destination, entry.name))
		if err != nil {
			t.Fatalf("read copied %s: %v", entry.name, err)
		}
		if !bytes.Equal(got, entry.data) {
			t.Fatalf("copied %s content differs", entry.name)
		}
	}
}
