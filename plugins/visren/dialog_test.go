package visren

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func setupDialogTest(t *testing.T) *Dialog {
	return setupDialogTestAt(t, 80, 25)
}

func setupDialogTestAt(t *testing.T, width, height int) *Dialog {
	t.Helper()
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(width, height)
	vtui.FrameManager.Init(scr)
	dir := t.TempDir()
	item := NewItem(dir, "example.txt", time.Now(), false)
	return newDialog(nil, &Plugin{}, vfs.NewOSVFS(dir), dir, []*Item{item})
}

func TestDialogExactNormalLayout(t *testing.T) {
	d := setupDialogTest(t)
	if d.X1 != 1 || d.Y1 != 1 || d.X2 != 78 || d.Y2 != 23 {
		t.Fatalf("dialog bounds=(%d,%d)-(%d,%d)", d.X1, d.Y1, d.X2, d.Y2)
	}
	assertPos := func(name string, element vtui.UIElement, x1, y1, x2, y2 int) {
		t.Helper()
		ax1, ay1, ax2, ay2 := element.GetPosition()
		if ax1 != x1 || ay1 != y1 || ax2 != x2 || ay2 != y2 {
			t.Errorf("%s=(%d,%d)-(%d,%d), want (%d,%d)-(%d,%d)", name, ax1, ay1, ax2, ay2, x1, y1, x2, y2)
		}
	}
	assertPos("name", d.nameEdit, 3, 4, 39, 4)
	assertPos("extension", d.extEdit, 42, 4, 76, 4)
	assertPos("name templates label", d.separators[4], 3, 5, 39, 5)
	assertPos("extension templates label", d.separators[5], 42, 5, 76, 5)
	assertPos("name templates", d.nameCombo, 3, 6, 32, 6)
	assertPos("name plus", d.namePlus, 35, 6, 37, 6)
	assertPos("extension templates", d.extCombo, 42, 6, 71, 6)
	assertPos("extension plus", d.extPlus, 74, 6, 76, 6)
	assertPos("preview", d.preview, 3, 11, 76, 19)
	assertCenteredPreviewHeading(t, d)
	assertEvenPreviewColumns(t, d)
	assertVisRenLayout(t, d)
}

func TestDropdownArrowsUseFieldBackground(t *testing.T) {
	d := setupDialogTest(t)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	assertArrow := func(name string, x, y int) {
		t.Helper()
		d.Show(scr)
		field := scr.GetCell(x-1, y).Attributes
		arrow := scr.GetCell(x, y).Attributes
		if field&vtui.IsBgRGB != arrow&vtui.IsBgRGB {
			t.Fatalf("%s background mode differs: field=%#x arrow=%#x", name, field, arrow)
		}
		if field&vtui.IsBgRGB != 0 {
			if vtui.GetRGBBack(field) != vtui.GetRGBBack(arrow) {
				t.Fatalf("%s RGB background differs: field=%#x arrow=%#x", name, field, arrow)
			}
		} else if vtui.GetIndexBack(field) != vtui.GetIndexBack(arrow) {
			t.Fatalf("%s indexed background differs: field=%#x arrow=%#x", name, field, arrow)
		}
		if field&vtui.BackgroundIntensity != arrow&vtui.BackgroundIntensity {
			t.Fatalf("%s background intensity differs: field=%#x arrow=%#x", name, field, arrow)
		}
	}

	assertArrow("name template", d.nameCombo.X2, d.nameCombo.Y1)
	assertArrow("extension template", d.extCombo.X2, d.extCombo.Y1)
	assertArrow("name history", d.nameEdit.X2, d.nameEdit.Y1)
	assertArrow("extension history", d.extEdit.X2, d.extEdit.Y1)
	assertArrow("search history", d.searchEdit.X2, d.searchEdit.Y1)
	assertArrow("replace history", d.replaceEdit.X2, d.replaceEdit.Y1)
	d.SetFocusedItem(d.nameCombo)
	assertArrow("focused name template", d.nameCombo.X2, d.nameCombo.Y1)
	d.SetFocusedItem(d.extCombo)
	assertArrow("focused extension template", d.extCombo.X2, d.extCombo.Y1)
	d.SetFocusedItem(d.nameEdit)
	assertArrow("focused name history", d.nameEdit.X2, d.nameEdit.Y1)
	d.SetFocusedItem(d.extEdit)
	assertArrow("focused extension history", d.extEdit.X2, d.extEdit.Y1)
	d.SetFocusedItem(d.searchEdit)
	assertArrow("focused search history", d.searchEdit.X2, d.searchEdit.Y1)
	d.SetFocusedItem(d.replaceEdit)
	assertArrow("focused replace history", d.replaceEdit.X2, d.replaceEdit.Y1)
}

