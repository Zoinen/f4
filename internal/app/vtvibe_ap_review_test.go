package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/internal/vtvibe/ap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func aiReviewTestMods() []ap.ModificationResult {
	return []ap.ModificationResult{
		{FilePath: "vfs/ai_vfs.go", ModIdx: 0, Action: "REPLACE", Locator: "return 0, ErrNotSupported", Status: ap.ModOK},
		{FilePath: "vfs/ai_vfs.go", ModIdx: 1, Action: "INSERT_AFTER", Locator: "func (d *aiDrive) Open(\n\tctx context.Context, p string", Status: ap.ModSkipped},
		{FilePath: "go.mod", ModIdx: 0, Action: "REPLACE", Locator: "require x v1", Status: ap.ModFailed,
			Err: &ap.AppError{Code: ap.ErrCode("SNIPPET_NOT_FOUND"), Message: "Snippet not found in go.mod & nowhere near"}},
		{FilePath: "old.txt", ModIdx: -1, Action: "RENAME", Locator: "new.txt", Status: ap.ModOK},
	}
}

// aiScreenText renders frame into a fresh buffer and returns the text part
// of vtui's screen dump (the same format Ctrl+Shift+P writes).
func aiScreenText(t *testing.T, scr *vtui.ScreenBuf, frame vtui.Frame) string {
	t.Helper()
	frame.Show(scr)
	var buf bytes.Buffer
	scr.Dump(&buf)
	text := buf.String()
	if i := strings.Index(text, "--- CELL METADATA"); i >= 0 {
		text = text[:i]
	}
	return text
}

func aiReviewButtons(w *vtui.Window) map[string]*vtui.Button {
	res := map[string]*vtui.Button{}
	for _, it := range w.GetChildren() {
		if b, ok := it.(*vtui.Button); ok {
			res[b.GetCaption()] = b
		}
	}
	return res
}

// aiCaption is a button label as Button.GetCaption reports it.
func aiCaption(key string) string { return strings.ReplaceAll(i18n.Msg(key), "&", "") }

func TestAIShowPatchReviewListsEveryModification(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "afailed.md"), []byte("report"), 0600); err != nil {
		t.Fatal(err)
	}
	patch := &vtvibe.Patch{ID: "aa000001", Text: "aa000001 AP 3.2\n"}
	dlg := aiShowPatchReview(nil, patch, root, aiReviewTestMods(), 2, "patcher output")
	if !aiReviewIsTop(dlg) {
		t.Fatalf("top frame = %T, want the review dialog", vtui.FrameManager.GetTopFrame())
	}

	text := aiScreenText(t, scr, dlg)
	t.Logf("screen dump:\n%s", text)
	for _, want := range []string{
		i18n.Msg("AI.ReviewColFile"), i18n.Msg("AI.ReviewColLocator"),
		"vfs/ai_vfs.go", "go.mod", "old.txt", "INSERT_AFTER", "RENAME",
		i18n.Msg("AI.ReviewOK"), i18n.Msg("AI.ReviewSkipped"), i18n.Msg("AI.ReviewFailed"),
		"return 0, ErrNotSupported",
		"func (d *aiDrive) Open( …",
		"new.txt",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("review screen lacks %q", want)
		}
	}
	// 2 would apply (REPLACE + RENAME), 1 already applied, 1 fails.
	if want := fmt.Sprintf(i18n.Msg("AI.ReviewTotals"), 2, 1, 1); !strings.Contains(text, want) {
		t.Errorf("review screen lacks totals %q", want)
	}
	// The first row is selected, so its locator is the detail line.
	if !strings.Contains(text, " return 0, ErrNotSupported") {
		t.Error("detail line missing")
	}

	btns := aiReviewButtons(dlg)
	for _, key := range []string{"AI.BtnApplyPatch", "AI.BtnViewLog", "AI.BtnAttachReport", "AI.ReviewBtnClose"} {
		if btns[aiCaption(key)] == nil {
			t.Errorf("review dialog lacks the %s button", key)
		}
	}
	if b := btns[aiCaption("AI.BtnApplyPatch")]; b != nil && !b.IsDefault {
		t.Error("Apply should be the default button when something would apply")
	}

	btns[aiCaption("AI.ReviewBtnClose")].OnClick()
	if !dlg.IsDone() {
		t.Fatal("Close did not close the review dialog")
	}
}

