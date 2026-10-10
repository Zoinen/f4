package vtui

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// writeTestFont drops a real font file on disk: the inventory answers from a
// parsed cmap, so a fixture has to be a font a parser accepts.
func writeTestFont(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, goregular.TTF, 0o600); err != nil {
		t.Fatalf("writing the test font: %v", err)
	}
}

func TestIsFontFileName(t *testing.T) {
	fonts := []string{"Nirmala.ttf", "MSYH.TTC", "NotoSans.otf", "Fonts.otc"}
	for _, name := range fonts {
		if !isFontFileName(name) {
			t.Errorf("isFontFileName(%q) = false, want true", name)
		}
	}
	others := []string{"vgasys.fon", "cour.pfb", "fonts.dir", "readme", "ttf"}
	for _, name := range others {
		if isFontFileName(name) {
			t.Errorf("isFontFileName(%q) = true, want false", name)
		}
	}
}

func TestFontFilesInDir(t *testing.T) {
	dir := t.TempDir()
	writeTestFont(t, filepath.Join(dir, "a.ttf"))
	writeTestFont(t, filepath.Join(dir, "b.TTF"))
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("writing the decoy file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.ttf"), 0o700); err != nil {
		t.Fatalf("making the decoy directory: %v", err)
	}

	got := fontFilesInDir(dir)
	want := []string{filepath.Join(dir, "a.ttf"), filepath.Join(dir, "b.TTF")}
	if len(got) != len(want) {
		t.Fatalf("fontFilesInDir = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fontFilesInDir[%d] = %q, want %q", i, got[i], want[i])
		}
	}

	if paths := fontFilesInDir(filepath.Join(dir, "absent")); paths != nil {
		t.Errorf("a missing directory yielded %v, want nil", paths)
	}
}

func TestFontFileCoversRune(t *testing.T) {
	dir := t.TempDir()
	font := filepath.Join(dir, "go-regular.ttf")
	writeTestFont(t, font)

	if !fontFileCoversRune(font, 'G') {
		t.Error("Go Regular reported as not carrying 'G'")
	}
	// Go Regular has no Devanagari, which is the whole point of asking a
	// file rather than trusting its name.
	if fontFileCoversRune(font, 'ह') {
		t.Error("Go Regular reported as carrying U+0939")
	}

	notAFont := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notAFont, []byte("not a font"), 0o600); err != nil {
		t.Fatalf("writing the decoy file: %v", err)
	}
	if fontFileCoversRune(notAFont, 'G') {
		t.Error("an unparseable file reported coverage")
	}
	if fontFileCoversRune(filepath.Join(dir, "absent.ttf"), 'G') {
		t.Error("a missing file reported coverage")
	}
}

func TestInstalledFontPathsForRune(t *testing.T) {
	dir := t.TempDir()
	covering := filepath.Join(dir, "covers.ttf")
	writeTestFont(t, covering)
	empty := filepath.Join(dir, "empty.ttf")
	if err := os.WriteFile(empty, []byte("not a font"), 0o600); err != nil {
		t.Fatalf("writing the decoy file: %v", err)
	}

	previous := installedFontFiles
	installedFontFiles = func() []string { return []string{empty, covering} }
	t.Cleanup(func() { installedFontFiles = previous })

	got := installedFontPathsForRune('G')
	if len(got) != 1 || got[0] != covering {
		t.Fatalf("installedFontPathsForRune('G') = %v, want [%s]", got, covering)
	}
	if got := installedFontPathsForRune('ह'); got != nil {
		t.Errorf("a rune no installed font carries yielded %v, want nil", got)
	}
}

func TestInstalledFontPathsForRuneStopsAtTheCap(t *testing.T) {
	dir := t.TempDir()
	var paths []string
	for i := 0; i < maxInstalledFontMatches+3; i++ {
		path := filepath.Join(dir, string(rune('a'+i))+".ttf")
		writeTestFont(t, path)
		paths = append(paths, path)
	}

	previous := installedFontFiles
	installedFontFiles = func() []string { return paths }
	t.Cleanup(func() { installedFontFiles = previous })

	if got := installedFontPathsForRune('G'); len(got) != maxInstalledFontMatches {
		t.Errorf("installedFontPathsForRune returned %d paths, want the cap of %d", len(got), maxInstalledFontMatches)
	}
}
