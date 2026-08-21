package vfs

import "context"

// PanelSide identifies one of the two file-manager halves from the point of
// view of the caller.  It deliberately does not expose the host's physical
// left/right implementation detail: a plugin normally wants the active panel
// as its source and the passive panel as its destination.
type PanelSide uint8

const (
	PanelActive PanelSide = iota
	PanelPassive
)

// PanelSnapshot is a cheap, UI-thread snapshot of one file panel.  VFS and
// Path are suitable for cache lookup and asynchronous work scheduling only;
// observers must not do blocking I/O before returning to the host UI loop.
type PanelSnapshot struct {
	Side         PanelSide
	VFS          VFS
	Path         string
	SelectedName string
}

// PanelObserver receives an updated snapshot after panel navigation, VFS
// replacement, focus changes, or a panel swap.  It is an optional host
// capability so existing applications and out-of-process plugins do not need
// to implement it.
type PanelObserver func(PanelSnapshot)

// PanelHost is an optional extension of App for plugins which need to keep a
// virtual view linked to a real file panel.  OpenPassiveVFS and RefreshVFS are
// called on the UI goroutine; observers must return quickly and dispatch slow
// work to their own cancellable workers.
type PanelHost interface {
	PanelSnapshot(PanelSide) PanelSnapshot
	ObservePanelChanges(PanelObserver) Registration
	OpenPassiveVFS(VFS) error
	RefreshVFS(VFS)
}

// PanelNavigationProvider observes panel snapshots for all live panel hosts.
// It is useful for cache warmers such as Git discovery: callbacks occur on the
// UI goroutine and must schedule any probing work asynchronously.
type PanelNavigationProvider interface {
	PanelNavigated(PanelHost, PanelSnapshot)
}

// PanelNavigationHost is an optional global contribution point implemented by
// a host that owns one or more live PanelsFrame instances.
type PanelNavigationHost interface {
	RegisterPanelNavigationProvider(PanelNavigationProvider) (Registration, error)
}

// NotificationHost is an optional, result-free message capability. Notify
// must enqueue the dialog for the host UI goroutine and return without waiting
// for the user to dismiss it. Plugins which need a button result must continue
// to use App.Message instead.
//
// Keeping notifications separate from App.Message prevents a plugin callback
// that already runs on the UI goroutine from posting a dialog and then waiting
// for that same goroutine to process it.
type NotificationHost interface {
	Notify(title, message string)
}

// PromptSegment is a cache-only addition to the interactive command prompt.
// Text must be small and already available when Segment is called.  The host
// resolves Color dynamically at render time, so theme changes do not leave
// plugins with stale colour values.
type PromptSegment struct {
	Text  string
	Color PromptSegmentColor
}

// PromptSegmentColor is semantic rather than a raw terminal attribute.
type PromptSegmentColor uint8

const (
	PromptSegmentDefault PromptSegmentColor = iota
	PromptSegmentGitBranch
	PromptSegmentGitDetached
	PromptSegmentGitUnborn
)

// PromptSegmentProvider must never synchronously probe the filesystem or a
// repository.  Providers are expected to maintain a session-only cache and
// schedule refreshes independently of the command-prompt render path.
type PromptSegmentProvider interface {
	PromptSegment(VFS, string) (PromptSegment, bool)
}

// PromptSegmentHost is an optional contribution point kept separate from
// ContributionHost for source compatibility with old plugin hosts.
type PromptSegmentHost interface {
	RegisterPromptSegmentProvider(PromptSegmentProvider) (Registration, error)
}

// FileDecoration is display-only metadata appended to a VFS item by a host.
// Prefix belongs before PresentationName and Attributes supplies opaque,
// namespaced values for extended panel attributes.  A provider must return
// cached values immediately; it may arrange a later panel refresh once a
// background scan completes.
type FileDecoration struct {
	Prefix     string
	Color      FileDecorationColor
	Attributes map[string]string
}

// FileDecorationColor has semantic values resolved at render time by the
// host.  A decoration only changes presentation, never Name or path identity.
type FileDecorationColor uint8

const (
	FileDecorationDefault FileDecorationColor = iota
	FileDecorationGitStaged
	FileDecorationGitUnstaged
	FileDecorationGitBoth
	FileDecorationGitUntracked
	FileDecorationGitConflict
	FileDecorationGitIgnored
)

// FileDecorationProvider returns a cached decoration for one entry.  The
// incoming item is a value so providers cannot accidentally mutate the VFS's
// operation identity.
type FileDecorationProvider interface {
	DecorateFile(VFS, string, VFSItem) (FileDecoration, bool)
}

// FileDecorationHost is an optional contribution point for asynchronous,
// cache-backed file decorations.
type FileDecorationHost interface {
	RegisterFileDecorationProvider(FileDecorationProvider) (Registration, error)
}

// PanelKeybarLabels lets a virtual VFS replace the normal labels for F1..F12
// while it is active. Empty entries preserve the host's ordinary label.  This
// is display-only metadata: key dispatch remains under the VFS controller or
// PanelActionHandler, so a user remapping cannot be silently bypassed.
type PanelKeybarLabels interface {
	PanelKeybarLabels() [12]string
}

// CommitDialogResult is the user input collected by a commit-style dialog.
// It deliberately carries no repository or signing implementation details so
// it can be used by any VFS contribution that needs a message and an optional
// signature request.
type CommitDialogResult struct {
	Message string
	Sign    bool
}

// CommitDialogRequest describes a non-blocking request to collect a commit
// message. OnCommit is invoked exactly once only after the user accepts the
// dialog. Hosts run it off the UI goroutine with a cancellable context; the
// callback must observe that context and return any user-visible failure.
//
// Title is optional. A host supplies its own suitable title when it is empty.
// InitialMessage and InitialSign make it possible to retry a rejected commit
// without discarding the user's input.
type CommitDialogRequest struct {
	Title          string
	InitialMessage string
	InitialSign    bool
	OnCommit       func(context.Context, CommitDialogResult) error
}

// CommitDialogHost is an optional UI capability for plugins that need a
// multi-line message plus a signature choice. OpenCommitDialog returns before
// the user makes a choice; work submitted through CommitDialogRequest.OnCommit
// is performed asynchronously by the host.
type CommitDialogHost interface {
	OpenCommitDialog(CommitDialogRequest) error
}
