package gitplugin

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	gogit "github.com/go-git/go-git/v6"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const gitStatusCommandID = "git.status"

func (plugin *Plugin) registerIntegration(api vfs.HostAPI) error {
	registrations := make([]vfs.Registration, 0, 4)
	rollback := func(err error) error {
		for index := len(registrations) - 1; index >= 0; index-- {
			registrations[index].Unregister()
		}
		return err
	}
	if host, ok := api.(vfs.ContributionHost); ok {
		registration, err := host.RegisterPluginCommand(vfs.PluginCommand{
			ID:          gitStatusCommandID,
			Location:    vfs.PluginCommandPanel,
			Label:       "Git: Status",
			Description: "Open staged and unstaged changes for the current Git repository",
			Shortcut:    "Git",
			Run:         plugin.openStatus,
		})
		if err != nil {
			return rollback(fmt.Errorf("Git: register status command: %w", err))
		}
		registrations = append(registrations, registration)
	}
	if host, ok := api.(vfs.PromptSegmentHost); ok {
		registration, err := host.RegisterPromptSegmentProvider(plugin)
		if err != nil {
			return rollback(fmt.Errorf("Git: register prompt provider: %w", err))
		}
		registrations = append(registrations, registration)
	}
	if host, ok := api.(vfs.FileDecorationHost); ok {
		registration, err := host.RegisterFileDecorationProvider(plugin)
		if err != nil {
			return rollback(fmt.Errorf("Git: register decoration provider: %w", err))
		}
		registrations = append(registrations, registration)
	}
	if host, ok := api.(vfs.PanelNavigationHost); ok {
		registration, err := host.RegisterPanelNavigationProvider(plugin)
		if err != nil {
			return rollback(fmt.Errorf("Git: register navigation observer: %w", err))
		}
		registrations = append(registrations, registration)
	}

	plugin.mu.Lock()
	if plugin.initialized {
		plugin.registrations = append(plugin.registrations, registrations...)
		plugin.mu.Unlock()
		return nil
	}
	plugin.mu.Unlock()
	return rollback(fmt.Errorf("Git: plugin closed during initialization"))
}

func localPanelDirectory(snapshot vfs.PanelSnapshot) (string, bool) {
	filesystem, ok := snapshot.VFS.(*vfs.OSVFS)
	if !ok || filesystem == nil || snapshot.Path == "" {
		return "", false
	}
	directory, err := filesystem.Abs(snapshot.Path)
	if err != nil {
		return "", false
	}
	return filepath.Clean(directory), true
}

func observationID(host vfs.PanelHost, side vfs.PanelSide) string {
	return fmt.Sprintf("git-panel:%p:%d", host, side)
}

// PanelNavigated starts a fresh, cancellable discovery probe for the entered
// local directory. The immediate branch/decorations path remains cache-only:
// Observe itself returns before doing any filesystem work.
func (plugin *Plugin) PanelNavigated(host vfs.PanelHost, snapshot vfs.PanelSnapshot) {
	directory, ok := localPanelDirectory(snapshot)
	if !ok || host == nil {
		return
	}

	plugin.mu.RLock()
	discovery := plugin.discovery
	initialized := plugin.initialized
	plugin.mu.RUnlock()
	if !initialized || discovery == nil {
		return
	}

	// Use a completed session answer right away, then refresh it in the
	// background so a deleted/moved .git marker cannot remain stale on a later
	// visit to this same folder.
	if cached := discovery.Lookup(directory); cached.Found() {
		plugin.followRepository(host, snapshot, cached)
		plugin.scheduleStatusRefresh(cached.Repository, func() {
			host.RefreshVFS(snapshot.VFS)
			plugin.refreshStatusViews()
		})
	}

	_, _ = discovery.Observe(context.Background(), observationID(host, snapshot.Side), directory, func(update DiscoveryUpdate) {
		if update.Err != nil {
			return
		}
		vtui.FrameManager.PostTask(func() {
			// Use the current snapshot rather than the old worker's value. A
			// newer Observe for this side supersedes this callback, but a panel
			// swap can still change its active/passive orientation meanwhile.
			current := host.PanelSnapshot(snapshot.Side)
			if current.VFS == nil {
				return
			}
			if update.Result.Found() {
				plugin.followRepository(host, current, update.Result)
				if plugin.cachedStatus(update.Result.Repository.Root) == nil {
					plugin.scheduleStatusRefresh(update.Result.Repository, func() {
						host.RefreshVFS(current.VFS)
						plugin.refreshStatusViews()
					})
				} else {
					// A delayed branch-only discovery update needs a redraw for the
					// prompt, but must not rescan an index we just completed.
					host.RefreshVFS(current.VFS)
				}
			} else {
				plugin.clearFollowedRepository(host, current)
			}
		})
	})
}

