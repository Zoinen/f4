package dialog

import (
	"bytes"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// dialogRows renders dlg and returns the text of each of its rows, from the
// top frame row to the bottom one, cut out of vtui's screen dump (the format
// Ctrl+Shift+P writes).
func dialogRows(t *testing.T, scr *vtui.ScreenBuf, dlg *FileDialog) []string {
	t.Helper()
	dlg.Show(scr)
	var buf bytes.Buffer
	scr.Dump(&buf)
	lines := strings.Split(buf.String(), "\n")
	var rows []string
	for y := dlg.Y1; y <= dlg.Y2; y++ {
		cells := []rune(lines[2+y])
		rows = append(rows, string(cells[dlg.X1:dlg.X2+1]))
	}
	return rows
}

// The rename dialog (f4#891) has no blank rows: the prompt and the field sit
// right under the title, a rule divides
// them from the buttons, and the buttons are the last row inside the frame.
//
// Screen Dump (80x25, the middle 40 columns):
//
//	╔═══════════════ Rename ═══════════[×]═╗
//	║ Rename 'a.txt' to:                   ║
//	║ a.txt                                ║
//	║──────────────────────────────────────║
//	║          [ Ok ]  [ Cancel ]          ║
//	╚══════════════════════════════════════╝
func TestFileInputBoxHasNoBlankRowsAndRulesOverTheButtons(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()

	dlg := FileInputBox("Rename", "Rename 'a.txt' to:", "a.txt", nil)
	rows := dialogRows(t, scr, dlg)
	if len(rows) != fileInputBoxHeight {
		t.Fatalf("dialog has %d rows, want %d:\n%s", len(rows), fileInputBoxHeight, strings.Join(rows, "\n"))
	}
	dump := strings.Join(rows, "\n")

	frame := []rune(rows[1])[0]
	for i := 1; i < len(rows)-1; i++ {
		inner := strings.TrimSpace(strings.Trim(rows[i], string(frame)))
		if inner == "" {
			t.Errorf("row %d of the dialog is blank:\n%s", i, dump)
		}
	}
	if !strings.Contains(rows[1], "Rename 'a.txt' to:") {
		t.Errorf("the prompt is not the first row inside the frame:\n%s", dump)
	}
	if !strings.Contains(rows[2], "a.txt") {
		t.Errorf("the field is not right under the prompt:\n%s", dump)
	}
	assertRule(t, rows[3], dump)
	if !strings.Contains(rows[4], "Ok") || !strings.Contains(rows[4], "Cancel") {
		t.Errorf("the buttons are not the row under the rule:\n%s", dump)
	}
}

// assertDialogLayout is vtui.AssertLayout without the blank row it asks for
// between the frame and the first and last controls: the file dialogs start
// right under the title and end right over the bottom frame, as far2l's do
// (#891). Everything else the validator checks still applies.
func assertDialogLayout(t *testing.T, dlg vtui.Container) {
	t.Helper()
	rules := vtui.DefaultLayoutRules
	rules.FrameClearanceY = 0
	vtui.AssertLayoutWithRules(t, dlg, rules)
}

// assertRule checks that row is one horizontal line between the two cells of
// the frame. vtui's modal dialogs clip their controls one cell inside the
// frame, so a rule does not paint its junctions with the frame yet: the cells
// at both ends are still the plain vertical frame.
func assertRule(t *testing.T, row string, dump string) {
	t.Helper()
	cells := []rune(row)
	line := cells[1]
	if line == ' ' {
		t.Errorf("the rule row is blank:\n%s", dump)
	}
	for _, c := range cells[1 : len(cells)-1] {
		if c != line {
			t.Errorf("the rule row is not one line %q:\n%s", string(line), dump)
			return
		}
	}
}
