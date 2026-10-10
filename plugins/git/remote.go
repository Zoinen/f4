package git

import (
	"fmt"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// The remote commands of the status panel (f4#659 part 20): Shift+F5 fetch,
// Shift+F6 pull, Shift+F7 push -- F-keys of the Shift row, which the file
// panel uses for copy/rename/mkdir-style commands that mean nothing here, so
// the host runs these first (vfs.PanelKeyProvider) and the keybar shows
// their captions. Each runs `git` off the UI goroutine (a remote can be
// slow), then reloads the panel and reports the outcome:
//
//   - fetch: `git fetch`;
//   - pull: `git pull --ff-only`, so it never creates a merge commit or
//     leaves a conflicted tree behind -- diverged history is reported, and
//     merging it is left to the user's own tools;
//   - push: `git push`, after a confirmation, since it changes the remote.
//
// A failure (no upstream, authentication, diverged history) shows git's own
// first line in an error dialog. Prompting for a password is switched off
// (GIT_TERMINAL_PROMPT=0, execGit): a prompt would sit invisibly behind the
// panel; use a credential helper or an SSH agent.

// showStash is Shift+F2: `git stash push` puts the tracked changes away (the
// working tree goes back to HEAD); showStashPop is Shift+F3: `git stash pop`
// brings the newest stash back. A pop that conflicts stays in the stash list
// and git says so in the error dialog (f4#659 part 21).
func (p *statusPanel) showStash() { p.runRemote("stash", "stash", "push") }

func (p *statusPanel) showStashPop() { p.runRemote("stash pop", "stash", "pop") }

func (p *statusPanel) showFetch() { p.runRemote("fetch", "fetch") }

func (p *statusPanel) showPull() { p.runRemote("pull", "pull", "--ff-only") }

// showPush asks first: pushing publishes commits to the remote.
func (p *statusPanel) showPush() {
	if vtui.FrameManager == nil {
		return
	}
	confirm := vtui.ShowMessageEx(i18n.Msg("GitStatus.PushTitle"), i18n.Msg("GitStatus.PushConfirm"),
		[]string{i18n.Msg("GitStatus.PushButton"), i18n.Msg("vtui.Cancel")}, vtui.MessageWarn)
	if confirm == nil {
		return
	}
	confirm.OnResult = func(code int) {
		if code == 0 {
			p.runRemote("push", "push")
		}
	}
}

// runRemote runs one git remote command in the background and reports it:
// a toast with git's first output line on success, an error dialog on failure.
func (p *statusPanel) runRemote(name string, args ...string) {
	dir := p.dir
	toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RemoteRunning"), name), 2e9)
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		output, err := runGitIn(ctx.Context, dir, args...)
		ctx.RunOnUI(func() {
			if err != nil {
				vtui.ShowMessage(i18n.Msg("Error.Title"),
					fmt.Sprintf(i18n.Msg("GitStatus.RemoteFailed"), name, firstLine(string(output), err)),
					[]string{i18n.Msg("vtui.Ok")})
				return
			}
			if rerr := p.reload(); rerr != nil {
				toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), rerr), 3e9)
			} else {
				toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RemoteDone"), name, firstOutputLine(string(output))), 3e9)
			}
			if vtui.FrameManager != nil {
				vtui.FrameManager.Redraw()
			}
		})
	})
}
