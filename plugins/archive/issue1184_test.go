package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
)

// issue1184ZIPContainer builds the bytes of a small but real ZIP container.
// An office document, a .jar and a plain archive differ by name only, which
// is the whole point of the issue.
func issue1184ZIPContainer(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("<Types/>")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func issue1184Write(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestIssue1184DocumentsKeepEnterForAssociation is the regression test for
// issue #1184: office documents are ZIP containers, so the content probe that
// finds self-extracting archives claimed them too, and Enter browsed them as
// archives instead of opening them by extension association. Ctrl+PgDn, which
// asks for the archive explicitly, must keep working on them.
//
// The documents are now named by ArchiveEnterExcludeMask rather than deduced
// from having an extension that is not an archive's. What the issue reported
// is unchanged; what changed is that the answer is a setting, and that the
// packages it also caught -- see TestIssue1184PackagesFollowTheirContent --
// are no longer caught by it.
func TestIssue1184DocumentsKeepEnterForAssociation(t *testing.T) {
	root := t.TempDir()
	container := issue1184ZIPContainer(t)
	documents := []string{"report.docx", "budget.xlsx", "deck.pptx", "notes.odt", "book.epub", "SHOUTING.DOCX"}
	for _, name := range documents {
		issue1184Write(t, root, name, container)
	}

	provider := &ArchiveProvider{}
	parent := vfs.NewOSVFS(root)
	ctx := context.Background()

	for _, name := range documents {
		if !provider.CanOpen(ctx, parent, name) {
			t.Errorf("%s: CanOpen=false, Ctrl+PgDn must still browse the container", name)
		}
		if provider.PanelEnterAllowed(ctx, parent, name) {
			t.Errorf("%s: ordinary Enter must go to the extension association, not into the archive", name)
		}
	}
}

// TestIssue1184PackagesFollowTheirContent records what the issue's fix caught
// beyond what it reported. A .jar and an .apk are ZIP containers, and the
// original rule -- any extension that is not an archive's keeps Enter for its
// association -- swept them up with the documents. Neither ancestor does
// that: arclite lists *.jar and *.[ah]pk among the names Enter may open
// (arclite/options.cpp), and far2l's KnownDocumentTypes does not mention
// them, so both browse a package on Enter. f4 follows, and anyone who wants
// the old behaviour names them in ArchiveEnterExcludeMask.
func TestIssue1184PackagesFollowTheirContent(t *testing.T) {
	root := t.TempDir()
	container := issue1184ZIPContainer(t)
	packages := []string{"lib.jar", "app.apk", "lib.aar"}
	for _, name := range packages {
		issue1184Write(t, root, name, container)
	}

	provider := &ArchiveProvider{}
	parent := vfs.NewOSVFS(root)
	ctx := context.Background()

	for _, name := range packages {
		if !provider.PanelEnterAllowed(ctx, parent, name) {
			t.Errorf("%s: Enter browses a package in Far and in far2l", name)
		}
	}

	// And the setting is what decides, so naming one puts it back.
	previous := config.App.ArchiveEnterExcludeMask
	config.App.ArchiveEnterExcludeMask = "*.jar"
	t.Cleanup(func() { config.App.ArchiveEnterExcludeMask = previous })

	if provider.PanelEnterAllowed(ctx, parent, "lib.jar") {
		t.Error("lib.jar: a name the mask lists must keep Enter for its association")
	}
	if !provider.PanelEnterAllowed(ctx, parent, "app.apk") {
		t.Error("app.apk: a name the mask does not list must still follow its content")
	}
}

// TestIssue1184ArchiveNamesKeepEnter pins the other side of the rule: an
// entry whose name declares an archive still opens on Enter, split volumes
// and extension-less archives included.
func TestIssue1184ArchiveNamesKeepEnter(t *testing.T) {
	root := t.TempDir()
	container := issue1184ZIPContainer(t)
	issue1184Write(t, root, "data.zip", container)
	issue1184Write(t, root, "volume.z01", container)
	issue1184Write(t, root, "legacy.r00", container)
	issue1184Write(t, root, "noextension", container)
	issue1184Write(t, root, "split.7z.001", validSevenZipStartHeader())

	provider := &ArchiveProvider{}
	parent := vfs.NewOSVFS(root)
	ctx := context.Background()

	for _, name := range []string{"data.zip", "volume.z01", "legacy.r00", "noextension", "split.7z.001"} {
		if !provider.PanelEnterAllowed(ctx, parent, name) {
			t.Errorf("%s: ordinary Enter must still open the archive", name)
		}
	}

	// A self-extracting archive stays out of ordinary Enter whether or not
	// it carries an extension: Enter runs it, Ctrl+PgDn browses it.
	issue1184Write(t, root, "setup.exe", append([]byte("stub code\n"), container...))
	issue1184Write(t, root, "setup", append([]byte("stub code\n"), container...))
	for _, name := range []string{"setup.exe", "setup"} {
		if provider.PanelEnterAllowed(ctx, parent, name) {
			t.Errorf("%s: self-extracting archive must not open on ordinary Enter", name)
		}
	}
}

// TestIssue1184EnterExcludeMask is the table the name rule used to carry,
// rewritten for the setting that replaced it. Only the names the mask lists
// lose Enter; everything else is left to its content, and a file whose
// content is not an archive never reaches this test because CanOpen turns it
// down first.
func TestIssue1184EnterExcludeMask(t *testing.T) {
	barred := []string{
		"report.docx", "report.DOCX", "budget.xlsx", "deck.pptx", "slides.ppsx",
		"notes.odt", "sheet.ods", "talk.odp", "theme.thmx", "book.epub",
	}
	for _, name := range barred {
		if !enterBarredByMask(name) {
			t.Errorf("%s: expected the default mask to keep Enter for the association", name)
		}
	}

	free := []string{
		"data.zip", "src.tar.gz", "media.7z", "split.7z.001", "volume.z01",
		"legacy.r00", "lib.jar", "app.apk", "noextension", "trailing.",
		"archive.zipx", "photo.jpeg", "setup.exe",
	}
	for _, name := range free {
		if enterBarredByMask(name) {
			t.Errorf("%s: expected the default mask to leave this name to its content", name)
		}
	}

	previous := config.App.ArchiveEnterExcludeMask
	t.Cleanup(func() { config.App.ArchiveEnterExcludeMask = previous })

	// An empty mask bars nothing: Enter then follows the content alone.
	config.App.ArchiveEnterExcludeMask = ""
	if enterBarredByMask("report.docx") {
		t.Error("an empty mask must bar nothing")
	}

	// far2l mask syntax all the way down, so "|" still carves an exception.
	config.App.ArchiveEnterExcludeMask = "*.docx,*.odt|notes.odt"
	if !enterBarredByMask("report.docx") {
		t.Error("report.docx: the include side of the mask must still bar it")
	}
	if enterBarredByMask("notes.odt") {
		t.Error("notes.odt: the exclude side of the mask must take it back")
	}
}
