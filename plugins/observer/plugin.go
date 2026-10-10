package observer

import (
	"path/filepath"

	"github.com/unxed/f4/vfs"
)

// Plugin registers this package's Provider (provider.go) with the host. It
// needs no panel commands or hotkeys yet -- browsing and reading an
// Observer-backed container through the VFS it registers, now driven by
// observer.ini/observer_user.ini when either exists (config.go), is all
// this does so far. An explicit "Open with Observer module..." command (for
// modules with an empty filter, mirroring Observer's own F11 behavior)
// remains a later, separate part (see status/1563.md in the accounting
// repository).
//
// Unlike plugins/archive.ArchivePlugin and plugins/multiarc.Plugin, this one
// is registered from both build tags (internal/plughost/manager.go), not
// from optionalVFSPlugins: an Observer module is a WASI reactor run through
// wazero, the same dependency internal/plughost/transport_wazero.go already
// pulls into both builds for the generic wasm plugin transport (unlike
// internal/editor's own colorer4go integration, which colorer_lite.go does
// cut from lite -- see cmd/f4/lite_deps_test.go, which asserts wazero itself
// stays in a lite build precisely because that transport needs it there
// too). So there is nothing here a small build needs to cut, and the
// ticket's own design explicitly calls for Observer-backed formats to work
// in lite too (plugins/archive is not there to cover them).
type Plugin struct {
	provider *Provider
}

// NewPlugin builds a Plugin whose modules directory is
// filepath.Join(configDir, "observer", "modules") -- see Provider's own doc
// comment for why a module lives on disk rather than in the f4 binary.
func NewPlugin(configDir string) *Plugin {
	return &Plugin{provider: NewProvider(filepath.Join(configDir, "observer", "modules"))}
}

func (p *Plugin) GetName() string { return "Observer" }

func (p *Plugin) Init(api vfs.HostAPI) error {
	api.RegisterVFSProvider(p.provider)
	return nil
}

func (p *Plugin) Close() error { return nil }
