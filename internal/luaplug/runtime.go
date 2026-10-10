// Package luaplug runs f4 plugins written in Lua inside the f4 process.
//
// The point of embedding an interpreter is distribution: a Lua plugin that
// needs a system Lua, a MessagePack rock and a working stdio pipe is a plugin
// most users will never manage to install. Embedded, it is just a file.
//
// The plugin API is deliberately identical to the out-of-process one. A plugin
// still writes f4rpc.register("VFS.ReadDir", ...) and f4rpc.call("Host.Log",
// ...); only the transport underneath differs, so the same script runs either
// way and the SDK, the documentation and the host method names stay singular.
package luaplug

import (
	"context"
	"errors"
	"fmt"
	"github.com/unxed/ffibridge"
	lua "github.com/yuin/gopher-lua"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultCallTimeout bounds one entry into the interpreter.
const DefaultCallTimeout = 30 * time.Second

// ErrClosed is returned once the runtime has been shut down.
var ErrClosed = errors.New("luaplug: runtime is closed")

// ErrInterrupted is returned by every entry into a runtime whose earlier call
// hit its deadline. The interpreter was stopped at an arbitrary instruction --
// halfway through updating a table, say -- so its state cannot be trusted and
// the runtime is discarded rather than reused; the caller starts a new one.
var ErrInterrupted = errors.New("luaplug: the runtime was interrupted by its call deadline and must be discarded")

// Host is the plugin's view of f4. Method names are the F4-RPC ones, so an
// embedded plugin and a subprocess plugin talk to the same surface.
type Host interface {
	Call(method string, params any) (any, error)
}

// HostFunc adapts a plain function to Host.
type HostFunc func(method string, params any) (any, error)

// Call implements Host.
func (f HostFunc) Call(method string, params any) (any, error) { return f(method, params) }

// Options configures one runtime.
type Options struct {
	// Name labels the runtime in error messages and chunk names.
	Name string

	// Host receives everything the plugin sends through f4rpc.call.
	Host Host

	// FFI, when not nil, exposes the f4ffi module to the plugin. The caller
	// owns the bridge and is responsible for closing it.
	FFI *ffibridge.Bridge

	// AllowUnsafeStdlib additionally opens the io and os libraries. Both can
	// write to the terminal f4 is drawing on and os can spawn processes, so
	// this is off unless a plugin has been granted it.
	AllowUnsafeStdlib bool

	// CallTimeout bounds one entry into the interpreter. Zero means
	// DefaultCallTimeout. A runtime that has hit its timeout is discarded
	// rather than reused (ErrInterrupted): the interpreter was interrupted at
	// an arbitrary instruction.
	CallTimeout time.Duration
}

type task struct {
	fn   func(*lua.LState) error
	done chan error
}

// Runtime owns one Lua state and the single goroutine allowed to touch it.
type Runtime struct {
	opts Options

	tasks chan *task
	quit  chan struct{}
	wg    sync.WaitGroup

	workerID    atomic.Int64
	closed      atomic.Bool
	interrupted atomic.Bool

	// deadline is the running outermost call's clock, nil between calls. It
	// is only set and cleared on the worker goroutine; PauseDeadline and
	// ResumeDeadline read it there too.
	deadline *callDeadline
	once     sync.Once

	// The fields below belong to the worker goroutine only.
	state    *lua.LState
	handlers map[string]*lua.LFunction
	depth    int
}

// New starts a runtime with the plugin API installed but nothing loaded yet.
func New(opts Options) (*Runtime, error) {
	r := &Runtime{
		opts:     opts,
		tasks:    make(chan *task),
		quit:     make(chan struct{}),
		handlers: make(map[string]*lua.LFunction),
	}
	if r.opts.Name == "" {
		r.opts.Name = "lua plugin"
	}

	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	r.state = L
	r.openLibs(L)
	r.openF4RPC(L)
	r.openFFI(L)

	started := make(chan struct{})
	r.wg.Add(1)
	go r.worker(started)
	<-started

	return r, nil
}

func (r *Runtime) worker(started chan struct{}) {
	defer r.wg.Done()
	r.workerID.Store(goID())
	close(started)

	defer r.state.Close()

	for {
		select {
		case t := <-r.tasks:
			t.done <- r.run(t.fn)
		case <-r.quit:
			return
		}
	}
}

// run executes one unit of work on the worker goroutine. Only the outermost
// entry installs the deadline: a nested call, such as a native callback
// re-entering Lua, must not restart or drop the deadline of the call it is
// running inside.
func (r *Runtime) run(fn func(*lua.LState) error) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("luaplug: %s panicked: %v", r.opts.Name, rec)
		}
	}()

	if r.depth == 0 {
		timeout := r.opts.CallTimeout
		if timeout <= 0 {
			timeout = DefaultCallTimeout
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		dl := newCallDeadline(timeout, cancel)
		r.deadline = dl
		defer func() { r.deadline = nil }()
		defer dl.stop()
		r.state.SetContext(ctx)
		defer r.state.RemoveContext()
		// Checked whatever the call returned: a plugin can catch the
		// interruption with pcall and carry on, and the state is no better
		// for it.
		defer func() {
			if dl.hit.Load() {
				r.interrupted.Store(true)
				if err == nil {
					err = ErrInterrupted
				} else if !errors.Is(err, ErrInterrupted) {
					err = fmt.Errorf("%w: %v", ErrInterrupted, err)
				}
			}
		}()
	}

	r.depth++
	defer func() { r.depth-- }()

	return fn(r.state)
}

