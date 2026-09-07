package archive

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	zipperarchive "github.com/unxed/zipper/archive"
)

func TestArchiveVFS_PromptsForEncryptedZip(t *testing.T) {
	testArchiveVFS_PromptsForEncryptedArchive(t, ".zip")
}

func TestArchiveVFS_PromptsForEncrypted7z(t *testing.T) {
	testArchiveVFS_PromptsForEncryptedArchive(t, ".7z")
}

func TestArchiveVFS_PromptsForExternalEncryptedZip(t *testing.T) {
	if _, err := exec.LookPath("zip"); err != nil {
		t.Skip("zip command is not installed")
	}
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "secret.txt"), []byte("secret data"), 0600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(tmp, "secret.zip")
	cmd := exec.Command("zip", "-q", "-P", "correct", archivePath, "secret.txt")
	cmd.Dir = tmp
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create encrypted zip: %v: %s", err, output)
	}
	testArchiveVFS_PromptsForEncryptedArchiveAtPath(t, archivePath, "correct")
}

func TestArchiveVFS_PromptsForExternalEncrypted7z(t *testing.T) {
	archivePath := createExternalEncrypted7z(t)
	testArchiveVFS_PromptsForEncryptedArchiveAtPathWithProgressAndPasswords(t, archivePath, []string{"Wrong", "Correct"})
}

func TestArchiveVFS_RetriesExternalEncrypted7zWithoutProgress(t *testing.T) {
	archivePath := createExternalEncrypted7z(t)
	testArchiveVFS_PromptsForEncryptedArchiveAtPathWithPasswords(t, archivePath, []string{"Wrong", "Correct"}, context.Background())
}

func createExternalEncrypted7z(t *testing.T) string {
	t.Helper()
	sevenZip, err := exec.LookPath("7z")
	if err != nil {
		t.Skip("7z command is not installed")
	}
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "secret.txt"), []byte("secret data"), 0600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(tmp, "secret.7z")
	// Keep headers visible: the password is first checked only when the file
	// payload is read, which is the retry scenario seen with real archives.
	cmd := exec.Command(sevenZip, "a", "-t7z", "-pCorrect", "-bd", archivePath, "secret.txt")
	cmd.Dir = tmp
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create encrypted 7z: %v: %s", err, output)
	}
	return archivePath
}

func testArchiveVFS_PromptsForEncryptedArchive(t *testing.T, extension string) {
	tmp := t.TempDir()
	archivePath := filepath.Join(tmp, "secret"+extension)
	filePath := filepath.Join(tmp, "secret.txt")
	if err := os.WriteFile(filePath, []byte("secret data"), 0600); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	archiver, err := zipperarchive.NewArchiver(archivePath, tmp, zipperarchive.Options{Password: "correct"})
	if err != nil {
		t.Fatal(err)
	}
	if err := archiver.Archive(context.Background(), map[string]os.FileInfo{filePath: info}); err != nil {
		t.Fatal(err)
	}
	if err := archiver.Close(); err != nil {
		t.Fatal(err)
	}
	testArchiveVFS_PromptsForEncryptedArchiveAtPath(t, archivePath, "correct")
}

func testArchiveVFS_PromptsForEncryptedArchiveAtPath(t *testing.T, archivePath, passwordValue string) {
	testArchiveVFS_PromptsForEncryptedArchiveAtPathWithContext(t, archivePath, passwordValue, context.Background())
}

func testArchiveVFS_PromptsForEncryptedArchiveAtPathWithProgressAndPasswords(t *testing.T, archivePath string, passwords []string) {
	ctx := context.WithValue(context.Background(), vfs.ProgressKey, vfs.ProgressCallback(func(string, int) {}))
	testArchiveVFS_PromptsForEncryptedArchiveAtPathWithPasswords(t, archivePath, passwords, ctx)
}

func testArchiveVFS_PromptsForEncryptedArchiveAtPathWithContext(t *testing.T, archivePath, passwordValue string, ctx context.Context) {
	testArchiveVFS_PromptsForEncryptedArchiveAtPathWithPasswords(t, archivePath, []string{passwordValue}, ctx)
}

func testArchiveVFS_PromptsForEncryptedArchiveAtPathWithPasswords(t *testing.T, archivePath string, passwords []string, ctx context.Context) {
	tmp := filepath.Dir(archivePath)
	if len(passwords) == 0 {
		t.Fatal("password list is empty")
	}

	prompts := 0
	previousPrompt := archivePasswordPrompt
	archivePasswordPrompt = func(context.Context, string) (string, error) {
		idx := prompts
		prompts++
		if idx >= len(passwords) {
			return passwords[len(passwords)-1], nil
		}
		return passwords[idx], nil
	}
	defer func() { archivePasswordPrompt = previousPrompt }()

	v, err := NewArchiveVFS(vfs.NewOSVFS(tmp), archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()
	var items []vfs.VFSItem
	if err := v.ReadDir(context.Background(), v.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "secret.txt" {
		t.Fatalf("archive listing = %#v, want secret.txt", items)
	}

	file, err := v.Open(ctx, v.Join(archivePath, "secret.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	data, err := io.ReadAll(ctxReader{r: file, ctx: ctx})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret data" {
		t.Fatalf("got %q, want %q", data, "secret data")
	}
	if prompts != len(passwords) {
		t.Fatalf("password prompt count = %d, want %d", prompts, len(passwords))
	}
}
