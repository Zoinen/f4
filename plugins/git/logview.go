package git

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// logMaxCount bounds how many commits one `git log` call loads: enough for
// a quick "what happened recently" read without paging, and small enough
// that even a repository with a very long history stays snappy to open --
// the same kind of first-cut, revisit-if-it-matters bound diffMaxFileSize
// (diff.go) sets for a diff side's size. A later part can turn this into
// "load more on demand" if a fixed 200 ever turns out to be the wrong
// number.
const logMaxCount = 200

// Column indices into logRow.GetCellText, mirroring statusRow's
// colStatus/colPath (panel.go).
const (
	colHash = iota
	colAuthor
	colDate
	colSubject
)

func logColumns() []vtui.TableColumn {
	return []vtui.TableColumn{
		{Title: i18n.Msg("GitLog.ColumnHash"), Width: 8},
		{Title: i18n.Msg("GitLog.ColumnAuthor"), Width: 16},
		{Title: i18n.Msg("GitLog.ColumnDate"), Width: 10},
		{Title: i18n.Msg("GitLog.ColumnSubject"), MinWidth: 20},
	}
}

// logRow adapts one logEntry to vtui.Table's TableRow contract, the same
// role statusRow (panel.go) plays for a status entry.
type logRow struct{ entry logEntry }

func (r logRow) GetCellText(col int) string {
	switch col {
	case colHash:
		return r.entry.ShortHash
	case colAuthor:
		return r.entry.Author
	case colDate:
		return r.entry.Date
	case colSubject:
		return r.entry.Subject
	default:
		return ""
	}
}

// LogView is Ctrl+E on the status panel (panel.go's PanelKeys): a read-only
// list of the repository's last logMaxCount commits, built from the same
// vtui.BorderedFrame+vtui.Table pair newStatusPanel (panel.go) composes for
// the working-tree status. Unlike the status panel, though, it is not a
// vfs.PanelController occupying a panel slot -- it is a full-screen
// vtui.Frame pushed with vtui.FrameManager.AddScreen, its own workspace, the
// same way internal/diffview.DiffView (f4#613) already opens on Enter from
// this same status panel. That, rather than a second vfs.PanelProvider
// replacing the status panel in its slot, is what "in a separate read-only
// panel" (the ticket's own words) means here: it needs no panel-provider
// "go back to what was open before" stack of its own -- Escape simply pops
// this workspace and the status panel underneath is exactly as it was.
//
// Sorting is deliberately not offered, unlike the status table's Sortable:
// a commit log's only meaningful order is the one `git log` already
// produced (newest first), and letting a click resort it by hash or subject
// would only make that order actively worse. QuickSearch stays on, the same
// type-to-filter gesture the status panel already has, here doubling as a
// quick way to jump to a commit by a word from its author or its subject.
type LogView struct {
	vtui.BaseFrame

	frame *vtui.BorderedFrame
	table *vtui.Table
	dir   string // repository-relative directory `git log` ran in.
}

// newLogView builds and immediately loads a LogView for dir. Like
// newStatusPanel (panel.go), loading runs synchronously on the caller's
// goroutine: a bounded `git log` in a local repository is the same
// "construct quickly" trade panel.go's own doc comment already accepts for
// `git status`.
func newLogView(dir string) (*LogView, error) {
	frame := vtui.NewBorderedFrame(0, 0, 1, 1, vtui.SingleBox, "")
	frame.ColorBoxIdx = theme.ColPanelBox
	frame.ColorTitleIdx = theme.ColPanelTitle
	frame.ColorBackgroundIdx = theme.ColPanelText

	table := vtui.NewTable(0, 0, 1, 1, logColumns())
	table.QuickSearch = true
	table.ColorBoxIdx = theme.ColPanelBox
	table.ColorTitleIdx = theme.ColPanelColumnTitle
	table.ColorTextIdx = theme.ColPanelText
	table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	// This view is always the focused (indeed the only) widget on its own
	// workspace, unlike the status table, which toggles between
	// ColPanelCursor/ColPanelInactiveCursor as the active panel slot
	// changes (panel.go's SetFocus) -- so the cursor colors are set once,
	// to their focused variant, and never need to change again.
	table.ColorSelectedTextIdx = theme.ColPanelCursor
	table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	table.SetFocus(true)

	lv := &LogView{frame: frame, table: table, dir: dir}
	if err := lv.reload(); err != nil {
		return nil, err
	}
	return lv, nil
}

