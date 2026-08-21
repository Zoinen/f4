package gitplugin

import (
	"context"
	"path"
	"sort"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v6"
)

// statusClass is deliberately independent from go-git's individual status
// letters. It is the compact state f4 needs for a one-symbol decoration and
// for grouping the status virtual panel.
type statusClass uint8

const (
	statusClean statusClass = iota
	statusStaged
	statusUnstaged
	statusBoth
	statusUntracked
	statusConflict
	statusIgnored
)

type statusEntry struct {
	Path     string
	Index    gogit.StatusCode
	Worktree gogit.StatusCode
	Class    statusClass
}

type repositoryStatus struct {
	root        string
	entries     map[string]statusEntry // slash-separated repository-relative path
	directories map[string]statusClass // aggregate state by repository-relative directory
	updatedAt   time.Time
}

func fileStatusClass(index, worktree gogit.StatusCode) statusClass {
	if index == gogit.UpdatedButUnmerged || worktree == gogit.UpdatedButUnmerged {
		return statusConflict
	}
	if index == gogit.Ignored && worktree == gogit.Ignored {
		return statusIgnored
	}
	if index == gogit.Untracked || worktree == gogit.Untracked {
		return statusUntracked
	}
	staged := index != gogit.Unmodified
	unstaged := worktree != gogit.Unmodified
	switch {
	case staged && unstaged:
		return statusBoth
	case staged:
		return statusStaged
	case unstaged:
		return statusUnstaged
	default:
		return statusClean
	}
}

// mergeStatusClass conservatively preserves every visible state in a subtree.
// There is only one decoration cell, so an unresolved conflict wins, followed
// by both layers, then the remaining individual layer states.
func mergeStatusClass(left, right statusClass) statusClass {
	if left == statusClean {
		return right
	}
	if right == statusClean || left == right {
		return left
	}
	if left == statusConflict || right == statusConflict {
		return statusConflict
	}
	if left == statusBoth || right == statusBoth {
		return statusBoth
	}
	if (left == statusStaged && right == statusUnstaged) || (left == statusUnstaged && right == statusStaged) {
		return statusBoth
	}
	// An untracked or ignored descendant combined with a tracked change still
	// needs a signal that work exists in both layers from the user's point of
	// view. Prefer untracked over ignored when neither staged/unstaged applies.
	if left == statusUntracked || right == statusUntracked {
		return statusUntracked
	}
	if left == statusIgnored || right == statusIgnored {
		return statusIgnored
	}
	return left
}

func buildRepositoryStatus(root string, status gogit.Status) *repositoryStatus {
	result := &repositoryStatus{
		root:        root,
		entries:     make(map[string]statusEntry, len(status)),
		directories: make(map[string]statusClass),
		updatedAt:   time.Now(),
	}
	for filename, fileStatus := range status {
		if fileStatus == nil {
			continue
		}
		filename = path.Clean(strings.ReplaceAll(filename, "\\", "/"))
		if filename == "." || strings.HasPrefix(filename, "../") {
			continue
		}
		entry := statusEntry{
			Path:     filename,
			Index:    fileStatus.Staging,
			Worktree: fileStatus.Worktree,
			Class:    fileStatusClass(fileStatus.Staging, fileStatus.Worktree),
		}
		if entry.Class == statusClean {
			continue
		}
		result.entries[filename] = entry
		for directory := path.Dir(filename); directory != "." && directory != "/"; directory = path.Dir(directory) {
			result.directories[directory] = mergeStatusClass(result.directories[directory], entry.Class)
		}
	}
	return result
}

// orderedEntries returns a stable snapshot for a virtual panel. It keeps
// staging and worktree grouping out of the cache, which makes one status scan
// useful for panel rows, attributes, and folder aggregation alike.
func (snapshot *repositoryStatus) orderedEntries() []statusEntry {
	if snapshot == nil {
		return nil
	}
	entries := make([]statusEntry, 0, len(snapshot.entries))
	for _, entry := range snapshot.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Path < entries[right].Path })
	return entries
}

func readRepositoryStatus(ctx context.Context, repository Repository) (*repositoryStatus, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	repo, err := gogit.PlainOpen(repository.Root)
	if err != nil {
		return nil, err
	}
	defer repo.Close()
	worktree, err := repo.Worktree()
	if err != nil {
		return nil, err
	}
	status, err := worktree.StatusContext(ctx, gogit.StatusOptions{
		IncludeIgnored:      true,
		RecursiveSubmodules: true,
	})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return buildRepositoryStatus(repository.Root, status), nil
}