func TestAIShowPatchReviewNothingToApply(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	mods := []ap.ModificationResult{
		{FilePath: "a.txt", Action: "REPLACE", Locator: "x", Status: ap.ModSkipped},
		{FilePath: "b.txt", Action: "DELETE", Status: ap.ModFailed, Err: &ap.AppError{Code: ap.ErrCode("FILE_NOT_FOUND")}},
	}
	dlg := aiShowPatchReview(nil, &vtvibe.Patch{}, t.TempDir(), mods, 2, "")
	btns := aiReviewButtons(dlg)
	if btns[aiCaption("AI.BtnApplyPatch")] != nil {
		t.Error("Apply offered although nothing would be written")
	}
	if btns[aiCaption("AI.BtnViewLog")] != nil {
		t.Error("View log offered for empty output")
	}
	if btns[aiCaption("AI.BtnAttachReport")] != nil {
		t.Error("Attach report offered without afailed.md")
	}
	if b := btns[aiCaption("AI.ReviewBtnClose")]; b == nil || !b.IsDefault {
		t.Error("Close should be the default button when nothing would apply")
	}
	dlg.Close()
}

func TestAIReviewHelpers(t *testing.T) {
	if got := aiReviewOneLine("\n  first  \n\nsecond\n"); got != "first …" {
		t.Errorf("aiReviewOneLine multi = %q", got)
	}
	if got := aiReviewOneLine("  only \r\n"); got != "only" {
		t.Errorf("aiReviewOneLine single = %q", got)
	}
	if got := aiReviewOneLine(" \n "); got != "" {
		t.Errorf("aiReviewOneLine blank = %q", got)
	}

	mods := aiReviewTestMods()
	if got := aiReviewDetail(mods[2]); got != "SNIPPET_NOT_FOUND: Snippet not found in go.mod & nowhere near" {
		t.Errorf("failed detail = %q", got)
	}
	if got := aiReviewDetail(ap.ModificationResult{Status: ap.ModFailed, Err: &ap.AppError{Code: "X"}}); got != "X" {
		t.Errorf("failed detail without message = %q", got)
	}
	if got := aiReviewDetail(mods[1]); got != "func (d *aiDrive) Open( …" {
		t.Errorf("ok detail = %q", got)
	}

	if ok, skipped, failed, excluded := aiReviewCounts(mods); ok != 2 || skipped != 1 || failed != 1 || excluded != 0 {
		t.Errorf("counts = %d/%d/%d/%d, want 2/1/1/0", ok, skipped, failed, excluded)
	}
	// An excluded row is neither an error nor "already applied".
	withExcluded := append(append([]ap.ModificationResult(nil), mods...), ap.ModificationResult{FilePath: "x", Status: ap.ModExcluded})
	if ok, skipped, failed, excluded := aiReviewCounts(withExcluded); ok != 2 || skipped != 1 || failed != 1 || excluded != 1 {
		t.Errorf("counts with an excluded row = %d/%d/%d/%d, want 2/1/1/1", ok, skipped, failed, excluded)
	}
	if got := aiReviewStatusText(ap.ModExcluded); got != i18n.Msg("AI.ReviewExcluded") || got == i18n.Msg("AI.ReviewFailed") {
		t.Errorf("excluded status text = %q", got)
	}
	if got, want := aiReviewTotals(withExcluded), fmt.Sprintf(i18n.Msg("AI.ReviewTotalsExcluded"), 2, 1, 1, 1); got != want {
		t.Errorf("totals with an excluded row = %q, want %q", got, want)
	}
	if got, want := aiReviewTotals(mods), fmt.Sprintf(i18n.Msg("AI.ReviewTotals"), 2, 1, 1); got != want {
		t.Errorf("totals = %q, want %q", got, want)
	}

	// '&' must survive as a literal, and the label is exactly w wide.
	if got := aiReviewLabel("a&b", 5); got != "a&&b  " {
		t.Errorf("aiReviewLabel = %q", got)
	}
	if got := aiReviewLabel("abcdefgh", 5); got != "abcd…" {
		t.Errorf("aiReviewLabel truncated = %q", got)
	}

	rev := newAIReview([]ap.ModificationResult{{FilePath: "f", Status: ap.ModOK}})
	row := aiReviewRow{r: rev, i: 0}
	if row.GetCellText(aiReviewColAction) != "-" || row.GetCellText(aiReviewColFile) != "f" ||
		row.GetCellText(aiReviewColStatus) != i18n.Msg("AI.ReviewOK") || row.GetCellText(99) != "" ||
		row.GetCellText(aiReviewColCheck) != "[x]" {
		t.Error("aiReviewRow cells")
	}
	rev.toggle(0)
	if row.GetCellText(aiReviewColCheck) != "[ ]" {
		t.Error("unchecked row still shows a check mark")
	}
	rev.toggle(5) // out of range: ignored
}

