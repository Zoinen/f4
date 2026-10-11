package observer

// This file is f4#1563's vfs.VFSProvider: a provider that drives Observer
// modules against whatever container each one's own filter names, so that
// Enter on a recognized file in a panel has something behind it at all.
// Part 5 shipped a single hardcoded module (isoimg, see isoimg_e2e_test.go)
// against exactly one family of container (ISO9660 images); part 7 added
// password retry for SOR_PASSWORD_REQUIRED (password.go) -- CanOpen below
// treats it the same as SOR_SUCCESS, recognized but locked, so Enter still
// reaches Open, which is where the user is actually asked, not this cheap
// probe; part 8 closed the ArchiveEnterExcludeMask-shaped gap the design
// named: PanelEnterAllowed below lets ObserverEnterExcludeMask hold ordinary
// Enter back from a recognized container while Ctrl+PgDn keeps opening it,
// the way plugins/archive already does. This part replaces the one
// hardcoded module with moduleEntries (config.go): an ordered list read from
// observer.ini/observer_user.ini when either exists, or the same single
// isoimg/"*.iso" row as before when neither does -- see config.go's own
// package comment for why "several modules for one format" turns out to be
// the same mechanism as "read observer.ini" rather than a second one.
// PlugRing distribution and real modules beyond isoimg remain later, still
// unstarted parts (status/1563.md, accounting repository). What is here is
// real, not a stub: a genuine unmodified isoimg.wasm (built the way
// scripts/build_isoimg_test_wasm.sh already does for the existing
// plugins/observer tests) opens a real ISO image and the resulting tree is
// browsable, which is the whole point of this part.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filemask"
	"github.com/unxed/f4/vfs"
)

// Provider also implements vfs.PanelEnterPolicyProvider (see
// PanelEnterAllowed below), the same optional capability
// plugins/multiarc.Provider and plugins/archive.ArchiveProvider assert for
// themselves.
var _ vfs.PanelEnterPolicyProvider = (*Provider)(nil)

// isoimgModuleFileName is the file name defaultModuleEntries (config.go)
// points at when neither observer.ini nor observer_user.ini exists.
// Observer modules are never embedded in the f4 binary (see doc.go and the
// licensing discussion in f4#1563): a user, or PlugRing once it grows a
// "modules Observer" category, drops a compiled .wasm there themselves. A
// missing file simply means the format is not available yet, exactly the
// way plugins/archive silently ignores a format none of its linked
// libraries understand.
const isoimgModuleFileName = "isoimg.wasm"

// Provider is the vfs.VFSProvider this package registers (see Plugin.Init
// in plugin.go).
type Provider struct {
	modulesDir string

	entriesOnce sync.Once
	entries     []moduleEntry

	mu    sync.Mutex
	bytes map[string][]byte // moduleEntry.FileName -> file contents; a present key with a nil value is a cached "not installed"
}

// NewProvider builds a Provider whose modules live under modulesDir --
// which module names it actually tries against a given file comes from
// moduleEntries (config.go), read lazily from
// filepath.Dir(modulesDir)/observer{,_user}.ini on first use.
func NewProvider(modulesDir string) *Provider {
	return &Provider{modulesDir: modulesDir}
}

// moduleEntries lazily loads and caches this Provider's ordered module list
// (config.go) -- once per Provider, the same one-shot cost isoimgBytes used
// to accept before this part, now for the config read instead of the wasm
// file read.
func (p *Provider) moduleEntries() []moduleEntry {
	p.entriesOnce.Do(func() {
		p.entries = loadModuleEntries(filepath.Dir(p.modulesDir))
	})
	return p.entries
}

// moduleBytes lazily reads and caches modulesDir/fileName. A read failure
// (most commonly: not installed) is cached as "no module" (a present map key
// with a nil value) rather than retried on every CanOpen -- the same
// one-shot cost LoadModule's own sharedCompilationCache accepts for a
// compiled module, here for the raw file bytes instead.
func (p *Provider) moduleBytes(fileName string) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bytes == nil {
		p.bytes = make(map[string][]byte)
	}
	if b, ok := p.bytes[fileName]; ok {
		return b
	}
	// #nosec G304 -- fileName comes from this Provider's own
	// observer.ini/observer_user.ini [Modules] section (or the hardcoded
	// default), resolved only against this Provider's own modulesDir, never
	// from panel or user path input.
	b, err := os.ReadFile(filepath.Join(p.modulesDir, fileName))
	if err != nil {
		b = nil
	}
	p.bytes[fileName] = b
	return b
}

// entryMatches reports whether name is claimed by entry's own [Filters]
// glob. An empty Filter never matches through a file name alone -- see
// moduleEntry's own doc comment in config.go for why that is upstream's own
// behaviour for a module like this, not a gap this part left open by
// accident.
func entryMatches(entry moduleEntry, name string) bool {
	if entry.Filter == "" {
		return false
	}
	return filemask.Match(name, entry.Filter, true)
}

func (p *Provider) Name() string { return "observer" }

// Priority mirrors plugins/archive.ArchiveProvider's own comment (higher
// polled sooner, archives usually low): actual polling order still follows
// registration order (vfs.FindProvider walks providerRegistry.items in the
// order RegisterProvider saw them, see vfs/vfs.go), and
// internal/plughost/manager.go registers this package after
// plugins/archive/plugins/multiarc for exactly that reason, matching the
// ticket's own "register after archive" requirement. Priority is set lower
// than ArchiveProvider's 10 only to document that intent for whenever
// vfs.RegisterProvider starts actually sorting by it.
func (p *Provider) Priority() int { return 5 }

