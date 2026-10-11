package vtui

import (
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

// borderText reads one border row of a menu the way it was painted.
func borderText(scr *ScreenBuf, m *VMenu, y int) string {
	var sb strings.Builder
	for x := m.X1; x <= m.X2; x++ {
		sb.WriteRune(rune(scr.GetCell(x, y).Char))
	}
	return sb.String()
}

func TestVMenu_SetTitleAndBottomTitleArePainted(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := NewVMenu("About")
	m.AddItem(MenuItem{Text: "row"})
	m.SetPosition(0, 0, 39, 4)

	m.SetTitle("About &f4 *")
	if got := m.GetTitle(); got != "About f4 *" {
		t.Fatalf("GetTitle = %q, want the ampersand dropped", got)
	}
	m.SetBottomTitle("Ctrl+C copy")
	if got := m.GetBottomTitle(); got != "Ctrl+C copy" {
		t.Fatalf("GetBottomTitle = %q", got)
	}

	scr := NewSilentScreenBuf()
	scr.AllocBuf(40, 6)
	m.Show(scr)
	if top := borderText(scr, m, m.Y1); !strings.Contains(top, " About f4 * ") {
		t.Errorf("top border = %q, want the new title", top)
	}
	if bottom := borderText(scr, m, m.Y2); !strings.Contains(bottom, " Ctrl+C copy ") {
		t.Errorf("bottom border = %q, want the bottom title", bottom)
	}

	m.SetBottomTitle("")
	scr = NewSilentScreenBuf()
	scr.AllocBuf(40, 6)
	m.Show(scr)
	if bottom := borderText(scr, m, m.Y2); strings.Contains(bottom, "Ctrl") {
		t.Errorf("bottom border = %q after clearing the bottom title", bottom)
	}
}

func TestVMenu_IgnoreSingleClickSelectsAndDoubleClickConfirms(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewSilentScreenBuf())
	m := NewVMenu("List")
	for _, s := range []string{"one", "two", "three"} {
		m.AddItem(MenuItem{Text: s})
	}
	m.SetPosition(0, 0, 30, 4)
	m.ClearDone()
	m.IgnoreSingleClick = true
	confirmed := -1
	m.OnAction = func(i int) { confirmed = i }

	click := func(flags uint32) {
		m.ProcessMouse(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          5,
			MouseY:          2, // the second row: "two"
			ButtonState:     vtinput.FromLeft1stButtonPressed,
			MouseEventFlags: flags,
			KeyDown:         true,
		})
	}

	click(0)
	if m.SelectPos != 1 {
		t.Fatalf("a single click should select row 1, SelectPos = %d", m.SelectPos)
	}
	if confirmed != -1 || m.IsDone() {
		t.Fatalf("a single click must not confirm: confirmed=%d done=%v", confirmed, m.IsDone())
	}

	click(vtinput.DoubleClick)
	if confirmed != 1 || !m.IsDone() {
		t.Fatalf("a double click should confirm row 1: confirmed=%d done=%v", confirmed, m.IsDone())
	}
}

func TestBackendDetailsReturnsACopyOfWhatTheBackendReported(t *testing.T) {
	previousName, previousDetails := ActiveBackend(), BackendDetails()
	t.Cleanup(func() { SetActiveBackend(previousName, previousDetails...) })

	SetActiveBackend("win32", "cell 8x16", "GDI SetDIBitsToDevice")
	details := BackendDetails()
	if len(details) != 2 || details[0] != "cell 8x16" || details[1] != "GDI SetDIBitsToDevice" {
		t.Fatalf("BackendDetails = %q", details)
	}
	details[0] = "changed"
	if again := BackendDetails(); again[0] != "cell 8x16" {
		t.Fatalf("BackendDetails handed out its own slice: %q", again)
	}

	SetActiveBackend("x11")
	if details := BackendDetails(); len(details) != 0 {
		t.Fatalf("BackendDetails after a backend with no details = %q", details)
	}
}
