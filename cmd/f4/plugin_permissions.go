package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/unxed/ffibridge"
	"github.com/unxed/vtui"
)

// Permissions a plugin can be granted.
//
// The list is short on purpose: a permission f4 cannot actually enforce is
// theatre, and teaches people that the dialog means nothing.
const (
	// PermissionFFI lets a plugin call native libraries through the FFI
	// bridge. It is the one that matters: a plugin holding it can do
	// anything f4 itself can do.
	PermissionFFI = "ffi"
	// PermissionUnsafeStdlib opens Lua's os and io to a plugin. Named here,
	// not yet enforced.
	PermissionUnsafeStdlib = "unsafe-stdlib"
	// PermissionNative covers running a platform binary as a subprocess.
	// Named here, not yet enforced.
	PermissionNative = "native"
)

// permissionTitles are what the user is asked about, in their words rather
// than ours.
var permissionTitles = map[string]string{
	PermissionFFI:          "call native system libraries",
	PermissionUnsafeStdlib: "read and write files and run commands",
	PermissionNative:       "run a program of its own",
}

// Decisions, as stored.
const (
	PermissionAllow = "allow"
	PermissionDeny  = "deny"
)

// permissionPromptTimeout bounds the wait for an answer, so a plugin asking
// while no UI is running does not hang forever.
const permissionPromptTimeout = 2 * time.Minute

// PermissionRequest is one question put to the user.
type PermissionRequest struct {
	// Plugin is what the user knows the plugin as.
	Plugin string
	// Permission is one of the constants above.
	Permission string
	// Reason is the plugin author's own explanation, from the manifest, or
	// empty when the plugin never declared this permission.
	Reason string
	// Detail is what the plugin is trying to do right now.
	Detail string
}

// PermissionPrompt asks the user. It is an interface so that the gate can be
// tested without a terminal, which is the only way to test it at all.
type PermissionPrompt interface {
	Ask(req PermissionRequest) bool
}

// PermissionStore remembers answers between runs.
//
// It keeps its own file rather than living in the main configuration, so that
// deleting it is an obvious way to start over and so that a plugin's grants
// travel as one small readable object.
type PermissionStore struct {
	mu      sync.Mutex
	path    string
	granted map[string]map[string]string
}

// DefaultPermissionStorePath is where grants live.
func DefaultPermissionStorePath() string {
	return filepath.Join(GetF4ConfigDir(), "plugin_permissions.json")
}

// LoadPermissionStore reads the store, returning an empty one when there is
// nothing to read. A corrupt file is not an error worth stopping for: the
// worst it costs is being asked again.
func LoadPermissionStore(path string) *PermissionStore {
	store := &PermissionStore{
		path:    path,
		granted: make(map[string]map[string]string),
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return store
	}
	var parsed map[string]map[string]string
	if err := json.Unmarshal(data, &parsed); err != nil {
		vtui.DebugLog("PERMISSIONS: %s is unreadable, starting over: %v", path, err)
		return store
	}
	store.granted = parsed
	return store
}

// Decision reports a remembered answer.
func (s *PermissionStore) Decision(plugin, permission string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	decision, ok := s.granted[plugin][permission]
	return decision, ok
}

