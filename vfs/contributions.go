package vfs

import "context"

// Registration is a live plugin contribution. Unregister is safe to call
// more than once.
type Registration interface {
	Unregister()
}

// PluginCommandLocation identifies the menu that hosts a plugin command.
type PluginCommandLocation uint8

const (
	PluginCommandPanel PluginCommandLocation = iota
	PluginCommandConfig
)

// PluginCommand contributes one command to an f4 plugin menu.
type PluginCommand struct {
	ID       string
	Location PluginCommandLocation
	Label    string
	Visible  func(App) bool
	Run      func(App)
}

// CommandPrefixRegistration controls a registered command-line prefix. An
// empty prefix disables it without discarding the handler.
type CommandPrefixRegistration interface {
	Registration
	SetPrefix(prefix string) error
}

// FileRef is a stable snapshot of one panel file in a VFS namespace.
type FileRef struct {
	VFS  VFS
	Dir  string
	Name string
	Path string
}

// MacroCallContext contains UI state captured before a macro provider runs.
// Providers run outside the UI goroutine and must treat this as a snapshot.
type MacroCallContext struct {
	Current FileRef
}

// MacroCallProvider exposes a synchronous Plugin.Call target to Lua macros.
// Values are limited to nil, bool, integer and floating-point numbers,
// strings, and recursively nested slices of those values.
type MacroCallProvider struct {
	IDs  []string
	Call func(context.Context, MacroCallContext, []any) ([]any, error)
}

// ContributionHost is an optional extension implemented by hosts that accept
// rich in-process plugin contributions. Keeping it separate from HostAPI
// preserves compatibility with existing and out-of-process plugins.
type ContributionHost interface {
	RegisterQuickViewProvider(QuickViewProvider) (Registration, error)
	RegisterPluginCommand(PluginCommand) (Registration, error)
	RegisterCommandPrefix(id, prefix string, handler func(App, string)) (CommandPrefixRegistration, error)
	RegisterMacroCallProvider(MacroCallProvider) (Registration, error)
}

// ProcessEnvironmentVariable is one name/value pair in f4's actual process
// environment. Snapshots are returned in a stable name order. Names are
// compared case-insensitively on Windows and case-sensitively elsewhere.
type ProcessEnvironmentVariable struct {
	Name  string
	Value string
}

// ProcessEnvironmentChange updates or removes one process environment
// variable. Name must be a portable identifier matching
// [A-Za-z_][A-Za-z0-9_]*. A set Value may not contain NUL, CR, or LF and may
// also be rejected when a host shell cannot transport it exactly (including
// cmd.exe's live-update length bound on Windows); Unset ignores Value. If a
// name occurs more than once, changes are applied in slice order and the last
// value wins. Hosts validate the whole batch before mutation and apply it
// atomically: when an OS update fails, earlier changes in the same batch are
// rolled back as far as the operating system permits.
type ProcessEnvironmentChange struct {
	Name  string
	Value string
	Unset bool
}

// ProcessEnvironmentSnapshot describes a process environment observed by the
// host. Successful reads and changes return the actual current state; after an
// OS mutation or rollback failure, Apply returns the actual state it could
// observe. Generation changes only when the observed environment changes and
// lets plugins detect edits made outside their own state.
type ProcessEnvironmentSnapshot struct {
	Generation uint64
	Variables  []ProcessEnvironmentVariable
}

// ProcessEnvironmentHost is an optional in-process capability. Applying a
// batch also schedules the same changes for f4's existing local shell
// workspaces; remote shells are deliberately outside this process-global API.
type ProcessEnvironmentHost interface {
	SnapshotProcessEnvironment() ProcessEnvironmentSnapshot
	ApplyProcessEnvironment([]ProcessEnvironmentChange) (ProcessEnvironmentSnapshot, error)
}

// TextEditorRequest opens an editor over supplied UTF-8 content. When
// Temporary is false, VFS and Path name the create-new save target. Temporary
// requests are backed by a core-owned scratch file removed on close.
type TextEditorRequest struct {
	VFS          VFS
	Path         string
	DisplayTitle string
	Content      []byte
	Modified     bool
	CursorLine   int
	CursorCol    int
	Temporary    bool
	// TargetKnownAbsent lets a caller that already performed the potentially
	// slow VFS probe off the UI goroutine skip the host's redundant check.
	// The editor still enforces no-replace semantics on its first save.
	TargetKnownAbsent bool
	OnClose           func([]byte, error)
}

// TextEditorHost is an optional UI capability exposed by PanelsFrame.
type TextEditorHost interface {
	OpenTextEditor(TextEditorRequest) error
}
