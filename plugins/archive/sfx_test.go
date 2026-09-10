package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	zipvolume "github.com/unxed/zip"
)

func TestFindEmbeddedArchive(t *testing.T) {
	tests := []struct {
		name   string
		magic  []byte
		format string
		suffix string
	}{
		{name: "zip", magic: []byte("PK\x03\x04"), format: "zip", suffix: ".zip"},
		{name: "7z", magic: []byte("7z\xBC\xAF\x27\x1C"), format: "fallback", suffix: ".7z"},
		{name: "rar", magic: []byte("Rar!\x1A\x07\x00"), format: "fallback", suffix: ".rar"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := []byte("self-extractor stub")
			filename := filepath.Join(t.TempDir(), "payload.exe")
			if err := os.WriteFile(filename, append(stub, test.magic...), 0600); err != nil {
				t.Fatal(err)
			}

			got, found, err := findEmbeddedArchive(filename)
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("embedded archive was not detected")
			}
			if got.offset != int64(len(stub)) || got.format != test.format || got.suffix != test.suffix {
				t.Fatalf("probe = %#v, want offset %d, format %q, suffix %q", got, len(stub), test.format, test.suffix)
			}
		})
	}
}

func TestArchiveProviderOpensZipSFX(t *testing.T) {
	var archiveBytes bytes.Buffer
	zipWriter := zip.NewWriter(&archiveBytes)
	entry, err := zipWriter.Create("inside.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("from sfx\n")); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	filename := filepath.Join(root, "archive.exe")
	stub := []byte("stub bytes before the archive\n")
	if err := os.WriteFile(filename, append(stub, archiveBytes.Bytes()...), 0600); err != nil {
		t.Fatal(err)
	}

	parent := vfs.NewOSVFS(root)
	provider := &ArchiveProvider{}
	if !provider.CanOpen(context.Background(), parent, "archive.exe") {
		t.Fatal("ArchiveProvider rejected a ZIP SFX")
	}
	opened, err := provider.Open(context.Background(), parent, "archive.exe")
	if err != nil {
		t.Fatalf("open ZIP SFX: %v", err)
	}
	archiveVFS, ok := opened.(*ArchiveVFS)
	if !ok {
		t.Fatalf("opened VFS = %T, want *ArchiveVFS", opened)
	}
	t.Cleanup(func() { _ = archiveVFS.Close() })

	var items []vfs.VFSItem
	if err := archiveVFS.ReadDir(context.Background(), archiveVFS.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("read ZIP SFX root: %v", err)
	}
	if len(items) != 1 || items[0].Name != "inside.txt" {
		t.Fatalf("root entries = %#v, want inside.txt", items)
	}
	if archiveVFS.sfxOffset != int64(len(stub)) || archiveVFS.sfxSuffix != ".zip" {
		t.Fatalf("SFX metadata = offset %d, suffix %q", archiveVFS.sfxOffset, archiveVFS.sfxSuffix)
	}

	clone, ok := archiveVFS.Clone().(*ArchiveVFS)
	if !ok {
		t.Fatal("SFX clone did not return *ArchiveVFS")
	}
	t.Cleanup(func() { _ = clone.Close() })
	var cloneItems []vfs.VFSItem
	if err := clone.ReadDir(context.Background(), clone.GetPath(), func(chunk []vfs.VFSItem) {
		cloneItems = append(cloneItems, chunk...)
	}); err != nil {
		t.Fatalf("read cloned ZIP SFX root: %v", err)
	}
	if len(cloneItems) != 1 || cloneItems[0].Name != "inside.txt" {
		t.Fatalf("cloned root entries = %#v, want inside.txt", cloneItems)
	}
}

func TestArchiveProviderOpensZipSFXMultiVolume(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "bundle.zip")
	multiWriter, err := zipvolume.NewMultiVolumeWriter(mainPath, 64)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(multiWriter)
	entry, err := zipWriter.Create("inside.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(bytes.Repeat([]byte("from split sfx\n"), 32)); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := multiWriter.Close(); err != nil {
		t.Fatal(err)
	}

	archiveBytes, err := os.ReadFile(mainPath)
	if err != nil {
		t.Fatal(err)
	}
	sfxPath := filepath.Join(root, "bundle.exe")
	if err := os.WriteFile(sfxPath, append([]byte("stub bytes before the archive\n"), archiveBytes...), 0600); err != nil { // #nosec G703 -- sfxPath is inside the per-test directory created by testing.T.TempDir.
		t.Fatal(err)
	}

	parent := vfs.NewOSVFS(root)
	provider := &ArchiveProvider{}
	opened, err := provider.Open(context.Background(), parent, "bundle.exe")
	if err != nil {
		t.Fatalf("open multi-volume ZIP SFX: %v", err)
	}
	archiveVFS, ok := opened.(*ArchiveVFS)
	if !ok {
		t.Fatalf("opened VFS = %T, want *ArchiveVFS", opened)
	}
	t.Cleanup(func() { _ = archiveVFS.Close() })

	var items []vfs.VFSItem
	if err := archiveVFS.ReadDir(context.Background(), archiveVFS.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("read multi-volume ZIP SFX root: %v", err)
	}
	if len(items) != 1 || items[0].Name != "inside.txt" {
		t.Fatalf("root entries = %#v, want inside.txt", items)
	}
}

func TestFindEmbeddedArchiveRejectsPlainFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "plain.exe")
	if err := os.WriteFile(filename, []byte("not an archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := findEmbeddedArchive(filename); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatal("plain file was detected as an embedded archive")
	}
}

func TestFindEmbeddedArchiveAcrossProbeChunks(t *testing.T) {
	stub := bytes.Repeat([]byte{'x'}, 64<<10-2)
	filename := filepath.Join(t.TempDir(), "split.exe")
	if err := os.WriteFile(filename, append(stub, []byte("7z\xBC\xAF\x27\x1C")...), 0600); err != nil {
		t.Fatal(err)
	}

	got, found, err := findEmbeddedArchive(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got.offset != int64(len(stub)) || got.suffix != ".7z" {
		t.Fatalf("probe = %#v, found=%t; want offset %d and .7z", got, found, len(stub))
	}
}

func TestMaterializeEmbeddedArchive(t *testing.T) {
	stub := []byte("stub")
	payload := []byte("archive payload")
	filename := filepath.Join(t.TempDir(), "archive.exe")
	if err := os.WriteFile(filename, append(stub, payload...), 0600); err != nil {
		t.Fatal(err)
	}

	materialized, closer, err := materializeEmbeddedArchive(filename, embeddedArchive{
		format: "fallback",
		suffix: ".7z",
		offset: int64(len(stub)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if closer == nil {
		t.Fatal("materialized archive has no cleanup closer")
	}
	got, err := os.ReadFile(materialized)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("materialized payload = %q, want %q", got, payload)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(materialized); !os.IsNotExist(err) {
		t.Fatalf("materialized file still exists after cleanup: %v", err)
	}

	same, noCloser, err := materializeEmbeddedArchive(filename, embeddedArchive{offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	if same != filename || noCloser != nil {
		t.Fatalf("zero-offset materialization = %q, closer=%v", same, noCloser)
	}
}

func TestMaterializeEmbeddedArchiveCopiesZipVolumes(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, "bundle.exe")
	if err := os.WriteFile(filename, []byte("stubarchive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bundle.z01"), []byte("volume one"), 0600); err != nil { // #nosec G703 -- root is the per-test directory created by testing.T.TempDir.
		t.Fatal(err)
	}

	materialized, closer, err := materializeEmbeddedArchive(filename, embeddedArchive{
		format: "zip",
		suffix: ".zip",
		offset: int64(len("stub")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if closer == nil {
		t.Fatal("multi-volume materialization has no cleanup closer")
	}
	if filepath.Base(materialized) != "bundle.zip" {
		t.Fatalf("materialized name = %q, want bundle.zip", filepath.Base(materialized))
	}
	if got, err := os.ReadFile(materialized); err != nil {
		t.Fatal(err)
	} else if string(got) != "archive" {
		t.Fatalf("materialized payload = %q, want archive", got)
	}
	if got, err := os.ReadFile(filepath.Join(filepath.Dir(materialized), "bundle.z01")); err != nil {
		t.Fatal(err)
	} else if string(got) != "volume one" {
		t.Fatalf("materialized volume = %q, want volume one", got)
	}
	targetDir := filepath.Dir(materialized)
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(targetDir); !os.IsNotExist(err) {
		t.Fatalf("materialized directory still exists after cleanup: %v", err)
	}
}

func TestMaterializeEmbeddedArchiveRejectsPastEnd(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "short.exe")
	if err := os.WriteFile(filename, []byte("short"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := materializeEmbeddedArchive(filename, embeddedArchive{offset: 100, suffix: ".zip"}); !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("error = %v, want os.ErrInvalid", err)
	}
}
