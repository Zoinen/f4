package gitplugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v6"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

const statusSeparatorName = "1:separator"

type statusLayer uint8

const (
	statusLayerStaged statusLayer = iota
	statusLayerUnstaged
)

// StatusVFS is the cache-backed virtual listing behind Git: Status. It never
// interprets its display labels as filesystem paths: the Name field is a
// stable, private row identity and DisplayName is solely presentation.
//
// The VFS deliberately owns no copy of the underlying OSVFS and therefore
// never closes it. The panel containing the real directory remains usable and
// is followed while this view is open.
type StatusVFS struct {
	plugin *Plugin
	source vfs.VFS

	mu         sync.RWMutex
	repository Repository
	host       vfs.PanelHost
	closed     bool
}

type statusRow struct {
	name     string
	item     vfs.VFSItem
	layer    statusLayer
	entry    statusEntry
	editable bool
}

func newStatusVFS(plugin *Plugin, repository Repository, source vfs.VFS, host ...vfs.PanelHost) *StatusVFS {
	view := &StatusVFS{plugin: plugin, repository: repository, source: source}
	if len(host) != 0 {
		view.host = host[0]
	}
	return view
}

func (view *StatusVFS) IsAtRoot() bool { return true }

func (view *StatusVFS) GetPath() string { return "/" }

func (view *StatusVFS) IsAbs(value string) bool { return path.IsAbs(value) }

func (view *StatusVFS) SetPath(value string) error {
	if path.Clean(value) == "." || path.Clean(value) == "/" {
		return nil
	}
	return os.ErrPermission
}

func (view *StatusVFS) ReadDir(ctx context.Context, _ string, onChunk func([]vfs.VFSItem)) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	if onChunk == nil {
		return nil
	}
	rows := view.rows()
	items := make([]vfs.VFSItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.item)
	}
	if len(items) != 0 {
		onChunk(items)
	}
	return contextError(ctx)
}

