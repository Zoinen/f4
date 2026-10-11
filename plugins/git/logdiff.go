package git

import (
	"context"
	"errors"
	"fmt"

	"github.com/unxed/f4/internal/diffview"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/textdiff"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// commitChangedFiles lists the paths <hash> touches, via `git show
// --name-status` (git-diff(1)'s --name-status format, parsed by
// parseNameStatus in log.go). A merge commit -- or, for that matter, one
// that happens to revert itself to a no-op -- comes back with zero entries
// rather than an error: `git show` with no -m/-c flag prints no diff at all
// for a commit with more than one parent (git-show(1)), and showDiff below
// already treats "not exactly one changed file" as "nothing to show"
// rather than mistaking either shape for a failure.
func commitChangedFiles(ctx context.Context, dir, hash string) ([]logDiffEntry, error) {
	output, err := runGitIn(ctx, dir, "show", "--no-color", "--pretty=format:", "--name-status", hash)
	if err != nil {
		return nil, errors.New(firstLine(string(output), err))
	}
	return parseNameStatus(output), nil
}

// revisionFileContent reads path's content as of rev (for example "<hash>"
// or "<hash>^") in dir's repository, split into diff lines -- the same
// diffSideFromBytes-wrapped `git show <rev>:./<path>` headFileContent
// (diff.go) runs for "HEAD", generalized to any revision so it can also read
// a commit's parent. A failed lookup (the path did not exist at rev -- a
// root commit's missing parent, or the path added/deleted by this very
// commit) is, like headFileContent's own HEAD miss, deliberately not an
// error: (nil, nil) gives an empty side, so the whole file shows as inserted
// or deleted, the same shape headFileContent/worktreeFileContent give the
// status panel's own Enter.
func revisionFileContent(ctx context.Context, dir, rev, path string) ([]string, error) {
	out, err := runGitIn(ctx, dir, "show", rev+":./"+path)
	if err != nil {
		return nil, nil
	}
	return diffSideFromBytes(out)
}

// showDiff is Enter on the log view (logview.go's ProcessKey): a
// side-by-side diff of the commit under the cursor, reusing
// internal/diffview exactly as the status panel's own showDiff (diff.go)
// does for a working-tree change -- just against the commit's parent and
// the commit itself (<hash>^ and <hash>) instead of HEAD and the worktree.
//
// diffview.DiffView takes two whole files, not a multi-file patch, so a
// commit touching more than one path cannot go straight to a diff the way a
// single-file commit does: showFileDiff below needs telling *which* one.
// Part 11 (f4#659) added exactly that missing step -- a changed-files
// sub-list (logdifffiles.go) the user picks one path from -- rather than a
// different, patch-shaped widget entirely, the same "reuse the two-file
// DiffView, just tell it which two files" shape f4#613 already settled for
// every other diff this plugin shows. Zero changed files (a merge commit,
// or a no-op one -- commitChangedFiles's own doc comment) still has no path
// to offer a choice between, so that case alone keeps the toast.
//
// Loading the changed-file list can mean a subprocess of its own, so this
// runs off the UI goroutine the same way the status panel's own showDiff
// (diff.go) does, posting the result back with vtui.TaskContext.RunOnUI.
func (lv *LogView) showDiff() {
	entry, ok := lv.selectedEntry()
	if !ok {
		return
	}
	hash, shortHash, dir := entry.Hash, entry.ShortHash, lv.dir
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		files, err := commitChangedFiles(ctx, dir, hash)
		if err != nil {
			ctx.RunOnUI(func() {
				toast.Show(fmt.Sprintf(i18n.Msg("GitLog.DiffFailed"), err), 3e9)
			})
			return
		}
		switch len(files) {
		case 0:
			ctx.RunOnUI(func() {
				toast.Show(i18n.Msg("GitLog.DiffNoChanges"), 3e9)
			})
		case 1:
			showFileDiff(ctx, dir, hash, shortHash, files[0])
		default:
			ctx.RunOnUI(func() {
				presentLogDiffFiles(dir, hash, shortHash, files)
			})
		}
	})
}

// showFileDiff loads one changed path's before/after content and presents
// it -- the shared tail end of both showDiff's own single-file case above
// and LogDiffFilesView's own Enter (logdifffiles.go) once the user has
// picked a path out of a multi-file commit. Always called already off the
// UI goroutine, inside a vtui.RunAsync callback, the same contract
// showDiff's own inline version of this code had before part 11 split it
// out to be shared.
func showFileDiff(ctx *vtui.TaskContext, dir, hash, shortHash string, f logDiffEntry) {
	left, leftErr := revisionFileContent(ctx, dir, hash+"^", f.oldPath())
	right, rightErr := revisionFileContent(ctx, dir, hash, f.Path)
	ctx.RunOnUI(func() {
		presentLogDiff(shortHash, f, left, right, leftErr, rightErr)
	})
}

// presentLogDiff runs on the UI goroutine only: it turns showFileDiff's
// loaded content into either an error dialog, a toast, or an open DiffView,
// the same background/UI split presentDiff (diff.go) keeps for the status
// panel's own Enter.
func presentLogDiff(shortHash string, f logDiffEntry, left, right []string, leftErr, rightErr error) {
	if leftErr != nil {
		vtui.ShowMessage(i18n.Msg("Error.Title"), fmt.Sprintf(i18n.Msg("GitDiff.ReadFailed"), f.oldPath(), leftErr), []string{i18n.Msg("vtui.Ok")})
		return
	}
	if rightErr != nil {
		vtui.ShowMessage(i18n.Msg("Error.Title"), fmt.Sprintf(i18n.Msg("GitDiff.ReadFailed"), f.Path, rightErr), []string{i18n.Msg("vtui.Ok")})
		return
	}

	dv, err := diffview.NewDiffView(shortHash+"^:"+f.oldPath(), shortHash+":"+f.Path, left, right)
	if err != nil {
		if err == textdiff.ErrTooLarge {
			vtui.ShowMessage(i18n.Msg("Error.Title"), i18n.Msg("GitDiff.TooLarge"), []string{i18n.Msg("vtui.Ok")})
			return
		}
		vtui.ShowMessage(i18n.Msg("Error.Title"), fmt.Sprintf(i18n.Msg("GitDiff.CompareFailed"), err), []string{i18n.Msg("vtui.Ok")})
		return
	}

	if vtui.FrameManager != nil {
		dv.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
		vtui.FrameManager.AddScreen(dv)
	}
}
