package archive

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/zip"
)

// issue1186BuildZipSFX writes a zip behind an executable stub with the offsets
// in its central directory counting the stub in, the way WinRAR writes a
// self-extracting archive.
func issue1186BuildZipSFX(t *testing.T, dir string) (string, map[string][]byte) {
	t.Helper()
	members := map[string][]byte{
		"first.txt":  bytes.Repeat([]byte("first member of the self-extracting archive\n"), 40),
		"second.txt": bytes.Repeat([]byte("second member of the self-extracting archive\n"), 40),
	}

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"first.txt", "second.txt"} {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(members[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()

	stub := bytes.Repeat([]byte("MZ the self-extracting stub goes here "), 64)
	end := bytes.LastIndex(data, []byte("PK\x05\x06"))
	if end < 0 {
		t.Fatal("end of central directory not found")
	}
	directoryOffset := int(binary.LittleEndian.Uint32(data[end+16 : end+20]))
	binary.LittleEndian.PutUint32(data[end+16:end+20], uint32(directoryOffset+len(stub)))
	for p := directoryOffset; p < end; {
		nameLen := int(binary.LittleEndian.Uint16(data[p+28 : p+30]))
		extraLen := int(binary.LittleEndian.Uint16(data[p+30 : p+32]))
		commentLen := int(binary.LittleEndian.Uint16(data[p+32 : p+34]))
		offset := int(binary.LittleEndian.Uint32(data[p+42 : p+46]))
		binary.LittleEndian.PutUint32(data[p+42:p+46], uint32(offset+len(stub)))
		p += 46 + nameLen + extraLen + commentLen
	}

	path := filepath.Join(dir, "setup.exe")
	if err := os.WriteFile(path, append(stub, data...), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, members
}

// Issue #1186: a zip inside a self-extracting archive is read where it lies.
// Copying it out from under the stub invalidates the offsets that counted the
// stub in, and the entries are then read without the parameters their headers
// carry -- for a WinRAR archive with AES that meant "zip: AES info missing"
// for every member, once the password had been given.
func TestIssue1186ZipSFXIsReadWhole(t *testing.T) {
	root := t.TempDir()
	path, members := issue1186BuildZipSFX(t, root)
	ctx := context.Background()

	embedded, backingPath, closer, err := materializeLocalSFX(path)
	if err != nil {
		t.Fatalf("materializeLocalSFX: %v", err)
	}
	if closer != nil {
		_ = closer.Close()
	}
	if embedded.format != "zip" || backingPath != path || closer != nil {
		t.Fatalf("materializeLocalSFX = %q, %s, closer %v; want the file itself, read as zip",
			embedded.format, filepath.Base(backingPath), closer != nil)
	}

	archiveVFS, err := NewArchiveVFSContext(ctx, vfs.NewOSVFS(root), filepath.Base(path))
	if err != nil {
		t.Fatalf("enter: %v", err)
	}
	listed := 0
	if err := archiveVFS.ReadDir(ctx, archiveVFS.GetPath(), func(items []vfs.VFSItem) { listed += len(items) }); err != nil {
		t.Fatalf("list: %v", err)
	}
	_ = archiveVFS.Close()
	if listed != len(members) {
		t.Fatalf("listed %d entries, want %d", listed, len(members))
	}

	dest := t.TempDir()
	if err := extractArchiveOnce(ctx, path, dest, "", &issue915ProgressRecorder{}); err != nil {
		t.Fatalf("extract: %v", err)
	}
	for name, want := range members {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("extracted %s: %d bytes, want %d (err %v)", name, len(got), len(want), err)
		}
	}

	if err := testArchiveOnce(ctx, path, path, "", &issue915ProgressRecorder{}); err != nil {
		t.Fatalf("test: %v", err)
	}
}
