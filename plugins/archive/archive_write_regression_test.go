package archive

import (
	stdzip "archive/zip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func writeArchiveTestZIP(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := stdzip.NewWriter(file)
	for name, contents := range entries {
		member, err := writer.Create(name)
		if err != nil {
			_ = writer.Close()
			_ = file.Close()
			t.Fatal(err)
		}
		if _, err := io.WriteString(member, contents); err != nil {
			_ = writer.Close()
			_ = file.Close()
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func readArchiveTestZIPMember(t *testing.T, archivePath, memberName string) string {
	t.Helper()

	reader, err := stdzip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	for _, member := range reader.File {
		if member.Name != memberName {
			continue
		}
		file, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		contents, readErr := io.ReadAll(file)
		closeErr := file.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
		return string(contents)
	}
	t.Fatalf("archive member %q was not found", memberName)
	return ""
}

func openMutableArchiveTestVFS(t *testing.T, archivePath string) *ArchiveVFS {
	t.Helper()

	archiveVFS, err := NewArchiveVFS(&vfs.OSVFS{}, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		archiveVFS.mu.Lock()
		archiveVFS.isClosed = true
		if err := archiveVFS.performCleanup(); err != nil {
			t.Errorf("clean up archive VFS: %v", err)
		}
		archiveVFS.mu.Unlock()
	})
	return archiveVFS
}

func TestArchiveWriteCloseFailurePreservesExistingEntry(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "existing.zip")
	const original = "the complete original contents"
	writeArchiveTestZIP(t, archivePath, map[string]string{"entry.txt": original})
	archiveVFS := openMutableArchiveTestVFS(t, archivePath)

	wc, err := archiveVFS.Create(context.Background(), archiveVFS.Join(archiveVFS.GetPath(), "entry.txt"))
	if err != nil {
		t.Fatal(err)
	}
	writer := wc.(*archiveWriteWrapper)
	if _, err := io.WriteString(writer, "truncated"); err != nil {
		t.Fatal(err)
	}
	// Force the wrapper's write-side close to fail. Before the fix that error
	// was ignored and the short staging file replaced the complete entry.
	if err := writer.tmpFile.Close(); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err == nil {
		t.Fatal("archive write committed after its staging-file close failed")
	}

	if contents := readArchiveTestZIPMember(t, archivePath, "entry.txt"); contents != original {
		t.Fatalf("existing entry = %q, want preserved contents %q", contents, original)
	}
}

func TestArchiveWriteReloadsAfterUpdaterFinalization(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "reload.zip")
	writeArchiveTestZIP(t, archivePath, nil)
	archiveVFS := openMutableArchiveTestVFS(t, archivePath)

	wc, err := archiveVFS.Create(context.Background(), archiveVFS.Join(archiveVFS.GetPath(), "new.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(wc, "new contents"); err != nil {
		t.Fatal(err)
	}
	if err := wc.Close(); err != nil {
		t.Fatal(err)
	}

	rc, err := archiveVFS.Open(context.Background(), archiveVFS.Join(archiveVFS.GetPath(), "new.txt"))
	if err != nil {
		t.Fatalf("open newly finalized entry through reloaded VFS: %v", err)
	}
	contents, readErr := io.ReadAll(ctxReader{r: rc, ctx: context.Background()})
	closeErr := rc.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	if string(contents) != "new contents" {
		t.Fatalf("new entry = %q, want %q", contents, "new contents")
	}
}

type archiveReloadSentinelFS struct {
	closeCalls int
}

func (*archiveReloadSentinelFS) Open(string) (fs.File, error) {
	return nil, os.ErrNotExist
}

func (*archiveReloadSentinelFS) ReadDir(string) ([]fs.DirEntry, error) {
	return nil, os.ErrNotExist
}

func (*archiveReloadSentinelFS) Stat(string) (fs.FileInfo, error) {
	return nil, os.ErrNotExist
}

func (f *archiveReloadSentinelFS) Close() error {
	f.closeCalls++
	return nil
}

func TestArchiveReloadFailurePreservesPreviousIndex(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "broken.zip")
	if err := os.WriteFile(archivePath, []byte("not an archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	previous := &archiveReloadSentinelFS{}
	archiveVFS := &ArchiveVFS{
		parent:  &vfs.OSVFS{},
		arcPath: archivePath,
		fsys:    previous,
	}

	if err := archiveVFS.reloadFS(); err == nil {
		t.Fatal("reload of malformed archive unexpectedly succeeded")
	}
	if archiveVFS.fsys != previous {
		t.Fatal("reload failure discarded the previous archive index")
	}
	if previous.closeCalls != 0 {
		t.Fatalf("reload failure closed the previous index %d time(s)", previous.closeCalls)
	}
}

func TestArchiveReadMaterializationsAreReopenedReadOnly(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "reads.zip")
	writeArchiveTestZIP(t, archivePath, map[string]string{"entry.txt": "contents"})
	archiveVFS := openMutableArchiveTestVFS(t, archivePath)
	memberPath := archiveVFS.Join(archiveVFS.GetPath(), "entry.txt")

	t.Run("progress open", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), vfs.ProgressKey, vfs.ProgressCallback(func(string, int) {}))
		rc, err := archiveVFS.Open(ctx, memberPath)
		if err != nil {
			t.Fatal(err)
		}
		wrapper := rc.(*archiveReadWrapper)
		if _, err := wrapper.tmpFile.Write([]byte("x")); err == nil {
			_ = rc.Close()
			t.Fatal("progress materialization remained open for writing")
		}
		if err := rc.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("lazy random access", func(t *testing.T) {
		rc, err := archiveVFS.Open(context.Background(), memberPath)
		if err != nil {
			t.Fatal(err)
		}
		wrapper := rc.(*archiveReadWrapper)
		if _, err := wrapper.ReadAt(context.Background(), make([]byte, 1), 0); err != nil {
			_ = rc.Close()
			t.Fatal(err)
		}
		if _, err := wrapper.tmpFile.Write([]byte("x")); err == nil {
			_ = rc.Close()
			t.Fatal("lazy materialization remained open for writing")
		}
		if err := rc.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

var errArchiveTestAttributesUnsupported = errors.New("test destination does not support attributes")

type archiveExtractionCompatibilityVFS struct {
	vfs.VFS
}

func (v archiveExtractionCompatibilityVFS) MkDir(ctx context.Context, path string) error {
	if item, err := v.Stat(ctx, path); err == nil && item.IsDir {
		return os.ErrExist
	}
	return v.VFS.MkDir(ctx, path)
}

func (archiveExtractionCompatibilityVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return errArchiveTestAttributesUnsupported
}

func TestArchiveExtractionAcceptsExistingDirsAndUnsupportedAttributes(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "compatibility.zip")
	writeArchiveTestZIP(t, archivePath, map[string]string{
		"folder/":         "",
		"folder/file.txt": "contents",
	})
	archiveVFS := openMutableArchiveTestVFS(t, archivePath)
	destinationPath := filepath.Join(root, "destination")
	baseDestination := vfs.NewOSVFS(destinationPath)
	if err := baseDestination.MkDir(context.Background(), destinationPath); err != nil {
		t.Fatal(err)
	}
	destination := archiveExtractionCompatibilityVFS{VFS: baseDestination}

	if err := archiveVFS.CopyBulk(
		context.Background(), []string{"folder"}, destination, destinationPath, &dummyReporter{},
	); err != nil {
		t.Fatalf("extract with compatible non-idempotent destination: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(destinationPath, "folder", "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "contents" {
		t.Fatalf("extracted contents = %q, want %q", contents, "contents")
	}
}
