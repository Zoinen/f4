package editor

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

// newWrapMemoryStore installs a file-state provider writing into a temporary
// directory and returns it, so a test can read back what the editor recorded.
func newWrapMemoryStore(t *testing.T) *fileops.F4FileStateProvider {
	t.Helper()
	fs := &fileops.F4FileStateProvider{
		Path:  filepath.Join(t.TempDir(), "file_states.json"),
		Limit: 10,
		Data:  make(map[string]*fileops.FileState),
	}
	old := fileops.GlobalFileState
	fileops.GlobalFileState = fs
	t.Cleanup(func() { fileops.GlobalFileState = old })
	return fs
}

func TestF4FileStateProvider_SaveEditorWrapKeepsPosition(t *testing.T) {
	fs := &fileops.F4FileStateProvider{
		Path:  filepath.Join(t.TempDir(), "file_states.json"),
		Limit: 10,
		Data:  make(map[string]*fileops.FileState),
	}

	fs.SaveEditorState("main.go", 42, 7, 40, 3, false)
	fs.SaveEditorWrap("main.go", true)

	state := fs.GetState("main.go")
	if state == nil {
		t.Fatal("expected state for main.go, got nil")
	}
	if !state.EditorWrap {
		t.Error("word wrap was not recorded")
	}
	if state.EditorLine != 42 || state.EditorPos != 7 || state.EditorTopRow != 40 || state.EditorLeft != 3 {
		t.Errorf("the wrap update overwrote the saved position: %+v", state)
	}
}

// Content the editor cannot wrap turns wrapping off for the session, but the
// setting the user chose belongs to the file, not to today's contents of it.
func TestEditorView_UnsafeWordWrapKeepsRememberedChoice(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	fs := newWrapMemoryStore(t)

	ev := NewEditorView(piecetable.New([]byte("text")), nil, "wrapped.txt")
	ev.SetWordWrap(true)

	ev.DisableUnsafeWordWrap()
	if ev.WordWrap {
		t.Fatal("unsafe content must turn word wrap off")
	}
	if !ev.wordWrapWanted {
		t.Fatal("suppressing wrapping must not discard the choice made for the file")
	}

	ev.Close()
	fs.Flush()

	state := fs.GetState(fileops.FileStateKey(nil, "wrapped.txt"))
	if state == nil || !state.EditorWrap {
		t.Fatal("closing a file whose wrapping was suppressed erased the remembered choice")
	}
}

// A mapped file opens with an empty index the background scan fills, so before
// that scan has found a newline the whole buffer looks like one enormous line.
// Reading that as unsafe content would suppress wrapping for good, on nothing
// more than a partial index.
func TestEditorView_GrowingIndexIsNotAnUnsafeLine(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	content := strings.Repeat("x", maxWordWrapLineBytes+1)
	ev := NewEditorViewIndexedLater(piecetable.New([]byte(content)), nil, "")
	defer ev.Close()
	ev.Mapped = &MappedFile{}

	if ev.CurrentLineUnsafeForWordWrap() {
		t.Fatal("the last line of an index that is still filling is a prefix, not a line")
	}

	ev.setIndexStatus(IndexStatus{Phase: IndexComplete, Lines: ev.Li.LineCount()})
	if !ev.CurrentLineUnsafeForWordWrap() {
		t.Fatal("once the index is complete a genuinely long line must be reported as unsafe")
	}
}
