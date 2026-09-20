package archive

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/zip"
	zipperarchive "github.com/unxed/zipper/archive"
)

// zipBytes returns a ZIP archive holding one stored member.
func zipBytes(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// openOuterArchive writes members into a ZIP on disk and returns it opened as
// a panel would hold it.
func openOuterArchive(t *testing.T, members map[string][]byte) (*ArchiveVFS, string) {
	t.Helper()
	root := t.TempDir()
	outerPath := filepath.Join(root, "outer.zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range members {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outerPath, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	outer, err := NewArchiveVFS(vfs.NewOSVFS(root), outerPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := outer.Close(); err != nil {
			t.Errorf("close outer archive VFS: %v", err)
		}
	})
	return outer, outerPath
}

// Inside another archive the name is not what says an archive: a .jar is a
// ZIP, and so is a member with a misleading extension or none at all. The
// content is what decides, exactly as far2l decides it. Before this, Enter on
// a nested .jar found no provider, fell through to the executor, and reported
// that non-runnable files cannot be run on a remote file system.
func TestArchiveProviderDetectsNestedArchiveByContent(t *testing.T) {
	inner := zipBytes(t, "META-INF/MANIFEST.MF", []byte("Manifest-Version: 1.0"))
	outer, outerPath := openOuterArchive(t, map[string][]byte{
		"app.jar":   inner,
		"blob.dat":  inner,
		"noext":     inner,
		"notes.txt": []byte("plain text, not an archive"),
	})

	provider := &ArchiveProvider{}
	ctx := context.Background()

	for _, name := range []string{"app.jar", "blob.dat", "noext"} {
		member := outer.Join(outerPath, name)
		if !provider.CanOpen(ctx, outer, member) {
			t.Errorf("CanOpen(%q) = false, want true: the member is a ZIP", name)
		}
		if !provider.PanelEnterAllowed(ctx, outer, member) {
			t.Errorf("PanelEnterAllowed(%q) = false: inside an archive Enter has nothing else to mean", name)
		}
	}

	if provider.CanOpen(ctx, outer, outer.Join(outerPath, "notes.txt")) {
		t.Error("CanOpen(notes.txt) = true, want false")
	}
}

// The detection has to survive being acted on: opening the nested member must
// produce a file system holding what the nested archive holds.
func TestArchiveProviderOpensNestedArchiveWithForeignExtension(t *testing.T) {
	outer, outerPath := openOuterArchive(t, map[string][]byte{
		"app.jar": zipBytes(t, "META-INF/MANIFEST.MF", []byte("Manifest-Version: 1.0")),
	})

	provider := &ArchiveProvider{}
	ctx := context.Background()
	inner, err := provider.Open(ctx, outer, outer.Join(outerPath, "app.jar"))
	if err != nil {
		t.Fatalf("open nested .jar: %v", err)
	}
	t.Cleanup(func() {
		if err := inner.Close(); err != nil {
			t.Errorf("close nested archive VFS: %v", err)
		}
	})

	var names []string
	if err := inner.ReadDir(ctx, inner.GetPath(), func(items []vfs.VFSItem) {
		for _, item := range items {
			names = append(names, item.Name)
		}
	}); err != nil {
		t.Fatalf("read nested .jar: %v", err)
	}
	if len(names) != 1 || names[0] != "META-INF" {
		t.Fatalf("nested .jar listing = %v, want [META-INF]", names)
	}
}

// A self-extracting archive is found behind its stub when nested, just as it
// is on disk. Enter is not held back from it the way it is on disk, because
// there is no execution and no association inside an archive for Enter to be
// held back in favour of.
func TestArchiveProviderFindsNestedSFX(t *testing.T) {
	stubbed := append([]byte("MZ stub bytes that are not an archive"),
		zipBytes(t, "payload.txt", []byte("payload"))...)
	outer, outerPath := openOuterArchive(t, map[string][]byte{"setup.exe": stubbed})

	provider := &ArchiveProvider{}
	ctx := context.Background()
	member := outer.Join(outerPath, "setup.exe")

	if !provider.CanOpen(ctx, outer, member) {
		t.Fatal("a nested self-extracting archive must still be openable")
	}
}

