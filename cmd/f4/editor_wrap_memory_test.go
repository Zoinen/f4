package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// newWrapMemoryStore installs a file-state provider writing into a temporary
// directory and returns it, so a test can read back what the editor recorded.
func newWrapMemoryStore(t *testing.T) *F4FileStateProvider {
	t.Helper()
	fs := &F4FileStateProvider{
		path:  filepath.Join(t.TempDir(), "file_states.json"),
		Limit: 10,
		Data:  make(map[string]*FileState),
	}
	old := GlobalFileState
	GlobalFileState = fs
	t.Cleanup(func() { GlobalFileState = old })
	return fs
}

func TestF4FileStateProvider_SaveEditorWrapKeepsPosition(t *testing.T) {
	fs := &F4FileStateProvider{
		path:  filepath.Join(t.TempDir(), "file_states.json"),
		Limit: 10,
		Data:  make(map[string]*FileState),
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

// The choice must reach the store when it is made, not only at close: an
// editor still open when f4 exits never reaches Close.
func TestEditorView_WordWrapToggleIsRemembered(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	fs := newWrapMemoryStore(t)
	t.Cleanup(func() { fs.Flush() })

	ev := NewEditorView(piecetable.New([]byte("some text")), nil, "wrapped.txt")
	defer ev.Close()

	pressKey(ev, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F3,
	})
	if !ev.WordWrap {
		t.Fatal("F3 did not turn word wrap on")
	}
	fs.Flush()

	state := fs.GetState(FileStateKey(nil, "wrapped.txt"))
	if state == nil || !state.EditorWrap {
		t.Fatal("word wrap turned on with F3 was not remembered for the file")
	}

	pressKey(ev, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_F3,
	})
	if ev.WordWrap {
		t.Fatal("F3 did not turn word wrap off again")
	}
	fs.Flush()

	if state = fs.GetState(FileStateKey(nil, "wrapped.txt")); state == nil || state.EditorWrap {
		t.Fatal("word wrap turned off again was not remembered for the file")
	}
}

// Content the editor cannot wrap turns wrapping off for the session, but the
// setting the user chose belongs to the file, not to today's contents of it.
func TestEditorView_UnsafeWordWrapKeepsRememberedChoice(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	drainPendingTasks()

	fs := newWrapMemoryStore(t)

	ev := NewEditorView(piecetable.New([]byte("text")), nil, "wrapped.txt")
	ev.setWordWrap(true)

	ev.disableUnsafeWordWrap()
	if ev.WordWrap {
		t.Fatal("unsafe content must turn word wrap off")
	}
	if !ev.wordWrapWanted {
		t.Fatal("suppressing wrapping must not discard the choice made for the file")
	}

	ev.Close()
	fs.Flush()

	state := fs.GetState(FileStateKey(nil, "wrapped.txt"))
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
	drainPendingTasks()

	content := strings.Repeat("x", maxWordWrapLineBytes+1)
	ev := NewEditorViewIndexedLater(piecetable.New([]byte(content)), nil, "")
	defer ev.Close()
	ev.mapped = &MappedFile{}

	if ev.currentLineUnsafeForWordWrap() {
		t.Fatal("the last line of an index that is still filling is a prefix, not a line")
	}

	ev.setIndexStatus(IndexStatus{Phase: IndexComplete, Lines: ev.li.LineCount()})
	if !ev.currentLineUnsafeForWordWrap() {
		t.Fatal("once the index is complete a genuinely long line must be reported as unsafe")
	}
}