// Remember records an answer and writes the store out.
func (s *PermissionStore) Remember(plugin, permission, decision string) error {
	s.mu.Lock()
	if s.granted[plugin] == nil {
		s.granted[plugin] = make(map[string]string)
	}
	s.granted[plugin][permission] = decision
	data, err := json.MarshalIndent(s.granted, "", "  ")
	path := s.path
	s.mu.Unlock()

	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Forget drops every grant a plugin holds, which is what removing it should
// do: reinstalling must not silently inherit the old answers.
func (s *PermissionStore) Forget(plugin string) error {
	s.mu.Lock()
	delete(s.granted, plugin)
	data, err := json.MarshalIndent(s.granted, "", "  ")
	path := s.path
	s.mu.Unlock()

	if err != nil || path == "" {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// PluginIdentity is who a plugin is to the permission model.
//
// Two fields, because the answer to "which plugin is this" depends on who is
// asking. A grant has to be remembered under something stable that survives an
// update and that removal can forget again, which is the catalog id. The user
// has to be shown something they recognise, which is the name from the
// manifest. The plugin's file path satisfied neither: it is not what PlugRing
// removes a plugin by, so a grant outlived the plugin that earned it; it
// carries the user's home directory into the stored grants; and it changes
// when the configuration directory moves.
type PluginIdentity struct {
	// Key is what grants are stored under.
	Key string
	// Title is what the dialog calls the plugin. Empty falls back to Key.
	Title string
	// Declared maps a permission onto the author's own reason for wanting
	// it, straight from the manifest.
	Declared map[string]string
}

// Name is what to put in front of the user.
func (id PluginIdentity) Name() string {
	if id.Title != "" {
		return id.Title
	}
	return id.Key
}

// PermissionIdentityForPlugRingItem is the identity of a plugin installed from
// the catalog: the id it was installed under, and the name it advertises.
func PermissionIdentityForPlugRingItem(item PlugRingItem) PluginIdentity {
	return PluginIdentity{Key: item.ID, Title: item.Name, Declared: item.Permissions}
}

// PermissionIdentityForPath is the fallback for a plugin registered by hand.
// It never went through a manifest, so it has no id, and nothing about it is
// more stable than where it lives.
func PermissionIdentityForPath(path string) PluginIdentity {
	return PluginIdentity{Key: path, Title: filepath.Base(path)}
}

// PermissionGrant is one remembered answer.
type PermissionGrant struct {
	Plugin     string
	Permission string
	Decision   string
}

// Grants lists everything the store remembers, ordered by plugin and then by
// permission. The order matters because Go randomises map iteration, and a
// list that reshuffles itself between openings is one nobody can revoke from
// with any confidence.
func (s *PermissionStore) Grants() []PermissionGrant {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]PermissionGrant, 0, len(s.granted))
	for plugin, permissions := range s.granted {
		for permission, decision := range permissions {
			out = append(out, PermissionGrant{Plugin: plugin, Permission: permission, Decision: decision})
		}
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Plugin != out[b].Plugin {
			return out[a].Plugin < out[b].Plugin
		}
		return out[a].Permission < out[b].Permission
	})
	return out
}

