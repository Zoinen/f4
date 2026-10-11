package git

import (
	"fmt"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Column indices into logDiffFileRow.GetCellText, mirroring statusRow's own
// colStatus/colPath (panel.go): a name-status letter (parseNameStatus's own
// doc comment, log.go) and a path, the exact same two-column shape a
// working-tree status entry already has, so this view reuses
// GitStatus.ColumnStatus/ColumnPath rather than naming its own pair of
// otherwise-identical captions.
const (
	colLogDiffFileStatus = iota
	colLogDiffFilePath
)

func logDiffFileColumns() []vtui.TableColumn {
	return []vtui.TableColumn{
		{Title: i18n.Msg("GitStatus.ColumnStatus"), Width: 4},
		{Title: i18n.Msg("GitStatus.ColumnPath"), MinWidth: 12},
	}
}

// logDiffFileRow adapts one logDiffEntry to vtui.Table's TableRow contract,
// the same role logRow (log.go) and branchRow (branchview.go) play for
// their own entries. The rename/copy "old -> new" rendering mirrors
// statusRow.GetCellText (panel.go) exactly, for the same reason: both come
// from a git command that reports a rename as one line with two paths.
type logDiffFileRow struct{ entry logDiffEntry }

func (r logDiffFileRow) GetCellText(col int) string {
	switch col {
	case colLogDiffFileStatus:
		return string(r.entry.Status)
	case colLogDiffFilePath:
		if r.entry.OrigPath != "" {
			return fmt.Sprintf("%s -> %s", r.entry.OrigPath, r.entry.Path)
		}
		return r.entry.Path
	default:
		return ""
	}
}

// LogDiffFilesView is Enter on a LogView commit that changed more than one
// path (logdiff.go's showDiff): a small read-only list of just that
// commit's changed paths, built from the same vtui.BorderedFrame+vtui.Table
// pair LogView (logview.go) and BranchView (branchview.go) already compose
// for their own lists. It exists purely to answer the one question showDiff
// cannot answer on its own for a multi-file commit -- which single path to
// hand to the two-file DiffView (f4#613) -- not to browse or filter a
// patch; Enter on an entry here opens exactly the same diff showDiff itself
// already gives a single-file commit.
//
// Like LogView and BranchView, this is not a vfs.PanelController occupying
// a panel slot -- it is its own full-screen vtui.Frame pushed with
// vtui.FrameManager.AddScreen on top of the LogView underneath, so Escape
// simply pops this workspace and the log view underneath is exactly as it
// was, needing no "go back to what was open before" stack of its own.
type LogDiffFilesView struct {
	vtui.BaseFrame

	frame *vtui.BorderedFrame
	table *vtui.Table

	dir, hash, shortHash string // the commit this list belongs to; hash/shortHash feed showFileDiff (logdiff.go) exactly as LogView.showDiff's own locals did before this view existed.
}

// newLogDiffFilesView builds a LogDiffFilesView from a commit's own
// already-loaded changed-file list -- unlike newLogView/newBranchView, it
// runs no git command of its own: commitChangedFiles (logdiff.go) already
// ran, off the UI goroutine, as part of showDiff's own vtui.RunAsync
// callback, before this constructor is ever called back on the UI thread.
func newLogDiffFilesView(dir, hash, shortHash string, files []logDiffEntry) *LogDiffFilesView {
	frame := vtui.NewBorderedFrame(0, 0, 1, 1, vtui.SingleBox, "")
	frame.ColorBoxIdx = theme.ColPanelBox
	frame.ColorTitleIdx = theme.ColPanelTitle
	frame.ColorBackgroundIdx = theme.ColPanelText

	table := vtui.NewTable(0, 0, 1, 1, logDiffFileColumns())
	table.QuickSearch = true
	table.ColorBoxIdx = theme.ColPanelBox
	table.ColorTitleIdx = theme.ColPanelColumnTitle
	table.ColorTextIdx = theme.ColPanelText
	table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	// This view is always the focused (indeed the only) widget on its own
	// workspace, the same reasoning newLogView's own comment (logview.go)
	// gives for the same fixed-to-focused cursor colors.
	table.ColorSelectedTextIdx = theme.ColPanelCursor
	table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	table.SetFocus(true)

	rows := make([]vtui.TableRow, len(files))
	for i, f := range files {
		rows[i] = logDiffFileRow{entry: f}
	}
	table.SetRows(rows)

	return &LogDiffFilesView{frame: frame, table: table, dir: dir, hash: hash, shortHash: shortHash}
}

func (v *LogDiffFilesView) selectedEntry() (logDiffEntry, bool) {
	idx := v.table.RowAt(v.table.SelectPos)
	if idx < 0 || idx >= len(v.table.Rows) {
		return logDiffEntry{}, false
	}
	row, ok := v.table.Rows[idx].(logDiffFileRow)
	if !ok {
		return logDiffEntry{}, false
	}
	return row.entry, true
}

// GetType identifies LogDiffFilesView on the frame stack, the next free
// vtui.TypeUser+N slot after BranchView's own +11 (branchview.go's doc
// comment traces the chain back to internal/diffview.DiffView's +9).
func (v *LogDiffFilesView) GetType() vtui.FrameType { return vtui.TypeUser + 12 }

func (v *LogDiffFilesView) GetTitle() string {
	return fmt.Sprintf(i18n.Msg("GitLog.FilesPanelTitle"), v.shortHash, v.table.ItemCount)
}

func (v *LogDiffFilesView) ResizeConsole(w, h int) {
	top := vtui.FrameManager.WorkspaceTopInset()
	v.SetPosition(0, top, w-1, h-2)
}

// SetPosition keeps the embedded BaseFrame's own X1..X2 in sync (so
// GetPosition/HitTest, both inherited from it, stay correct) and repositions
// the frame+table pair inside it, the same split LogView.SetPosition
// (logview.go) and BranchView.SetPosition (branchview.go) keep for the same
// two widgets.
func (v *LogDiffFilesView) SetPosition(x1, y1, x2, y2 int) {
	v.BaseFrame.SetPosition(x1, y1, x2, y2)
	v.frame.SetPosition(x1, y1, x2, y2)
	b := v.frame.GetBorderThickness()
	ix1, iy1, ix2, iy2 := x1+b, y1+b, x2-b, y2-b
	if ix2 < ix1 {
		ix2 = ix1
	}
	if iy2 < iy1 {
		iy2 = iy1
	}
	v.table.SetPosition(ix1, iy1, ix2, iy2)
}

// GetKeyLabels labels only F10 (close): unlike LogView/BranchView, this view
// has no refresh of its own (the changed-file list belongs to one already-
// committed, immutable revision, so nothing about it can go stale the way a
// working-tree status or a branch list can) and no other F-key gesture.
func (v *LogDiffFilesView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{"", "", "", "", "", "", "", "", "", i18n.Msg("GitLog.Close")},
	}
}

