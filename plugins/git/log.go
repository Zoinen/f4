package git

import "strings"

// logEntry is one commit from `git log`, as listed by LogView (logview.go).
type logEntry struct {
	Hash      string // full commit hash; used for git show/diff commands.
	ShortHash string // git's own abbreviated hash, as %h formats it.
	Author    string
	Date      string // --date=short: YYYY-MM-DD.
	Subject   string // the commit message's first line.
}

// logFieldSep separates the fields logview.go's reload asks `git log
// --pretty=format:` for on one line, and logRecordSep would separate commits
// from each other -- except format: (unlike tformat:) already puts exactly
// one newline between commits and none after the last one, so \n itself is
// the record separator and there is nothing to add for it. \x1f (ASCII unit
// separator), not a literal tab or comma: a commit subject can contain
// either of those but is exceedingly unlikely to contain a byte whose whole
// purpose is field separation, the same reasoning status.go's rename lines
// (a literal TAB) rely on for a narrower case.
const logFieldSep = "\x1f"

// logPrettyFormat is the exact --pretty=format: string reload (logview.go)
// passes to `git log`: full hash, abbreviated hash, author name, author date
// and subject, in that order -- parseLog below must agree on both the
// separator and the field order.
const logPrettyFormat = "%H" + logFieldSep + "%h" + logFieldSep + "%an" + logFieldSep + "%ad" + logFieldSep + "%s"

// parseLog parses `git log --pretty=format:logPrettyFormat`'s output: one
// line per commit, five logFieldSep-joined fields. Pure and independent of
// runGitIn, the same testability split parseStatus (status.go) keeps for
// `git status`.
//
// A line with an unexpected field count (a truncated write, or a subject
// that happens to contain logFieldSep -- a known, unhandled edge case, the
// same kind status.go's own doc comment accepts for a literal quote or
// backslash in a path) is skipped rather than guessed at.
func parseLog(output []byte) []logEntry {
	var entries []logEntry
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, logFieldSep, 5)
		if len(fields) != 5 {
			continue
		}
		entries = append(entries, logEntry{
			Hash:      fields[0],
			ShortHash: fields[1],
			Author:    fields[2],
			Date:      fields[3],
			Subject:   fields[4],
		})
	}
	return entries
}

// logDiffEntry is one changed path from `git show --name-status <hash>`
// (logdiff.go): the same shape statusEntry (status.go) gives a working-tree
// change, minus the two-letter XY code -- name-status reports one status
// letter (plus, for a rename or copy, a similarity percentage folded into
// it) per line instead. OrigPath is non-empty only for a rename or copy
// ('R'/'C').
type logDiffEntry struct {
	Status   byte
	Path     string
	OrigPath string
}

// oldPath is the path to look up in the commit's parent revision: the old
// name for a rename or copy, the current name otherwise -- the same
// headPath (diff.go) computes for a working-tree rename against HEAD.
func (e logDiffEntry) oldPath() string {
	if e.OrigPath != "" {
		return e.OrigPath
	}
	return e.Path
}

// parseNameStatus parses `git show --name-status <hash>`'s output
// (git-diff(1)'s --name-status format): one line per changed path, either
// "<status>\t<path>" or, for a rename/copy, "<status><similarity>\t<old>\t<new>".
// Pure and independent of runGitIn for the same reason parseLog above is.
func parseNameStatus(output []byte) []logDiffEntry {
	var entries []logDiffEntry
	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 || fields[0] == "" {
			continue
		}
		status := fields[0][0]
		if (status == 'R' || status == 'C') && len(fields) >= 3 {
			entries = append(entries, logDiffEntry{Status: status, OrigPath: fields[1], Path: fields[2]})
			continue
		}
		entries = append(entries, logDiffEntry{Status: status, Path: fields[1]})
	}
	return entries
}
