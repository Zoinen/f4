package archive

import (
	"testing"

	"github.com/unxed/vtui"
)

func addArchiveMethodIndex(t *testing.T, format int, id string) int {
	t.Helper()
	for i, m := range addArchiveFormats[format].Methods {
		if m.ID == id {
			return i
		}
	}
	t.Fatalf("format %d has no method %q", format, id)
	return -1
}

// noteShown reports whether the note next to the method says anything. The
// window shows every item again on each redraw, so a label is silenced by
// emptying its text, not by hiding it (tarlabnor, f4#2002, 17-10 22:26).
func noteShown(d *addArchiveDialog) bool { return d.methodNote.GetText() != "" }

// Zstandard and LZMA in a zip are said to open in fewer tools, next to the
// method; Deflate and Store are not (f4#2002).
func TestAddArchiveDialogNotesTheRareZipMethods(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {})
	zipIdx := addArchiveFormatIndex(t, ".zip")

	choose := func(id string) {
		i := addArchiveMethodIndex(t, zipIdx, id)
		d.method.Menu.SetSelectPos(i)
		d.method.Menu.OnAction(i)
	}
	if noteShown(d) {
		t.Fatal("the note is shown for Deflate, the default")
	}
	for _, tc := range []struct {
		id   string
		note bool
	}{{"zstd", true}, {"lzma", true}, {"store", false}, {zipDeflateMethod, false}} {
		choose(tc.id)
		if noteShown(d) != tc.note {
			t.Errorf("method %s: note shown = %v, want %v", tc.id, noteShown(d), tc.note)
		}
	}

	choose("zstd")
	sevenIdx := addArchiveFormatIndex(t, ".7z")
	d.format.Menu.SetSelectPos(sevenIdx)
	d.format.Menu.OnAction(sevenIdx)
	if d.methodNote.GetText() != vtui.Msg("Archive.Add.Method7zNote") {
		t.Errorf("7z: note %q, want what the engine uses", d.methodNote.GetText())
	}

	d.format.Menu.SetSelectPos(zipIdx)
	d.format.Menu.OnAction(zipIdx)
	if noteShown(d) {
		t.Error("the 7z note stays after switching back to zip with Deflate")
	}
}

// The note goes away for every format that has nothing to say, not only for
// zip with Deflate: the 7z note used to stay under tar and the like.
func TestAddArchiveDialogNoteGoesAwayForTar(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {})
	for _, ext := range []string{".7z", ".tar.gz", ".7z", ".tar.xz"} {
		i := addArchiveFormatIndex(t, ext)
		d.format.Menu.SetSelectPos(i)
		d.format.Menu.OnAction(i)
		if want := ext == ".7z"; noteShown(d) != want {
			t.Errorf("%s: note shown = %v (%q), want %v", ext, noteShown(d), d.methodNote.GetText(), want)
		}
	}
}

// Choosing a method in the list puts it in the field of the combo, which
// used to stay on Deflate while the archive was written with the choice.
func TestAddArchiveDialogMethodFieldFollowsTheChoice(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {})
	zipIdx := addArchiveFormatIndex(t, ".zip")
	for i, m := range addArchiveFormats[zipIdx].Methods {
		d.method.Menu.SetSelectPos(i)
		d.method.Menu.OnAction(i)
		want := m.Label
		if want == "" {
			want = vtui.Msg("Archive.Add.MethodStore")
		}
		if got := d.method.Edit.GetText(); got != want {
			t.Errorf("method %d: the field shows %q, want %q", i, got, want)
		}
		if got := d.settings().Method; got != i {
			t.Errorf("method %d: the window hands over %d", i, got)
		}
	}
}

// The keep-extension flag matters for an archive of one file; for a folder
// or several files it is not offered (tarlabnor, f4#2002, 17-10 22:26).
func TestAddArchiveDialogKeepExtensionOnlyForOneFile(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	for _, tc := range []struct {
		name string
		args []bool
		off  bool
	}{{"unknown source", nil, true}, {"folder or several", []bool{false}, true}, {"one file", []bool{true}, false}} {
		d := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {}, tc.args...)
		if d.keepExtension.IsDisabled() != tc.off {
			t.Errorf("%s: disabled = %v, want %v", tc.name, d.keepExtension.IsDisabled(), tc.off)
		}
	}
}