func (view *StatusVFS) Stat(ctx context.Context, value string) (vfs.VFSItem, error) {
	if err := contextError(ctx); err != nil {
		return vfs.VFSItem{}, err
	}
	if path.Clean(value) == "." || path.Clean(value) == "/" {
		return vfs.VFSItem{Name: "/", IsDir: true}, nil
	}
	if row, ok := view.row(value); ok {
		return row.item, nil
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

func (view *StatusVFS) Join(parts ...string) string { return path.Join(parts...) }

func (view *StatusVFS) Abs(value string) (string, error) {
	if path.IsAbs(value) {
		return path.Clean(value), nil
	}
	return path.Join(view.GetPath(), value), nil
}

func (view *StatusVFS) Base(value string) string { return path.Base(value) }

func (view *StatusVFS) Dir(value string) string { return path.Dir(value) }

func (view *StatusVFS) MkDir(context.Context, string) error { return os.ErrPermission }

func (view *StatusVFS) Remove(context.Context, string) error { return os.ErrPermission }

func (view *StatusVFS) Rename(context.Context, string, string) error { return os.ErrPermission }

func (view *StatusVFS) GetCapabilities() vfs.VFSCapabilities {
	// The view exposes historical status data, not a writable filesystem. A
	// selected row can be viewed through Open, but file-manager mutations must
	// use the explicit Git operations below.
	return vfs.VFSCapabilities{HasRandomAccess: view.sourceCapabilities().HasRandomAccess}
}

func (view *StatusVFS) sourceCapabilities() vfs.VFSCapabilities {
	view.mu.RLock()
	source := view.source
	view.mu.RUnlock()
	if source == nil {
		return vfs.VFSCapabilities{}
	}
	return source.GetCapabilities()
}

func (view *StatusVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, errors.New("Git status search is not supported")
}

// Open resolves a regular status row back to the local working-tree file for
// ordinary viewer integrations. F3/F4 are intercepted by ProcessPanelKey and
// operate on a Git diff instead, so callers never mistake this for an index
// snapshot.
func (view *StatusVFS) Open(ctx context.Context, value string) (vfs.ReadAtCloser, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	row, ok := view.row(value)
	if !ok || row.item.Kind != vfs.VFSItemRegular {
		return nil, os.ErrNotExist
	}
	view.mu.RLock()
	repository, source, closed := view.repository, view.source, view.closed
	view.mu.RUnlock()
	if closed || source == nil {
		return nil, os.ErrClosed
	}
	return source.Open(ctx, filepath.Join(repository.Root, filepath.FromSlash(row.entry.Path)))
}

func (view *StatusVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, os.ErrPermission
}

func (view *StatusVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return os.ErrPermission
}

func (view *StatusVFS) ParentVFS() vfs.VFS { return nil }

func (view *StatusVFS) Clone() vfs.VFS {
	view.mu.RLock()
	plugin, repository, source, host, closed := view.plugin, view.repository, view.source, view.host, view.closed
	view.mu.RUnlock()
	clone := newStatusVFS(plugin, repository, source, host)
	if closed {
		clone.closed = true
	}
	if plugin != nil && !closed {
		plugin.registerStatusView(clone)
	}
	return clone
}

func (view *StatusVFS) Close() error {
	view.mu.Lock()
	if view.closed {
		view.mu.Unlock()
		return nil
	}
	view.closed = true
	plugin := view.plugin
	view.mu.Unlock()
	if plugin != nil {
		plugin.unregisterStatusView(view)
	}
	return nil
}

func (view *StatusVFS) GetTitle() string { return "Git: Status" }

func (view *StatusVFS) PanelTitle(_ string) string {
	view.mu.RLock()
	repository := view.repository
	closed := view.closed
	view.mu.RUnlock()
	if closed || repository.Root == "" {
		return "Git: Status (no repository)"
	}
	branch := repository.Branch.Prompt()
	if branch == "" {
		branch = filepath.Base(repository.Root)
	}
	if repository.Branch.Unborn {
		branch += " (unborn)"
	}
	return "Git: Status — " + branch
}

func (view *StatusVFS) PanelKeybarLabels() [12]string {
	return [12]string{
		1: "Commit",
		2: "Diff",
		3: "Edit diff",
		6: "Log",
	}
}

// HandlePanelAction prevents a separator or an ordinary status row from
// falling through to file-manager mutation semantics. The corresponding
// keyboard actions use ProcessPanelKey so they keep the same behavior under
// menu/action remapping.
func (view *StatusVFS) HandlePanelAction(app vfs.App, action vfs.PanelAction, paths []string) bool {
	switch action {
	case vfs.PanelActionActivate:
		view.showDiff(app, pathsToNames(paths))
		return true
	case vfs.PanelActionEdit:
		view.editDiff(app, pathsToNames(paths))
		return true
	case vfs.PanelActionCreate, vfs.PanelActionDelete:
		notify(app, " Git ", "Use Git commands in the status panel; this virtual view is read-only.")
		return true
	default:
		return false
	}
}

// ProcessPanelKey is discovered structurally by f4's main package. Keeping
// this method in the plugin avoids introducing f4 UI types into the go-git
// fork or into the public VFS core API.
func (view *StatusVFS) ProcessPanelKey(app vfs.App, event *vtinput.InputEvent) bool {
	if event == nil || event.Type != vtinput.KeyEventType || !event.KeyDown || event.ControlKeyState != 0 {
		return false
	}
	switch event.VirtualKeyCode {
	case vtinput.VK_F2:
		view.beginCommit(app)
		return true
	case vtinput.VK_F3:
		view.showDiff(app, app.GetSelectedNames())
		return true
	case vtinput.VK_F4:
		view.editDiff(app, app.GetSelectedNames())
		return true
	case vtinput.VK_F7:
		view.openLog(app)
		return true
	case vtinput.VK_SPACE:
		view.toggleStage(app, app.GetSelectedNames())
		return true
	default:
		return false
	}
}

func (view *StatusVFS) rows() []statusRow {
	view.mu.RLock()
	plugin := view.plugin
	repository := view.repository
	closed := view.closed
	view.mu.RUnlock()
	if plugin == nil || closed || repository.Root == "" {
		return nil
	}
	snapshot := plugin.cachedStatus(repository.Root)
	if snapshot == nil {
		return nil
	}
	entries := snapshot.orderedEntries()
	staged := make([]statusRow, 0, len(entries))
	unstaged := make([]statusRow, 0, len(entries))
	for _, entry := range entries {
		if belongsToStagedLayer(entry) {
			staged = append(staged, newStatusRow(statusLayerStaged, entry, snapshot.updatedAt))
		}
		if belongsToUnstagedLayer(entry) {
			unstaged = append(unstaged, newStatusRow(statusLayerUnstaged, entry, snapshot.updatedAt))
		}
	}
	rows := make([]statusRow, 0, len(staged)+len(unstaged)+1)
	rows = append(rows, staged...)
	if len(staged) != 0 || len(unstaged) != 0 {
		rows = append(rows, statusRow{name: statusSeparatorName, item: vfs.VFSItem{
			Name:        statusSeparatorName,
			DisplayName: "──────── staged / unstaged ────────",
			Kind:        vfs.VFSItemSeparator,
			NoExtension: true,
			MTime:       snapshot.updatedAt,
		}})
	}
	rows = append(rows, unstaged...)
	return rows
}

func belongsToStagedLayer(entry statusEntry) bool {
	if entry.Class == statusConflict || entry.Class == statusIgnored || entry.Class == statusUntracked {
		return false
	}
	return entry.Index != gogit.Unmodified && entry.Index != gogit.Untracked && entry.Index != gogit.Ignored
}

func belongsToUnstagedLayer(entry statusEntry) bool {
	// Ignored paths are useful as ordinary-panel decorations, but they do not
	// belong in the status VFS: the view is explicitly ordered as staged then
	// unstaged, and an ignored path has neither a Git diff nor a staging
	// operation. Showing it here used to make F3 report a misleading generic
	// "binary changes" read-only error for perfectly ordinary source files
	// beneath an ignored build directory.
	if entry.Class == statusIgnored {
		return false
	}
	if entry.Class == statusConflict || entry.Class == statusUntracked {
		return true
	}
	return entry.Worktree != gogit.Unmodified
}

func newStatusRow(layer statusLayer, entry statusEntry, modifiedAt time.Time) statusRow {
	// The artificial sortable prefixes keep the layer order intact even when
	// the ordinary panel sort mode is Name. They never appear in DisplayName.
	prefix := "0:"
	label := "staged"
	if layer == statusLayerUnstaged {
		prefix = "2:"
		label = "unstaged"
	}
	name := prefix + entry.Path
	attributes := map[string]string{
		"git.layer":          label,
		"git.path":           entry.Path,
		"git.indexStatus":    string(entry.Index),
		"git.worktreeStatus": string(entry.Worktree),
	}
	if entry.Class == statusConflict {
		attributes["git.readOnly"] = "conflict"
	}
	if entry.Class == statusIgnored {
		attributes["git.readOnly"] = "ignored"
	}
	return statusRow{
		name:  name,
		layer: layer,
		entry: entry,
		item: vfs.VFSItem{
			Name:               name,
			DisplayName:        entry.Path,
			NoExtension:        true,
			ExtendedAttributes: attributes,
			MTime:              modifiedAt,
		},
		editable: entry.Class != statusConflict && entry.Class != statusIgnored,
	}
}

func (view *StatusVFS) row(value string) (statusRow, bool) {
	name := strings.TrimPrefix(path.Clean(value), "/")
	for _, row := range view.rows() {
		if row.name == name {
			return row, true
		}
	}
	return statusRow{}, false
}

func (view *StatusVFS) rowsForNames(names []string) []statusRow {
	seen := make(map[string]struct{}, len(names))
	rows := make([]statusRow, 0, len(names))
	for _, name := range names {
		if row, ok := view.row(name); ok {
			if _, duplicate := seen[row.name]; !duplicate {
				seen[row.name] = struct{}{}
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func (view *StatusVFS) toggleStage(app vfs.App, names []string) {
	rows := view.rowsForNames(names)
	if len(rows) == 0 {
		notify(app, " Git ", "Select a staged or unstaged file first.")
		return
	}
	layer := rows[0].layer
	paths := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.item.Kind != vfs.VFSItemRegular {
			continue
		}
		if row.layer != layer {
			notify(app, " Git ", "Select one homogeneous staged or unstaged group.")
			return
		}
		if !row.editable {
			notify(app, " Git ", "Conflicts and ignored paths are read-only in this Git status view.")
			return
		}
		if _, duplicate := seen[row.entry.Path]; !duplicate {
			seen[row.entry.Path] = struct{}{}
			paths = append(paths, filepath.FromSlash(row.entry.Path))
		}
	}
	if len(paths) == 0 {
		return
	}
	repository, ok := view.repositorySnapshot()
	if !ok {
		notify(app, " Git ", "The linked repository is no longer available.")
		return
	}
	verb, title := "Stage", " Git stage "
	if layer == statusLayerStaged {
		verb, title = "Unstage", " Git unstage "
	}
	app.RunProgressTask(title, verb+" selected paths…", false, func(ctx context.Context, update func(string, int)) error {
		update(verb+" selected paths…", -1)
		repositoryHandle, err := gogit.PlainOpen(repository.Root)
		if err != nil {
			return err
		}
		defer repositoryHandle.Close()
		worktree, err := repositoryHandle.Worktree()
		if err != nil {
			return err
		}
		if layer == statusLayerStaged {
			err = worktree.UnstageContext(ctx, paths...)
		} else {
			err = worktree.StageContext(ctx, paths...)
		}
		if err != nil {
			return err
		}
		if err := view.plugin.refreshStatus(ctx, repository); err != nil {
			return err
		}
		update("Git status updated", 100)
		return nil
	}, func(err error) {
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				notify(app, " Git ", fmt.Sprintf("%s failed:\n%v", verb, err))
			}
			return
		}
		view.refreshLinkedPanels(app)
	})
}

func (view *StatusVFS) beginCommit(app vfs.App) {
	if dialogHost, supported := app.(vfs.CommitDialogHost); supported {
		err := dialogHost.OpenCommitDialog(vfs.CommitDialogRequest{
			OnCommit: func(ctx context.Context, result vfs.CommitDialogResult) error {
				if strings.TrimSpace(result.Message) == "" {
					return errors.New("a commit message is required")
				}
				return view.commit(ctx, result.Message, result.Sign, app)
			},
		})
		if err != nil {
			notify(app, " Git commit ", fmt.Sprintf("Cannot open commit dialog:\n%v", err))
		}
		return
	}

	// Older hosts can still submit a simple one-line commit message. New f4
	// hosts use the multiline theme-aware dialog above.
	app.InputBox(" Git commit ", "Commit message:", "", func(message string) {
		if strings.TrimSpace(message) == "" {
			return
		}
		app.RunProgressTask(" Git commit ", "Creating commit…", false, func(ctx context.Context, update func(string, int)) error {
			update("Creating commit…", -1)
			return view.commit(ctx, message, false, app)
		}, func(err error) {
			if err != nil && !errors.Is(err, context.Canceled) {
				notify(app, " Git commit ", fmt.Sprintf("Cannot commit:\n%v", err))
			}
		})
	})
}

func (view *StatusVFS) commit(ctx context.Context, message string, sign bool, app vfs.App) error {
	repository, ok := view.repositorySnapshot()
	if !ok {
		return errors.New("the linked repository is no longer available")
	}
	repositoryHandle, err := gogit.PlainOpen(repository.Root)
	if err != nil {
		return err
	}
	defer repositoryHandle.Close()
	worktree, err := repositoryHandle.Worktree()
	if err != nil {
		return err
	}
	options := &gogit.CommitOptions{}
	if sign {
		// Resolve exactly the configured OpenPGP/SSH key in the fork. A missing
		// key or gpg.format=x509 is an explicit error; the Sign checkbox must
		// never silently fall back to an unsigned commit.
		signer, signerErr := repositoryHandle.ResolveCommitSigner(ctx, nil)
		if signerErr != nil {
			return signerErr
		}
		options.Signer = signer
	}
	// The fork resolves the common Git directory and core.hooksPath itself.
	// A nil runner selects the standard OS runner, which executes only the
	// user's configured hook program directly (never git.exe or a shell).
	if _, err := worktree.CommitWithHooksContext(ctx, message, options, nil); err != nil {
		return err
	}
	if err := view.plugin.refreshStatus(ctx, repository); err != nil {
		return err
	}
	view.refreshLinkedPanelsFromWorker(app)
	return nil
}

func (view *StatusVFS) showDiff(app vfs.App, names []string) {
	layer, paths, err := view.diffSelection(names)
	if err != nil {
		notify(app, " Git diff ", err.Error())
		return
	}
	repository, ok := view.repositorySnapshot()
	if !ok {
		notify(app, " Git diff ", "The linked repository is no longer available.")
		return
	}
	var result struct {
		sync.Mutex
		patch string
	}
	app.RunProgressTask(" Git diff ", "Building unified diff…", false, func(ctx context.Context, update func(string, int)) error {
		update("Reading Git objects…", -1)
		patch, err := worktreeDiff(ctx, repository, layer, paths)
		if err != nil {
			return err
		}
		result.Lock()
		result.patch = patch
		result.Unlock()
		update("Diff ready", 100)
		return nil
	}, func(err error) {
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				notify(app, " Git diff ", fmt.Sprintf("Cannot build diff:\n%v", err))
			}
			return
		}
		result.Lock()
		patch := result.patch
		result.Unlock()
		if patch == "" {
			notify(app, " Git diff ", "No textual changes for this layer.")
			return
		}
		if editor, supported := app.(vfs.TextEditorHost); supported {
			if openErr := editor.OpenTextEditor(vfs.TextEditorRequest{
				Temporary:    true,
				DisplayTitle: "Git diff — " + diffLayerLabel(layer),
				Content:      []byte(patch),
			}); openErr != nil {
				notify(app, " Git diff ", fmt.Sprintf("Cannot open diff viewer:\n%v", openErr))
			}
			return
		}
		notify(app, " Git diff ", patch)
	})
}

func (view *StatusVFS) editDiff(app vfs.App, names []string) {
	layer, paths, err := view.editableDiffSelection(names)
	if err != nil {
		notify(app, " Git edit diff ", err.Error())
		return
	}
	repository, ok := view.repositorySnapshot()
	if !ok {
		notify(app, " Git edit diff ", "The linked repository is no longer available.")
		return
	}
	if _, supported := app.(vfs.TextEditorHost); !supported {
		notify(app, " Git edit diff ", "This host cannot open a patch editor.")
		return
	}
	var result struct {
		sync.Mutex
		patch string
	}
	app.RunProgressTask(" Git edit diff ", "Building editable patch…", false, func(ctx context.Context, update func(string, int)) error {
		update("Reading Git objects…", -1)
		patch, err := worktreeDiff(ctx, repository, layer, paths)
		if err != nil {
			return err
		}
		if !editableUnifiedPatch(patch) {
			return errors.New("binary changes, gitlinks, and unresolved conflicts are read-only")
		}
		result.Lock()
		result.patch = patch
		result.Unlock()
		update("Patch ready", 100)
		return nil
	}, func(err error) {
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				notify(app, " Git edit diff ", fmt.Sprintf("Cannot prepare editable patch:\n%v", err))
			}
			return
		}
		result.Lock()
		patch := result.patch
		result.Unlock()
		if patch == "" {
			notify(app, " Git edit diff ", "No textual changes for this layer.")
			return
		}
		editor := app.(vfs.TextEditorHost)
		if openErr := editor.OpenTextEditor(vfs.TextEditorRequest{
			Temporary:    true,
			DisplayTitle: "Git patch — " + diffLayerLabel(layer),
			Content:      []byte(patch),
			OnSave: func(ctx context.Context, edited []byte) error {
				if bytes.Equal(edited, []byte(patch)) {
					// Saving an untouched temporary patch must not needlessly rewrite
					// the worktree/index (or fail because a fingerprint moved after
					// opening the viewer).
					return nil
				}
				if !editableUnifiedPatch(string(edited)) {
					return errors.New("only textual unified patches can be applied")
				}
				repositoryHandle, openErr := gogit.PlainOpen(repository.Root)
				if openErr != nil {
					return openErr
				}
				defer repositoryHandle.Close()
				worktree, worktreeErr := repositoryHandle.Worktree()
				if worktreeErr != nil {
					return worktreeErr
				}
				options := &gogit.ApplyOptions{Staged: layer == statusLayerStaged}
				// A displayed diff describes base -> current. ApplyContext verifies
				// that a forward patch's base is present, so applying the edited
				// patch directly would correctly reject the already-current state.
				// Restore the selected files to their recorded base first, then
				// apply the user's rewritten base -> result patch. Each operation is
				// atomic in the fork; if the second one fails, compensate with the
				// original patch and leave the view untouched.
				if restoreErr := worktree.ApplyContext(ctx, []byte(patch), &gogit.ApplyOptions{
					Staged:  options.Staged,
					Reverse: true,
				}); restoreErr != nil {
					return fmt.Errorf("cannot restore the original patch base: %w", restoreErr)
				}
				if applyErr := worktree.ApplyContext(ctx, edited, options); applyErr != nil {
					rollbackErr := worktree.ApplyContext(context.Background(), []byte(patch), options)
					if rollbackErr != nil {
						return fmt.Errorf("cannot apply edited patch: %w; automatic rollback failed: %v", applyErr, rollbackErr)
					}
					return fmt.Errorf("cannot apply edited patch; the original changes were restored: %w", applyErr)
				}
				if refreshErr := view.plugin.refreshStatus(ctx, repository); refreshErr != nil {
					return refreshErr
				}
				view.refreshLinkedPanelsFromWorker(app)
				return nil
			},
		}); openErr != nil {
			notify(app, " Git edit diff ", fmt.Sprintf("Cannot open patch editor:\n%v", openErr))
		}
	})
}