// The local file system applies the Enter mask, and a .jar is not on it: both
// ancestors browse a package on Enter, so f4 does too. A document is on it, so
// it keeps Enter for its association while Ctrl+PgDn still browses it.
func TestArchiveProviderLocalEnterFollowsTheMask(t *testing.T) {
	root := t.TempDir()
	container := zipBytes(t, "a.txt", []byte("x"))
	for _, name := range []string{"app.jar", "report.docx"} {
		if err := os.WriteFile(filepath.Join(root, name), container, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	provider := &ArchiveProvider{}
	parent := vfs.NewOSVFS(root)
	ctx := context.Background()
	for _, name := range []string{"app.jar", "report.docx"} {
		if !provider.CanOpen(ctx, parent, name) {
			t.Fatalf("%s: Ctrl+PgDn must open the container", name)
		}
	}
	if !provider.PanelEnterAllowed(ctx, parent, "app.jar") {
		t.Error("Enter browses a .jar in Far and in far2l")
	}
	if provider.PanelEnterAllowed(ctx, parent, "report.docx") {
		t.Error("Enter on a .docx belongs to its association")
	}
}

// The probe runs where the panel waits for it, so a file system that has not
// offered to produce the bytes cheaply is never asked for them. It keeps
// answering from the name, as it did before.
func TestArchiveProviderDoesNotProbeFileSystemsWithoutAHead(t *testing.T) {
	remote := &remoteArchiveFixtureVFS{
		uri:  "cloud://bucket/app.jar",
		name: "app.jar",
		data: zipBytes(t, "a.txt", []byte("x")),
	}

	provider := &ArchiveProvider{}
	if provider.CanOpen(context.Background(), remote, remote.uri) {
		t.Error("a remote .jar must not be probed, and its name declares nothing")
	}
	if got := remote.openCount; got != 0 {
		t.Errorf("remote Open calls = %d, want 0: the probe must not reach the network", got)
	}

	named := &remoteArchiveFixtureVFS{
		uri:  "cloud://bucket/app.zip",
		name: "app.zip",
		data: zipBytes(t, "a.txt", []byte("x")),
	}
	if !provider.CanOpen(context.Background(), named, named.uri) {
		t.Error("a remote .zip is still recognized from its name")
	}
}

// The probe must never be the thing that raises a password dialog: it can run
// on the very thread that would have to draw it. An encrypted member whose
// password is not installed reports an error, and the file is simply not
// treated as an archive.
func TestArchiveHeadReadDoesNotPromptForAPassword(t *testing.T) {
	root := t.TempDir()
	member := filepath.Join(root, "inner.jar")
	if err := os.WriteFile(member, zipBytes(t, "a.txt", []byte("x")), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(member)
	if err != nil {
		t.Fatal(err)
	}

	outerPath := filepath.Join(root, "secret.zip")
	archiver, err := zipperarchive.NewArchiver(outerPath, root, zipperarchive.Options{Password: "correct"})
	if err != nil {
		t.Fatal(err)
	}
	if err := archiver.Archive(context.Background(), map[string]os.FileInfo{member: info}); err != nil {
		t.Fatal(err)
	}
	if err := archiver.Close(); err != nil {
		t.Fatal(err)
	}

	prompts := 0
	previousPrompt := archivePasswordPrompt
	archivePasswordPrompt = func(context.Context, string) (string, error) {
		prompts++
		return "", context.Canceled
	}
	t.Cleanup(func() { archivePasswordPrompt = previousPrompt })

	// A ZIP keeps its names readable, so the archive itself opens with no
	// password at all and only the member refuses to be read.
	outer, err := NewArchiveVFS(vfs.NewOSVFS(root), outerPath)
	if err != nil {
		t.Fatalf("open encrypted outer archive: %v", err)
	}
	t.Cleanup(func() {
		if err := outer.Close(); err != nil {
			t.Errorf("close outer archive VFS: %v", err)
		}
	})

	buf := make([]byte, 64)
	if _, err := outer.ReadHead(context.Background(), outer.Join(outerPath, "inner.jar"), buf); err == nil {
		t.Error("reading an encrypted member without its password must fail, not succeed")
	}
	if prompts != 0 {
		t.Errorf("password prompts during ReadHead = %d, want 0", prompts)
	}
}

// Both sides ask the same question with the same scanner; the only thing that
// differs is how many bytes it is worth asking for, and that is a budget, not
// a second rule. On disk the scan reaches an installer stub of any size. Off
// it every byte is decompressed while the panel waits, so it stops at far2l's
// window and the file falls back to being judged by its name.
func TestArchiveProbeBudgetFollowsWhereTheBytesAre(t *testing.T) {
	stubbed := append(bytes.Repeat([]byte{'M'}, headProbeLimit+(64<<10)),
		zipBytes(t, "payload.txt", []byte("payload"))...)

	provider := &ArchiveProvider{}
	ctx := context.Background()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "setup.exe"), stubbed, 0o600); err != nil {
		t.Fatal(err)
	}
	if !provider.CanOpen(ctx, vfs.NewOSVFS(root), "setup.exe") {
		t.Error("on disk the scan must reach an archive behind a stub of any size")
	}

	outer, outerPath := openOuterArchive(t, map[string][]byte{"setup.exe": stubbed})
	if provider.CanOpen(ctx, outer, outer.Join(outerPath, "setup.exe")) {
		t.Error("off disk the scan must stop at the window rather than decompress a whole member")
	}
}

// A ZIP member is decoded as it is read, so a bounded read really is bounded.
// A member of anything else is unpacked whole before its first byte comes
// back -- a RAR member is copied to a temporary file in full -- so those
// containers decline the probe outright rather than charge a file's entire
// size for its first block while the panel waits.
func TestArchiveHeadReadDeclinesOutsideZip(t *testing.T) {
	root := t.TempDir()
	member := filepath.Join(root, "inner.jar")
	if err := os.WriteFile(member, zipBytes(t, "a.txt", []byte("x")), 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(member)
	if err != nil {
		t.Fatal(err)
	}

	outerPath := filepath.Join(root, "outer.tar")
	archiver, err := zipperarchive.NewArchiver(outerPath, root, zipperarchive.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := archiver.Archive(context.Background(), map[string]os.FileInfo{member: info}); err != nil {
		t.Fatal(err)
	}
	if err := archiver.Close(); err != nil {
		t.Fatal(err)
	}

	outer, err := NewArchiveVFS(vfs.NewOSVFS(root), outerPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := outer.Close(); err != nil {
			t.Errorf("close outer archive VFS: %v", err)
		}
	})

	inner := outer.Join(outerPath, "inner.jar")
	if _, err := outer.ReadHead(context.Background(), inner, make([]byte, 64)); !errors.Is(err, vfs.ErrHeadUnavailable) {
		t.Errorf("ReadHead error = %v, want vfs.ErrHeadUnavailable", err)
	}
	if (&ArchiveProvider{}).CanOpen(context.Background(), outer, inner) {
		t.Error("a container that cannot serve a bounded read must fall back to the name")
	}
}

// Detecting a nested self-extracting archive is only half the job: what
// CanOpen accepts, Open has to be able to consume. A 7z behind a stub has to
// be carved out of the materialized member first, exactly as it is carved out
// of a file on disk, because the reader looks for the signature at byte zero.
func TestArchiveProviderOpensNestedSevenZipSFX(t *testing.T) {
	sevenZip, err := exec.LookPath("7z")
	if err != nil {
		t.Skip("7z command is not installed")
	}

	build := t.TempDir()
	if err := os.WriteFile(filepath.Join(build, "payload.txt"), []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	sevenZipPath := filepath.Join(build, "payload.7z")
	cmd := exec.Command(sevenZip, "a", "-t7z", "-bd", sevenZipPath, "payload.txt")
	cmd.Dir = build
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create 7z: %v: %s", err, output)
	}
	sevenZipBytes, err := os.ReadFile(sevenZipPath)
	if err != nil {
		t.Fatal(err)
	}

	stubbed := append([]byte("MZ this is the installer stub, not an archive"), sevenZipBytes...)
	outer, outerPath := openOuterArchive(t, map[string][]byte{"setup.exe": stubbed})

	provider := &ArchiveProvider{}
	ctx := context.Background()
	member := outer.Join(outerPath, "setup.exe")
	if !provider.CanOpen(ctx, outer, member) {
		t.Fatal("a nested self-extracting 7z must be detected")
	}

	inner, err := provider.Open(ctx, outer, member)
	if err != nil {
		t.Fatalf("open nested self-extracting 7z: %v", err)
	}
	t.Cleanup(func() {
		if err := inner.Close(); err != nil {
			t.Errorf("close nested archive VFS: %v", err)
		}
	})
	assertListing(t, inner, "payload.txt")

	// A clone materializes the member again on its own, and has to carve the
	// same stub away rather than hand the reader the installer.
	clone := inner.Clone()
	t.Cleanup(func() {
		if err := clone.Close(); err != nil {
			t.Errorf("close cloned archive VFS: %v", err)
		}
	})
	assertListing(t, clone, "payload.txt")
}

// assertListing reads v's current directory and fails unless it holds exactly
// the named entries.
func assertListing(t *testing.T, v vfs.VFS, want ...string) {
	t.Helper()
	var names []string
	if err := v.ReadDir(context.Background(), v.GetPath(), func(items []vfs.VFSItem) {
		for _, item := range items {
			names = append(names, item.Name)
		}
	}); err != nil {
		t.Fatalf("read archive: %v", err)
	}
	sort.Strings(names)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if strings.Join(names, ",") != strings.Join(sorted, ",") {
		t.Fatalf("listing = %v, want %v", names, sorted)
	}
}

// A name can lie about a container: DetectFormat answers ".zip" from the
// suffix alone, while the reader is chosen from the content, so a RAR renamed
// to .zip still opens through the volume-aware RAR reader -- which unpacks a
// member whole before returning any of it. The gate has to follow the reader
// that was actually built, or Enter on a big member would write the whole
// thing to the temporary directory with the panel waiting on it.
func TestArchiveHeadReadFollowsTheBackendNotTheName(t *testing.T) {
	root := t.TempDir()
	outerPath := filepath.Join(root, "outer.zip")
	if err := os.WriteFile(outerPath, zipBytes(t, "inner.jar", zipBytes(t, "a.txt", []byte("x"))), 0o600); err != nil {
		t.Fatal(err)
	}

	outer, err := NewArchiveVFS(vfs.NewOSVFS(root), outerPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := outer.Close(); err != nil {
			t.Errorf("close outer archive VFS: %v", err)
		}
	})

	inner := outer.Join(outerPath, "inner.jar")
	if _, err := outer.ReadHead(context.Background(), inner, make([]byte, 64)); err != nil {
		t.Fatalf("a real ZIP must serve the head read: %v", err)
	}

	// The same .zip name over a reader that unpacks members whole.
	rar, err := newRARArchiveFileSystem(outer.backingPath, "")
	if err != nil {
		t.Fatal(err)
	}
	outer.mu.Lock()
	previous := outer.fsys
	outer.fsys = rar
	outer.mu.Unlock()
	t.Cleanup(func() {
		outer.mu.Lock()
		outer.fsys = previous
		outer.mu.Unlock()
		_ = rar.Close()
	})

	if _, err := outer.ReadHead(context.Background(), inner, make([]byte, 64)); !errors.Is(err, vfs.ErrHeadUnavailable) {
		t.Errorf("ReadHead error = %v, want vfs.ErrHeadUnavailable", err)
	}
}

// The scan advances only on bytes it received, so a reader that returns
// nothing and no error -- legal, if discouraged -- must end the scan instead
// of spinning on it forever.
func TestScanEmbeddedArchiveGivesUpOnAStalledReader(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		_, _, err := scanEmbeddedArchive(stalledReader{}, bytes.NewReader(nil), headProbeLimit)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, io.ErrNoProgress) {
			t.Errorf("scan error = %v, want io.ErrNoProgress", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the scan spun on a reader that never made progress")
	}
}

// stalledReader always reports that nothing happened, which io.Reader permits.
type stalledReader struct{}

func (stalledReader) Read([]byte) (int, error) { return 0, nil }

// Carving a self-extracting archive out of a materialized copy must not go
// looking for the rest of a split set around it. That copy lives in the system
// temporary directory, where the siblings cannot be and where the search costs
// a full directory read on every open -- and fails the open when the read
// fails. A self-extractor that is where its author put it is the opposite
// case, and the two carves are what tell them apart.
func TestNestedSFXCarveDoesNotScanForVolumes(t *testing.T) {
	root := t.TempDir()
	stub := []byte("MZ stub bytes that are not an archive")
	source := filepath.Join(root, "bundle")
	if err := os.WriteFile(source, append(append([]byte(nil), stub...), []byte("archive payload")...), 0o600); err != nil {
		t.Fatal(err)
	}
	// A volume named after the file, the way a split set beside a real
	// self-extractor is.
	if err := os.WriteFile(filepath.Join(root, "bundle.7z.002"), []byte("volume"), 0o600); err != nil {
		t.Fatal(err)
	}
	embedded := embeddedArchive{format: "fallback", suffix: ".7z", offset: int64(len(stub))}

	collected := func(carve func(string, embeddedArchive) (string, io.Closer, error)) []string {
		t.Helper()
		carved, closer, err := carve(source, embedded)
		if err != nil {
			t.Fatalf("carve: %v", err)
		}
		t.Cleanup(func() {
			if closer != nil {
				_ = closer.Close()
			}
		})
		entries, err := os.ReadDir(filepath.Dir(carved))
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".7z.002") {
				names = append(names, entry.Name())
			}
		}
		return names
	}

	if got := collected(materializeEmbeddedArchive); len(got) != 1 {
		t.Errorf("volumes beside a local self-extractor = %v, want the one that is there", got)
	}
	if got := collected(materializeEmbeddedArchiveAlone); len(got) != 0 {
		t.Errorf("the nested carve collected %v, which cannot be a volume of a materialized copy", got)
	}
}
