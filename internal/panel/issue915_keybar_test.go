package panel

import (
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
)

// The archive plugin owns Shift+F1, Shift+F2 and Shift+F3 as global hotkeys,
// which HotkeyManager knows nothing about, so the panels' Shift row has to
// name them itself or the slots stay blank (#915).
func TestPanelsFrame_ShiftKeyBarNamesArchiveHotkeys(t *testing.T) {
	oldHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = nil
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = oldHotkeys })

	pf := NewPanelsFrame()
	defer pf.Close()

	ks := pf.GetKeyLabels()
	if ks == nil {
		t.Fatal("PanelsFrame labels are nil")
	}
	for index, key := range []string{"KeyBar.ShiftF1", "KeyBar.ShiftF2", "KeyBar.ShiftF3"} {
		want := i18n.Msg(key)
		if want == "" {
			t.Fatalf("%s resolves to an empty caption", key)
		}
		if got := ks.Shift[index]; got != want {
			t.Errorf("Shift+F%d key bar label = %q, want %q", index+1, got, want)
		}
	}
}
