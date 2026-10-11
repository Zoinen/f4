package archive

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// issue1504NameApp captures the archive name actionAddArchive suggests as the
// InputBox default text, without running the actual archiving flow: the
// InputBox callback is invoked with an empty name, which actionAddArchive
// treats as a cancelled dialog (f4#1504).
type issue1504NameApp struct {
	activeVfs  vfs.VFS
	names      []string
	inputBoxes int
	suggested  string
}

func (m *issue1504NameApp) GetActivePanelVFS() vfs.VFS  { return m.activeVfs }
func (m *issue1504NameApp) GetPassivePanelVFS() vfs.VFS { return m.activeVfs }
func (m *issue1504NameApp) GetSelectedNames() []string  { return m.names }
func (m *issue1504NameApp) GetSelectedName() string {
	if len(m.names) == 0 {
		return ""
	}
	return m.names[0]
}
func (m *issue1504NameApp) RefreshAll()                     {}
func (m *issue1504NameApp) SetPendingSelection(name string) {}
func (m *issue1504NameApp) RunProgressTask(title, startMsg string, forked bool, worker func(ctx context.Context, update func(msg string, percent int)) error, onComplete func(err error)) {
}
func (m *issue1504NameApp) RunAdvancedProgressTask(title string, forked bool, worker func(ctx context.Context, reporter vfs.TaskReporter) error, onComplete func(err error)) {
}
func (m *issue1504NameApp) Message(title, msg string, buttons []string) int { return 0 }
func (m *issue1504NameApp) InputBox(title, prompt, defaultText string, callback func(string)) {
	m.inputBoxes++
	m.suggested = defaultText
	// An empty name aborts before actionAddArchive spawns the archiving
	// goroutine, so the test only exercises the name-suggestion logic.
	callback("")
}
func (m *issue1504NameApp) Menu(title string, items []string, callback func(int)) {}

// TestActionAddArchive_DefaultName_SingleSelected: with exactly one item
// explicitly marked, the suggested archive name is that item's own name
// plus the extension, not the panel directory's (f4#1504).
func TestActionAddArchive_DefaultName_SingleSelected(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "file.ext"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	app := &issue1504NameApp{
		activeVfs: vfs.NewOSVFS(tmpDir),
		names:     []string{"file.ext"},
	}

	actionAddArchive(app)

	if app.inputBoxes != 1 {
		t.Fatalf("inputBoxes = %d, want 1", app.inputBoxes)
	}
	if want := "file.ext.zip"; app.suggested != want {
		t.Fatalf("suggested archive name = %q, want %q", app.suggested, want)
	}
}

// TestActionAddArchive_DefaultName_NoneSelectedCursorOnFile: with nothing
// explicitly marked, GetSelectedNames falls back to the item under the
// cursor (its documented contract, internal/panel/list.go). actionAddArchive
// must treat that the same as a single explicit selection and suggest the
// cursor file's own name, not the panel directory's (f4#1504).
func TestActionAddArchive_DefaultName_NoneSelectedCursorOnFile(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "notes.txt"), []byte("data"), 0600); err != nil {
		t.Fatal(err)
	}

	app := &issue1504NameApp{
		activeVfs: vfs.NewOSVFS(tmpDir),
		// GetSelectedNames() already returns just the cursor item when
		// nothing is marked -- this is what a real FileSystemPanel with an
		// empty selection and the cursor on "notes.txt" would return.
		names: []string{"notes.txt"},
	}

	actionAddArchive(app)

	if want := "notes.txt.zip"; app.suggested != want {
		t.Fatalf("suggested archive name = %q, want %q", app.suggested, want)
	}
}

// TestActionAddArchive_DefaultName_NoneSelectedCursorOnDirectory: packing a
// directory under the cursor with nothing marked must suggest that
// directory's own name, not its parent's (f4#1504).
func TestActionAddArchive_DefaultName_NoneSelectedCursorOnDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmpDir, "subdir"), 0700); err != nil {
		t.Fatal(err)
	}

	app := &issue1504NameApp{
		activeVfs: vfs.NewOSVFS(tmpDir),
		names:     []string{"subdir"},
	}

	actionAddArchive(app)

	if want := "subdir.zip"; app.suggested != want {
		t.Fatalf("suggested archive name = %q, want %q", app.suggested, want)
	}
}

// TestActionAddArchive_DefaultName_MultipleSelected: with more than one item
// explicitly marked there is no single item to name the archive after, so
// the suggestion keeps the existing directory-derived name (f4#1504).
func TestActionAddArchive_DefaultName_MultipleSelected(t *testing.T) {
	tmpDir := t.TempDir()
	for _, name := range []string{"file1.txt", "file2.txt"} {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
	}

	app := &issue1504NameApp{
		activeVfs: vfs.NewOSVFS(tmpDir),
		names:     []string{"file1.txt", "file2.txt"},
	}

	actionAddArchive(app)

	want := filepath.Base(tmpDir) + ".zip"
	if app.suggested != want {
		t.Fatalf("suggested archive name = %q, want %q", app.suggested, want)
	}
}
