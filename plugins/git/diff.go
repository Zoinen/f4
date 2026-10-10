package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/diffview"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/textdiff"
	"github.com/unxed/vtui"
)

// diffMaxFileSize caps how large a side of the diff this version reads into
// memory. It mirrors internal/app/compare_content_ui.go's own
// compareContentMaxFileSize and the same reasoning (f4#613): this first
// version has no progress dialog or streaming, so 8 MiB -- comfortably
// enough for source files and configs -- is the line between "just read it"
// and "needs a smarter version later".
const diffMaxFileSize = 8 << 20

// headPath is the path to look up at HEAD for entry: the old name for a
// rename or copy (porcelain v2's "2" line kind), the current name otherwise.
func headPath(entry statusEntry) string {
	if entry.OrigPath != "" {
		return entry.OrigPath
	}
	return entry.Path
}

// diffSideFromBytes turns one side's raw content into lines for
// internal/diffview, applying the same two guards
// internal/app/compare_content_ui.go's readTextFileForCompare does for
// "Compare files by content" (f4#613): reject anything over
// diffMaxFileSize, and treat a NUL byte as "probably binary" -- a
// line-by-line diff of binary data is meaningless either way. Trailing
// newline is trimmed the same way, so a file that ends with one does not
// show a spurious trailing empty line.
func diffSideFromBytes(data []byte) ([]string, error) {
	if len(data) > diffMaxFileSize {
		return nil, fmt.Errorf("file is larger than %d MiB", diffMaxFileSize>>20)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, errors.New("file looks like a binary file")
	}
	text := strings.TrimSuffix(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

// headFileContent returns path's content as of HEAD in dir's repository,
// split into diff lines. A failed `git show` -- the file has no HEAD
// version at all, because it is new/untracked, or because the repository
// has no commits yet -- is deliberately not an error: it is reported back
// as (nil, nil), an empty left side, so the whole file shows as inserted,
// the same shape `git diff --no-index /dev/null <file>` would produce.
func headFileContent(ctx context.Context, dir, path string) ([]string, error) {
	// The "./" prefix is required so <path> resolves relative to dir (this
	// invocation's cwd) rather than the repository's top level -- see
	// gitrevisions(7) on the "<rev>:<path>" syntax. entry.Path/OrigPath come
	// from `git status`, which reports them the same way, relative to the
	// directory it ran in.
	out, err := runGitIn(ctx, dir, "show", "HEAD:./"+path)
	if err != nil {
		return nil, nil
	}
	return diffSideFromBytes(out)
}

// worktreeFileContent reads path's current working-tree content under dir,
// split into diff lines. A missing file (deleted in the worktree since
// `git status` ran) is, like a missing HEAD version above, not an error:
// (nil, nil) gives an empty right side, so the whole HEAD content shows as
// deleted.
func worktreeFileContent(dir, path string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(dir, path)) // #nosec G304 -- dir is the active panel's own directory, path is one of its own git status entries.
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return diffSideFromBytes(data)
}

// headTitle is the DiffView pane title for entry's HEAD side.
func headTitle(entry statusEntry) string {
	return "HEAD:" + headPath(entry)
}

// showDiff opens a side-by-side DiffView (internal/diffview, f4#613) for the
// working-tree status entry under the cursor: HEAD's content on the left,
// the current working-tree content on the right. Git status already told
// the user *that* the file changed; this is the natural next, atomic step
// promised in plugin.go's own doc comment -- let them see *how*, reusing the
// exact widget internal/app/compare_content_ui.go already uses for "Compare
// files by content", per the scope split agreed in f4#613.
//
// It shows one combined HEAD-vs-worktree diff rather than separate "staged"
// and "unstaged" halves: splitting index vs. worktree changes is a natural
// follow-up once staging itself (git add/reset) exists, but there is nothing
// to view separately yet when nothing can be staged separately.
//
// Loading both sides can mean a subprocess and a file read, so it runs off
// the UI goroutine the same way actionCompareFilesByContent
// (internal/app/compare_content_ui.go) does, posting the result back with
// vtui.TaskContext.RunOnUI.
func (p *statusPanel) showDiff() {
	entry, ok := p.selectedEntry()
	if !ok {
		return
	}
	dir := p.dir
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		left, leftErr := headFileContent(ctx, dir, headPath(entry))
		right, rightErr := worktreeFileContent(dir, entry.Path)
		ctx.RunOnUI(func() {
			presentDiff(entry, left, right, leftErr, rightErr)
		})
	})
}

// presentDiff runs on the UI goroutine only: it turns showDiff's loaded
// content into either an error dialog or an open DiffView, the same split
// showCompareFilesByContentResult (internal/app/compare_content_ui.go) keeps
// between background loading and UI presentation.
func presentDiff(entry statusEntry, left, right []string, leftErr, rightErr error) {
	if leftErr != nil {
		vtui.ShowMessage(i18n.Msg("Error.Title"), fmt.Sprintf(i18n.Msg("GitDiff.ReadFailed"), headPath(entry), leftErr), []string{i18n.Msg("vtui.Ok")})
		return
	}
	if rightErr != nil {
		vtui.ShowMessage(i18n.Msg("Error.Title"), fmt.Sprintf(i18n.Msg("GitDiff.ReadFailed"), entry.Path, rightErr), []string{i18n.Msg("vtui.Ok")})
		return
	}

	dv, err := diffview.NewDiffView(headTitle(entry), entry.Path, left, right)
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
