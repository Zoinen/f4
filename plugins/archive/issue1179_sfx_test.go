package archive

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
	"unicode"

	"github.com/unxed/archives"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// validSevenZipStartHeader returns a 7z start header whose CRC holds, the
// smallest thing findEmbeddedArchive accepts as the start of a 7z archive.
func validSevenZipStartHeader() []byte {
	header := make([]byte, sevenZipStartHeaderSize)
	copy(header, "7z\xBC\xAF\x27\x1C")
	header[7] = 4
	binary.LittleEndian.PutUint32(header[8:12], crc32.ChecksumIEEE(header[12:]))
	return header
}

// issue1179InstallerStub imitates the stub of the official 7-Zip installer
// (7z2603-x64.exe): it holds the 7z signature as data of its own, followed by
// bytes that are not a valid start header, well before the real archive.
func issue1179InstallerStub() []byte {
	stub := bytes.Repeat([]byte("MZ stub code "), 64)
	fake := make([]byte, sevenZipStartHeaderSize)
	copy(fake, "7z\xBC\xAF\x27\x1C")
	copy(fake[12:], []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x80, 0x9d, 0x40})
	stub = append(stub, fake...)
	return append(stub, bytes.Repeat([]byte("more stub "), 128)...)
}

func issue1179BuildSevenZipSFX(t *testing.T, dir string) (string, int64) {
	t.Helper()
	archivePath, _ := issue915BuildArchive(t, dir, "payload.7z", archives.SevenZip{}, 2, 64*1024)
	payload, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	stub := issue1179InstallerStub()
	sfxPath := filepath.Join(dir, "setup.exe")
	if err := os.WriteFile(sfxPath, append(stub, payload...), 0o600); err != nil {
		t.Fatal(err)
	}
	return sfxPath, int64(len(stub))
}

func TestIssue1179FindEmbeddedArchiveSkipsSignatureInsideStub(t *testing.T) {
	sfxPath, archiveOffset := issue1179BuildSevenZipSFX(t, t.TempDir())

	got, found, err := findEmbeddedArchive(sfxPath)
	if err != nil {
		t.Fatal(err)
	}
	if !found || got.offset != archiveOffset || got.suffix != ".7z" {
		t.Fatalf("probe = %#v, found=%t; want the real archive at %d, not the signature inside the stub", got, found, archiveOffset)
	}
}

func TestIssue1179FindEmbeddedArchiveRejectsBareSevenZipSignature(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "stub.exe")
	if err := os.WriteFile(filename, issue1179InstallerStub(), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, found, err := findEmbeddedArchive(filename); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatalf("probe = %#v; a 7z signature without a valid start header is not an archive", got)
	}
}

func TestIssue1179SevenZipSFXOpensTestsAndExtracts(t *testing.T) {
	root := t.TempDir()
	sfxPath, _ := issue1179BuildSevenZipSFX(t, root)
	ctx := context.Background()

	archiveVFS, err := NewArchiveVFSContext(ctx, vfs.NewOSVFS(root), filepath.Base(sfxPath))
	if err != nil {
		t.Fatalf("enter SFX: %v", err)
	}
	var listed int
	if err := archiveVFS.ReadDir(ctx, archiveVFS.GetPath(), func(items []vfs.VFSItem) { listed += len(items) }); err != nil {
		t.Fatalf("list SFX: %v", err)
	}
	if listed == 0 {
		t.Fatal("list SFX: no members")
	}
	_ = archiveVFS.Close()

	if err := testArchiveWithPasswordPrompt(ctx, sfxPath, &issue915ProgressRecorder{}); err != nil {
		t.Fatalf("test SFX: %v", err)
	}

	dest := t.TempDir()
	if err := extractArchiveWithPasswordPrompt(ctx, sfxPath, dest, &issue915ProgressRecorder{}); err != nil {
		t.Fatalf("extract SFX: %v", err)
	}
	if stat, err := os.Stat(filepath.Join(dest, "member01.bin")); err != nil || stat.Size() != 64*1024 {
		t.Fatalf("extracted member01.bin: stat=%v err=%v", stat, err)
	}
}

func TestIssue1179ArchiveTestFailureButtonsHaveDistinctHotkeys(t *testing.T) {
	seen := map[rune]string{}
	for _, button := range archiveTestFailureButtons {
		_, hotkey, _ := vtui.ParseAmpersandString(button)
		if hotkey == 0 {
			t.Fatalf("button %q has no hotkey", button)
		}
		hotkey = unicode.ToLower(hotkey)
		if other, ok := seen[hotkey]; ok {
			t.Fatalf("buttons %q and %q share hotkey %q", other, button, hotkey)
		}
		seen[hotkey] = button
	}
}