func TestAIReviewSelection(t *testing.T) {
	mods := append(aiReviewTestMods(), ap.ModificationResult{FilePath: "late.txt", ModIdx: 0, Action: "CREATE", Status: ap.ModExcluded})
	rev := newAIReview(mods)
	// The dry run's own exclusions come back unchecked, the rest checked.
	if got := rev.checked(); got != 4 {
		t.Fatalf("checked = %d, want 4", got)
	}
	if only := rev.only(); len(only) != 4 || only[ap.ModKey{FilePath: "late.txt", ModIdx: 0}] {
		t.Fatalf("only = %v, want the four checked rows", only)
	}
	rev.toggle(4)
	if rev.only() != nil {
		t.Fatal("everything checked should run the whole patch (nil Only)")
	}
	// Only the skipped and the failing row left: nothing to write.
	rev.toggle(0)
	rev.toggle(3)
	rev.toggle(4)
	if rev.canApply() {
		t.Error("canApply with only skipped/failing rows checked")
	}
	want := map[ap.ModKey]bool{{FilePath: "vfs/ai_vfs.go", ModIdx: 1}: true, {FilePath: "go.mod", ModIdx: 0}: true}
	if got := rev.only(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("only = %v, want %v", got, want)
	}
	// A re-checked excluded row may apply: the dry run never looked at it.
	rev.toggle(4)
	if !rev.canApply() {
		t.Error("canApply false with an excluded row checked again")
	}
}

// aiReviewIsTop reports whether dlg's review screen is the frame on top.
func aiReviewIsTop(dlg *vtui.Window) bool {
	s, ok := vtui.FrameManager.GetTopFrame().(*aiReviewScreen)
	return ok && s.Window == dlg
}

// TestAIShowPatchReviewResize: a resized terminal gets the review laid out
// again over the whole workspace, and what the reader has done stays.
func TestAIShowPatchReviewResize(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	dlg := aiShowPatchReview(nil, &vtvibe.Patch{ID: "aa000001", Text: "aa000001 AP 3.2\n"}, t.TempDir(), aiReviewTestMods(), 2, "")
	table := aiReviewTableOf(t, dlg)
	pane := aiReviewPaneOf(t, dlg)
	table.ProcessKey(aiKey(vtinput.VK_SPACE, ' '))
	sel := table.SelectPos

	for _, size := range [][2]int{{140, 40}, {80, 30}} {
		w, h := size[0], size[1]
		scr.AllocBuf(w, h)
		vtui.FrameManager.GetTopFrame().ResizeConsole(w, h)
		_, y1, x2, y2 := dlg.GetPosition()
		if x2 != w-1 || y2 != h-2 || y1 < 0 {
			t.Errorf("%dx%d: review at y %d..%d, right edge %d; want it to fill the workspace", w, h, y1, y2, x2)
		}
		if _, _, px2, _ := pane.GetPosition(); px2 > x2 {
			t.Errorf("%dx%d: diff pane's right edge %d is outside the dialog (%d)", w, h, px2, x2)
		}
		if _, py1, _, py2 := pane.GetPosition(); py2-py1+1 != min(max(h/4, 6), 14) || py2 > y2 {
			t.Errorf("%dx%d: diff pane rows %d..%d", w, h, py1, py2)
		}
		if table.SelectPos != sel {
			t.Errorf("%dx%d: the table's cursor moved from %d to %d", w, h, sel, table.SelectPos)
		}
		if text := aiScreenText(t, scr, dlg); !strings.Contains(text, i18n.Msg("AI.ReviewTitle")) {
			t.Errorf("%dx%d: title missing after the re-layout:\n%s", w, h, text)
		}
	}
}

// aiReviewTableOf finds the review table in the dialog.
func aiReviewTableOf(t *testing.T, w *vtui.Window) *aiReviewTable {
	t.Helper()
	for _, it := range w.GetChildren() {
		if tb, ok := it.(*aiReviewTable); ok {
			return tb
		}
	}
	t.Fatal("review dialog has no table")
	return nil
}

// aiReviewPaneOf finds the permanent diff pane in the dialog.
func aiReviewPaneOf(t *testing.T, w *vtui.Window) *aiReviewDiffPane {
	t.Helper()
	for _, it := range w.GetChildren() {
		if p, ok := it.(*aiReviewDiffPane); ok {
			return p
		}
	}
	t.Fatal("review dialog has no diff pane")
	return nil
}

