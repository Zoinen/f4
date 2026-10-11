// Package git is f4's built-in git client (f4#659): status, diffs, staging,
// log and branches, wrapping the host's own `git` binary via os/exec rather
// than linking a Go git implementation -- the same trade-off
// plugins/multiarc makes for archive tools, in the spirit of f4#609 and per
// the ticket's own text ("go-git только если окажется необходимым").
//
// v1 (f4#659 part 1 of N) is deliberately narrow: a read-only working-tree
// status view, in the same "view first, act later" order plugins/proclist
// took for f4#312. It registers one vfs.PanelProvider ("f4.gitstatus") that
// lists `git status --porcelain=v2`'s changed, unmerged and untracked paths
// for the active panel's current directory, with F5 to refresh.
//
// v2 (f4#659 part 2 of N, diff.go) adds Enter on a listed entry: a
// side-by-side HEAD-vs-worktree diff of that one file, reusing
// internal/diffview/internal/textdiff exactly as internal/app's own
// "Compare files by content" does (f4#613).
//
// v3 (f4#659 part 3 of N, stage.go) adds Insert on a listed entry: stage
// (git add) or unstage (git restore --staged) that whole file, whichever
// applies, then reload the panel.
//
// v4 (f4#659 part 4 of N, commit.go) adds Ctrl+K: a commit message prompt
// over whatever Insert has already staged, then `git commit -m`. Nothing
// staged shows a toast instead of opening the dialog.
//
// v5 (f4#659 part 5 of N, log.go/logview.go/logdiff.go) adds Ctrl+E: a
// read-only list of the repository's recent commits, pushed as its own
// screen (vtui.FrameManager.AddScreen) the same way Enter's diff already is,
// with Enter on a commit reusing the diff widget again for a commit that
// changes exactly one file.
//
// v6 (f4#659 part 7 of N, branch.go/branchview.go) adds Ctrl+S: a read-only
// list of local branches, the current one marked, pushed as its own screen
// exactly the way Ctrl+E's log already is; Enter runs `git switch` on the
// highlighted branch and reloads both that list and the status panel
// underneath.
//
// v7 (f4#659 part 9 of N, commit.go) upgrades Ctrl+K's dialog from a
// one-line prompt to a vtui.MultiLineEdit field, so a commit message can
// have a subject line, a blank line, and a longer body -- the "editor for
// commit messages" the ticket asked for from the start.
//
// Parts 12-14 (hunk.go/hunkview.go) add F4 and Shift+F4: stage or unstage
// part of a file, hunk by hunk or line by line (`git add -p` and
// `git reset -p`). Part 15 adds F8 on the same view: discard picked hunks
// or lines of the working file (`git checkout -p`), after a confirmation.
// See README.md for what each part does and the ticket for what is left.
package git

import (
	"errors"
	"fmt"
	"sync"

	"github.com/unxed/f4/vfs"
)

// panelProviderID is also the ID plughost.RegisterPanelProvider derives its
// auto-generated "Git status" command ID from: "panel." + this ID,
// lowercased (internal/plughost/panel_providers.go). internal/app's own
// menu row and hotkey (git_actions.go) call that derived command by ID,
// duplicated there as a constant for the same reason
// plugins/proclist/proclist_actions.go duplicates plugins/proclist's own ID.
const panelProviderID = "f4.gitstatus"

// Plugin exposes the git working-tree status as an in-process f4 panel
// plugin.
type Plugin struct {
	mu           sync.Mutex
	registration vfs.Registration
	initialized  bool
}

// NewPlugin constructs the built-in git client plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) GetName() string { return "Git" }

// Init registers the git status panel provider. Unlike plugins/proclist,
// this has no platform gate: the plugin itself is always registered,
// whether or not a `git` binary happens to be on PATH -- Available() (used
// by internal/app/git_actions.go's Visible check) covers that instead, the
// same way the plugin/action split works for plugins/sqlite's CLI backend.
func (p *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("Git: nil host API")
	}
	host, ok := api.(vfs.PanelContributionHost)
	if !ok {
		return errors.New("Git: host does not support panel contributions")
	}

	p.mu.Lock()
	if p.initialized {
		p.mu.Unlock()
		return errors.New("Git: plugin is already initialized")
	}
	p.mu.Unlock()

	registration, err := host.RegisterPanelProvider(vfs.PanelProvider{
		ID:          panelProviderID,
		Title:       "Git status",
		Description: "Working tree status (git status) of the active panel's directory",
		Open:        newStatusPanel,
	})
	if err != nil {
		return fmt.Errorf("Git: register panel provider: %w", err)
	}

	p.mu.Lock()
	p.registration = registration
	p.initialized = true
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	registration := p.registration
	p.registration = nil
	p.initialized = false
	p.mu.Unlock()
	if registration != nil {
		registration.Unregister()
	}
	return nil
}
