package app

import (
	"context"
	"errors"
	"fmt"
	"github.com/unxed/f4/internal/panel"
	"strconv"
	"strings"
	"time"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// The Advanced Compare dialog and everything it needs to reach the panels.
// The comparison itself is in compare_folders.go.

// compareDialogMinWidth keeps the dialog from shrinking around short
// captions into something that reads as a stray message box.
const compareDialogMinWidth = 54

// compareDialogMaxWidth is the widest we let a translation push the dialog.
// Beyond this a caption is too long for the dialog to be the right shape,
// and an 80 column terminal has to be considered too.
const compareDialogMaxWidth = 100

// compareProgressInterval is how often the progress dialog is refreshed.
// A local comparison walks thousands of names a second; repainting for
// each of them costs more than the comparison itself.
const compareProgressInterval = 50 * time.Millisecond

// panelCanCompareFolders reports whether there are two file panels to
// compare. Anything else on the other side (a terminal, the info panel)
// has no folder of its own, so the menu entry stays out of sight rather
// than being offered and refused.
func panelCanCompareFolders() bool {
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	return pf.GetActivePanel() != nil && pf.GetInactivePanel() != nil
}

// compareCaptionWidth is how many columns a checkbox with this caption
// paints, indent included: the "[x] " prefix is four columns wide.
func compareCaptionWidth(indent int, caption string) int {
	return indent + 4 + vtui.StringWidth(action.PlainLabel(caption))
}

// compareRadioWidth is the same measurement for a radio group, whose
// widest row decides the width: four columns of prefix and two of padding.
func compareRadioWidth(indent int, items []string) int {
	widest := 0
	for _, item := range items {
		if w := vtui.StringWidth(action.PlainLabel(item)); w > widest {
			widest = w
		}
	}
	return indent + 6 + widest
}

// ShowCompareFoldersDialog asks what to compare and how, then runs the
// comparison over the two panels.
func ShowCompareFoldersDialog(pf *panel.PanelsFrame) {
	if pf == nil {
		return
	}
	opts := config.App.Compare.Normalize()

	const (
		subIndent   = 4
		deepIndent  = 8
		depthWidth  = 5
		groupInset  = 4 // group border and clearance on both sides
		dialogInset = 6 // dialog border and margin on both sides
	)

	capRecursive := i18n.Msg("Compare.Recursive")
	capDepth := i18n.Msg("Compare.MaxDepth")
	capMarked := i18n.Msg("Compare.MarkedOnly")
	capTime := i18n.Msg("Compare.ByTime")
	capSlack := i18n.Msg("Compare.TimeSlack")
	capZones := i18n.Msg("Compare.IgnoreZones")
	capSize := i18n.Msg("Compare.BySize")
	capContent := i18n.Msg("Compare.ByContent")
	capIgnore := i18n.Msg("Compare.Ignore")
	capReport := i18n.Msg("Compare.ReportEqual")
	ignoreItems := []string{i18n.Msg("Compare.IgnoreEOL"), i18n.Msg("Compare.IgnoreSpaces")}

	inGroup := 0
	for _, w := range []int{
		compareCaptionWidth(0, capRecursive),
		compareCaptionWidth(subIndent, capDepth) + 1 + depthWidth,
		compareCaptionWidth(0, capMarked),
		compareCaptionWidth(0, capTime),
		compareCaptionWidth(subIndent, capSlack),
		compareCaptionWidth(subIndent, capZones),
		compareCaptionWidth(0, capSize),
		compareCaptionWidth(0, capContent),
		compareCaptionWidth(subIndent, capIgnore),
		compareRadioWidth(deepIndent, ignoreItems),
	} {
		if w > inGroup {
			inGroup = w
		}
	}

	width := inGroup + groupInset + dialogInset
	if w := compareCaptionWidth(0, capReport) + dialogInset; w > width {
		width = w
	}
	if width < compareDialogMinWidth {
		width = compareDialogMinWidth
	}
	if width > compareDialogMaxWidth {
		width = compareDialogMaxWidth
	}
	// Three rows of "process", eight of "compare", both framed, plus the
	// message checkbox, a blank row and the buttons.
	const processRows, compareRows = 3, 8
	height := (processRows + 2) + (compareRows + 2) + 1 + 1 + 1 + 4

	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("Compare.Title"))
	dlg.ShowClose = true

	// NewGroupBox takes corners, not a size: the bottom row of a box
	// holding n rows is n+1 below its top one.
	gbProcess := vtui.NewGroupBox(0, 0, width-dialogInset, processRows+1, i18n.Msg("Compare.Process"))
	gbCompare := vtui.NewGroupBox(0, 0, width-dialogInset, compareRows+1, i18n.Msg("Compare.Criteria"))
	cbReport := vtui.NewCheckbox(0, 0, capReport, false)
	cbReport.State = compareCheckState(opts.ReportEqual)
	btnOk := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))

	for _, item := range []vtui.UIElement{gbProcess, gbCompare, cbReport, btnOk, btnCancel} {
		dlg.AddItem(item)
	}

	mainVBox := vtui.NewVBoxLayout(dlg.X1+3, dlg.Y1+2, width-dialogInset, height-4)
	mainVBox.Add(gbProcess, vtui.Margins{}, vtui.AlignFill)
	mainVBox.Add(gbCompare, vtui.Margins{}, vtui.AlignFill)
	mainVBox.Add(cbReport, vtui.Margins{}, vtui.AlignLeft)
	btnRow := vtui.NewHBoxLayout(0, 0, width-dialogInset, 1)
	btnRow.HorizontalAlign = vtui.AlignCenter
	btnRow.Spacing = 2
	btnRow.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	btnRow.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	mainVBox.Add(btnRow, vtui.Margins{Top: 1}, vtui.AlignFill)
	// First pass: the group boxes now know where they are, which is what
	// their contents have to be positioned against.
	mainVBox.Apply()

	// "Process" group.
	cbRecursive := vtui.NewCheckbox(0, 0, capRecursive, false)
	cbRecursive.State = compareCheckState(opts.Recursive)
	cbDepth := vtui.NewCheckbox(0, 0, capDepth, false)
	cbDepth.State = compareCheckState(opts.LimitDepth)
	edDepth := vtui.NewEdit(0, 0, depthWidth, strconv.Itoa(opts.MaxDepth))
	edDepth.Validator = &vtui.IntRangeValidator{Min: 1, Max: config.CompareMaxDepthLimit, Title: i18n.Msg("Compare.Title")}
	cbMarked := vtui.NewCheckbox(0, 0, capMarked, false)
	cbMarked.State = compareCheckState(opts.MarkedOnly)

	processBox := vtui.NewVBoxLayout(gbProcess.X1+2, gbProcess.Y1+1, gbProcess.X2-gbProcess.X1-3, processRows)
	processBox.Add(cbRecursive, vtui.Margins{}, vtui.AlignLeft)
	depthRow := vtui.NewHBoxLayout(0, 0, gbProcess.X2-gbProcess.X1-3, 1)
	depthRow.Add(cbDepth, vtui.Margins{Left: subIndent}, vtui.AlignTop)
	depthRow.Add(edDepth, vtui.Margins{}, vtui.AlignTop)
	processBox.Add(depthRow, vtui.Margins{}, vtui.AlignFill)
	processBox.Add(cbMarked, vtui.Margins{}, vtui.AlignLeft)
	for _, item := range []vtui.UIElement{cbRecursive, cbDepth, edDepth, cbMarked} {
		gbProcess.AddItem(item)
	}
	processBox.Apply()
	gbProcess.SetFocus(false)

	// "Compare" group.
	cbTime := vtui.NewCheckbox(0, 0, capTime, false)
	cbTime.State = compareCheckState(opts.ByTime)
	cbSlack := vtui.NewCheckbox(0, 0, capSlack, false)
	cbSlack.State = compareCheckState(opts.TimeSlack)
	cbZones := vtui.NewCheckbox(0, 0, capZones, false)
	cbZones.State = compareCheckState(opts.IgnoreZones)
	cbSize := vtui.NewCheckbox(0, 0, capSize, false)
	cbSize.State = compareCheckState(opts.BySize)
	cbContent := vtui.NewCheckbox(0, 0, capContent, false)
	cbContent.State = compareCheckState(opts.ByContent)
	cbIgnore := vtui.NewCheckbox(0, 0, capIgnore, false)
	cbIgnore.State = compareCheckState(opts.Ignore)
	rgIgnore := vtui.NewRadioGroup(0, 0, 1, ignoreItems)
	if opts.IgnoreMode == config.CompareIgnoreSpaces {
		rgIgnore.Selected = 1
	}

	compareBox := vtui.NewVBoxLayout(gbCompare.X1+2, gbCompare.Y1+1, gbCompare.X2-gbCompare.X1-3, compareRows)
	compareBox.Add(cbTime, vtui.Margins{}, vtui.AlignLeft)
	compareBox.Add(cbSlack, vtui.Margins{Left: subIndent}, vtui.AlignLeft)
	compareBox.Add(cbZones, vtui.Margins{Left: subIndent}, vtui.AlignLeft)
	compareBox.Add(cbSize, vtui.Margins{}, vtui.AlignLeft)
	compareBox.Add(cbContent, vtui.Margins{}, vtui.AlignLeft)
	compareBox.Add(cbIgnore, vtui.Margins{Left: subIndent}, vtui.AlignLeft)
	compareBox.Add(rgIgnore, vtui.Margins{Left: deepIndent}, vtui.AlignLeft)
	for _, item := range []vtui.UIElement{cbTime, cbSlack, cbZones, cbSize, cbContent, cbIgnore, rgIgnore} {
		gbCompare.AddItem(item)
	}
	compareBox.Apply()
	gbCompare.SetFocus(false)

	// A sub-option of something switched off is not editable, the way Far
	// greys out the rows below an unchecked box.
	syncEnabled := func() {
		recursive := cbRecursive.State == 1
		cbDepth.SetDisabled(!recursive)
		edDepth.SetDisabled(!recursive || cbDepth.State != 1)
		cbSlack.SetDisabled(cbTime.State != 1)
		cbZones.SetDisabled(cbTime.State != 1)
		cbIgnore.SetDisabled(cbContent.State != 1)
		rgIgnore.SetDisabled(cbContent.State != 1 || cbIgnore.State != 1)
		if vtui.FrameManager != nil {
			vtui.FrameManager.Redraw()
		}
	}
	for _, cb := range []*vtui.Checkbox{cbRecursive, cbDepth, cbTime, cbContent, cbIgnore} {
		cb.OnChange = func(int) { syncEnabled() }
	}
	syncEnabled()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		next := config.CompareOptions{
			Recursive:   cbRecursive.State == 1,
			LimitDepth:  cbDepth.State == 1,
			MaxDepth:    opts.MaxDepth,
			MarkedOnly:  cbMarked.State == 1,
			ByTime:      cbTime.State == 1,
			TimeSlack:   cbSlack.State == 1,
			IgnoreZones: cbZones.State == 1,
			BySize:      cbSize.State == 1,
			ByContent:   cbContent.State == 1,
			Ignore:      cbIgnore.State == 1,
			IgnoreMode:  rgIgnore.Selected,
			ReportEqual: cbReport.State == 1,
		}
		if depth, err := strconv.Atoi(strings.TrimSpace(edDepth.GetText())); err == nil {
			next.MaxDepth = depth
		}
		if !next.HasCriteria() {
			// Comparing by name alone would call two folders equal
			// whenever they hold the same names, which is not an answer
			// anybody asked for.
			vtui.ShowMessage(i18n.Msg("Compare.Title"), i18n.Msg("Compare.NoCriteria"), []string{"&Ok"})
			return
		}
		next = next.Normalize()
		config.App.Compare = next
		if config.App.AutoSaveDialogSettings {
			config.SaveConfig()
		}
		dlg.Close()
		runCompareFolders(pf, next)
	}

	vtui.FrameManager.Push(dlg)
}

