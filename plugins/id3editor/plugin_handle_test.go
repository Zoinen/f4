package id3editor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/id3-go"
	v1 "github.com/unxed/id3-go/v1"
	"github.com/unxed/vtui"
)

type nonLocalID3VFS struct{ vfs.VFS }

func newID3FrameManager(t *testing.T) *vtui.FrameManagerType {
	t.Helper()
	old := vtui.FrameManager
	fm := vtui.NewFrameManager()
	fm.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager = fm
	t.Cleanup(func() {
		for fm.GetTopFrame() != nil {
			fm.Pop()
		}
		fm.Shutdown()
		vtui.FrameManager = old
	})
	return fm
}

func TestHandleEditRejectsUnsupportedSelections(t *testing.T) {
	local := vfs.NewOSVFS(t.TempDir())
	tests := []struct {
		name      string
		activeVFS vfs.VFS
		selected  []string
		wantTitle string
	}{
		{name: "no active VFS", wantTitle: " Error "},
		{name: "remote VFS", activeVFS: nonLocalID3VFS{}, selected: []string{"song.mp3"}, wantTitle: " Error "},
		{name: "no selection", activeVFS: local, wantTitle: " ID3 Editor "},
		{name: "non MP3", activeVFS: local, selected: []string{"song.txt"}, wantTitle: " ID3 Editor "},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fm := newID3FrameManager(t)
			app := &id3AppStub{activeVFS: test.activeVFS, selected: test.selected}
			(&ID3EditorPlugin{}).handleEdit(app)

			dlg, ok := fm.GetTopFrame().(*vtui.Window)
			if !ok {
				t.Fatalf("top frame = %T, want message dialog", fm.GetTopFrame())
			}
			if dlg.GetTitle() != test.wantTitle {
				t.Errorf("dialog title = %q, want %q", dlg.GetTitle(), test.wantTitle)
			}
		})
	}
}

func writeID3TestFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "song.mp3")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(make([]byte, 100)); err != nil {
		t.Fatal(err)
	}
	tag := &v1.Tag{}
	tag.SetTitle("Initial Title")
	tag.SetArtist("Initial Artist")
	tag.SetAlbum("Initial Album")
	tag.SetYear("2020")
	tag.SetGenre("Rock")
	tag.SetComment("Initial Comment")
	if _, err := file.Write(tag.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestHandleEditOpensAndSavesSelectedMP3(t *testing.T) {
	fm := newID3FrameManager(t)
	path := writeID3TestFile(t)
	app := &id3AppStub{
		activeVFS: vfs.NewOSVFS(filepath.Dir(path)),
		selected:  []string{filepath.Base(path)},
	}

	(&ID3EditorPlugin{}).handleEdit(app)
	dlg, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want editor dialog", fm.GetTopFrame())
	}
	if strings.Trim(strings.TrimSpace(dlg.GetTitle()), "{}") != "ID3Editor.Title" {
		t.Errorf("dialog title = %q, want ID3Editor.Title", dlg.GetTitle())
	}

	var edits []*vtui.Edit
	var save *vtui.Button
	for _, child := range dlg.GetChildren() {
		switch item := child.(type) {
		case *vtui.Edit:
			edits = append(edits, item)
		case *vtui.Button:
			if item.IsDefault {
				save = item
			}
		}
	}
	if len(edits) != 6 {
		t.Fatalf("editor fields = %d, want 6", len(edits))
	}
	if save == nil || save.OnClick == nil {
		t.Fatal("save button callback is not installed")
	}
	edits[0].SetText("Saved Title")
	edits[1].SetText("Saved Artist")
	save.OnClick()
	if app.refreshCalls != 1 {
		t.Fatalf("RefreshAll calls = %d, want 1", app.refreshCalls)
	}

	file, err := id3.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(strings.TrimRight(file.Title(), "\x00")) != "Saved Title" {
		t.Errorf("saved title = %q, want Saved Title", file.Title())
	}
	if strings.TrimSpace(strings.TrimRight(file.Artist(), "\x00")) != "Saved Artist" {
		t.Errorf("saved artist = %q, want Saved Artist", file.Artist())
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestShowEditorDialogCancelClosesFile(t *testing.T) {
	fm := newID3FrameManager(t)
	path := writeID3TestFile(t)
	app := &id3AppStub{}
	(&ID3EditorPlugin{}).showEditorDialog(app, path)
	dlg, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want editor dialog", fm.GetTopFrame())
	}

	var cancel *vtui.Button
	for _, child := range dlg.GetChildren() {
		button, ok := child.(*vtui.Button)
		if ok && !button.IsDefault {
			cancel = button
		}
	}
	if cancel == nil || cancel.OnClick == nil {
		t.Fatal("cancel button callback is not installed")
	}
	cancel.OnClick()
	if !dlg.IsDone() {
		t.Fatal("cancel did not close the editor dialog")
	}
}

var _ vfs.App = (*id3AppStub)(nil)
var _ vfs.VFS = (*nonLocalID3VFS)(nil)
