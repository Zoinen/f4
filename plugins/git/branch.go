package git

import "strings"

// branchEntry is one local branch from `git branch --list --no-color`, as
// listed by BranchView (branchview.go).
type branchEntry struct {
	Name    string
	Current bool
}

// parseBranchList parses `git branch --list --no-color`'s output
// (git-branch(1)): one line per ref, a two-character marker ("* " for the
// checked-out branch, "+ " for a branch checked out in some other linked
// worktree, "  " otherwise) followed by the branch name. Pure and
// independent of runGitIn, the same testability split parseStatus
// (status.go) and parseLog (log.go) keep for their own git output.
//
// A "+ " entry is kept, not filtered out: attempting to switch to a branch
// already checked out in another worktree is exactly the kind of failure
// BranchView's own switchBranch is supposed to surface as git's own toast
// error (branchview.go), not silently hide from the list.
//
// A line whose name starts with "(" -- git's own placeholder for a detached
// HEAD or an in-progress rebase, e.g. "(HEAD detached at abcd123)" or
// "(no branch, rebasing main)" -- is skipped: it names no actual local
// branch a checkout/switch could target, and the status panel's own title
// (panel.go's title) already reports a detached HEAD by other means.
func parseBranchList(output []byte) []branchEntry {
	var entries []branchEntry
	for _, line := range strings.Split(string(output), "\n") {
		if len(line) < 3 {
			continue
		}
		name := line[2:]
		if name == "" || strings.HasPrefix(name, "(") {
			continue
		}
		entries = append(entries, branchEntry{Name: name, Current: line[0] == '*'})
	}
	return entries
}