func TestDialogInitiallyFocusesSearchAndSelectsFirstPreviewRow(t *testing.T) {
	d := setupDialogTest(t)
	if d.GetFocusedItem() != d.searchEdit {
		t.Fatalf("initial focus=%T, want search edit", d.GetFocusedItem())
	}
	if d.preview.cursor != 0 {
		t.Fatalf("initial preview cursor=%d, want 0", d.preview.cursor)
	}
	if len(d.preview.rows) == 0 {
		t.Fatal("preview has no initial row")
	}
}

func TestPreviewShowsAndOperatesScrollbarWhenRowsOverflow(t *testing.T) {
	d := setupDialogTest(t)
	d.engine.Items = nil
	for idx := 0; idx < 20; idx++ {
		d.engine.Items = append(d.engine.Items, testItem(fmt.Sprintf("file-%02d.txt", idx)))
	}
	d.refreshPreview()
	if !d.preview.scrollbarNeeded() {
		t.Fatal("preview scrollbar is not enabled for overflowing rows")
	}
	if d.preview.scrollBar.X1 != d.preview.X2 || d.preview.scrollBar.Y1 != d.preview.Y1 || d.preview.scrollBar.Y2 != d.preview.Y2 {
		t.Fatalf("scrollbar bounds=(%d,%d)-(%d,%d), preview=(%d,%d)-(%d,%d)",
			d.preview.scrollBar.X1, d.preview.scrollBar.Y1, d.preview.scrollBar.X2, d.preview.scrollBar.Y2,
			d.preview.X1, d.preview.Y1, d.preview.X2, d.preview.Y2)
	}
	if got, want := d.preview.scrollBar.Max, len(d.preview.rows)-d.preview.visibleHeight(); got != want {
		t.Fatalf("scrollbar max=%d, want %d", got, want)
	}
	if got, want := d.preview.divider, d.preview.contentWidth()/2; got != want {
		t.Fatalf("divider=%d, want %d with scrollbar", got, want)
	}

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	d.preview.Show(scr)
	if got := scr.GetCell(d.preview.X2, d.preview.Y1).Char; got != vtui.ScrollUpArrow {
		t.Fatalf("scrollbar top char=%#x, want %#x", got, vtui.ScrollUpArrow)
	}
	down := &vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: visrenMouseCoordinate(d.preview.X2), MouseY: visrenMouseCoordinate(d.preview.Y2), ButtonState: vtinput.FromLeft1stButtonPressed}
	if !d.preview.ProcessMouse(down) {
		t.Fatal("scrollbar down arrow click was not handled")
	}
	d.preview.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: down.MouseX, MouseY: down.MouseY})
	if d.preview.top != 1 || d.preview.scrollBar.Value != 1 {
		t.Fatalf("after scrollbar click top=%d value=%d, want 1", d.preview.top, d.preview.scrollBar.Value)
	}

	d.engine.Items = d.engine.Items[:d.preview.visibleHeight()]
	d.refreshPreview()
	if d.preview.scrollbarNeeded() {
		t.Fatal("preview scrollbar remained enabled when every row fits")
	}
	if got, want := d.preview.divider, d.preview.contentWidth()/2; got != want {
		t.Fatalf("divider=%d, want %d after scrollbar disappeared", got, want)
	}
}

func visrenMouseCoordinate(value int) int16 {
	return int16(value) // #nosec G115 -- this test dialog is allocated inside an 80x25 screen.
}