func compareCheckState(on bool) int {
	if on {
		return 1
	}
	return 0
}

// comparePanelSnapshot is what a panel looked like when the comparison
// started. The scan runs off the UI thread and may take a while, so the
// marks are only applied if the panel is still showing the same listing.
type comparePanelSnapshot struct {
	pnl   *panel.FileSystemPanel
	fs    vfs.VFS
	root  string
	allow map[string]bool
	epoch uint64
}

func captureComparePanel(fsp *panel.FileSystemPanel, opts config.CompareOptions) (comparePanelSnapshot, bool) {
	if fsp == nil || fsp.Vfs == nil {
		return comparePanelSnapshot{}, false
	}
	snap := comparePanelSnapshot{
		pnl:   fsp,
		fs:    fsp.Vfs,
		root:  fsp.Vfs.GetPath(),
		epoch: fsp.DirectoryEpoch,
	}
	if opts.MarkedOnly {
		marked := fsp.GetMarkedNames()
		if len(marked) == 0 {
			// Far compares everything when nothing is marked, rather
			// than comparing nothing at all.
			return snap, true
		}
		snap.allow = make(map[string]bool, len(marked))
		for _, name := range marked {
			snap.allow[name] = true
		}
	}
	return snap, true
}

// stillCurrent reports whether the panel is showing what it was showing
// when the comparison started.
func (s comparePanelSnapshot) stillCurrent() bool {
	fsp := s.pnl
	return fsp != nil && fsp.Vfs != nil && fileops.SameVFSInstance(fsp.Vfs, s.fs) &&
		fsp.Vfs.GetPath() == s.root && fsp.DirectoryEpoch == s.epoch
}

