package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// TestIssue1250_SFXRARVolumesNamedPartN opens a self-extracting RAR set named
// the way WinRAR names one in the new volume scheme: test.part01.exe,
// test.part02.rar. The companions used to be looked for under
// "test.part01", were not found, and the first volume was read alone until
// rardecode failed to open a next volume named after the temporary copy.
func TestIssue1250_SFXRARVolumesNamedPartN(t *testing.T) {
	sourceDir := archivesRarFixtureDir(t)
	tmp := t.TempDir()
	part01, err := os.ReadFile(filepath.Join(sourceDir, "test.part01.rar"))
	if err != nil {
		t.Fatal(err)
	}
	part02, err := os.ReadFile(filepath.Join(sourceDir, "test.part02.rar"))
	if err != nil {
		t.Fatal(err)
	}
	sfx := append([]byte("stub bytes before the archive\n"), part01...)
	if err := os.WriteFile(filepath.Join(tmp, "test.part01.exe"), sfx, 0600); err != nil { // #nosec G703 -- tmp is the per-test directory created by testing.T.TempDir.
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "test.part02.rar"), part02, 0600); err != nil { // #nosec G703 -- tmp is the per-test directory created by testing.T.TempDir.
		t.Fatal(err)
	}

	v, err := NewArchiveVFS(vfs.NewOSVFS(tmp), "test.part01.exe")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = v.Close() })
	var items []vfs.VFSItem
	if err := v.ReadDir(context.Background(), v.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(items) != 1 || items[0].Name != "test.txt" {
		t.Fatalf("entries = %#v", items)
	}
	file, err := v.Open(context.Background(), v.Join(v.GetPath(), "test.txt"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	data, readErr := io.ReadAll(ctxReader{r: file, ctx: context.Background()})
	if closeErr := file.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	if readErr != nil {
		t.Fatalf("Read: %v", readErr)
	}
	hash := sha256.Sum256(data)
	if got := hex.EncodeToString(hash[:]); got != "b1040e9bde2125471abc00773c7c589c32ee879354dd188a919988f70b84ea19" {
		t.Fatalf("test.txt SHA-256 = %s, want the complete multi-volume payload", got)
	}
}

func TestRARVolumeSetStem(t *testing.T) {
	for stem, want := range map[string]string{
		"tests.part01":  "tests",
		"Tests.PART1":   "Tests",
		"a.part2.part3": "a.part2",
		"tests":         "",
		"tests.part":    "",
		".part01":       "",
	} {
		got, ok := rarVolumeSetStem(stem)
		if ok != (want != "") || got != want {
			t.Errorf("rarVolumeSetStem(%q) = %q, %v; want %q", stem, got, ok, want)
		}
	}
}

// issue1250RAR4EncryptedHeaders is a RAR 1.5-4.x format archive with
// encrypted headers, password "Correct", holding secret.txt = "secret data".
// Made with RAR 6.24: rar a -ma4 -hpCorrect s.rar secret.txt (RAR 7 no longer
// writes this format).
const issue1250RAR4EncryptedHeaders = "" +
	"526172211a0700ce997380000d00000000000000c0593f1246be6dc0aea19fd2" +
	"ae1408c93441f51a49ec1276c24d8fd7e29fd18e86bcb626d35ded63f7bf3d3e" +
	"b4d920ca585935d03a05e49fa28dae9170e6fa1d5d660a1487508d0f75e73e9d" +
	"13c04c3f62bf67d460da0ae5310239a67dcd144020b65dfbeb59a54bc0593f12" +
	"46be6dc053902107a47e15596bda448ee23ed1b7"

func issue1250RAR4Fixture(t *testing.T) string {
	t.Helper()
	data, err := hex.DecodeString(issue1250RAR4EncryptedHeaders)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "s.rar")
	if err := os.WriteFile(p, data, 0600); err != nil { // #nosec G703 -- p is inside the per-test directory created by testing.T.TempDir.
		t.Fatal(err)
	}
	return p
}

// TestIssue1250_RAR4EncryptedHeadersWrongPasswordReprompts: a wrong password
// for such an archive surfaced as "bad header crc" or "unexpected EOF" and
// ended the operation instead of asking for the password again.
func TestIssue1250_RAR4EncryptedHeadersWrongPasswordReprompts(t *testing.T) {
	for _, withProgress := range []bool{false, true} {
		p := issue1250RAR4Fixture(t)
		res, prompts := runIssue816Scenario(t, p, []any{"Wrong", "", "Wrong2", "Correct"}, withProgress)
		if res != `data="secret data"` || prompts != 4 {
			t.Fatalf("progress=%v wrong then correct: got %s after %d prompts, want the data after 4", withProgress, res, prompts)
		}
		res, prompts = runIssue816Scenario(t, p, []any{"Wrong", context.Canceled}, withProgress)
		if !strings.Contains(res, context.Canceled.Error()) || prompts != 2 {
			t.Fatalf("progress=%v wrong then cancel: got %s after %d prompts", withProgress, res, prompts)
		}
	}
}

// TestIssue1250_RAR4EncryptedHeadersWrongPasswordTestAndExtract covers the
// same wrong password in Test archive and in extraction from the panel.
func TestIssue1250_RAR4EncryptedHeadersWrongPasswordTestAndExtract(t *testing.T) {
	answers := []string{"Wrong", "Correct"}
	prompts := 0
	prev := archivePasswordPrompt
	archivePasswordPrompt = func(context.Context, string) (string, error) {
		if prompts >= len(answers) {
			return "", errors.New("too many prompts")
		}
		prompts++
		return answers[prompts-1], nil
	}
	t.Cleanup(func() { archivePasswordPrompt = prev })

	p := issue1250RAR4Fixture(t)
	if err := testArchiveWithPasswordPrompt(context.Background(), p, &dummyReporter{}); err != nil || prompts != 2 {
		t.Fatalf("test: err=%v after %d prompts, want success after 2", err, prompts)
	}

	prompts = 0
	dest := t.TempDir()
	if err := extractArchiveWithPasswordPrompt(context.Background(), p, dest, &dummyReporter{}); err != nil || prompts != 2 {
		t.Fatalf("extract: err=%v after %d prompts, want success after 2", err, prompts)
	}
	data, err := os.ReadFile(filepath.Join(dest, "secret.txt"))
	if err != nil || string(data) != "secret data" {
		t.Fatalf("extracted secret.txt = %q, %v", data, err)
	}
}