// reload runs `git log` in lv.dir and replaces the table's rows. Called once
// from newLogView and again on every F5. --no-color is defensive, the same
// as commitChangedFiles's own `git show --no-color` (logdiff.go): the
// format string below has no %C() directive of its own, so a user's
// color.ui=always should not be able to inject escape codes into
// parseLog's fields, but it costs nothing to say so explicitly rather than
// rely on that staying true.
func (lv *LogView) reload() error {
	output, err := runGitIn(context.Background(), lv.dir, "log", "--no-color",
		"--max-count="+strconv.Itoa(logMaxCount), "--date=short", "--pretty=format:"+logPrettyFormat)
	if err != nil {
		return errors.New(firstLine(string(output), err))
	}
	entries := parseLog(output)
	rows := make([]vtui.TableRow, len(entries))
	for i, e := range entries {
		rows[i] = logRow{entry: e}
	}
	lv.table.SetRows(rows)
	return nil
}

func (lv *LogView) selectedEntry() (logEntry, bool) {
	idx := lv.table.RowAt(lv.table.SelectPos)
	if idx < 0 || idx >= len(lv.table.Rows) {
		return logEntry{}, false
	}
	row, ok := lv.table.Rows[idx].(logRow)
	if !ok {
		return logEntry{}, false
	}
	return row.entry, true
}

// GetType identifies LogView on the frame stack, the next free
// vtui.TypeUser+N slot after internal/diffview.DiffView's +9 (see that
// package's own doc comment on the convention).
func (lv *LogView) GetType() vtui.FrameType { return vtui.TypeUser + 10 }

func (lv *LogView) GetTitle() string {
	return fmt.Sprintf(i18n.Msg("GitLog.PanelTitle"), lv.table.ItemCount)
}

func (lv *LogView) ResizeConsole(w, h int) {
	top := vtui.FrameManager.WorkspaceTopInset()
	lv.SetPosition(0, top, w-1, h-2)
}

// SetPosition keeps the embedded BaseFrame's own X1..X2 in sync (so
// GetPosition/HitTest, both inherited from it, stay correct) and repositions
// the frame+table pair inside it, the same split statusPanel.SetPosition
// (panel.go) keeps for the same two widgets.
func (lv *LogView) SetPosition(x1, y1, x2, y2 int) {
	lv.BaseFrame.SetPosition(x1, y1, x2, y2)
	lv.frame.SetPosition(x1, y1, x2, y2)
	b := lv.frame.GetBorderThickness()
	ix1, iy1, ix2, iy2 := x1+b, y1+b, x2-b, y2-b
	if ix2 < ix1 {
		ix2 = ix1
	}
	if iy2 < iy1 {
		iy2 = iy1
	}
	lv.table.SetPosition(ix1, iy1, ix2, iy2)
}

func (lv *LogView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{"", "", "", "", i18n.Msg("GitLog.Refresh"), "", "", "", "", i18n.Msg("GitLog.Close")},
	}
}

// ProcessKey handles Esc/F10 (close, unconditionally -- the same modifier-
// agnostic close DiffView.ProcessKey gives its own Esc/F10), F5 (refresh)
// and Enter (diff, logdiff.go), then falls through to the table for
// navigation and QuickSearch, the same layering statusPanel.ProcessKey
// (panel.go) uses for its own extra keys.
func (lv *LogView) ProcessKey(e *vtinput.InputEvent) bool {
	if e == nil || !e.KeyDown {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		lv.SetExitCode(-1)
		return true
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift := e.ControlKeyState&vtinput.ShiftPressed != 0
	if !ctrl && !alt && !shift {
		switch e.VirtualKeyCode {
		case vtinput.VK_F5:
			if err := lv.reload(); err != nil {
				toast.Show(fmt.Sprintf(i18n.Msg("GitLog.RefreshFailed"), err), 3e9)
			}
			if vtui.FrameManager != nil {
				vtui.FrameManager.Redraw()
			}
			return true
		case vtinput.VK_RETURN:
			lv.showDiff()
			return true
		}
	}
	return lv.table.ProcessKey(e)
}

func (lv *LogView) ProcessMouse(e *vtinput.InputEvent) bool { return lv.table.ProcessMouse(e) }

func (lv *LogView) Show(scr *vtui.ScreenBuf) {
	lv.frame.SetTitle(lv.GetTitle())
	lv.frame.Show(scr)
	lv.table.Show(scr)
}

// showLog is Ctrl+E on the status panel (panel.go's PanelKeys doc comment
// explains why Ctrl+E, and not Ctrl+L or a bare letter): opens a LogView of this
// repository's recent commit history on top of the status panel, the same
// "push a screen" gesture showDiff (diff.go) already uses for Enter.
func (p *statusPanel) showLog() {
	lv, err := newLogView(p.dir)
	if err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitLog.OpenFailed"), err), 3e9)
		return
	}
	if vtui.FrameManager != nil {
		lv.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
		vtui.FrameManager.AddScreen(lv)
	}
}
