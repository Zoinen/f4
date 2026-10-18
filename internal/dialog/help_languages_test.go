package dialog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelpSearchDirsResolvesExecutableSymlink(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "install")
	linkDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(installDir, "f4")
	if err := os.WriteFile(target, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "f4")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	// helpSearchDirs follows symlinks, so the expected folder must be the
	// real one too: t.TempDir() itself sits behind a symlink on macOS
	// (/var -> /private/var) and may be a short 8.3 name on Windows.
	realInstallDir, err := filepath.EvalSymlinks(installDir)
	if err != nil {
		t.Fatal(err)
	}
	dirs := helpSearchDirs(link, filepath.Join(root, "profile", "help"))
	if got, want := dirs[0], filepath.Join(realInstallDir, "help"); got != want {
		t.Fatalf("executable help directory = %q, want resolved target directory %q", got, want)
	}
}