func aiKey(vk uint16, ch rune) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, Char: ch}
}

// TestAIShowPatchReviewToggles switches rows off with Space/Ins and checks
// that Apply and Dry run hand exactly the checked rows to the patcher.
func TestAIShowPatchReviewToggles(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	type call struct {
		dry  bool
		only map[ap.ModKey]bool
	}
	var calls []call
	saved := aiReviewRunPatcher
	t.Cleanup(func() { aiReviewRunPatcher = saved })
	aiReviewRunPatcher = func(_ *panel.PanelsFrame, _ *vtvibe.Patch, _ string, dry bool, only map[ap.ModKey]bool) {
		calls = append(calls, call{dry, only})
	}

	patch := &vtvibe.Patch{ID: "aa000001", Text: "aa000001 AP 3.2\n"}
	open := func() (*vtui.Window, *aiReviewTable) {
		dlg := aiShowPatchReview(nil, patch, t.TempDir(), aiReviewTestMods(), 2, "")
		return dlg, aiReviewTableOf(t, dlg)
	}

	dlg, table := open()
	// Ins on the first row (REPLACE in vfs/ai_vfs.go): off, and the cursor
	// moves down as it does in the panels.
	if !table.ProcessKey(aiKey(vtinput.VK_INSERT, 0)) || table.SelectPos != 1 {
		t.Fatalf("Ins not handled or did not move down (pos %d)", table.SelectPos)
	}
	// Space on the RENAME row: off, the cursor stays.
	table.MoveSelection(2)
	if !table.ProcessKey(aiKey(vtinput.VK_SPACE, ' ')) || table.SelectPos != 3 {
		t.Fatalf("Space not handled or moved the cursor (pos %d)", table.SelectPos)
	}
	text := aiScreenText(t, scr, dlg)
	t.Logf("screen dump with two of four edits switched off:\n%s", text)
	if want := fmt.Sprintf(i18n.Msg("AI.ReviewChecked"), 2, 4); !strings.Contains(text, want) {
		t.Errorf("screen lacks %q", want)
	}
	if strings.Count(text, "[ ]") != 2 || strings.Count(text, "[x]") != 2 {
		t.Errorf("want two unchecked and two checked rows on screen")
	}
	btns := aiReviewButtons(dlg)
	apply := btns[aiCaption("AI.BtnApplyPatch")]
	if apply == nil || !apply.IsDisabled() || apply.IsDefault {
		t.Fatal("Apply should stay on the dialog but be disabled: only the skipped and the failing row are checked")
	}
	if b := btns[aiCaption("AI.ReviewBtnClose")]; b == nil || !b.IsDefault {
		t.Error("Close should be the default button while Apply is disabled")
	}
	apply.OnClick()
	if len(calls) != 0 || dlg.IsDone() {
		t.Fatal("a disabled Apply still ran the patcher")
	}

	// RENAME back on (the cursor is still on it).
	table.ProcessKey(aiKey(vtinput.VK_SPACE, ' '))
	if apply.IsDisabled() || !apply.IsDefault {
		t.Fatal("Apply should be enabled and default again with RENAME checked")
	}
	apply.OnClick()
	want := map[ap.ModKey]bool{
		{FilePath: "vfs/ai_vfs.go", ModIdx: 1}: true,
		{FilePath: "go.mod", ModIdx: 0}:        true,
		{FilePath: "old.txt", ModIdx: -1}:      true,
	}
	if len(calls) != 1 || calls[0].dry || fmt.Sprint(calls[0].only) != fmt.Sprint(want) || !dlg.IsDone() {
		t.Fatalf("Apply called %+v, want one real run with Only = %v", calls, want)
	}

	// Dry run again with the current choice.
	calls = nil
	dlg, table = open()
	table.ProcessKey(aiKey(vtinput.VK_SPACE, ' '))
	aiReviewButtons(dlg)[aiCaption("AI.ReviewBtnDryRun")].OnClick()
	if len(calls) != 1 || !calls[0].dry || len(calls[0].only) != 3 || calls[0].only[ap.ModKey{FilePath: "vfs/ai_vfs.go", ModIdx: 0}] {
		t.Fatalf("Dry run called %+v, want one dry run without the first row", calls)
	}

	// With nothing switched off, both run the whole patch (nil Only).
	calls = nil
	dlg, _ = open()
	aiReviewButtons(dlg)[aiCaption("AI.BtnApplyPatch")].OnClick()
	if len(calls) != 1 || calls[0].only != nil {
		t.Fatalf("Apply with everything checked called %+v, want nil Only", calls)
	}
}

