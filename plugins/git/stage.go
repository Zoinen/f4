package git

import (
	"context"
	"fmt"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// unmergedXY is the fixed set of two-letter status codes `git status
// --short`/`--porcelain=v2` uses for a merge conflict (documented in
// git-status(1)'s "Unmerged" table): both sides deleted, added, or modified,
// in every combination. parseStatus's "u " branch (status.go) is the only
// producer of these codes -- see its own doc comment.
var unmergedXY = map[string]bool{
	"DD": true, "AU": true, "UD": true,
	"UA": true, "DU": true, "AA": true, "UU": true,
}

// stageAction is what pressing the stage/unstage key on an entry should run.
type stageAction int

const (
	stageActionAdd stageAction = iota
	stageActionRestore
)

// stageActionFor decides whether entry's toggle key should stage (git add)
// or unstage (git restore --staged) it.
//
// A conflicted entry (unmergedXY above) always resolves to "add": the only
// sensible thing to do with a merge conflict from this panel is mark it
// resolved, and there is nothing meaningful to "unstage" from one -- "add"
// either way, never a toggle.
//
// Otherwise this reads the index (left) column of entry.XY, the same column
// `git status --short` uses to mean "already staged": ' ' (nothing staged
// for this path yet) or '?' (untracked, nothing in the index at all) means
// the toggle key should stage; any other letter (M, A, D, R, C, ...) means
// something is already staged, so the toggle key should unstage it.
//
// parseStatus (status.go) actually reads `git status --porcelain=v2` output,
// not the `--short` (v1) format the paragraph above describes: v2's "1 "/"2 "
// (ordinary/rename) lines spell that same "nothing staged" state as '.'
// rather than a space (git-status(1)'s "Porcelain Format Version 2": XY uses
// "." for an unmodified side). So both placeholders are accepted here --
// '.' for what parseStatus actually produces, ' ' for the `--short` spelling
// this function's own tests (and its doc comment) are written against.
func stageActionFor(entry statusEntry) stageAction {
	if unmergedXY[entry.XY] || len(entry.XY) != 2 {
		return stageActionAdd
	}
	switch entry.XY[0] {
	case ' ', '.', '?':
		return stageActionAdd
	default:
		return stageActionRestore
	}
}

// stagePathsFor returns the path(s) entry's stage/unstage command should act
// on. A plain entry is just its own path. A rename or copy (OrigPath
// non-empty) passes both the old and new names: git stores a rename as two
// separate index changes (a delete of the old path, an add of the new one)
// that `git status` only *displays* combined as one "R" line by similarity
// detection, so a command that names just the new path would touch only
// half of it -- leaving, for example, an "unstage" halfway done, with the
// old path's staged deletion still in the index. Passing both paths keeps
// stage and unstage symmetric and complete for a rename in one keypress.
func stagePathsFor(entry statusEntry) []string {
	if entry.OrigPath != "" {
		return []string{entry.OrigPath, entry.Path}
	}
	return []string{entry.Path}
}

// toggleStage runs the stage/unstage command for the entry under the cursor
// (Insert, panel.go's PanelKeys) and reloads the panel, keeping the cursor
// on the same path so repeatedly toggling one file, or stepping down the
// list and toggling several, does not keep jumping the cursor back to the
// top of a re-sorted table.
func (p *statusPanel) toggleStage() {
	entry, ok := p.selectedEntry()
	if !ok {
		return
	}

	args := append([]string{"add", "--"}, stagePathsFor(entry)...)
	failMsg := i18n.Msg("GitStatus.StageFailed")
	if stageActionFor(entry) == stageActionRestore {
		args = append([]string{"restore", "--staged", "--"}, stagePathsFor(entry)...)
		failMsg = i18n.Msg("GitStatus.UnstageFailed")
	}

	if output, err := runGitIn(context.Background(), p.dir, args...); err != nil {
		toast.Show(fmt.Sprintf(failMsg, firstLine(string(output), err)), 3e9)
		return
	}

	selected := entry.Path
	if err := p.reload(); err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), err), 3e9)
	} else {
		p.restoreSelectionByPath(selected)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

// restoreSelectionByPath re-points the cursor at path after reload has
// replaced the table's rows and re-sorted them. Table.SetRows on its own
// only clamps SelectPos to stay within the new row count (vtui's own doc
// comment on SetRows) -- it has no notion of "the same row as before", so
// without this the cursor would silently snap back to whatever display
// position 0 (or the old numeric position) now holds, which after toggling
// one file's stage state is very rarely still that same file.
//
// path not found -- the entry dropped out of git status entirely, which can
// happen when unstaging fully reverts a file to match HEAD -- leaves
// whatever position SetRows already clamped SelectPos to.
func (p *statusPanel) restoreSelectionByPath(path string) {
	for displayPos := 0; displayPos < p.table.ItemCount; displayPos++ {
		idx := p.table.RowAt(displayPos)
		if idx < 0 || idx >= len(p.table.Rows) {
			continue
		}
		row, ok := p.table.Rows[idx].(statusRow)
		if ok && row.entry.Path == path {
			p.table.SelectPos = displayPos
			p.table.EnsureVisible()
			return
		}
	}
}
