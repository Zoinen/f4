package git

import (
	"context"
	"errors"
	"fmt"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Column indices into statusRow.GetCellText, mirroring plugins/proclist's
// colPID/colName/... constants.
const (
	colStatus = iota
	colPath
)

func statusColumns() []vtui.TableColumn {
	return []vtui.TableColumn{
		{Title: i18n.Msg("GitStatus.ColumnStatus"), Width: 4},
		{Title: i18n.Msg("GitStatus.ColumnPath"), MinWidth: 12},
	}
}

// statusRow adapts one statusEntry to vtui.Table's TableRow contract.
type statusRow struct {
	entry statusEntry
}

func (r statusRow) GetCellText(col int) string {
	switch col {
	case colStatus:
		return r.entry.XY
	case colPath:
		if r.entry.OrigPath != "" {
			return fmt.Sprintf("%s -> %s", r.entry.OrigPath, r.entry.Path)
		}
		return r.entry.Path
	default:
		return ""
	}
}

// statusPanel is the panel a PanelProvider.Open returns for f4.gitstatus: a
// theme-colored vtui.Table drawn inside a vtui.BorderedFrame, the same pair
// plugins/proclist/panel.go composes for its own live process list.
//
// Unlike ProcList, this panel does not refresh on a ticker: `git status` is
// a point-in-time snapshot the user asks for (opening the panel, or F5), not
// a continuously changing view like /proc, so there is no background
// goroutine or Close() cleanup to speak of here -- both the initial load and
// F5 run git synchronously, on the UI goroutine, the same "construct
// quickly" trade internal/app/compare_content_ui.go's first version makes
// for its own local, sub-second git/file work.
type statusPanel struct {
	frame *vtui.BorderedFrame
	table *vtui.Table
	dir   string // repository-relative directory git status ran in; also this panel's identity for GetSelectedName's callers.

	branch    string
	detached  bool
	hasCommit bool // the repository has at least one commit (something to amend)

	// expanded holds the untracked directories ("dir/", as parseStatus
	// reports them) shown as the files inside them, so that a file of one
	// can be opened in the hunk view line by line (f4#659 part 27).
	expanded map[string]bool
}

// newStatusPanel is a vfs.PanelProvider.Open callback: dir comes from
// ctx.Current.Path, the active panel's current directory. If dir is not
// inside a git repository (or git is missing from PATH, or dir is not a
// local OS path git can chdir into at all), the underlying `git status`
// call fails and this returns that error; internal/panel/plugins.go's
// OpenRegisteredPanelProvider turns it into a toast for the user, so there
// is nothing extra to do here.
func newStatusPanel(ctx vfs.PanelContext) (vfs.PanelController, error) {
	dir := ctx.Current.Path
	if dir == "" {
		return nil, errors.New(i18n.Msg("GitStatus.NoPath"))
	}

	frame := vtui.NewBorderedFrame(0, 0, 1, 1, vtui.SingleBox, "")
	frame.ColorBoxIdx = theme.ColPanelBox
	frame.ColorTitleIdx = theme.ColPanelTitle
	frame.ColorBackgroundIdx = theme.ColPanelText

	table := vtui.NewTable(0, 0, 1, 1, statusColumns())
	table.Sortable = true
	table.QuickSearch = true
	table.ColorBoxIdx = theme.ColPanelBox
	table.ColorTitleIdx = theme.ColPanelColumnTitle
	table.ColorTextIdx = theme.ColPanelText
	table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	// No custom SortCompare: both columns are plain text (the status code
	// and the path), which is exactly what Table's default cell-text
	// comparator already sorts correctly -- unlike ProcList's numeric
	// PID/Mem/CPU% columns (plugins/proclist/panel.go), nothing here needs
	// its own comparator.
	table.SetSort(colPath, true)

	p := &statusPanel{frame: frame, table: table, dir: dir}
	p.SetFocus(false)
	p.SetPosition(ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3])

	if err := p.reload(); err != nil {
		return nil, err
	}
	return p, nil
}