// TestAIShowPatchReviewAfterExclusion: the screen a re-run dry run ends on -
// the rows left out come back as "excluded", unchecked, counted apart from
// the errors.
func TestAIShowPatchReviewAfterExclusion(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	mods := aiReviewTestMods()
	mods[0].Status = ap.ModExcluded
	mods[3].Status = ap.ModExcluded
	dlg := aiShowPatchReview(nil, &vtvibe.Patch{}, t.TempDir(), mods, 2, "")
	text := aiScreenText(t, scr, dlg)
	t.Logf("screen dump after a dry run with two edits excluded:\n%s", text)
	for _, want := range []string{
		i18n.Msg("AI.ReviewExcluded"),
		fmt.Sprintf(i18n.Msg("AI.ReviewTotalsExcluded"), 0, 1, 1, 2),
		fmt.Sprintf(i18n.Msg("AI.ReviewChecked"), 2, 4),
	} {
		if !strings.Contains(text, want) {
			t.Errorf("screen lacks %q", want)
		}
	}
	// Nothing checked would write, but the excluded rows could: Apply is
	// there, disabled until one of them is checked again.
	apply := aiReviewButtons(dlg)[aiCaption("AI.BtnApplyPatch")]
	if apply == nil || !apply.IsDisabled() {
		t.Fatal("Apply should be present and disabled")
	}
	aiReviewTableOf(t, dlg).ProcessKey(aiKey(vtinput.VK_INSERT, 0))
	if apply.IsDisabled() {
		t.Error("checking an excluded row again should enable Apply")
	}
	dlg.Close()
}