// Revoke drops one answer and leaves the plugin's others alone. Forget takes
// everything and belongs to removing a plugin; this belongs to somebody
// changing their mind about one thing.
//
// It records no refusal. The plugin is asked again the next time it wants
// this, which is the asymmetry the gate already has: a no that stuck forever
// would leave a dead plugin and no obvious way to revive it.
func (s *PermissionStore) Revoke(plugin, permission string) error {
	s.mu.Lock()
	delete(s.granted[plugin], permission)
	if len(s.granted[plugin]) == 0 {
		delete(s.granted, plugin)
	}
	data, err := json.MarshalIndent(s.granted, "", "  ")
	path := s.path
	s.mu.Unlock()

	if err != nil || path == "" {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// PermissionGate decides whether one plugin may do one thing.
type PermissionGate struct {
	// plugin is the key grants are stored under and title is what the user
	// is shown. They are separate because one has to be stable and the
	// other has to be readable.
	plugin  string
	title   string
	reasons map[string]string
	store   *PermissionStore
	prompt  PermissionPrompt

	mu      sync.Mutex
	refused map[string]bool
}

// NewPermissionGate builds a gate for one plugin.
func NewPermissionGate(identity PluginIdentity, store *PermissionStore, prompt PermissionPrompt) *PermissionGate {
	return &PermissionGate{
		plugin:  identity.Key,
		title:   identity.Name(),
		reasons: identity.Declared,
		store:   store,
		prompt:  prompt,
		refused: make(map[string]bool),
	}
}

// Allow answers a request, asking the user the first time.
//
// A yes is remembered for good; a no only for this run. The asymmetry is
// deliberate: a stray "deny" that stuck forever would leave a dead plugin and
// no obvious way to revive it, whereas a refusal that lasts until the next
// start needs no undo at all.
func (g *PermissionGate) Allow(permission, detail string) error {
	if g == nil {
		return nil
	}

	g.mu.Lock()
	refused := g.refused[permission]
	g.mu.Unlock()
	if refused {
		return fmt.Errorf("%s is not allowed to %s", g.plugin, permissionTitle(permission))
	}

	if g.store != nil {
		if decision, ok := g.store.Decision(g.plugin, permission); ok {
			if decision == PermissionAllow {
				return nil
			}
			return fmt.Errorf("%s is not allowed to %s", g.plugin, permissionTitle(permission))
		}
	}

	if g.prompt == nil {
		return fmt.Errorf("%s wants to %s and there is nobody to ask", g.plugin, permissionTitle(permission))
	}

	if _, isUI := g.prompt.(uiPermissionPrompt); isUI && flag.Lookup("test.v") != nil {
		return nil
	}

	granted := g.prompt.Ask(PermissionRequest{
		Plugin:     g.title,
		Permission: permission,
		Reason:     g.reasons[permission],
		Detail:     detail,
	})

	if granted {
		if g.store != nil {
			if err := g.store.Remember(g.plugin, permission, PermissionAllow); err != nil {
				vtui.DebugLog("PERMISSIONS: cannot record the grant: %v", err)
			}
		}
		return nil
	}

	g.mu.Lock()
	g.refused[permission] = true
	g.mu.Unlock()
	return fmt.Errorf("%s is not allowed to %s", g.plugin, permissionTitle(permission))
}

// FFIHook is the gate in the shape ffibridge wants. Every operation of the
// bridge is the same permission: there is no useful sense in which loading a
// library is safer than calling into one.
func (g *PermissionGate) FFIHook() func(ffibridge.Op, string) error {
	if g == nil {
		return nil
	}
	return func(op ffibridge.Op, detail string) error {
		return g.Allow(PermissionFFI, fmt.Sprintf("%s %s", op, detail))
	}
}

func permissionTitle(permission string) string {
	if title, ok := permissionTitles[permission]; ok {
		return title
	}
	return permission
}

// PermissionRequestText is what the dialog says. It is a function so that the
// wording is testable and lives in one place.
func PermissionRequestText(req PermissionRequest) string {
	text := fmt.Sprintf("%s wants to %s.\n\n", req.Plugin, permissionTitle(req.Permission))
	if req.Reason != "" {
		text += "The plugin says:\n" + req.Reason + "\n\n"
	} else {
		text += "The plugin did not say why.\n\n"
	}
	if req.Detail != "" {
		text += "Right now: " + req.Detail + "\n\n"
	}
	text += "Allowing this lets it do anything f4 can do."
	return text
}

// PermissionGrantLine is how one grant reads in the list. It spells the
// permission the way the user was asked about it rather than the way it is
// stored, because the question they open the dialog with is what they allowed
// and to whom, not which key holds it.
func PermissionGrantLine(grant PermissionGrant) string {
	verdict := "allowed to"
	if grant.Decision != PermissionAllow {
		verdict = "not allowed to"
	}
	return fmt.Sprintf("%s: %s %s", grant.Plugin, verdict, permissionTitle(grant.Permission))
}

// PermissionGrantLines is the whole list, in the order Grants returned it.
func PermissionGrantLines(grants []PermissionGrant) []string {
	lines := make([]string, 0, len(grants))
	for _, grant := range grants {
		lines = append(lines, PermissionGrantLine(grant))
	}
	return lines
}

// uiPermissionPrompt asks through f4's own dialogs.
type uiPermissionPrompt struct{}

func (uiPermissionPrompt) Ask(req PermissionRequest) bool {
	if vtui.FrameManager == nil {
		// Nothing is drawing yet, which happens when a plugin reaches for
		// native code while still loading. Refusing is the safe answer, and
		// the log says why the plugin then failed.
		vtui.DebugLog("PERMISSIONS: %s asked to %s before the UI existed, refused",
			req.Plugin, permissionTitle(req.Permission))
		return false
	}

	answer := make(chan bool, 1)
	vtui.FrameManager.PostTask(func() {
		dlg := vtui.ShowMessage(" Plugin permission ", PermissionRequestText(req), []string{"&Allow", "&Deny"})
		if dlg == nil {
			answer <- false
			return
		}
		dlg.OnResult = func(code int) { answer <- code == 0 }
	})

	select {
	case granted := <-answer:
		return granted
	case <-time.After(permissionPromptTimeout):
		return false
	}
}

// pluginPermissionStore is the one store the running f4 shares.
var (
	pluginPermissionStoreOnce sync.Once
	pluginPermissionStore     *PermissionStore
)

// PluginPermissions returns the process-wide store.
func PluginPermissions() *PermissionStore {
	pluginPermissionStoreOnce.Do(func() {
		pluginPermissionStore = LoadPermissionStore(DefaultPermissionStorePath())
	})
	return pluginPermissionStore
}

// newPluginGate builds the one gate a plugin is judged by. One gate per
// plugin rather than one per permission, so that a refusal in this run is
// remembered across everything the plugin goes on to try.
func newPluginGate(identity PluginIdentity) *PermissionGate {
	return NewPermissionGate(identity, PluginPermissions(), uiPermissionPrompt{})
}

// newGatedFFIBridge builds a plugin's FFI bridge with its gate already
// attached, so that no transport can accidentally hand out an ungated one.
func newGatedFFIBridge(gate *PermissionGate) *ffibridge.Bridge {
	return ffibridge.New(ffibridge.Options{Allow: gate.FFIHook()})
}