func (view *StatusVFS) diffSelection(names []string) (statusLayer, []string, error) {
	return view.diffSelectionWithPolicy(names, false)
}

// editableDiffSelection is deliberately stricter than diffSelection: F3 can
// display a binary-diff marker, while F4 must only operate on a patch that can
// safely be edited and re-applied. Conflicts are retained in the status view
// for visibility but cannot be edited.
func (view *StatusVFS) editableDiffSelection(names []string) (statusLayer, []string, error) {
	return view.diffSelectionWithPolicy(names, true)
}

func (view *StatusVFS) diffSelectionWithPolicy(names []string, requireEditable bool) (statusLayer, []string, error) {
	rows := view.rowsForNames(names)
	if len(rows) == 0 {
		return statusLayerUnstaged, nil, errors.New("Select a staged or unstaged file first.")
	}
	layer := rows[0].layer
	paths := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row.item.Kind != vfs.VFSItemRegular {
			continue
		}
		if row.layer != layer {
			return statusLayerUnstaged, nil, errors.New("Select one homogeneous staged or unstaged group.")
		}
		if requireEditable && !row.editable {
			return statusLayerUnstaged, nil, errors.New("Unresolved conflicts are read-only in the Git patch editor.")
		}
		if _, duplicate := seen[row.entry.Path]; !duplicate {
			seen[row.entry.Path] = struct{}{}
			paths = append(paths, row.entry.Path)
		}
	}
	if len(paths) == 0 {
		return statusLayerUnstaged, nil, errors.New("Select a staged or unstaged file first.")
	}
	return layer, paths, nil
}

