package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
)

// untrackedFiles lists the untracked, not ignored files under dir (a path as
// the status panel shows it: relative to the panel's directory, "sub/" for a
// directory), in the order git reports them.
func untrackedFiles(ctx context.Context, panelDir, dir string) ([]string, error) {
	out, err := runGitIn(ctx, panelDir, "ls-files", "--others", "--exclude-standard", "-z", "--", dir)
	if err != nil {
		return nil, fmt.Errorf("%s", firstLine(string(out), err))
	}
	var files []string
	for _, name := range strings.Split(string(out), "\x00") {
		if name != "" {
			files = append(files, name)
		}
	}
	return files, nil
}

// expandUntrackedDirs replaces every entry for an untracked directory the
// user expanded by one "??" entry per file inside it. A directory whose files
// cannot be listed stays a single entry.
func (p *statusPanel) expandUntrackedDirs(entries []statusEntry) []statusEntry {
	if len(p.expanded) == 0 {
		return entries
	}
	out := make([]statusEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.XY != "??" || !p.expanded[entry.Path] {
			out = append(out, entry)
			continue
		}
		files, err := untrackedFiles(context.Background(), p.dir, entry.Path)
		if err != nil || len(files) == 0 {
			out = append(out, entry)
			continue
		}
		for _, f := range files {
			out = append(out, statusEntry{XY: "??", Path: f})
		}
	}
	return out
}

// expandUntrackedDir is F4 on an untracked directory: the row turns into the
// files inside it, with the cursor on the first, and F4 on one of them opens
// the line picker of f4#659 part 24. Insert on the directory still stages
// all of it at once.
func (p *statusPanel) expandUntrackedDir(entry statusEntry) {
	files, err := untrackedFiles(context.Background(), p.dir, entry.Path)
	if err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitHunks.OpenFailed"), err), 3e9)
		return
	}
	if len(files) == 0 {
		toast.Show(fmt.Sprintf(i18n.Msg("GitHunks.NoHunks"), entry.Path), 3e9)
		return
	}
	if p.expanded == nil {
		p.expanded = make(map[string]bool)
	}
	p.expanded[entry.Path] = true
	if err := p.reload(); err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), err), 3e9)
		return
	}
	p.restoreSelectionByPath(files[0])
}