// reload runs `git status --porcelain=v2 --branch` in p.dir and replaces the
// table's rows. Called once from newStatusPanel and again on every F5.
func (p *statusPanel) reload() error {
	output, err := runGitIn(context.Background(), p.dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return errors.New(firstLine(string(output), err))
	}
	result := parseStatus(output)
	result.Entries = p.expandUntrackedDirs(result.Entries)
	p.branch = result.Branch
	p.detached = result.Detached
	p.hasCommit = result.HasCommit

	rows := make([]vtui.TableRow, len(result.Entries))
	for i, entry := range result.Entries {
		rows[i] = statusRow{entry: entry}
	}
	p.table.SetRows(rows)
	return nil
}

// firstLine turns a failed git invocation into a one-line message: git
// itself explains the failure on stderr (captured into output by runGitIn),
// and prepending err would only add exec's own uninformative "exit status
// 128" ahead of it.
func firstLine(output string, err error) string {
	for i := 0; i < len(output); i++ {
		if output[i] == '\n' {
			output = output[:i]
			break
		}
	}
	if output == "" {
		return err.Error()
	}
	return output
}

func (p *statusPanel) SetPosition(x1, y1, x2, y2 int) {
	p.frame.SetPosition(x1, y1, x2, y2)
	b := p.frame.GetBorderThickness()
	ix1, iy1, ix2, iy2 := x1+b, y1+b, x2-b, y2-b
	if ix2 < ix1 {
		ix2 = ix1
	}
	if iy2 < iy1 {
		iy2 = iy1
	}
	p.table.SetPosition(ix1, iy1, ix2, iy2)
}

func (p *statusPanel) GetPosition() (int, int, int, int) { return p.frame.GetPosition() }

func (p *statusPanel) SetFocus(focused bool) {
	if focused {
		p.table.ColorSelectedTextIdx = theme.ColPanelCursor
		p.table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	} else {
		p.table.ColorSelectedTextIdx = theme.ColPanelInactiveCursor
		p.table.ColorItemSelectCursorIdx = theme.ColPanelInactiveSelectedCursor
	}
	p.table.SetFocus(focused)
}

func (p *statusPanel) IsFocused() bool { return p.table.IsFocused() }

var _ vfs.PanelKeyProvider = (*statusPanel)(nil)