func worktreeDiff(ctx context.Context, repository Repository, layer statusLayer, paths []string) (string, error) {
	repositoryHandle, err := gogit.PlainOpen(repository.Root)
	if err != nil {
		return "", err
	}
	defer repositoryHandle.Close()
	worktree, err := repositoryHandle.Worktree()
	if err != nil {
		return "", err
	}
	return worktree.DiffContext(ctx, gogit.DiffOptions{
		Staged:           layer == statusLayerStaged,
		Paths:            paths,
		IncludeUntracked: layer == statusLayerUnstaged,
	})
}

func diffLayerLabel(layer statusLayer) string {
	if layer == statusLayerStaged {
		return "staged"
	}
	return "unstaged"
}

func editableUnifiedPatch(patch string) bool {
	if strings.TrimSpace(patch) == "" || strings.Contains(patch, "GIT binary patch") || strings.Contains(patch, "Binary files ") {
		return false
	}
	for _, line := range strings.Split(patch, "\n") {
		line = strings.TrimSuffix(line, "\r")
		// A gitlink can look superficially like a one-line text diff. The
		// history/status UI nevertheless keeps it read-only, matching the
		// fork's deliberate ApplyContext boundary.
		if strings.HasPrefix(line, "index ") && strings.HasSuffix(line, " 160000") {
			return false
		}
		if strings.HasPrefix(line, "-Subproject commit ") || strings.HasPrefix(line, "+Subproject commit ") {
			return false
		}
	}
	return true
}