func TestPreviewHighlightsSearchMatchesInSourceColumn(t *testing.T) {
	d := setupDialogTest(t)
	d.searchEdit.SetText("amp")
	d.refreshPreview()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	d.preview.Show(scr)

	base := vtui.Palette[vtui.ColDialogText]
	highlight := d.preview.matchAttr(base)
	matchX := d.preview.X1 + 2 // "example.txt": the match "amp" begins at rune 2.
	for x := matchX; x < matchX+3; x++ {
		if got := scr.GetCell(x, d.preview.Y1).Attributes; got != highlight {
			t.Fatalf("match cell x=%d attr=%#x, want %#x", x, got, highlight)
		}
	}
	if got := scr.GetCell(d.preview.X1, d.preview.Y1).Attributes; got != base {
		t.Fatalf("non-match attr=%#x, want %#x", got, base)
	}

	d.SetFocusedItem(d.preview)
	d.preview.srcOffset = 1
	d.preview.Show(scr)
	selected := vtui.Palette[vtui.ColDialogSelectedButton]
	selectedHighlight := d.preview.matchAttr(selected)
	if got := scr.GetCell(d.preview.X1+1, d.preview.Y1).Attributes; got != selectedHighlight {
		t.Fatalf("scrolled selected match attr=%#x, want %#x", got, selectedHighlight)
	}
	if got := scr.GetCell(d.preview.X1, d.preview.Y1).Attributes; got != selected {
		t.Fatalf("scrolled selected non-match attr=%#x, want %#x", got, selected)
	}
}

func TestPreviewHighlightsReplacementInDestinationColumn(t *testing.T) {
	d := setupDialogTest(t)
	d.searchEdit.SetText("amp")
	d.replaceEdit.SetText("XYZ")
	d.refreshPreview()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	d.preview.Show(scr)

	base := vtui.Palette[vtui.ColDialogText]
	highlight := d.preview.matchAttr(base)
	rightX := d.preview.X1 + d.preview.divider + 1
	matchX := rightX + 2 // "exXYZle.txt": inserted text begins at rune 2.
	for x := matchX; x < matchX+3; x++ {
		if got := scr.GetCell(x, d.preview.Y1).Attributes; got != highlight {
			t.Fatalf("replacement cell x=%d attr=%#x, want %#x", x, got, highlight)
		}
	}
	if got := scr.GetCell(rightX, d.preview.Y1).Attributes; got != base {
		t.Fatalf("destination non-replacement attr=%#x, want %#x", got, base)
	}

	d.SetFocusedItem(d.preview)
	d.preview.dstOffset = 1
	d.preview.Show(scr)
	selected := vtui.Palette[vtui.ColDialogSelectedButton]
	selectedHighlight := d.preview.matchAttr(selected)
	if got := scr.GetCell(rightX+1, d.preview.Y1).Attributes; got != selectedHighlight {
		t.Fatalf("scrolled selected replacement attr=%#x, want %#x", got, selectedHighlight)
	}
	if got := scr.GetCell(rightX, d.preview.Y1).Attributes; got != selected {
		t.Fatalf("scrolled selected destination attr=%#x, want %#x", got, selected)
	}
}

func TestDialogTabMovesFromSearchToReplace(t *testing.T) {
	d := setupDialogTest(t)
	tab := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_TAB}
	d.ProcessKey(tab)
	if d.GetFocusedItem() != d.replaceEdit {
		t.Fatalf("focus after Tab from search=%T, want replace edit", d.GetFocusedItem())
	}
}

func TestEditorColumnChoicesDescribeTheirActions(t *testing.T) {
	choices := editorColumnChoices()
	want := []string{"Source + target", "Targets only", "Cancel"}
	if len(choices) != len(want) {
		t.Fatalf("editor choices=%v, want %v", choices, want)
	}
	for idx := range choices {
		clean, _, _ := vtui.ParseAmpersandString(choices[idx])
		if clean != want[idx] {
			t.Errorf("editor choice %d=%q, want %q", idx, clean, want[idx])
		}
	}
	if hotkey := vtui.ExtractHotkey(choices[0]); hotkey != 's' {
		t.Errorf("source + target hotkey=%q, want s", hotkey)
	}
	if hotkey := vtui.ExtractHotkey(choices[1]); hotkey != 'o' {
		t.Errorf("targets only hotkey=%q, want o", hotkey)
	}
}

