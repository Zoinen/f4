package panel

import (
	"errors"
	"fmt"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"sort"
	"strings"
	"sync"
)

var (
	errCommandPrefixUnregistered = errors.New("command prefix registration is no longer active")
	CommandPrefixRegistry        = struct {
		sync.RWMutex
		ByID     map[string]*CommandPrefixRegistration
		ByPrefix map[string]*CommandPrefixRegistration
	}{
		ByID:     make(map[string]*CommandPrefixRegistration),
		ByPrefix: make(map[string]*CommandPrefixRegistration),
	}
)

type CommandPrefixRegistration struct {
	Id      string
	Prefix  string
	Handler func(vfs.App, string)
	Active  bool
	Once    sync.Once
}

type commandPrefixSnapshotEntry struct {
	Id     string
	Prefix string
}

func CommandPrefixSnapshot() []commandPrefixSnapshotEntry {
	CommandPrefixRegistry.RLock()
	defer CommandPrefixRegistry.RUnlock()
	result := make([]commandPrefixSnapshotEntry, 0, len(CommandPrefixRegistry.ByID))
	for id, registration := range CommandPrefixRegistry.ByID {
		if registration == nil || !registration.Active || registration.Prefix == "" {
			continue
		}
		result = append(result, commandPrefixSnapshotEntry{Id: id, Prefix: registration.Prefix})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Id < result[j].Id })
	return result
}

func NormalizeCommandPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", nil
	}
	for index, r := range prefix {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if index > 0 {
			valid = valid || r >= '0' && r <= '9' || r == '_' || r == '-'
		}
		if !valid {
			return "", fmt.Errorf("invalid command prefix %q", prefix)
		}
	}
	return strings.ToLower(prefix), nil
}

func (r *CommandPrefixRegistration) SetPrefix(prefix string) error {
	normalized, err := NormalizeCommandPrefix(prefix)
	if err != nil {
		return err
	}
	CommandPrefixRegistry.Lock()
	defer CommandPrefixRegistry.Unlock()
	if r == nil || !r.Active || CommandPrefixRegistry.ByID[r.Id] != r {
		return errCommandPrefixUnregistered
	}
	if normalized == r.Prefix {
		return nil
	}
	if owner, exists := CommandPrefixRegistry.ByPrefix[normalized]; normalized != "" && exists && owner != r {
		return fmt.Errorf("command prefix %q is already registered by %q", prefix, owner.Id)
	}
	if r.Prefix != "" {
		delete(CommandPrefixRegistry.ByPrefix, r.Prefix)
	}
	r.Prefix = normalized
	if normalized != "" {
		CommandPrefixRegistry.ByPrefix[normalized] = r
	}
	return nil
}

func (r *CommandPrefixRegistration) Unregister() {
	if r == nil {
		return
	}
	r.Once.Do(func() {
		CommandPrefixRegistry.Lock()
		if CommandPrefixRegistry.ByID[r.Id] == r {
			delete(CommandPrefixRegistry.ByID, r.Id)
		}
		if r.Prefix != "" && CommandPrefixRegistry.ByPrefix[r.Prefix] == r {
			delete(CommandPrefixRegistry.ByPrefix, r.Prefix)
		}
		r.Active = false
		CommandPrefixRegistry.Unlock()
	})
}

// DispatchCommandPrefix consumes input when the text before its first colon
// names either a registered prefix or a plugin/platform drive. Registered
// prefixes get the raw argument so each plugin can apply its own quoting
// rules; drive prefixes intentionally accept only the bare form (for example,
// "Android:" or the platform alias "reg:").
func DispatchCommandPrefix(app vfs.App, input string) bool {
	colon := strings.IndexByte(input, ':')
	if colon <= 0 {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(input[:colon]))
	CommandPrefixRegistry.RLock()
	registration := CommandPrefixRegistry.ByPrefix[normalized]
	if registration != nil && registration.Active {
		handler := registration.Handler
		CommandPrefixRegistry.RUnlock()
		handler(app, input[colon+1:])
		return true
	}
	CommandPrefixRegistry.RUnlock()
	return dispatchDriveCommandPrefix(app, normalized, input[colon+1:])
}

// dispatchDriveCommandPrefix opens a registered plugin drive in the active
// panel. Drive names are already the user-facing names shown by the drive
// menu, so exposing the same names as command prefixes keeps the two entry
// points consistent. The AI drive is deliberately excluded: its ai: prefix
// is an established command language of its own.
func dispatchDriveCommandPrefix(app vfs.App, prefix, argument string) bool {
	if strings.TrimSpace(argument) != "" || prefix == "ai" {
		return false
	}

	pf, ok := app.(*PanelsFrame)
	if !ok || pf == nil || pf.Closed {
		return false
	}
	fsp := pf.GetActivePanel()
	if fsp == nil {
		return false
	}

	if prefix == "tmp" {
		ActionOpenTempPanel(pf)
		return true
	}

	for _, drive := range sysinfo.DriveRegistrySnapshot() {
		if strings.ToLower(strings.TrimSpace(drive.Name)) != prefix || drive.Factory == nil {
			continue
		}
		newVFS := drive.Factory()
		if newVFS == nil {
			return false
		}
		pf.SwitchToVFS(fsp, newVFS)
		return true
	}

	for _, drive := range sysinfo.GetPlatformDrives() {
		if platformDriveCommandPrefix(drive.Name) != prefix || drive.Factory == nil {
			continue
		}
		newVFS := drive.Factory()
		if newVFS == nil {
			return false
		}
		pf.SwitchToVFS(fsp, newVFS)
		return true
	}
	return false
}

// platformDriveCommandPrefix exposes the short command-line aliases for
// platform drives that cannot use their menu labels as bare prefixes.
func platformDriveCommandPrefix(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "windows registry":
		return "reg"
	default:
		return ""
	}
}