func (view *StatusVFS) openLog(app vfs.App) {
	host, supported := app.(vfs.PanelHost)
	if !supported || host == nil {
		notify(app, " Git log ", "This host cannot open a Git history virtual panel.")
		return
	}
	repository, ok := view.repositorySnapshot()
	if !ok {
		notify(app, " Git log ", "The linked repository is no longer available.")
		return
	}
	if err := host.OpenPassiveVFS(NewLogVFS(repository)); err != nil {
		notify(app, " Git log ", fmt.Sprintf("Cannot open Git history:\n%v", err))
	}
}

func (view *StatusVFS) refreshLinkedPanels(app vfs.App) {
	view.mu.RLock()
	host, source := view.host, view.source
	view.mu.RUnlock()
	if appHost, ok := app.(vfs.PanelHost); ok && appHost != nil {
		appHost.RefreshVFS(view)
		if source != nil {
			appHost.RefreshVFS(source)
		}
		return
	}
	if host != nil {
		host.RefreshVFS(view)
		if source != nil {
			host.RefreshVFS(source)
		}
	}
	app.RefreshAll()
}

// refreshLinkedPanelsFromWorker is the save-callback counterpart of
// refreshLinkedPanels. TextEditorRequest.OnSave runs on a worker, so only the
// PanelHost's explicitly thread-safe refresh method may be used here.
func (view *StatusVFS) refreshLinkedPanelsFromWorker(app vfs.App) {
	view.mu.RLock()
	host, source := view.host, view.source
	view.mu.RUnlock()
	if appHost, ok := app.(vfs.PanelHost); ok && appHost != nil {
		appHost.RefreshVFS(view)
		if source != nil {
			appHost.RefreshVFS(source)
		}
		return
	}
	if host != nil {
		host.RefreshVFS(view)
		if source != nil {
			host.RefreshVFS(source)
		}
	}
}

