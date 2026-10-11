package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// The panel title of an archive names what opened it and the format, instead
// of the path of the file: "Zipper:7z:Far.7z" (#1383).
func TestArchivePanelTitleNamesTheHandlerAndTheFormat(t *testing.T) {
	tmp := t.TempDir()
	zipped := archiveFixtureZIP(t, 128)
	for _, test := range []struct{ file, want string }{
		{"sample.zip", "Zipper:zip:sample.zip"},
		// A .jar is a zip that no extension announces; the reader that
		// opened it says what it is.
		{"app.jar", "Zipper:zip:app.jar"},
	} {
		file := filepath.Join(tmp, test.file)
		if err := os.WriteFile(file, zipped, 0o600); err != nil {
			t.Fatal(err)
		}
		// The panel opens an archive by its full path.
		opened, err := NewArchiveVFS(vfs.NewOSVFS(tmp), file)
		if err != nil {
			t.Fatalf("%s: %v", test.file, err)
		}
		if got := opened.PanelTitle(opened.GetPath()); got != test.want {
			t.Errorf("%s: title at the top = %q, want %q", test.file, got, test.want)
		}
		if err := opened.SetPath(opened.Join(opened.GetPath(), "folder")); err != nil {
			_ = opened.Close()
			t.Fatalf("%s: %v", test.file, err)
		}
		if got, want := opened.PanelTitle(opened.GetPath()), test.want+string(filepath.Separator)+"folder"; got != want {
			t.Errorf("%s: title inside a folder = %q, want %q", test.file, got, want)
		}
		if got := opened.PanelTitle(filepath.Join(tmp, "elsewhere")); got != "" {
			t.Errorf("%s: a path outside the archive got the title %q", test.file, got)
		}
		_ = opened.Close()
	}
}

func TestArchiveFormatLabel(t *testing.T) {
	for _, test := range []struct{ name, format, sfxSuffix, want string }{
		{"Far30b6735.x64.20260919.7z", "fallback", "", "7z"},
		{"a.zip", "zip", "", "zip"},
		{"a.part1.rar", "fallback", "", "rar"},
		{"a.tar", "tar", "", "tar"},
		{"a.tar.gz", "tar", "", "tar.gz"},
		{"backup.tar.2024.xz", "tar", "", "tar.xz"},
		{"a.TGZ", "tar", "", "tgz"},
		{"a.gz", "fallback", "", "gz"},
		{"setup.exe", "fallback", ".7z", "7z"},
		{"setup.exe", "zip", "", "zip"},
		{"a.z01", "zip", "", "zip"},
		// Nothing the plugin went by names the format: the title leaves
		// it out instead of guessing one.
		{"data.7z.001", "", "", ""},
	} {
		if got := archiveFormatLabel(test.name, test.format, test.sfxSuffix); got != test.want {
			t.Errorf("archiveFormatLabel(%q, %q, %q) = %q, want %q", test.name, test.format, test.sfxSuffix, got, test.want)
		}
	}

	unknown := &ArchiveVFS{arcPath: filepath.Join(t.TempDir(), "data.7z.001"), displayName: "data.7z.001"}
	if got, want := unknown.PanelTitle(unknown.GetPath()), "Zipper:data.7z.001"; got != want {
		t.Errorf("title without a known format = %q, want %q", got, want)
	}
}
