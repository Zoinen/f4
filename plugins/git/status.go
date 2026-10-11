package git

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
)

// execGit actually runs the git binary with args in the working directory
// dir and returns its combined output. It is the one seam every command in
// this package eventually goes through, a package-level var the same way
// plugins/multiarc/tools.go's runToolIn is: a test can substitute it and
// assert on the exact command this plugin would have run, without ever
// executing a real git binary.
var execGit = func(ctx context.Context, dir string, args []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...) // #nosec G204 -- "git" is our own literal, args are our own literals plus a host path.
	cmd.Dir = dir
	// A prompt for a password (fetch/pull/push over https) would wait on a
	// terminal nobody sees; fail instead and let git say why.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// runGitIn prepends the flags every call needs and hands off to execGit.
// core.quotepath=false is passed on every call so a path containing
// non-ASCII characters comes back as literal UTF-8 instead of git's default
// octal-escaped quoting (git's own quotepath default), which parseStatus
// does not attempt to reverse. A path containing a literal double quote or
// backslash is a known, deliberately unhandled edge case for this first
// version -- the same kind of narrow first cut internal/app/compare_content_ui.go
// documents for f4#613.
func runGitIn(ctx context.Context, dir string, args ...string) ([]byte, error) {
	full := append([]string{"-c", "core.quotepath=false"}, args...)
	return execGit(ctx, dir, full)
}

// lookupGit resolves "git" on PATH. A package-level var for the same
// testability reason as plugins/multiarc's lookupTool: a test can pretend
// git is or is not installed without touching the real PATH.
var lookupGit = exec.LookPath

// Available reports whether a git binary is on PATH. f4's own Git status
// action (internal/app) gates its menu entry and hotkey on this, the same
// way plugins/proclist.Supported() gates ProcList's -- except this is a
// runtime PATH lookup rather than a build-time platform check, since git is
// an external tool this plugin never bundles (f4#659, in the spirit of
// f4#609: wrap the host's own CLI tools rather than link a library).
func Available() bool {
	_, err := lookupGit("git")
	return err == nil
}

// statusEntry is one changed, unmerged or untracked path from `git status
// --porcelain=v2`. XY is the same two-letter index/worktree status pair
// `git status --short` prints (for example "M ", " M", "A ", "??", "R ");
// this plugin displays it as-is instead of inventing its own vocabulary, so
// a user who already knows `git status -s` recognizes it immediately.
// OrigPath is non-empty only for a rename or copy (porcelain v2's "2" line
// kind), and Path is then the new name.
type statusEntry struct {
	XY       string
	Path     string
	OrigPath string
}

// statusResult is the parsed result of one `git status --porcelain=v2
// --branch` run.
type statusResult struct {
	// Branch is the current branch name, empty when Detached is true.
	Branch   string
	Detached bool
	// HasCommit is false in a repository with no commit yet (branch.oid is
	// "(initial)"), and also when the output carries no branch.oid line.
	HasCommit bool
	Entries   []statusEntry
}

// parseStatus parses `git status --porcelain=v2 --branch` output. It is
// deliberately independent of runGitIn so it can be exercised with fixed
// sample output, matching internal/textdiff's own split between the
// algorithm and the thing that calls it (f4#613).
//
// Ahead/behind and upstream tracking (the "# branch.ab"/"# branch.upstream"
// header lines) are read by git but not kept here: nothing in this first
// version's panel displays them yet, and keeping unused fields untested is
// the wrong trade for a first cut. A later part of f4#659 that wants them
// only has to add the two lines back.
func parseStatus(output []byte) statusResult {
	var res statusResult
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimPrefix(line, "# branch.head ")
			if head == "(detached)" {
				res.Detached = true
			} else {
				res.Branch = head
			}
		case strings.HasPrefix(line, "# branch.oid "):
			oid := strings.TrimPrefix(line, "# branch.oid ")
			res.HasCommit = oid != "" && oid != "(initial)"
		case strings.HasPrefix(line, "# "):
			// Other header lines ( branch.upstream, branch.ab):
			// not needed by this version, see the doc comment above.
		case strings.HasPrefix(line, "1 "):
			if fields := splitNFields(line, 8); fields != nil {
				res.Entries = append(res.Entries, statusEntry{XY: fields[1], Path: fields[8]})
			}
		case strings.HasPrefix(line, "2 "):
			if fields := splitNFields(line, 9); fields != nil {
				if idx := strings.IndexByte(fields[9], '\t'); idx >= 0 {
					res.Entries = append(res.Entries, statusEntry{
						XY:       fields[1],
						Path:     fields[9][:idx],
						OrigPath: fields[9][idx+1:],
					})
				}
			}
		case strings.HasPrefix(line, "u "):
			if fields := splitNFields(line, 10); fields != nil {
				res.Entries = append(res.Entries, statusEntry{XY: fields[1], Path: fields[10]})
			}
		case strings.HasPrefix(line, "? "):
			res.Entries = append(res.Entries, statusEntry{XY: "??", Path: strings.TrimPrefix(line, "? ")})
		}
		// A "! " (ignored file) line, or any other unrecognized line, falls
		// through unmatched here: ignored files are not requested (no
		// --ignored flag), so this should never actually appear, and it is
		// skipped defensively -- with no case to match, the loop simply
		// moves on -- rather than mis-parsed as a change.
	}
	return res
}

// splitNFields splits line into exactly n+1 parts on the first n ASCII
// spaces, the last part being the unsplit remainder of the line (which may
// itself contain spaces or, for a rename/copy line, a TAB separating the two
// paths). It returns nil if line has fewer than n spaces -- a malformed or
// unrecognized line, which callers skip rather than panic on.
func splitNFields(line string, n int) []string {
	fields := make([]string, 0, n+1)
	rest := line
	for i := 0; i < n; i++ {
		idx := strings.IndexByte(rest, ' ')
		if idx < 0 {
			return nil
		}
		fields = append(fields, rest[:idx])
		rest = rest[idx+1:]
	}
	fields = append(fields, rest)
	return fields
}
