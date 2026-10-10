package multiarc

import (
	"fmt"
	"os"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// tempDirs tracks every directory Open extracted a member into (vfs.go),
// so the plugin can clean them up on Close instead of leaving them for the
// OS's own temp-directory housekeeping. Removing one earlier, on the
// MultiArcVFS.Close of whichever clone happened to extract it, would be
// wrong: a viewer or editor session, or another clone's directory listing
// racing it, can still be reading the extracted file.
var (
	tempDirsMu sync.Mutex
	tempDirs   []string
)

func registerTempDir(dir string) {
	tempDirsMu.Lock()
	tempDirs = append(tempDirs, dir)
	tempDirsMu.Unlock()
}

func closeSharedMultiArcTempDirs() {
	tempDirsMu.Lock()
	dirs := tempDirs
	tempDirs = nil
	tempDirsMu.Unlock()
	for _, dir := range dirs {
		_ = os.RemoveAll(dir)
	}
}

// Plugin registers multiarc's VFS provider and its Add to archive command.
// It is the lite build's replacement for plugins/archive.ArchivePlugin
// (f4#1178, part 2): see plugins/multiarc's own doc comment and Provider's
// for what it covers and why it is scoped to local-disk archives only.
type Plugin struct {
	registrations []vfs.Registration
}

// Init registers the command first, the way plugins/archive does: if the
// host refuses it, Init fails before the provider or the hotkey exists, so
// nothing is left half-registered.
func (p *Plugin) Init(api vfs.HostAPI) error {
	if contributions, ok := api.(vfs.ContributionHost); ok {
		registration, err := contributions.RegisterPluginCommand(addCommand())
		if err != nil {
			return fmt.Errorf("multiarc: register add command: %w", err)
		}
		p.registrations = append(p.registrations, registration)
	}
	api.RegisterVFSProvider(&Provider{})
	// far2l's Files-menu shortcut, the same one the regular build binds.
	api.RegisterGlobalHotkey(vtinput.VK_F1, vtinput.ShiftPressed, actionAddArchive)
	return nil
}

func (p *Plugin) Close() error {
	registrations := p.registrations
	p.registrations = nil
	for i := len(registrations) - 1; i >= 0; i-- {
		registrations[i].Unregister()
	}
	closeSharedMultiArcTempDirs()
	return nil
}

func (p *Plugin) GetName() string { return "MultiArc (CLI archivers)" }