// baseName returns parent.Base(path), falling back to path itself for a nil
// parent -- defensive the way plugins/archive.ArchiveProvider.CanOpen is,
// since a real caller always supplies one.
func baseName(parent vfs.VFS, path string) string {
	if parent == nil {
		return path
	}
	if base := parent.Base(path); base != "" {
		return base
	}
	return path
}

// PanelEnterAllowed is ArchiveProvider.PanelEnterAllowed's counterpart for
// Observer containers (f4#1563): ObserverEnterExcludeMask names files Enter
// must leave to their extension association even though a Provider module
// recognizes them, and Ctrl+PgDn keeps opening them regardless, the same
// deliberate escape hatch plugins/archive gives self-extracting archives and
// masked documents. Unlike ArchiveProvider there is no self-extracting-exe
// case to also guard here: isoimg, the only module Provider drives out of
// the box (see config.go's defaultModuleEntries), never is itself a program
// Enter would otherwise run, so the mask is the whole policy -- a
// self-extracting-installer module added later through observer.ini would
// need this reconsidered, the same way plugins/archive itself does for SFX.
func (p *Provider) PanelEnterAllowed(ctx context.Context, parent vfs.VFS, path string) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if _, isLocal := parent.(*vfs.OSVFS); !isLocal {
		// Off the local file system there is no association and no system
		// opener for Enter to be held back in favour of, exactly the
		// reasoning ArchiveProvider.PanelEnterAllowed uses for the same
		// case.
		return true
	}
	return !observerEnterBarredByMask(baseName(parent, path))
}

// observerEnterBarredByMask reports whether the configured mask claims this
// name for its association, mirroring plugins/archive.enterBarredByMask.
// An empty mask (the default -- see config.ObserverEnterExcludeMask) bars
// nothing, which is the setting a user writes when they want Enter to follow
// the content and only the content.
func observerEnterBarredByMask(name string) bool {
	mask := strings.TrimSpace(config.App.ObserverEnterExcludeMask)
	if mask == "" {
		return false
	}
	return filemask.Match(name, mask, true)
}

// CanOpen tries moduleEntries (config.go) in order and reports true on the
// first one whose own [Filters] glob claims name and whose module (once
// installed) actually recognizes the file, mirroring
// Observer's own ModulesController::OpenStorageFile ("call OpenStorage on
// each configured module in turn, stop at the first that is not
// SOR_INVALID_FILE") -- with several entries this is genuine module
// selection, not just a fallback list; with the single default entry it is
// exactly what parts 5-8 already did.
func (p *Provider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if parent == nil {
		return false
	}
	name := baseName(parent, path)
	for _, entry := range p.moduleEntries() {
		if !entryMatches(entry, name) {
			continue
		}
		wasmBytes := p.moduleBytes(entry.FileName)
		if wasmBytes == nil {
			continue
		}
		if ok, err := probeModule(ctx, parent, path, wasmBytes, entry.Settings); err == nil && ok {
			return true
		}
	}
	return false
}

// probeModule drives just enough of the ABI (LoadModule, LoadSubModule,
// OpenStorage) to answer "does this module recognize this file", then tears
// everything down -- CanOpen has no tree to keep and no ExtractItem to make,
// so it does not pay for WithExtractDir the way newObserverVFS's real Open
// does. It never passes a password: SOR_PASSWORD_REQUIRED counts as
// recognized here too (see below), so CanOpen stays a cheap, non-interactive
// probe and never itself pops the password dialog -- newObserverVFS's
// openStorageWithPasswordPrompt (password.go) is what actually asks, once
// Open is called.
func probeModule(ctx context.Context, parent vfs.VFS, path string, wasmBytes []byte, settings string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ra, err := parent.Open(ctx, path)
	if err != nil {
		return false, err
	}
	defer func() { _ = ra.Close() }()

	guestName := baseName(parent, path)
	if guestName == "" {
		guestName = "target"
	}

	modCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	mount := NewSingleFileFS(modCtx, guestName, ra)
	mod, err := LoadModule(modCtx, wasmBytes, mount, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = mod.Close() }()

	if _, err := mod.LoadSubModule(settings); err != nil {
		return false, err
	}
	res, err := mod.OpenStorage(StorageOpenParams{FilePath: "/" + guestName})
	if err != nil {
		return false, err
	}
	switch res.Code {
	case SORSuccess:
		_ = mod.CloseStorage(res.Storage)
		return true, nil
	case SORPasswordRequired:
		// Recognized, just locked -- there is no open storage handle to
		// close here (OpenStorage never got past the password check), and
		// nothing to ask the user: that is Open's job, not this probe's.
		return true, nil
	default:
		return false, nil
	}
}

// Open tries moduleEntries (config.go) in the same order and with the same
// [Filters] gate as CanOpen, building a real ObserverVFS (LoadModule with a
// real extract directory this time, LoadSubModule, OpenStorage, then a
// complete GetItem walk -- see vfs.go) on the first entry that both matches
// and successfully opens. A matching entry whose module file is not
// installed is skipped, not treated as a hard error, exactly like CanOpen;
// the error returned distinguishes that case (named in the message, the
// same wording part 5 already used for the single hardcoded module) from an
// installed module actively refusing the file.
func (p *Provider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	name := baseName(parent, path)
	var missing []string
	var lastErr error
	for _, entry := range p.moduleEntries() {
		if !entryMatches(entry, name) {
			continue
		}
		wasmBytes := p.moduleBytes(entry.FileName)
		if wasmBytes == nil {
			missing = append(missing, entry.FileName)
			continue
		}
		v, err := newObserverVFS(ctx, parent, path, wasmBytes, entry.Settings, "")
		if err == nil {
			return v, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("observer: %s is not installed in %s", strings.Join(missing, ", "), p.modulesDir)
	}
	return nil, fmt.Errorf("observer: no configured module recognizes %s", name)
}
