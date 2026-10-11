package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// confirmDeleteUntracked is F8 (showDiscardHunks, hunkview.go) on an
// untracked entry ("??", status.go's parseStatus): `git diff` shows
// nothing for a path with no committed or indexed version to compare it
// against, so there are no hunks for HunkView to open and F8's usual
// discard has nothing to do. The nearest reading of "discard" left for a
// path git does not track at all is removing it from disk outright, so
// this asks first -- the same confirm-before-doing-it shape hunkview.go's
// own discard uses, down to the MessageWarn kind and the "Cancel" second
// button -- and only calls deleteUntracked once the destructive button is
// picked.
//
// vtui.ShowMessageEx rather than hunkview.go's anchored ShowMessageOnEx:
// statusPanel is a vfs.PanelController living inside a host panel frame,
// not itself a vtui.Frame the dialog could be anchored on -- the same
// unanchored shape commit.go's own showCommitMessageEditor uses for a
// dialog opened straight from this panel's keys.
func (p *statusPanel) confirmDeleteUntracked(entry statusEntry) {
	if vtui.FrameManager == nil {
		return
	}
	confirm := vtui.ShowMessageEx(i18n.Msg("GitStatus.DeleteUntrackedTitle"),
		fmt.Sprintf(i18n.Msg("GitStatus.DeleteUntrackedConfirm"), entry.Path),
		[]string{i18n.Msg("GitStatus.DeleteUntrackedButton"), i18n.Msg("vtui.Cancel")}, vtui.MessageWarn)
	if confirm == nil {
		return
	}
	confirm.OnResult = func(code int) {
		if code == 0 {
			p.deleteUntracked(entry)
		}
	}
}

// deleteUntracked removes entry.Path from disk, relative to p.dir the same
// way every git invocation in this file runs there, and reloads the panel
// -- toggleStage's (stage.go) own shape for "run the command, then refresh
// and put the cursor back where it was, toasting only a failure".
//
// entry.Path may name a directory: an untracked directory is one
// first-level "?? <dir>/" entry (parseStatus's own doc comment, status.go)
// rather than one entry per file inside it, so os.Remove alone would
// leave a non-empty directory behind; the trailing "/" git always prints
// for that case is what tells the two apart here.
func (p *statusPanel) deleteUntracked(entry statusEntry) {
	full := filepath.Join(p.dir, entry.Path)
	var err error
	if strings.HasSuffix(entry.Path, "/") {
		err = os.RemoveAll(full)
	} else {
		err = os.Remove(full)
	}
	if err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.DeleteUntrackedFailed"), entry.Path, err), 3e9)
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
