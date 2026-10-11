package vtui

import "testing"

// f4 #891: the history dropdown is as wide as the field it drops from (it was
// capped at 50 columns).
func TestEdit_HistoryDropdownMatchesTheFieldWidth(t *testing.T) {
	fm, _, edit, _ := historyPickDialog(t)
	edit.SetPosition(5, 3, 5+59, 3) // a field of 60 columns
	edit.History = []string{"/very/long/directory/name/that/does/not/fit/into/the/field/at/all/really/yes"}
	edit.OpenHistory()

	top, ok := fm.GetTopFrame().(*VMenu)
	if !ok {
		t.Fatalf("top frame is %T, want the history menu", fm.GetTopFrame())
	}
	x1, _, x2, _ := top.GetPosition()
	if x1 != 5 || x2 != 5+59 {
		t.Fatalf("history menu spans %d..%d, want the field's 5..64", x1, x2)
	}
	if top.TruncateMark != ">" {
		t.Fatalf("truncate mark %q, want >", top.TruncateMark)
	}
}