// PanelKeys declares this panel's own keys through the host's shared
// panel-plugin key primitive (vfs.PanelKeyProvider, f4#312): F5 (refresh),
// Enter (diff, diff.go), Insert (stage/unstage, stage.go), Ctrl+K (commit,
// commit.go), Ctrl+E (log, logview.go) and Ctrl+S (branches,
// branchview.go). Declaring them, rather than switching on them in
// ProcessKey, is what makes the host run them ahead of the file panel's own
// bindings for the same keys (F5 Copy, Insert mark, Enter open) and puts
// F5's caption on the keybar; the other keys have no keybar row to show.
//
// F4 (hunks, hunkview.go) stages part of a file, `git add -p` style. It
// takes over the file panel's F4 Edit -- this panel edits no files, and
// "edit what goes into the index" is the nearest reading of F4 here -- and,
// being an F-key, gets a keybar caption, so the feature can be found
// without reading docs. Shift+F4 is its reverse, unstaging part of a file
// (`git reset -p`), with its own caption on the Shift keybar row: a
// separate key rather than F4 guessing the direction from the XY status,
// because an "MM" file has hunks on both sides and either may be wanted.
// Shift+F4 is the file panel's "edit a new file", which has no meaning
// here either.
//
// F8 (hunks again, hunkview.go) throws picked hunks or lines of the
// working file away, `git checkout -p` style, after a confirmation. F8 is
// Delete in a file panel and in this plugin's own branch list
// (branchview.go) -- the destructive key of the Far-style keybar, and this
// is the one destructive thing this panel does to a file; its keybar
// caption says "Discard", not "Delete", because the file itself stays --
// except on an untracked entry, where there is no diff and so nothing to
// discard back to: there (untracked.go) F8 offers to delete the path from
// disk outright, after the same kind of confirmation.
//
// Enter, Insert, F4, Shift+F4 and F8 act on the entry under the cursor, so they are disabled
// while the list is empty: the key is still consumed, exactly as before,
// and nothing runs. Ctrl+K stays enabled with nothing staged on purpose --
// showCommitDialog answers that case with its own "Nothing staged to
// commit" toast, which a silently disabled key would swallow.
//
// None of the first three is a letter key: QuickSearch claims printable
// characters while the table is focused (plugins/proclist/panel.go avoids
// the same trap by keying its own actions off F-keys) -- and that includes
// plain Space, which is why staging is bound to Insert instead of the Space
// lazygit/tig use, following the existing "mark an item" key of
// Far/Norton-Commander-style file panels (internal/panel/menukeys.go's
// isAddItemKey) rather than a foreign tool's convention. F5/Enter are the
// refresh and open gestures a file panel already uses -- this panel has no
// file Copy or directory-enter of its own for either to collide with.
// Commit and log are both bound to Ctrl+<letter> rather than a bare letter
// for the same QuickSearch reason.
//
// Ctrl+E, not the more mnemonic Ctrl+L ("Log") or Ctrl+G ("Git"): this
// panel is itself one of PanelsFrame's AltPanels (internal/panel/plugins.go
// stores the PluginPanelInstance wrapping it exactly there), and Ctrl+L is
// already global Shell-area Panel.InfoPanel, which the frame's own
// ProcessKey (internal/panel/frame.go) deliberately lets fall through past
// *any* focused AltPanel -- info, quick view, or a panel plugin like this
// one -- precisely so Ctrl+L still opens an info panel on the *other* side
// no matter what the active side is showing. Claiming Ctrl+L here for
// something unrelated would break that fallthrough while this panel is
// open. Ctrl+G is likewise already global Files-area File.ApplyCommand.
// Ctrl+E has no such claim anywhere in the application (checked with
// `grep DefaultKeys` across the whole tree, the same check that justified
// Ctrl+K in commit.go) and, unlike Ctrl+I/Ctrl+J, is not a letter whose
// Ctrl form collides with a control character (Tab/Line Feed) some
// terminals may not even deliver distinguishably from the key itself.
//
// Ctrl+S ("Switch branch") for branchview.go's branch list is free by that
// same `grep DefaultKeys` check -- plain Ctrl+S is bound only inside
// internal/media/image_view.go, as that view's own local slide-show toggle,
// never as a global AltPanel-independent action this panel's own claim on
// it could end up shadowing (unlike Ctrl+B, which is the global
// Panel.ToggleKeyBar and, per internal/keymap/remap.go's own note, is also
// the kind of chord a terminal multiplexer like tmux may claim before it
// ever reaches f4 -- one more reason not to reach for it here).
func (p *statusPanel) PanelKeys() []vfs.PanelKey {
	return []vfs.PanelKey{
		{VK: vtinput.VK_F1, Label: i18n.Msg("KeyBar.F1"), Run: p.showHelp},
		{VK: vtinput.VK_F5, Label: i18n.Msg("GitStatus.KeyBar.Refresh"), Run: p.refresh},
		{VK: vtinput.VK_RETURN, Run: p.showDiff, Enabled: p.hasSelectedEntry},
		{VK: vtinput.VK_INSERT, Run: p.toggleStage, Enabled: p.hasSelectedEntry},
		{VK: vtinput.VK_F4, Label: i18n.Msg("GitStatus.KeyBar.Hunks"), Run: p.showHunks, Enabled: p.hasSelectedEntry},
		{VK: vtinput.VK_F4, Mods: vtinput.ShiftPressed, Label: i18n.Msg("GitStatus.KeyBar.UnstageHunks"), Run: p.showStagedHunks, Enabled: p.hasSelectedEntry},
		{VK: vtinput.VK_F8, Label: i18n.Msg("GitStatus.KeyBar.DiscardHunks"), Run: p.showDiscardHunks, Enabled: p.hasSelectedEntry},
		{VK: vtinput.VK_K, Mods: vtinput.LeftCtrlPressed, Run: p.showCommitDialog},
		{VK: vtinput.VK_E, Mods: vtinput.LeftCtrlPressed, Run: p.showLog},
		{VK: vtinput.VK_S, Mods: vtinput.LeftCtrlPressed, Run: p.showBranches},
		{VK: vtinput.VK_F2, Mods: vtinput.ShiftPressed, Label: i18n.Msg("GitStatus.KeyBar.Stash"), Run: p.showStash},
		{VK: vtinput.VK_F3, Mods: vtinput.ShiftPressed, Label: i18n.Msg("GitStatus.KeyBar.StashPop"), Run: p.showStashPop},
		{VK: vtinput.VK_F5, Mods: vtinput.ShiftPressed, Label: i18n.Msg("GitStatus.KeyBar.Fetch"), Run: p.showFetch},
		{VK: vtinput.VK_F6, Mods: vtinput.ShiftPressed, Label: i18n.Msg("GitStatus.KeyBar.Pull"), Run: p.showPull},
		{VK: vtinput.VK_F7, Mods: vtinput.ShiftPressed, Label: i18n.Msg("GitStatus.KeyBar.Push"), Run: p.showPush},
	}
}