// applyCompareMarks replaces the panel's selection with the comparison
// result. The previous selection is kept as the restorable one, so Ctrl+M
// undoes a comparison the same way it undoes any other mass selection.
func (s comparePanelSnapshot) applyCompareMarks(marks map[string]bool) {
	fsp := s.pnl
	if fsp == nil {
		return
	}
	fsp.SaveSelection()
	fsp.SetAllItemsSelected(false)
	for name := range marks {
		fsp.SetSelectedByName(name, true)
	}
}

// runCompareFolders compares the two panels and marks what differs.
func runCompareFolders(pf *panel.PanelsFrame, opts config.CompareOptions) {
	if pf == nil {
		return
	}
	active, passive := pf.GetActivePanel(), pf.GetInactivePanel()
	if active == nil || passive == nil {
		vtui.ShowMessage(i18n.Msg("Compare.Title"), i18n.Msg("Compare.NoPanels"), []string{"&Ok"})
		return
	}
	leftSnap, okLeft := captureComparePanel(active, opts)
	rightSnap, okRight := captureComparePanel(passive, opts)
	if !okLeft || !okRight {
		vtui.ShowMessage(i18n.Msg("Compare.Title"), i18n.Msg("Compare.NoPanels"), []string{"&Ok"})
		return
	}

	opDlg := fileops.NewFileOpProgressDialog(i18n.Msg("Compare.Progress"))
	var taskCtx *vtui.TaskContext
	opDlg.SetOnCancel(func() {
		if taskCtx != nil {
			taskCtx.Cancel()
		}
		opDlg.Close()
	})
	vtui.FrameManager.PostTask(func() {
		vtui.FrameManager.AddScreenHeadless(opDlg)
	})

	taskCtx = vtui.RunAsync(func(ctx *vtui.TaskContext) {
		lastUpdate := time.Now()
		show := func(action, path string, done, total int) {
			now := time.Now()
			if now.Sub(lastUpdate) < compareProgressInterval {
				return
			}
			lastUpdate = now
			ctx.RunOnUI(func() {
				opDlg.UpdateCounting(action, path, int64(done), int64(total))
				vtui.FrameManager.Redraw()
			})
		}

		scanning := i18n.Msg("Compare.Scanning")
		comparing := i18n.Msg("Compare.Comparing")
		leftItems, err := fileops.CollectCompareSide(ctx.Context, leftSnap.fs, leftSnap.root, leftSnap.allow, opts,
			func(path string) { show(scanning, path, 0, 0) })
		var rightItems map[string]fileops.CompareItem
		if err == nil {
			rightItems, err = fileops.CollectCompareSide(ctx.Context, rightSnap.fs, rightSnap.root, rightSnap.allow, opts,
				func(path string) { show(scanning, path, 0, 0) })
		}
		var outcome *fileops.CompareOutcome
		if err == nil {
			outcome, err = fileops.CompareSides(ctx.Context, leftSnap.fs, rightSnap.fs, leftItems, rightItems, opts,
				func(path string, done, total int) { show(comparing, path, done, total) })
		}

		ctx.RunOnUI(func() {
			opDlg.Close()
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				vtui.ShowMessage(i18n.Msg("Compare.Title"), fmt.Sprintf(i18n.Msg("Compare.Failed"), err.Error()), []string{"&Ok"})
				return
			}
			if !leftSnap.stillCurrent() || !rightSnap.stillCurrent() {
				// Both panels moved on while the tree was being read;
				// marking them now would mark the wrong listing.
				vtui.ShowMessage(i18n.Msg("Compare.Title"), i18n.Msg("Compare.Moved"), []string{"&Ok"})
				return
			}
			leftSnap.applyCompareMarks(outcome.Left)
			rightSnap.applyCompareMarks(outcome.Right)
			vtui.FrameManager.Redraw()

			if outcome.ReadErr != nil {
				vtui.ShowMessage(i18n.Msg("Compare.Title"),
					fmt.Sprintf(i18n.Msg("Compare.ReadFailed"), outcome.ReadErr.Error()), []string{"&Ok"})
				return
			}
			if outcome.Differing == 0 && opts.ReportEqual {
				vtui.ShowMessage(i18n.Msg("Compare.Title"), i18n.Msg("Compare.Equal"), []string{"&Ok"})
			}
		})
	})
}