// TestAIShowPatchReviewDiff: the diff pane under the table is permanent
// (f4#1606 step e) and always shows the row under the cursor's own edit,
// without Enter (that used to open the same fragment as a modal
// internal/diffview screen, f4#1606 8/N), and Tab moves the keyboard focus
// onto the pane and back.
func TestAIShowPatchReviewDiff(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)

	var calls int
	saved := aiReviewRunPatcher
	t.Cleanup(func() { aiReviewRunPatcher = saved })
	aiReviewRunPatcher = func(*panel.PanelsFrame, *vtvibe.Patch, string, bool, map[ap.ModKey]bool) { calls++ }

	mods := aiReviewTestMods()
	mods[0].Preview = &ap.Preview{
		StartLine: 117,
		Before:    []string{"}", "", "func (d *aiDrive) Open(", "\tctx context.Context, p string", "\treturn 0, ErrNotSupported", "}"},
		After: []string{"}", "", "func (d *aiDrive) Open(", "\tctx context.Context, p string",
			"\te, ok := d.lookup(p)", "\tif !ok {", "\t\treturn 0, ErrNotFound", "\t}", "\treturn d.openEntry(ctx, e)", "}"},
	}
	dlg := aiShowPatchReview(nil, &vtvibe.Patch{ID: "aa000001", Text: "aa000001 AP 3.2\n"}, t.TempDir(), mods, 2, "")
	table := aiReviewTableOf(t, dlg)
	pane := aiReviewPaneOf(t, dlg)

	text := aiScreenText(t, scr, dlg)
	t.Logf("review screen, table focused, diff pane on the first row:\n%s", text)
	if !strings.Contains(text, i18n.Msg("AI.ReviewDiffHint")) {
		t.Errorf("review screen lacks the diff hint %q", i18n.Msg("AI.ReviewDiffHint"))
	}
	if !strings.Contains(text, "vfs/ai_vfs.go:117") {
		t.Errorf("diff pane lacks its title:\n%s", text)
	}

	// The pane already parsed the edit into a unified-style line list: the
	// deletion and the first addition share line 121 (a same-position
	// replacement), the trailing context is renumbered past the four extra
	// lines the edit inserted (docs/VTVIBE.md §7.3's own mockup, 117..126).
	want := []aiReviewDiffLine{
		{' ', 117, "}"}, {' ', 118, ""}, {' ', 119, "func (d *aiDrive) Open("},
		{' ', 120, "\tctx context.Context, p string"},
		{'-', 121, "\treturn 0, ErrNotSupported"}, {'+', 121, "\te, ok := d.lookup(p)"},
		{'+', 122, "\tif !ok {"}, {'+', 123, "\t\treturn 0, ErrNotFound"}, {'+', 124, "\t}"},
		{'+', 125, "\treturn d.openEntry(ctx, e)"}, {' ', 126, "}"},
	}
	if len(pane.lines) != len(want) {
		t.Fatalf("pane.lines = %+v, want %d lines", pane.lines, len(want))
	}
	for i, w := range want {
		if pane.lines[i] != w {
			t.Errorf("pane.lines[%d] = %+v, want %+v", i, pane.lines[i], w)
		}
	}

	// Scrolled to the change itself, both the removed and the added line
	// are on screen (the pane is only 4 rows tall on a 24-row test screen).
	pane.topPos = 4
	text = aiScreenText(t, scr, dlg)
	t.Logf("diff pane scrolled to the change:\n%s", text)
	for _, want := range []string{"return 0, ErrNotSupported", "e, ok := d.lookup(p)"} {
		if !strings.Contains(text, want) {
			t.Errorf("scrolled diff pane lacks %q:\n%s", want, text)
		}
	}

	// With nothing wired to them (onOpen/onViewPatch are what open the file
	// and the patch), Enter and F3 are still swallowed: no modal, no patcher
	// call, and the row underneath does not even need a Preview (the "already applied" row
	// has none). Down is a real key press (not MoveSelection), the same
	// path OnSelect fires from - moving the cursor is what is supposed to
	// refresh the pane here.
	if !table.ProcessKey(aiKey(vtinput.VK_DOWN, 0)) {
		t.Fatal("Down not handled by the review table")
	}
	table.onOpen, table.onViewPatch = nil, nil
	for _, vk := range []uint16{vtinput.VK_RETURN, vtinput.VK_F3} {
		if !table.ProcessKey(aiKey(vk, 0)) {
			t.Fatalf("key %d not handled by the review table", vk)
		}
	}
	if dlg.IsDone() || calls != 0 || !aiReviewIsTop(dlg) {
		t.Fatal("Enter or F3 closed the review, ran the patcher, or opened another screen")
	}

	// Moving the cursor - not Enter - is what updates the pane: on a row
	// without a Preview it falls back to AI.ReviewNoDiff.
	if pane.message != i18n.Msg("AI.ReviewNoDiff") || len(pane.lines) != 0 {
		t.Fatalf("pane after the cursor moved off the diff: message=%q lines=%+v", pane.message, pane.lines)
	}
	if text := aiScreenText(t, scr, dlg); !strings.Contains(text, i18n.Msg("AI.ReviewNoDiff")) {
		t.Errorf("pane does not show %q:\n%s", i18n.Msg("AI.ReviewNoDiff"), text)
	}

	// Tab moves the keyboard focus onto the pane and back; while it
	// has focus, arrow keys scroll it instead of moving the table's cursor.
	if !table.ProcessKey(aiKey(vtinput.VK_UP, 0)) { // back to the row with a diff
		t.Fatal("Up not handled by the review table")
	}
	if pane.message != "" || len(pane.lines) == 0 {
		t.Fatalf("pane after moving back onto the diff row: message=%q lines=%+v", pane.message, pane.lines)
	}
	ctrlTab := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.LeftCtrlPressed}
	tab := aiKey(vtinput.VK_TAB, 0)
	table.ProcessKey(ctrlTab)
	if dlg.GetFocusedItem() != table {
		t.Fatal("Ctrl+Tab must stay with the workspace switcher, not the review's focus switch")
	}
	if !table.ProcessKey(tab) {
		t.Fatal("Tab not handled by the review table")
	}
	if got := dlg.GetFocusedItem(); got != pane {
		t.Fatalf("Tab on the table did not focus the pane (focus = %T)", got)
	}
	t.Logf("review screen, diff pane focused after Tab:\n%s", aiScreenText(t, scr, dlg))
	selBefore := table.SelectPos
	if !pane.ProcessKey(aiKey(vtinput.VK_DOWN, 0)) {
		t.Fatal("Down not handled by the focused pane")
	}
	if pane.topPos == 0 {
		t.Error("Down on the focused pane did not scroll it")
	}
	if table.SelectPos != selBefore {
		t.Error("scrolling the pane moved the table's cursor")
	}
	if !pane.ProcessKey(tab) {
		t.Fatal("Tab not handled by the pane")
	}
	if got := dlg.GetFocusedItem(); got != table {
		t.Fatalf("Tab on the pane did not return focus to the table (focus = %T)", got)
	}
}