func (view *StatusVFS) repositorySnapshot() (Repository, bool) {
	view.mu.RLock()
	repository, closed := view.repository, view.closed
	view.mu.RUnlock()
	return repository, !closed && repository.Root != ""
}

// follow updates a view only when the panel that originally supplied its real
// filesystem moves. It deliberately ignores the status VFS itself, so opening
// and refreshing the passive pane does not create a feedback loop.
func (view *StatusVFS) follow(host vfs.PanelHost, snapshot vfs.PanelSnapshot, result LookupResult) bool {
	if !result.Found() || !sameVFS(view.sourceSnapshot(), snapshot.VFS) {
		return false
	}
	view.mu.Lock()
	if view.closed {
		view.mu.Unlock()
		return false
	}
	// A delayed branch cache update does not replace the worktree, but it does
	// change the status panel title. Treat it as a presentation refresh while
	// preserving the same source VFS linkage.
	changed := !sameRepository(view.repository, result.Repository) || view.repository.Branch != result.Repository.Branch
	view.repository = result.Repository
	if host != nil {
		view.host = host
	}
	view.mu.Unlock()
	return changed
}

func (view *StatusVFS) clearFollow(host vfs.PanelHost, snapshot vfs.PanelSnapshot) bool {
	if !sameVFS(view.sourceSnapshot(), snapshot.VFS) {
		return false
	}
	view.mu.Lock()
	if view.closed || view.repository.Root == "" {
		view.mu.Unlock()
		return false
	}
	view.repository = Repository{}
	if host != nil {
		view.host = host
	}
	view.mu.Unlock()
	return true
}

