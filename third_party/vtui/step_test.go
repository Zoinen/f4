package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

func TestStep_HeadlessSyntheticEvents(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	defer fm.Shutdown()

	dlg := NewDialog(0, 0, 40, 10, "Test")
	edit := NewEdit(2, 2, 36, "")
	dlg.AddItem(edit)
	fm.Push(dlg)

	// Post 100 character key events
	const eventCount = 100
	for i := 0; i < eventCount; i++ {
		char := rune('a' + (i % 26))
		fm.PostEvent(vtinput.InputEvent{
			Type:    vtinput.KeyEventType,
			KeyDown: true,
			Char:    char,
		})
	}

	// Process all events using Step(0)
	steps := 0
	for steps < eventCount+10 {
		if !fm.Step(0) {
			t.Fatal("Step returned false unexpectedly")
		}
		steps++
		if len(edit.GetText()) == eventCount {
			break
		}
	}

	if len(edit.GetText()) != eventCount {
		t.Errorf("Expected edit text length %d, got %d (text: %q)", eventCount, len(edit.GetText()), edit.GetText())
	}
}