// aiReviewTestSession gives the review a session of its own for the draft
// and starts with no rejections on hand.
func aiReviewTestSession(t *testing.T) *vtvibe.Session {
	t.Helper()
	s := vtvibe.NewSession()
	savedSession, savedRejected := aiReviewSession, aiRejected
	aiReviewSession = func() *vtvibe.Session { return s }
	aiRejected.patch, aiRejected.byKey = nil, nil
	t.Cleanup(func() { aiReviewSession, aiRejected = savedSession, savedRejected })
	return s
}

// TestAIReviewReject: a rejected row is left out of Only like a row switched
// off, and its line with the reason is in the draft; rejections pile up,
// a second one edits the reason, taking one back removes its line.
func TestAIReviewReject(t *testing.T) {
	s := aiReviewTestSession(t)
	patch := &vtvibe.Patch{ID: "aa000001"}
	rev := newAIReview(aiReviewTestMods())
	rev.attachRejections(patch)

	rev.reject(2, "no new dependencies in this project")
	if rev.on[2] || !rev.isRejected(2) || rev.rejected() != 1 {
		t.Fatal("rejected row still checked")
	}
	if only := rev.only(); len(only) != 3 || only[ap.ModKey{FilePath: "go.mod", ModIdx: 0}] {
		t.Fatalf("only = %v, want every row but the rejected one", only)
	}
	rev.toggle(2)
	if rev.on[2] {
		t.Fatal("Space checked a rejected row again")
	}
	want := fmt.Sprintf(i18n.Msg("AI.RejectDraftHeader"), "aa000001") + "\n" +
		fmt.Sprintf(i18n.Msg("AI.RejectDraftLine"), 3, "go.mod, REPLACE «require x v1»", "no new dependencies in this project")
	if got := s.Draft(); got != want {
		t.Fatalf("draft = %q, want %q", got, want)
	}

	// A second rejection is added in screen order, the first one stays.
	rev.reject(0, "")
	draft := s.Draft()
	line1 := fmt.Sprintf(i18n.Msg("AI.RejectDraftLine"), 1, "vfs/ai_vfs.go, REPLACE «return 0, ErrNotSupported»", i18n.Msg("AI.RejectNoReason"))
	if i, j := strings.Index(draft, line1), strings.Index(draft, "go.mod, REPLACE"); i < 0 || j < 0 || i > j {
		t.Fatalf("draft lacks the second rejection before the first:\n%s", draft)
	}

	// Rejecting again edits the reason in place.
	rev.reject(2, "use the standard library")
	if draft := s.Draft(); strings.Contains(draft, "no new dependencies") || !strings.Contains(draft, "use the standard library") ||
		strings.Count(draft, "go.mod") != 1 {
		t.Fatalf("editing the reason left the draft as:\n%s", draft)
	}

	// The list survives a new dialog for the same patch (the review is
	// rebuilt after every dry run), and is gone for another patch.
	again := newAIReview(aiReviewTestMods())
	again.attachRejections(patch)
	if !again.isRejected(0) || !again.isRejected(2) || again.on[0] || again.on[2] {
		t.Fatal("rejections lost when the review was rebuilt")
	}
	// Taking both back: the rows are checked again and the draft is empty.
	again.unreject(0)
	again.unreject(2)
	if !again.on[0] || !again.on[2] || again.only() != nil || s.Draft() != "" {
		t.Fatalf("taking the rejections back left on=%v draft=%q", again.on, s.Draft())
	}

	// Sending the draft ends the list: the next review starts clean.
	again.reject(1, "x")
	s.ClearDraft()
	fresh := newAIReview(aiReviewTestMods())
	fresh.attachRejections(patch)
	if fresh.rejected() != 0 {
		t.Fatal("rejections outlived the sent draft")
	}
	other := newAIReview(aiReviewTestMods())
	other.reject(0, "x") // no patch attached: rejecting is not offered
	if other.isRejected(0) || s.Draft() != "" {
		t.Fatal("a review without a patch rejected a row")
	}
}