// ProcessKey routes the declared PanelKeys first -- the host normally
// dispatches them before the key ever gets here, but a host without the
// PanelKeyProvider hook (and this package's tests) still reaches them --
// and hands everything else to the table's own navigation/sort/quick-search
// handling.
func (p *statusPanel) ProcessKey(e *vtinput.InputEvent) bool {
	if vfs.DispatchPanelKey(p.PanelKeys(), e) {
		return true
	}
	return p.table.ProcessKey(e)
}

// refresh is F5: re-run git status, reporting a failure as a toast and
// keeping the old rows.
func (p *statusPanel) refresh() {
	if err := p.reload(); err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), err), 3e9)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

func (p *statusPanel) ProcessMouse(e *vtinput.InputEvent) bool { return p.table.ProcessMouse(e) }

// hasSelectedEntry is the Enabled predicate of the keys that act on the
// entry under the cursor (Enter, Insert, F4, Shift+F4, F8).
func (p *statusPanel) hasSelectedEntry() bool {
	_, ok := p.selectedEntry()
	return ok
}

func (p *statusPanel) selectedEntry() (statusEntry, bool) {
	idx := p.table.RowAt(p.table.SelectPos)
	if idx < 0 || idx >= len(p.table.Rows) {
		return statusEntry{}, false
	}
	row, ok := p.table.Rows[idx].(statusRow)
	if !ok {
		return statusEntry{}, false
	}
	return row.entry, true
}

// GetSelectedName reports the path under the cursor, the closest thing this
// panel has to a file panel's selected file name.
func (p *statusPanel) GetSelectedName() string {
	entry, ok := p.selectedEntry()
	if !ok {
		return ""
	}
	return entry.Path
}

func (p *statusPanel) SetContext(vfs.PanelContext) {}

// SavePanelState and RestorePanelState let a bookmark return to the entry
// under the cursor (vfs.PanelStateProvider); an entry that has left git status
// since is simply not found and the cursor stays where the panel put it.
func (p *statusPanel) SavePanelState() string { return p.GetSelectedName() }

func (p *statusPanel) RestorePanelState(state string) {
	if state != "" {
		p.restoreSelectionByPath(state)
	}
}

func (p *statusPanel) Show(scr *vtui.ScreenBuf) {
	p.frame.SetTitle(p.title())
	p.frame.Show(scr)
	p.table.Show(scr)
}

// title renders the frame's border title: the branch name (or a fixed
// "detached HEAD" caption) and the current change count, the same shape
// plugins/proclist/panel.go's own Show uses for "ProcList (%d)".
func (p *statusPanel) title() string {
	branch := p.branch
	if p.detached {
		branch = i18n.Msg("GitStatus.Detached")
	}
	return fmt.Sprintf(i18n.Msg("GitStatus.PanelTitle"), branch, p.table.ItemCount)
}

func (p *statusPanel) Close() error { return nil }
