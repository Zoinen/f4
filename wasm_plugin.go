package main

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/ffibridge"
	"github.com/unxed/vtui"
)

// wasmShutdownGrace is how long Close waits for a guest to unwind before
// giving up on it.
const wasmShutdownGrace = 2 * time.Second

// WasmPlugin runs a plugin compiled to WebAssembly, inside the f4 process.
//
// The guest is a WASI command, not a module of exported functions: it reads
// F4-RPC from stdin and writes it to stdout exactly as a subprocess plugin
// does. That choice is the whole design. It means the same plugin source
// builds either to a native binary or to a .wasm with no changes, the existing
// SDKs work untouched, and this transport costs a pair of pipes rather than a
// third plugin ABI.
//
// It is also the first transport that is genuinely a sandbox: the guest is
// given no filesystem at all, only stdio, a clock and a random source.
type WasmPlugin struct {
	path         string
	sess         *f4rpc.Session
	runtime      wazero.Runtime
	cancel       context.CancelFunc
	toGuest      *io.PipeWriter
	done         chan struct{}
	closing      bool
	bridge       *ffibridge.Bridge
	registration vfs.Registration
	// identity is who this plugin is to the permission model, taken from
	// the manifest when it came from the catalog.
	identity PluginIdentity
}

// SetPermissionIdentity passes on who the manifest says this plugin is.
func (p *WasmPlugin) SetPermissionIdentity(identity PluginIdentity) {
	p.identity = identity
}

// permissionIdentity falls back to the path for a plugin registered by hand,
// which has no manifest and therefore no id.
func (p *WasmPlugin) permissionIdentity() PluginIdentity {
	if p.identity.Key == "" {
		return PermissionIdentityForPath(p.path)
	}
	return p.identity
}

// NewWasmPlugin prepares a plugin from a .wasm module.
func NewWasmPlugin(path string) *WasmPlugin {
	return &WasmPlugin{path: path}
}

// IsWasmEntrypoint reports whether an entrypoint is a bare WebAssembly module.
func IsWasmEntrypoint(entrypoint string) bool {
	return isBareEntrypointWithExt(entrypoint, ".wasm")
}

func (p *WasmPlugin) GetName() string {
	return p.path + " (wasm)"
}

func (p *WasmPlugin) Init(api vfs.HostAPI) error {
	module, err := os.ReadFile(p.path)
	if err != nil {
		return err
	}

	// WithCloseOnContextDone is what makes a runaway guest killable: without
	// it wazero never looks at the context while the guest is executing, and
	// Close would have nothing to interrupt.
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel

	runtime := wazero.NewRuntimeWithConfig(ctx,
		wazero.NewRuntimeConfig().WithCloseOnContextDone(true))
	p.runtime = runtime

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, runtime); err != nil {
		p.shutdown()
		return fmt.Errorf("wasi: %w", err)
	}

	compiled, err := runtime.CompileModule(ctx, module)
	if err != nil {
		p.shutdown()
		return fmt.Errorf("compiling %s: %w", p.path, err)
	}

	guestStdin, hostToGuest := io.Pipe()
	hostFromGuest, guestStdout := io.Pipe()
	p.toGuest = hostToGuest

	name := filepath.Base(p.path)
	config := wazero.NewModuleConfig().
		WithName(name).
		WithArgs(name).
		WithStdin(guestStdin).
		WithStdout(guestStdout).
		WithStderr(&wasmLogWriter{name: p.path}).
		WithSysWalltime().
		WithSysNanotime().
		WithRandSource(rand.Reader)

	p.done = make(chan struct{})
	go func() {
		defer close(p.done)
		_, runErr := runtime.InstantiateModule(ctx, compiled, config)

		// Closing both ends unblocks the host session, which would otherwise
		// wait forever on a guest that has exited.
		guestStdout.Close()
		hostToGuest.Close()

		if runErr != nil && !p.closing && !isCleanWasmExit(runErr) {
			vtui.DebugLog("WASM Plugin %q terminated: %v", p.path, runErr)
		}
	}()

	p.sess = f4rpc.NewSession(hostFromGuest, hostToGuest)
	// A wasm guest cannot load a library itself, which is the point of it.
	// The FFI it gets is the host's, projected over the same protocol.
	p.bridge = newGatedFFIBridge(newPluginGate(p.permissionIdentity()))

	registration, err := startPluginSession(p.sess, api, p.path, p.bridge, func(err error) {
		if !p.closing {
			vtui.DebugLog("WASM Plugin %q session ended: %v", p.path, err)
		}
	})
	if err != nil {
		p.Close()
		return err
	}
	p.registration = registration
	return nil
}

func (p *WasmPlugin) Close() error {
	p.closing = true
	if p.registration != nil {
		p.registration.Unregister()
		p.registration = nil
	}

	// Closing the guest's stdin lets a well behaved plugin see EOF and unwind
	// on its own before the runtime is torn down under it.
	if p.toGuest != nil {
		p.toGuest.Close()
	}
	if p.done != nil {
		select {
		case <-p.done:
		case <-time.After(wasmShutdownGrace):
		}
	}
	p.shutdown()
	return nil
}

func (p *WasmPlugin) shutdown() {
	if p.bridge != nil {
		p.bridge.Close()
		p.bridge = nil
	}
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	if p.runtime != nil {
		p.runtime.Close(context.Background())
		p.runtime = nil
	}
}

// isCleanWasmExit reports whether an instantiation error is just the guest
// calling exit(0), which wazero reports as an error like any other.
func isCleanWasmExit(err error) bool {
	var exit *sys.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 0
}

// wasmLogWriter forwards a guest's stderr into the debug log, line buffering
// being unnecessary since DebugLog already writes one entry per call.
type wasmLogWriter struct {
	name string
}

func (w *wasmLogWriter) Write(p []byte) (int, error) {
	vtui.DebugLog("WASM_STDERR [%s]: %s", w.name, string(p))
	return len(p), nil
}