// ProcessKey handles Esc/F10 (close, unconditionally -- the same modifier-
// agnostic close LogView.ProcessKey/BranchView.ProcessKey give their own
// Esc/F10) and Enter (diff the selected path, showDiff below), then falls
// through to the table for navigation and QuickSearch, the same layering
// LogView.ProcessKey (logview.go) uses for its own extra keys.
func (v *LogDiffFilesView) ProcessKey(e *vtinput.InputEvent) bool {
	if e == nil || !e.KeyDown {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		v.SetExitCode(-1)
		return true
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift := e.ControlKeyState&vtinput.ShiftPressed != 0
	if !ctrl && !alt && !shift && e.VirtualKeyCode == vtinput.VK_RETURN {
		v.showDiff()
		return true
	}
	return v.table.ProcessKey(e)
}

func (v *LogDiffFilesView) ProcessMouse(e *vtinput.InputEvent) bool { return v.table.ProcessMouse(e) }

func (v *LogDiffFilesView) Show(scr *vtui.ScreenBuf) {
	v.frame.SetTitle(v.GetTitle())
	v.frame.Show(scr)
	v.table.Show(scr)
}

// showDiff is Enter on the changed-file list: the same "load off the UI
// goroutine, present back on it" split LogView.showDiff (logdiff.go) uses
// for a single-file commit, just handing showFileDiff the one path picked
// here instead of the only path a single-file commit ever had.
func (v *LogDiffFilesView) showDiff() {
	entry, ok := v.selectedEntry()
	if !ok {
		return
	}
	dir, hash, shortHash := v.dir, v.hash, v.shortHash
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		showFileDiff(ctx, dir, hash, shortHash, entry)
	})
}

// presentLogDiffFiles runs on the UI goroutine only: it is showDiff's
// (logdiff.go) multi-file branch, opening a LogDiffFilesView on top of the
// log view the same way presentLogDiff opens a DiffView for its own
// single-file case.
func presentLogDiffFiles(dir, hash, shortHash string, files []logDiffEntry) {
	v := newLogDiffFilesView(dir, hash, shortHash, files)
	if vtui.FrameManager != nil {
		v.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
		vtui.FrameManager.AddScreen(v)
	}
}