// callDeadline is the time one outermost call may take, as a clock that can
// be stopped while the call waits for something that is not the script's
// doing (a dialog the user has to answer).
type callDeadline struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	timer     *time.Timer
	remaining time.Duration
	started   time.Time
	hit       atomic.Bool
}

func newCallDeadline(timeout time.Duration, cancel context.CancelFunc) *callDeadline {
	d := &callDeadline{cancel: cancel, remaining: timeout}
	d.start()
	return d
}

// start runs the clock for what is left. Callers hold mu, or own d alone.
func (d *callDeadline) start() {
	d.started = time.Now()
	d.timer = time.AfterFunc(d.remaining, func() {
		d.hit.Store(true)
		d.cancel()
	})
}

func (d *callDeadline) stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
		d.timer = nil
	}
}

func (d *callDeadline) pause() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer == nil || !d.timer.Stop() {
		return // not running, or already expired
	}
	d.timer = nil
	d.remaining = max(d.remaining-time.Since(d.started), time.Millisecond)
}

func (d *callDeadline) resume() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil || d.hit.Load() {
		return
	}
	d.start()
}

// WhileWaiting runs fn, which must be called from inside the script (a host
// function the script called), with the call's deadline stopped: time spent
// waiting for the user, or for anything else that is not the script running,
// does not count against it. Outside a call it just runs fn.
func (r *Runtime) WhileWaiting(fn func()) {
	dl := r.deadline
	if dl == nil {
		fn()
		return
	}
	dl.pause()
	defer dl.resume()
	fn()
}

// Do runs fn with exclusive access to the Lua state.
func (r *Runtime) Do(fn func(*lua.LState) error) error {
	if r.closed.Load() {
		return ErrClosed
	}
	if r.interrupted.Load() {
		return ErrInterrupted
	}
	if goID() == r.workerID.Load() {
		return r.run(fn)
	}

	t := &task{fn: fn, done: make(chan error, 1)}
	select {
	case r.tasks <- t:
	case <-r.quit:
		return ErrClosed
	}
	select {
	case err := <-t.done:
		return err
	case <-r.quit:
		return ErrClosed
	}
}

// Interrupted reports whether a call hit its deadline. Such a runtime refuses
// all further work with ErrInterrupted and should be closed and replaced.
func (r *Runtime) Interrupted() bool { return r.interrupted.Load() }

// LoadString compiles and runs a chunk. Running the plugin body is what
// populates its handler table, so this is how a plugin is started.
func (r *Runtime) LoadString(name, source string) error {
	if strings.HasPrefix(source, "#!") {
		source = "--" + source[2:]
	}
	return r.Do(func(L *lua.LState) error {
		fn, err := L.Load(strings.NewReader(source), name)
		if err != nil {
			return err
		}
		L.Push(fn)
		return L.PCall(0, lua.MultRet, nil)
	})
}

// LoadFile runs a plugin from disk, with the plugin's own directory added to
// package.path so that it can require its sibling modules.
func (r *Runtime) LoadFile(path string) error {
	source, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := r.addPackagePath(dir); err != nil {
			return err
		}
	}
	return r.LoadString(path, string(source))
}

func (r *Runtime) addPackagePath(dir string) error {
	return r.Do(func(L *lua.LState) error {
		pkg, ok := L.GetGlobal("package").(*lua.LTable)
		if !ok {
			return nil
		}
		current := lua.LVAsString(pkg.RawGetString("path"))
		prefix := filepath.Join(dir, "?.lua")
		pkg.RawSetString("path", lua.LString(prefix+";"+current))
		return nil
	})
}

// Close shuts the runtime down and releases the Lua state. It must not be
// called from inside Lua, since that would have the worker wait for itself.
func (r *Runtime) Close() error {
	if goID() == r.workerID.Load() {
		return errors.New("luaplug: Close called from inside the interpreter")
	}
	r.once.Do(func() {
		r.closed.Store(true)
		close(r.quit)
		r.wg.Wait()
	})
	return nil
}