// TestAIShowPatchReviewRejectKey drives F8 on the screen: the dialog asks
// for the reason, the row turns into "rejected" and Apply leaves it out;
// F8 on it again shows the reason and can take the rejection back.
func TestAIShowPatchReviewRejectKey(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 24)
	vtui.FrameManager.Init(scr)
	s := aiReviewTestSession(t)

	var gotOnly []map[ap.ModKey]bool
	saved := aiReviewRunPatcher
	t.Cleanup(func() { aiReviewRunPatcher = saved })
	aiReviewRunPatcher = func(_ *panel.PanelsFrame, _ *vtvibe.Patch, _ string, _ bool, only map[ap.ModKey]bool) {
		gotOnly = append(gotOnly, only)
	}

	patch := &vtvibe.Patch{ID: "aa000001", Text: "aa000001 AP 3.2\n"}
	dlg := aiShowPatchReview(nil, patch, t.TempDir(), aiReviewTestMods(), 2, "")
	if text := aiScreenText(t, scr, dlg); !strings.Contains(text, "F8") {
		t.Errorf("review screen does not mention F8:\n%s", text)
	}
	table := aiReviewTableOf(t, dlg)
	table.MoveSelection(2) // go.mod REPLACE

	rejectDialog := func() (*vtui.Window, *vtui.Edit) {
		t.Helper()
		if !table.ProcessKey(aiKey(vtinput.VK_F8, 0)) {
			t.Fatal("F8 not handled by the review table")
		}
		w, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		if !ok || w == dlg {
			t.Fatalf("F8: top frame %T, want the reason dialog", vtui.FrameManager.GetTopFrame())
		}
		for _, it := range w.GetChildren() {
			if e, ok := it.(*vtui.Edit); ok {
				return w, e
			}
		}
		t.Fatal("reason dialog has no input line")
		return nil, nil
	}

	w, edit := rejectDialog()
	text := aiScreenText(t, scr, w)
	t.Logf("reason dialog:\n%s", text)
	if want := fmt.Sprintf(i18n.Msg("AI.RejectWhat"), 3, "go.mod, REPLACE «require x v1»"); !strings.Contains(text, want) {
		t.Errorf("reason dialog lacks %q", want)
	}
	if aiReviewButtons(w)[aiCaption("AI.RejectBtnUndo")] != nil {
		t.Error("Take back offered for a row that is not rejected")
	}
	edit.SetText("new dependencies are not welcome here")
	aiReviewButtons(w)[aiCaption("AI.RejectBtn")].OnClick()
	if !w.IsDone() {
		t.Fatal("Reject did not close the reason dialog")
	}

	text = aiScreenText(t, scr, dlg)
	t.Logf("review after F8 on the third edit:\n%s", text)
	for _, want := range []string{
		"[-]", i18n.Msg("AI.ReviewRejected"),
		fmt.Sprintf(i18n.Msg("AI.ReviewCheckedRejected"), 3, 4, 1),
		fmt.Sprintf(i18n.Msg("AI.ReviewRejectedDetail"), "new dependencies are not welcome here"),
	} {
		// Long lines are cut to the dialog's width; their start is enough.
		if r := []rune(want); len(r) > 60 {
			want = string(r[:60])
		}
		if !strings.Contains(text, want) {
			t.Errorf("review screen lacks %q", want)
		}
	}
	draft := s.Draft()
	t.Logf("draft of the next message:\n%s", draft)
	if want := fmt.Sprintf(i18n.Msg("AI.RejectDraftLine"), 3, "go.mod, REPLACE «require x v1»", "new dependencies are not welcome here"); !strings.Contains(draft, want) {
		t.Errorf("draft lacks %q", want)
	}

	// F8 again: the reason is there to edit, and Take back undoes it all.
	w, edit = rejectDialog()
	if edit.GetText() != "new dependencies are not welcome here" {
		t.Errorf("reason dialog shows %q, want the current reason", edit.GetText())
	}
	undo := aiReviewButtons(w)[aiCaption("AI.RejectBtnUndo")]
	if undo == nil {
		t.Fatal("no Take back button for a rejected row")
	}
	undo.OnClick()
	if s.Draft() != "" || strings.Contains(aiScreenText(t, scr, dlg), "[-]") {
		t.Fatal("Take back left the rejection in the draft or on screen")
	}

	// Rejected once more, then Apply: the rejected row is not in Only.
	w, edit = rejectDialog()
	edit.SetText("no")
	aiReviewButtons(w)[aiCaption("AI.RejectBtn")].OnClick()
	aiReviewButtons(dlg)[aiCaption("AI.BtnApplyPatch")].OnClick()
	if len(gotOnly) != 1 || len(gotOnly[0]) != 3 || gotOnly[0][ap.ModKey{FilePath: "go.mod", ModIdx: 0}] {
		t.Fatalf("Apply ran with Only = %v, want the three rows not rejected", gotOnly)
	}
}
