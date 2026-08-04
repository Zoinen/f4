package luaplug

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingHost struct {
	mu      sync.Mutex
	calls   []string
	params  []any
	replies map[string]any
	fail    map[string]error
}

func newRecordingHost() *recordingHost {
	return &recordingHost{replies: map[string]any{}, fail: map[string]error{}}
}

func (h *recordingHost) Call(method string, params any) (any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, method)
	h.params = append(h.params, params)
	if err, ok := h.fail[method]; ok {
		return nil, err
	}
	return h.replies[method], nil
}

func (h *recordingHost) methods() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.calls...)
}

func newTestRuntime(t *testing.T, host Host) *Runtime {
	t.Helper()
	r, err := New(Options{Name: "test", Host: host, CallTimeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestRegisterAndDispatch(t *testing.T) {
	r := newTestRuntime(t, nil)

	err := r.LoadString("plugin", `
		local f4rpc = require('f4rpc')
		f4rpc.register("Plugin.Init", function()
			return { Drives = { "Lua Virtual Drive" } }
		end)
		f4rpc.register("VFS.Stat", function(req)
			return { Name = req.Path, IsDir = false, Size = 12 }
		end)
		f4rpc.serve()
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	if got := r.Methods(); !reflect.DeepEqual(got, []string{"Plugin.Init", "VFS.Stat"}) {
		t.Fatalf("Methods = %v", got)
	}
	if !r.HasHandler("Plugin.Init") || r.HasHandler("VFS.Nope") {
		t.Fatal("HasHandler disagrees with Methods")
	}

	init, err := r.Dispatch("Plugin.Init", nil)
	if err != nil {
		t.Fatalf("Dispatch(Plugin.Init): %v", err)
	}
	want := map[string]any{"Drives": []any{"Lua Virtual Drive"}}
	if !reflect.DeepEqual(init, want) {
		t.Fatalf("Plugin.Init = %#v, want %#v", init, want)
	}

	stat, err := r.Dispatch("VFS.Stat", map[string]any{"Path": "a.txt"})
	if err != nil {
		t.Fatalf("Dispatch(VFS.Stat): %v", err)
	}
	wantStat := map[string]any{"Name": "a.txt", "IsDir": false, "Size": int64(12)}
	if !reflect.DeepEqual(stat, wantStat) {
		t.Fatalf("VFS.Stat = %#v, want %#v", stat, wantStat)
	}
}

func TestDispatchUnknownMethod(t *testing.T) {
	r := newTestRuntime(t, nil)
	if err := r.LoadString("plugin", "return true"); err != nil {
		t.Fatalf("LoadString: %v", err)
	}
	if _, err := r.Dispatch("VFS.ReadDir", nil); err == nil {
		t.Fatal("dispatching an unregistered method succeeded")
	}
}

func TestDispatchPropagatesLuaError(t *testing.T) {
	r := newTestRuntime(t, nil)
	err := r.LoadString("plugin", `
		require('f4rpc').register("VFS.Stat", function() error("file not found") end)
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}
	_, err = r.Dispatch("VFS.Stat", nil)
	if err == nil || !strings.Contains(err.Error(), "file not found") {
		t.Fatalf("Dispatch error = %v, want the Lua message", err)
	}
}

func TestHostCallFromLua(t *testing.T) {
	host := newRecordingHost()
	host.replies["Host.GetVersion"] = "4.0-test"
	r := newTestRuntime(t, host)

	err := r.LoadString("plugin", `
		local f4rpc = require('f4rpc')
		f4rpc.call("Host.Log", "starting up")
		version = f4rpc.call("Host.GetVersion")
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	if got := host.methods(); !reflect.DeepEqual(got, []string{"Host.Log", "Host.GetVersion"}) {
		t.Fatalf("host saw %v", got)
	}
	if host.params[0] != "starting up" {
		t.Fatalf("Host.Log received %#v", host.params[0])
	}

	var version any
	if err := r.Do(func(L *luaState) error {
		version = fromLua(L.GetGlobal("version"))
		return nil
	}); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if version != "4.0-test" {
		t.Fatalf("host reply reached Lua as %#v", version)
	}
}

func TestHostErrorReachesLua(t *testing.T) {
	host := newRecordingHost()
	host.fail["Host.Message"] = errors.New("no ui attached")
	r := newTestRuntime(t, host)

	err := r.LoadString("plugin", `
		local ok, msg = pcall(function() require('f4rpc').call("Host.Message", "hi") end)
		failed = not ok
		reason = tostring(msg)
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	var failed, reason any
	_ = r.Do(func(L *luaState) error {
		failed = fromLua(L.GetGlobal("failed"))
		reason = fromLua(L.GetGlobal("reason"))
		return nil
	})
	if failed != true {
		t.Fatal("host error did not raise in Lua")
	}
	if text, _ := reason.(string); !strings.Contains(text, "no ui attached") {
		t.Fatalf("Lua saw %q", reason)
	}
}

func TestPrintGoesToHostLog(t *testing.T) {
	host := newRecordingHost()
	r := newTestRuntime(t, host)

	if err := r.LoadString("plugin", `print("hello", 42)`); err != nil {
		t.Fatalf("LoadString: %v", err)
	}
	if got := host.methods(); !reflect.DeepEqual(got, []string{"Host.Log"}) {
		t.Fatalf("print did not reach the host log: %v", got)
	}
	if text, _ := host.params[0].(string); text != "hello\t42" {
		t.Fatalf("logged %q", text)
	}
}

func TestSandboxHidesUnsafeLibraries(t *testing.T) {
	r := newTestRuntime(t, nil)
	err := r.LoadString("plugin", `
		has_os = os ~= nil
		has_io = io ~= nil
		has_string = string ~= nil
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	var hasOS, hasIO, hasString any
	_ = r.Do(func(L *luaState) error {
		hasOS = fromLua(L.GetGlobal("has_os"))
		hasIO = fromLua(L.GetGlobal("has_io"))
		hasString = fromLua(L.GetGlobal("has_string"))
		return nil
	})
	if hasOS != false || hasIO != false {
		t.Fatalf("os/io are reachable by default: os=%v io=%v", hasOS, hasIO)
	}
	if hasString != true {
		t.Fatal("string library is missing")
	}
}

func TestUnsafeStdlibOptIn(t *testing.T) {
	r, err := New(Options{Name: "test", AllowUnsafeStdlib: true})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	if err := r.LoadString("plugin", "has_os = os ~= nil"); err != nil {
		t.Fatalf("LoadString: %v", err)
	}
	var hasOS any
	_ = r.Do(func(L *luaState) error {
		hasOS = fromLua(L.GetGlobal("has_os"))
		return nil
	})
	if hasOS != true {
		t.Fatal("AllowUnsafeStdlib did not open the os library")
	}
}

func TestRuntimeTimeout(t *testing.T) {
	r, err := New(Options{Name: "test", CallTimeout: 150 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.Close()

	start := time.Now()
	err = r.LoadString("spin", "while true do end")
	if err == nil {
		t.Fatal("an endless loop was allowed to run to completion")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("timeout took %v to fire", elapsed)
	}
}

func TestLoadStringSkipsShebang(t *testing.T) {
	r := newTestRuntime(t, nil)
	err := r.LoadString("plugin", "#!/usr/bin/env lua\nrequire('f4rpc')")
	if err != nil {
		t.Fatalf("LoadString with shebang failed: %v", err)
	}
}
func TestLoadFileAddsPackagePath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "helper.lua"), []byte("return { answer = 42 }"), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	main := filepath.Join(dir, "plugin.lua")
	if err := os.WriteFile(main, []byte("answer = require('helper').answer"), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	r := newTestRuntime(t, nil)
	if err := r.LoadFile(main); err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	var answer any
	_ = r.Do(func(L *luaState) error {
		answer = fromLua(L.GetGlobal("answer"))
		return nil
	})
	if answer != int64(42) {
		t.Fatalf("required module returned %#v", answer)
	}
}

func TestConcurrentDispatch(t *testing.T) {
	r := newTestRuntime(t, nil)
	err := r.LoadString("plugin", `
		local n = 0
		require('f4rpc').register("Plugin.Bump", function()
			n = n + 1
			return n
		end)
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	const workers = 8
	const rounds = 25
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rounds; j++ {
				if _, err := r.Dispatch("Plugin.Bump", nil); err != nil {
					t.Errorf("Dispatch: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	final, err := r.Dispatch("Plugin.Bump", nil)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if final != int64(workers*rounds+1) {
		t.Fatalf("counter = %#v, want %d", final, workers*rounds+1)
	}
}

func TestClosedRuntimeRejectsWork(t *testing.T) {
	r, err := New(Options{Name: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := r.LoadString("plugin", "return 1"); !errors.Is(err, ErrClosed) {
		t.Fatalf("LoadString after Close = %v, want ErrClosed", err)
	}
}