func TestVisRenHelpUsesWideAdaptiveLayout(t *testing.T) {
	setupDialogTestAt(t, 140, 35)
	engine := vtui.NewHelpEngine(nil)
	engine.AddTopic(&vtui.HelpTopic{Name: "VisRen", Lines: []string{"VisRen"}, StickyRows: 1})
	oldEngine := vtui.GlobalHelpEngine
	vtui.GlobalHelpEngine = engine
	t.Cleanup(func() { vtui.GlobalHelpEngine = oldEngine })

	view := newVisRenHelpView(140, 35)
	x1, y1, x2, y2 := view.GetPosition()
	if width, height := x2-x1+1, y2-y1+1; width != 120 || height != 31 {
		t.Fatalf("wide help size=%dx%d, want 120x31", width, height)
	}
	if x1 != 10 || y1 != 2 {
		t.Fatalf("wide help origin=(%d,%d), want (10,2)", x1, y1)
	}

	view.ResizeConsole(100, 25)
	x1, y1, x2, y2 = view.GetPosition()
	if width, height := x2-x1+1, y2-y1+1; width != 96 || height != 21 {
		t.Fatalf("resized help size=%dx%d, want 96x21", width, height)
	}
	if x1 != 2 || y1 != 2 {
		t.Fatalf("resized help origin=(%d,%d), want (2,2)", x1, y1)
	}
}

func TestDialogFieldHotkeysFocusWithoutEditing(t *testing.T) {
	d := setupDialogTest(t)
	d.SetFocusedItem(d.cancelButton)
	fields := []struct {
		label  *vtui.Text
		target *vtui.Edit
		want   rune
	}{
		{d.separators[0], d.nameEdit, 'n'},
		{d.separators[1], d.extEdit, 'e'},
		{d.separators[2], d.searchEdit, 's'},
		{d.separators[3], d.replaceEdit, 'p'},
	}
	for _, field := range fields {
		if got := field.label.GetHotkey(); got != field.want {
			t.Fatalf("label %q hotkey=%q, want %q", field.label.GetText(), got, field.want)
		}
		before := field.target.GetText()
		event := &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, Char: field.want,
			ControlKeyState: vtinput.LeftAltPressed,
		}
		if !d.ProcessKey(event) {
			t.Fatalf("Alt+%c was not handled", field.want)
		}
		if d.GetFocusedItem() != field.target {
			t.Fatalf("Alt+%c focused %T, want %T", field.want, d.GetFocusedItem(), field.target)
		}
		if got := field.target.GetText(); got != before {
			t.Fatalf("Alt+%c changed field text from %q to %q", field.want, before, got)
		}
		d.SetFocusedItem(d.cancelButton)
	}
}

func TestDialogCheckboxHotkeysFocusAndToggle(t *testing.T) {
	d := setupDialogTest(t)
	d.SetFocusedItem(d.searchEdit)
	tests := []struct {
		key      rune
		checkbox *vtui.Checkbox
		before   int
		after    int
	}{
		{key: 'c', checkbox: d.caseCheck, before: 1, after: 0},
		{key: 'g', checkbox: d.regexCheck, before: 0, after: 1},
	}
	for _, tc := range tests {
		if got := tc.checkbox.GetHotkey(); got != tc.key {
			t.Fatalf("checkbox %q hotkey=%q, want %q", tc.checkbox.GetText(), got, tc.key)
		}
		if tc.checkbox.State != tc.before {
			t.Fatalf("checkbox %q initial state=%d, want %d", tc.checkbox.GetText(), tc.checkbox.State, tc.before)
		}
		event := &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, Char: tc.key,
			ControlKeyState: vtinput.LeftAltPressed,
		}
		if !d.ProcessKey(event) {
			t.Fatalf("Alt+%c was not handled", tc.key)
		}
		if d.GetFocusedItem() != tc.checkbox {
			t.Fatalf("Alt+%c focused %T, want checkbox", tc.key, d.GetFocusedItem())
		}
		if tc.checkbox.State != tc.after {
			t.Fatalf("Alt+%c state=%d, want %d", tc.key, tc.checkbox.State, tc.after)
		}
	}
}