// PromptSegment only reads the completed discovery map. This method is called
// while the command line is rendered and must never open a repository or stat
// .git.
func (plugin *Plugin) PromptSegment(filesystem vfs.VFS, directory string) (vfs.PromptSegment, bool) {
	osvfs, ok := filesystem.(*vfs.OSVFS)
	if !ok || osvfs == nil || directory == "" {
		return vfs.PromptSegment{}, false
	}
	absDirectory, err := osvfs.Abs(directory)
	if err != nil {
		return vfs.PromptSegment{}, false
	}
	plugin.mu.RLock()
	discovery := plugin.discovery
	plugin.mu.RUnlock()
	if discovery == nil {
		return vfs.PromptSegment{}, false
	}
	branch, ok := discovery.CachedBranch(filepath.Clean(absDirectory))
	if !ok || branch.Name == "" {
		return vfs.PromptSegment{}, false
	}
	color := vfs.PromptSegmentGitBranch
	text := "[" + branch.Prompt() + "]"
	if branch.Detached {
		color = vfs.PromptSegmentGitDetached
	} else if branch.Unborn {
		color = vfs.PromptSegmentGitUnborn
		text = "[" + branch.Prompt() + " (unborn)]"
	}
	return vfs.PromptSegment{Text: text, Color: color}, true
}

// DecorateFile provides the cached extended attributes requested by the normal
// file panel. It intentionally returns no value until discovery and one status
// scan have completed; a completion posts a targeted panel refresh.
func (plugin *Plugin) DecorateFile(filesystem vfs.VFS, directory string, item vfs.VFSItem) (vfs.FileDecoration, bool) {
	osvfs, ok := filesystem.(*vfs.OSVFS)
	if !ok || osvfs == nil || item.Name == "" || item.Name == ".." {
		return vfs.FileDecoration{}, false
	}
	absDirectory, err := osvfs.Abs(directory)
	if err != nil {
		return vfs.FileDecoration{}, false
	}
	plugin.mu.RLock()
	discovery := plugin.discovery
	plugin.mu.RUnlock()
	if discovery == nil {
		return vfs.FileDecoration{}, false
	}
	result := discovery.Lookup(filepath.Clean(absDirectory))
	if !result.Found() {
		return vfs.FileDecoration{}, false
	}
	itemPath := filepath.Join(absDirectory, item.Name)
	relative, err := filepath.Rel(result.Repository.Root, itemPath)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return vfs.FileDecoration{}, false
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	snapshot := plugin.cachedStatus(result.Repository.Root)
	if snapshot == nil {
		return vfs.FileDecoration{}, false
	}
	if item.IsDir {
		return decorationForStatus(snapshot.directories[relative], gogit.Unmodified, gogit.Unmodified, true)
	}
	entry, ok := snapshot.entries[relative]
	if !ok {
		return vfs.FileDecoration{}, false
	}
	return decorationForStatus(entry.Class, entry.Index, entry.Worktree, false)
}

func decorationForStatus(class statusClass, index, worktree gogit.StatusCode, aggregate bool) (vfs.FileDecoration, bool) {
	if class == statusClean {
		return vfs.FileDecoration{}, false
	}
	attributes := map[string]string{
		"git.status": string([]byte{byte(class)}),
	}
	if aggregate {
		attributes["git.aggregate"] = "true"
	} else {
		attributes["git.indexStatus"] = string(index)
		attributes["git.worktreeStatus"] = string(worktree)
	}
	switch class {
	case statusStaged:
		return vfs.FileDecoration{Prefix: "+", Color: vfs.FileDecorationGitStaged, Attributes: attributes}, true
	case statusUnstaged:
		return vfs.FileDecoration{Prefix: "~", Color: vfs.FileDecorationGitUnstaged, Attributes: attributes}, true
	case statusBoth:
		return vfs.FileDecoration{Prefix: "±", Color: vfs.FileDecorationGitBoth, Attributes: attributes}, true
	case statusUntracked:
		return vfs.FileDecoration{Prefix: "?", Color: vfs.FileDecorationGitUntracked, Attributes: attributes}, true
	case statusConflict:
		return vfs.FileDecoration{Prefix: "!", Color: vfs.FileDecorationGitConflict, Attributes: attributes}, true
	case statusIgnored:
		attributes["git.ignored"] = "true"
		return vfs.FileDecoration{Prefix: "·", Color: vfs.FileDecorationGitIgnored, Attributes: attributes}, true
	default:
		return vfs.FileDecoration{}, false
	}
}

func (plugin *Plugin) cachedStatus(root string) *repositoryStatus {
	plugin.mu.RLock()
	status := plugin.statuses[root]
	plugin.mu.RUnlock()
	return status
}

func (plugin *Plugin) refreshStatus(ctx context.Context, repository Repository) error {
	status, err := readRepositoryStatus(ctx, repository)
	if err != nil {
		return err
	}
	plugin.mu.Lock()
	if plugin.initialized && plugin.statuses != nil {
		plugin.statuses[repository.Root] = status
	}
	plugin.mu.Unlock()
	return nil
}

