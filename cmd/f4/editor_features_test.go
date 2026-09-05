package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestEditor_TabBehavior(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pt := piecetable.New(nil)
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()
	ev.TabSize = 4

	// 1. ExpandTabs = 0 (Insert raw tabs)
	ev.ExpandTabs = 0
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if ev.pt.String() != "\t" {
		t.Errorf("Expected raw tab, got %q", ev.pt.String())
	}

	// 2. ExpandTabs = 1 (Insert spaces)
	ev.SetText("")
	ev.ExpandTabs = 1
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	if ev.pt.String() != "    " {
		t.Errorf("Expected 4 spaces, got %q", ev.pt.String())
	}

	// 3. Align to Tab Stop
	ev.SetText("a") // 1 char
	ev.CursorPos = 1
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
	// "a" (1) + 3 spaces to reach 4. Total "a   "
	if ev.pt.String() != "a   " {
		t.Errorf("Expected alignment to tab stop, got %q (len %d)", ev.pt.String(), len(ev.pt.String()))
	}
}

func TestEditor_AutoIndent(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	// Test with spaces
	pt := piecetable.New([]byte("    line1"))
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()
	ev.AutoIndent = true
	ev.CursorLine = 0
	ev.CursorPos = 9 // End of line

	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if ev.CursorLine != 1 || ev.CursorPos != 4 {
		t.Errorf("AutoIndent failed for spaces: Line %d, Pos %d", ev.CursorLine, ev.CursorPos)
	}

	// Test with mixed tabs/spaces
	ev.SetText("\t  mixed")
	ev.CursorLine = 0
	ev.CursorPos = 8
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	line2, _ := ev.pt.GetRange(ev.li.GetLineOffset(1), 3)
	if string(line2) != "\t  " {
		t.Errorf("AutoIndent failed for mixed prefix: %q", string(line2))
	}
}

func TestEditor_CursorBeyondEOL(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pt := piecetable.New([]byte("line"))
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()
	ev.CursorBeyondEOL = true
	ev.CursorPos = 4 // End of "line"

	// 1. Move right into virtual space
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT})
	if ev.CursorPos != 4 || ev.CursorVirtualSpaces != 1 {
		t.Errorf("Failed to move into virtual space: Pos %d, Virt %d", ev.CursorPos, ev.CursorVirtualSpaces)
	}

	// 2. Typing in virtual space must materialize spaces
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'X'})
	// Expected: "line " + "X"
	if ev.pt.String() != "line X" {
		t.Errorf("Materialization failed: %q", ev.pt.String())
	}
	if ev.CursorVirtualSpaces != 0 {
		t.Error("Virtual spaces not reset after typing")
	}

	// 3. Vertical navigation preserves virtual column
	ev.SetText("line1\nline2long")
	ev.CursorBeyondEOL = true
	ev.CursorLine = 0
	ev.CursorPos = 5
	ev.CursorVirtualSpaces = 10
	ev.updateDesiredVisualCol()

	// Move down to a longer line
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	// Should land at the end of line 2 but keep high DesiredVisualCol
	if ev.CursorLine != 1 {
		t.Fatal("Move down failed")
	}
	if ev.CursorPos != 9 {
		t.Errorf("Expected to land at EOL, got pos %d", ev.CursorPos)
	}
	if ev.CursorVirtualSpaces <= 0 {
		t.Error("Virtual spaces lost on down move")
	}
}

func TestEditor_EditorConfigIntegration(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	tmpDir := t.TempDir()

	// Create .editorconfig
	config := `
[*]
indent_style = space
indent_size = 2

[*.go]
indent_style = tab
tab_width = 4
`
	if err := os.WriteFile(filepath.Join(tmpDir, ".editorconfig"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}

	v := vfs.NewOSVFS(tmpDir)

	// 1. Test .go file (should use tabs)
	goFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(goFile, []byte("package main"), 0600); err != nil {
		t.Fatal(err)
	}

	evGo := NewEditorView(piecetable.New(nil), v, goFile)
	defer evGo.Close()
	if evGo.ExpandTabs != 0 || evGo.TabSize != 4 {
		t.Errorf("EditorConfig failed for .go: style=%d, size=%d", evGo.ExpandTabs, evGo.TabSize)
	}

	// 2. Test other file (should use 2 spaces)
	txtFile := filepath.Join(tmpDir, "readme.txt")
	if err := os.WriteFile(txtFile, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}

	evTxt := NewEditorView(piecetable.New(nil), v, txtFile)
	defer evTxt.Close()
	if evTxt.ExpandTabs != 1 || evTxt.TabSize != 2 {
		t.Errorf("EditorConfig failed for .txt: style=%d, size=%d", evTxt.ExpandTabs, evTxt.TabSize)
	}
}
func TestEditor_DeleteLine(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pt := piecetable.New([]byte("line1\nline2\nline3"))
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()
	ev.CursorLine = 1
	ev.CursorPos = 2

	pressKey(ev, &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_Y,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})

	expected := "line1\nline3"
	if ev.pt.String() != expected {
		t.Errorf("Expected %q, got %q", expected, ev.pt.String())
	}

	if ev.CursorLine != 1 {
		t.Errorf("Expected CursorLine 1, got %d", ev.CursorLine)
	}

	pressKey(ev, &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_Y,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})

	expected2 := "line1"
	if ev.pt.String() != expected2 {
		t.Errorf("Expected %q, got %q", expected2, ev.pt.String())
	}

	if ev.CursorLine != 0 || ev.CursorPos != 5 {
		t.Errorf("Cursor misplaced after last line delete: Line %d, Pos %d", ev.CursorLine, ev.CursorPos)
	}

	ev.Undo()
	if ev.pt.String() != "line1\nline3" {
		t.Errorf("Undo last line delete failed, got %q", ev.pt.String())
	}
}

func TestEditor_Tab_MaterializeBeyondEOL(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pt := piecetable.New([]byte("line"))
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()
	ev.CursorBeyondEOL = true
	ev.CursorPos = 4
	ev.CursorVirtualSpaces = 2 // Virtual cursor at col 6
	ev.ExpandTabs = 1          // Spaces
	ev.TabSize = 4

	// Press Tab at col 6
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})

	// Should materialize 2 virtual spaces + 2 spaces for tab (to reach col 8)
	expected := "line    "
	if ev.pt.String() != expected {
		t.Errorf("Tab failed to materialize virtual spaces. Got %q, want %q", ev.pt.String(), expected)
	}
}

func TestEditor_AutoIndent_EmptyLine(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	// Entering Enter on an empty line should not crash and should produce a new empty line
	pt := piecetable.New(nil)
	ev := NewEditorView(pt, nil, "test.txt")
	defer ev.Close()
	ev.AutoIndent = true

	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if ev.li.LineCount() != 2 {
		t.Errorf("Enter failed on empty file, lines: %d", ev.li.LineCount())
	}
}