func TestDialogTemplateLabelsAreVisibleAndFocusLists(t *testing.T) {
	d := setupDialogTest(t)
	tests := []struct {
		label  *vtui.Text
		target *vtui.ComboBox
		key    rune
		text   string
	}{
		{label: d.separators[4], target: d.nameCombo, key: 't', text: "Name templates"},
		{label: d.separators[5], target: d.extCombo, key: 'l', text: "Extension templates"},
	}
	for _, tc := range tests {
		clean, _, _ := vtui.ParseAmpersandString(tc.label.GetText())
		if clean != tc.text {
			t.Fatalf("template label text=%q, want %q", clean, tc.text)
		}
		if tc.label.GetHotkey() != tc.key {
			t.Fatalf("template label %q hotkey=%q, want %q", tc.label.GetText(), tc.label.GetHotkey(), tc.key)
		}
		before := tc.target.Edit.GetText()
		d.SetFocusedItem(d.searchEdit)
		event := &vtinput.InputEvent{
			Type: vtinput.KeyEventType, KeyDown: true, Char: tc.key,
			ControlKeyState: vtinput.LeftAltPressed,
		}
		if !d.ProcessKey(event) || d.GetFocusedItem() != tc.target {
			t.Fatalf("Alt+%c did not focus its template list", tc.key)
		}
		if got := tc.target.Edit.GetText(); got != before {
			t.Fatalf("Alt+%c changed template from %q to %q", tc.key, before, got)
		}
	}
}

func TestTemplateSelectionAndPlusFollowOriginalBehavior(t *testing.T) {
	d := setupDialogTest(t)
	d.nameEdit.SetText("prefix-")
	d.nameEdit.ClearSelection()
	d.nameCombo.Menu.SetSelectPos(4)
	d.nameCombo.Menu.OnAction(4)
	if got := d.nameEdit.GetText(); got != "prefix-" {
		t.Fatalf("choosing a template inserted it immediately: %q", got)
	}
	if got := d.nameCombo.Edit.GetText(); got != "[L]" {
		t.Fatalf("selected name template=%q, want [L]", got)
	}
	d.SetFocusedItem(d.namePlus)
	if !d.namePlus.fire() {
		t.Fatal("name [+] was not handled")
	}
	if got := d.nameEdit.GetText(); got != "prefix-[L]" {
		t.Fatalf("name [+] result=%q, want prefix-[L]", got)
	}
	if d.GetFocusedItem() != d.nameEdit {
		t.Fatalf("name [+] focus=%T, want name edit", d.GetFocusedItem())
	}

	d.extEdit.SetText("")
	d.extEdit.ClearSelection()
	d.extCombo.Menu.SetSelectPos(2)
	d.extCombo.Menu.OnAction(2)
	d.SetFocusedItem(d.extPlus)
	if !d.extPlus.fire() {
		t.Fatal("extension [+] was not handled")
	}
	if got := d.extEdit.GetText(); got != "[C1+1]" {
		t.Fatalf("extension [+] result=%q, want [C1+1]", got)
	}
	if d.GetFocusedItem() != d.extEdit {
		t.Fatalf("extension [+] focus=%T, want extension edit", d.GetFocusedItem())
	}
}

