package luaplug

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/ffibridge"
)

func newFFIRuntime(t *testing.T) *Runtime {
	t.Helper()
	if !ffibridge.Supported {
		t.Skip("ffibridge: FFI is disabled in this build")
	}

	bridge := ffibridge.New(ffibridge.Options{})
	if _, err := bridge.OpenLibC(); err != nil {
		bridge.Close()
		t.Skipf("no system C library available: %v", err)
	}

	r, err := New(Options{Name: "ffi test", FFI: bridge, CallTimeout: 5 * time.Second})
	if err != nil {
		bridge.Close()
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		_ = r.Close()
		bridge.Close()
	})
	return r
}

func TestFFIIsAbsentWithoutABridge(t *testing.T) {
	r := newTestRuntime(t, nil)
	if err := r.LoadString("plugin", `
		local ffi = require('f4ffi')
		supported = ffi.supported
		has_call = ffi.call ~= nil
	`); err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	var supported, hasCall any
	_ = r.Do(func(L *luaState) error {
		supported = fromLua(L.GetGlobal("supported"))
		hasCall = fromLua(L.GetGlobal("has_call"))
		return nil
	})
	if supported != false {
		t.Error("f4ffi claims to be supported without a bridge")
	}
	if hasCall != false {
		t.Error("f4ffi exposes call without a bridge")
	}
}

func TestFFICallFromLua(t *testing.T) {
	r := newFFIRuntime(t)

	err := r.LoadString("plugin", `
		local ffi = require('f4ffi')
		local libc = ffi.openlibc()
		length = ffi.callsym(libc, "strlen", "i64(str)", "hello")
		value = ffi.callsym(libc, "atof", "f64(str)", "2.5")
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	var length, value any
	_ = r.Do(func(L *luaState) error {
		length = fromLua(L.GetGlobal("length"))
		value = fromLua(L.GetGlobal("value"))
		return nil
	})
	if length != int64(5) {
		t.Errorf("strlen returned %#v, want 5", length)
	}
	if value != 2.5 {
		t.Errorf("atof returned %#v, want 2.5", value)
	}
}

func TestFFIMemoryFromLua(t *testing.T) {
	r := newFFIRuntime(t)

	err := r.LoadString("plugin", `
		local ffi = require('f4ffi')
		local libc = ffi.openlibc()
		local src = ffi.cstring("payload")
		local dst = ffi.alloc(8)
		ffi.callsym(libc, "memcpy", "ptr(ptr,ptr,i64)", dst, src, 8)
		copied = ffi.tostring(dst)
		ffi.free(src)
		ffi.free(dst)
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	var copied any
	_ = r.Do(func(L *luaState) error {
		copied = fromLua(L.GetGlobal("copied"))
		return nil
	})
	if copied != "payload" {
		t.Fatalf("memcpy through Lua produced %#v", copied)
	}
}

func TestFFICallbackReentersLua(t *testing.T) {
	r := newFFIRuntime(t)

	// Calling the trampoline through the bridge re-enters Lua on the very
	// goroutine that is already inside the interpreter, which is the case that
	// would deadlock if Do queued the work instead of running it inline.
	err := r.LoadString("plugin", `
		local ffi = require('f4ffi')
		local calls = 0
		local adder = ffi.callback("i32(i32,i32)", function(a, b)
			calls = calls + 1
			return a + b
		end)
		total = ffi.call(adder, "i32(i32,i32)", 20, 22)
		invocations = calls
	`)
	if err != nil {
		t.Fatalf("LoadString: %v", err)
	}

	var total, invocations any
	_ = r.Do(func(L *luaState) error {
		total = fromLua(L.GetGlobal("total"))
		invocations = fromLua(L.GetGlobal("invocations"))
		return nil
	})
	if total != int64(42) {
		t.Errorf("callback returned %#v, want 42", total)
	}
	if invocations != int64(1) {
		t.Errorf("callback ran %#v times, want 1", invocations)
	}
}

func TestFFIErrorsArePcallable(t *testing.T) {
	r := newFFIRuntime(t)

	err := r.LoadString("plugin", `
		local ffi = require('f4ffi')
		local libc = ffi.openlibc()
		local ok, msg = pcall(function()
			ffi.callsym(libc, "f4_no_such_symbol_here", "i32()")
		end)
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
		t.Fatal("a missing symbol did not raise in Lua")
	}
	if text, _ := reason.(string); !strings.Contains(text, "f4_no_such_symbol_here") {
		t.Fatalf("Lua saw %q", reason)
	}
}