// scheduleStatusRefresh deduplicates scans per repository. A status result is
// consumed by every panel inside the same worktree, so duplicating an index
// scan for each panel would defeat the navigation debounce.
func (plugin *Plugin) scheduleStatusRefresh(repository Repository, complete func()) {
	plugin.mu.Lock()
	if !plugin.initialized || plugin.statusTasks == nil {
		plugin.mu.Unlock()
		return
	}
	if _, busy := plugin.statusTasks[repository.Root]; busy {
		plugin.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	plugin.statusTasks[repository.Root] = cancel
	plugin.mu.Unlock()

	go func() {
		_ = plugin.refreshStatus(ctx, repository)
		plugin.mu.Lock()
		if current, ok := plugin.statusTasks[repository.Root]; ok {
			// There is at most one scan per root. Removing the entry lets a
			// later filesystem event schedule the next refresh.
			_ = current
			delete(plugin.statusTasks, repository.Root)
		}
		alive := plugin.initialized
		plugin.mu.Unlock()
		if alive && ctx.Err() == nil && complete != nil {
			vtui.FrameManager.PostTask(complete)
		}
	}()
}

func (plugin *Plugin) openStatus(app vfs.App) {
	host, ok := app.(vfs.PanelHost)
	if !ok || host == nil {
		app.Message(" Git ", "This host cannot open a Git virtual panel.", []string{"&Ok"})
		return
	}
	snapshot := host.PanelSnapshot(vfs.PanelActive)
	directory, local := localPanelDirectory(snapshot)
	if !local {
		app.Message(" Git ", "Git status is available only for a local filesystem panel.", []string{"&Ok"})
		return
	}
	plugin.mu.RLock()
	discovery := plugin.discovery
	plugin.mu.RUnlock()
	if discovery == nil {
		return
	}
	result := discovery.Lookup(directory)
	if !result.Found() {
		plugin.PanelNavigated(host, snapshot)
		app.Message(" Git ", "Git repository detection is running in the background. Try Git: Status again once the prompt updates.", []string{"&Ok"})
		return
	}

	app.RunProgressTask(" Git status ", "Reading Git status…", false, func(ctx context.Context, update func(string, int)) error {
		update("Reading index and worktree…", -1)
		if err := plugin.refreshStatus(ctx, result.Repository); err != nil {
			return err
		}
		update("Git status ready", 100)
		return nil
	}, func(err error) {
		if err != nil {
			if err != context.Canceled {
				app.Message(" Git status ", fmt.Sprintf("Cannot read Git status:\n%v", err), []string{"&Ok"})
			}
			return
		}
		// The index scan deliberately runs away from the UI thread. Do not
		// replace an unrelated passive panel with a status view for a directory
		// the user has already left while that scan was in flight.
		current := host.PanelSnapshot(vfs.PanelActive)
		if !sameVFS(current.VFS, snapshot.VFS) || current.Path != snapshot.Path {
			plugin.PanelNavigated(host, current)
			app.Message(" Git status ", "The source panel changed while Git status was loading. Run Git: Status for the current directory.", []string{"&Ok"})
			return
		}
		view := newStatusVFS(plugin, result.Repository, snapshot.VFS, host)
		plugin.registerStatusView(view)
		if err := host.OpenPassiveVFS(view); err != nil {
			plugin.unregisterStatusView(view)
			_ = view.Close()
			app.Message(" Git status ", fmt.Sprintf("Cannot open Git panel:\n%v", err), []string{"&Ok"})
			return
		}
	})
}

func (plugin *Plugin) registerStatusView(view *StatusVFS) {
	plugin.mu.Lock()
	if plugin.initialized && plugin.statusViews != nil {
		plugin.statusViews[view] = struct{}{}
	}
	plugin.mu.Unlock()
}

func (plugin *Plugin) unregisterStatusView(view *StatusVFS) {
	plugin.mu.Lock()
	delete(plugin.statusViews, view)
	plugin.mu.Unlock()
}

// refreshStatusViews performs only targeted cache invalidation for open Git
// panels. The regular source panel is refreshed by its own navigation or
// mutation caller, keeping a status scan from needlessly reloading unrelated
// panes.
func (plugin *Plugin) refreshStatusViews() {
	plugin.mu.RLock()
	views := make([]*StatusVFS, 0, len(plugin.statusViews))
	for view := range plugin.statusViews {
		views = append(views, view)
	}
	plugin.mu.RUnlock()
	for _, view := range views {
		if host := view.panelHost(); host != nil {
			host.RefreshVFS(view)
		}
	}
}

func (plugin *Plugin) followRepository(host vfs.PanelHost, snapshot vfs.PanelSnapshot, result LookupResult) {
	plugin.mu.RLock()
	views := make([]*StatusVFS, 0, len(plugin.statusViews))
	for view := range plugin.statusViews {
		views = append(views, view)
	}
	plugin.mu.RUnlock()
	for _, view := range views {
		if view.follow(host, snapshot, result) {
			host.RefreshVFS(view)
		}
	}
}

func (plugin *Plugin) clearFollowedRepository(host vfs.PanelHost, snapshot vfs.PanelSnapshot) {
	plugin.mu.RLock()
	views := make([]*StatusVFS, 0, len(plugin.statusViews))
	for view := range plugin.statusViews {
		views = append(views, view)
	}
	plugin.mu.RUnlock()
	for _, view := range views {
		if view.clearFollow(host, snapshot) {
			host.RefreshVFS(view)
		}
	}
}