func (view *StatusVFS) sourceSnapshot() vfs.VFS {
	view.mu.RLock()
	source := view.source
	view.mu.RUnlock()
	return source
}

func (view *StatusVFS) panelHost() vfs.PanelHost {
	view.mu.RLock()
	host := view.host
	view.mu.RUnlock()
	return host
}

func sameRepository(left, right Repository) bool {
	return filepath.Clean(left.Root) == filepath.Clean(right.Root) && filepath.Clean(left.GitDir) == filepath.Clean(right.GitDir)
}

func sameVFS(left, right vfs.VFS) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	leftType, rightType := reflect.TypeOf(left), reflect.TypeOf(right)
	return leftType == rightType && leftType.Comparable() && left == right
}

func pathsToNames(paths []string) []string {
	names := make([]string, 0, len(paths))
	for _, value := range paths {
		value = strings.TrimPrefix(path.Clean(value), "/")
		if value != "." && value != "" {
			names = append(names, value)
		}
	}
	return names
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	return ctx.Err()
}

var _ vfs.VFS = (*StatusVFS)(nil)
var _ vfs.TitleProvider = (*StatusVFS)(nil)
var _ vfs.PanelTitleProvider = (*StatusVFS)(nil)
var _ vfs.PanelKeybarLabels = (*StatusVFS)(nil)
var _ vfs.PanelActionHandler = (*StatusVFS)(nil)