func TestDialogShowsEnabledRenameLogControl(t *testing.T) {
	d := setupDialogTest(t)
	found := false
	for _, child := range d.GetChildren() {
		if child == d.logLine {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("rename log control is not attached to the dialog")
	}
	if !d.logging {
		t.Fatal("rename logging must be enabled by default")
	}
	if text := d.logLine.GetText(); !strings.Contains(text, tr("VisRen.Log", "Log renaming")) {
		t.Fatalf("rename log control text=%q", text)
	}
	assertCenteredLogLine(t, d)
}

func TestDialogF5MaximizeAndRestore(t *testing.T) {
	d := setupDialogTestAt(t, 100, 35)
	d.preview.divider = 12
	f5 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}
	d.ProcessKey(f5)
	if d.X1 != 1 || d.Y1 != 1 || d.X2-d.X1+1 != 98 || d.Y2-d.Y1+1 != 33 {
		t.Fatalf("maximized bounds=(%d,%d)-(%d,%d)", d.X1, d.Y1, d.X2, d.Y2)
	}
	assertCenteredPreviewHeading(t, d)
	assertEvenPreviewColumns(t, d)
	d.preview.divider = 18
	d.ProcessKey(f5)
	if d.X2-d.X1+1 != 78 || d.Y2-d.Y1+1 != 23 {
		t.Fatalf("restored size=%dx%d", d.X2-d.X1+1, d.Y2-d.Y1+1)
	}
	assertCenteredPreviewHeading(t, d)
	assertEvenPreviewColumns(t, d)
}

func TestDialogConsoleResizeRecalculatesPreviewColumns(t *testing.T) {
	d := setupDialogTest(t)
	d.maximized = true
	d.preview.divider = 12
	d.ResizeConsole(100, 35)
	if d.X2-d.X1+1 != 98 || d.Y2-d.Y1+1 != 33 {
		t.Fatalf("resized size=%dx%d", d.X2-d.X1+1, d.Y2-d.Y1+1)
	}
	assertCenteredPreviewHeading(t, d)
	assertEvenPreviewColumns(t, d)
}

func assertCenteredPreviewHeading(t *testing.T, d *Dialog) {
	t.Helper()
	text := d.separators[7].GetText()
	title := tr("VisRen.BeforeAfter", "name before - name after")
	width := d.separators[7].X2 - d.separators[7].X1 + 1
	if got := runewidth.StringWidth(text); got != width {
		t.Fatalf("preview heading width=%d, want %d: %q", got, width, text)
	}
	index := strings.Index(text, title)
	if index < 0 {
		t.Fatalf("preview heading does not contain %q: %q", title, text)
	}
	left := runewidth.StringWidth(text[:index])
	right := width - left - runewidth.StringWidth(title)
	if left-right < -1 || left-right > 1 {
		t.Fatalf("preview heading is not centered: left=%d right=%d text=%q", left, right, text)
	}
}

func assertCenteredLogLine(t *testing.T, d *Dialog) {
	t.Helper()
	text := d.logLine.GetText()
	caption := "√ " + tr("VisRen.Log", "Log renaming")
	width := d.logLine.X2 - d.logLine.X1 + 1
	if got := runewidth.StringWidth(text); got != width {
		t.Fatalf("log line width=%d, want %d: %q", got, width, text)
	}
	index := strings.Index(text, caption)
	if index < 0 {
		t.Fatalf("log line does not contain %q: %q", caption, text)
	}
	left := runewidth.StringWidth(text[:index])
	right := width - left - runewidth.StringWidth(caption)
	if left-right < -1 || left-right > 1 {
		t.Fatalf("log line is not centered: left=%d right=%d text=%q", left, right, text)
	}
	wantMarkX := d.logLine.X1 + left
	if d.logMarkX != wantMarkX {
		t.Fatalf("log mark x=%d, want %d", d.logMarkX, wantMarkX)
	}
}

func assertEvenPreviewColumns(t *testing.T, d *Dialog) {
	t.Helper()
	width := d.preview.X2 - d.preview.X1 + 1
	left, right := d.preview.divider, width-d.preview.divider-1
	if left-right < -1 || left-right > 1 {
		t.Fatalf("preview columns are not even: left=%d right=%d width=%d", left, right, width)
	}
}

func assertVisRenLayout(t *testing.T, d *Dialog) {
	t.Helper()
	for _, err := range vtui.ValidateLayout(d) {
		layoutErr, ok := err.(vtui.LayoutError)
		if ok && strings.Contains(layoutErr.Message, "must have vertical air") &&
			(layoutErr.Element1 == d.logLine || layoutErr.Element2 == d.logLine) {
			// VisRen's 78x23 reference layout intentionally puts the decorated
			// log line immediately above the bottom button row.
			continue
		}
		t.Errorf("layout validation failed: %v", err)
	}
}
