package luaplug

import (
	"math"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unxed/ffibridge"
	lua "github.com/yuin/gopher-lua"
)

func newFFIRuntime(t *testing.T) *Runtime {
	t.Helper()
	if !ffibridge.Supported {
		t.Skip("ffibridge: FFI is disabled in this build")
	}

	bridge := ffibridge.New(ffibridge.Options{})
	if _, err := bridge.OpenLibC(); err != nil {
		_ = bridge.Close() // bridge cleanup cannot affect a skipped test
		t.Skipf("no system C library available: %v", err)
	}

	r, err := New(Options{Name: "ffi test", FFI: bridge, CallTimeout: 5 * time.Second})
	if err != nil {
		_ = bridge.Close() // bridge cleanup cannot affect the primary failure
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Errorf("close runtime: %v", err)
		}
		if err := bridge.Close(); err != nil {
			t.Errorf("close FFI bridge: %v", err)
		}
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

// TestCheckAddr exercises checkAddr directly, without a bridge: it only
// needs a Lua argument on the stack, valid or not. Invalid addresses raise a
// Lua error (L.ArgError), which requires an active call frame to unwind
// cleanly, so the check runs through a real Lua call rather than calling
// checkAddr on a bare, call-less state.
func TestCheckAddr(t *testing.T) {
	// largeAddr is a high address that still fits in uintptr and is exactly
	// representable as a float64 on every pointer width: 1<<62 on 64-bit
	// targets, 1<<30 on 32-bit ones (linux/arm, windows/386, ...).
	const largeAddr = uintptr(1) << (strconv.IntSize - 2)
	cases := []struct {
		name    string
		arg     lua.LNumber
		wantErr bool
		want    uintptr
	}{
		{"zero", 0, false, 0},
		{"ordinary address", 4096, false, 4096},
		{"large exact address", lua.LNumber(largeAddr), false, largeAddr},
		{"negative", -1, true, 0},
		{"nan", lua.LNumber(math.NaN()), true, 0},
		{"positive infinity", lua.LNumber(math.Inf(1)), true, 0},
		{"fractional", 1.5, true, 0},
		{"far beyond any pointer width", lua.LNumber(1e20), true, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L := lua.NewState(lua.Options{SkipOpenLibs: true})
			defer L.Close()

			var got uintptr
			fn := L.NewFunction(func(L *lua.LState) int {
				got = checkAddr(L, 1)
				return 0
			})
			L.Push(fn)
			L.Push(tc.arg)
			err := L.PCall(1, 0, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("checkAddr(%v) did not raise an error", tc.arg)
				}
				return
			}
			if err != nil {
				t.Fatalf("checkAddr(%v): %v", tc.arg, err)
			}
			if got != tc.want {
				t.Errorf("checkAddr(%v) = %#x, want %#x", tc.arg, got, tc.want)
			}
		})
	}
}

// TestWidenCallbackSignature checks the Windows-only narrow-to-ptr widening
// that ffiCallback relies on. Off Windows, widenCallbackSignature always
// takes its early return and must hand the signature back unchanged; on
// Windows it must widen every narrow slot and remember each one's original
// kind so the callback trampoline can narrow the value back afterwards.
func TestWidenCallbackSignature(t *testing.T) {
	cases := []struct {
		name         string
		sig          string
		windowsSig   string
		windowsKinds []ffibridge.Kind
	}{
		{
			name:         "no narrow slots stay unchanged even on windows",
			sig:          "i64(ptr,i64)",
			windowsSig:   "i64(ptr,i64)",
			windowsKinds: nil,
		},
		{
			name:         "narrow return and args widen to ptr on windows",
			sig:          "i32(i8,bool,i64)",
			windowsSig:   "ptr(ptr,ptr,i64)",
			windowsKinds: []ffibridge.Kind{ffibridge.KindI8, ffibridge.KindBool, ffibridge.KindVoid},
		},
		{
			name:         "unparsable signature is returned unchanged",
			sig:          "not a signature",
			windowsSig:   "not a signature",
			windowsKinds: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSig, gotKinds := widenCallbackSignature(tc.sig)
			if runtime.GOOS != "windows" {
				if gotSig != tc.sig {
					t.Errorf("signature = %q, want unchanged %q on %s", gotSig, tc.sig, runtime.GOOS)
				}
				if gotKinds != nil {
					t.Errorf("kinds = %v, want nil on %s", gotKinds, runtime.GOOS)
				}
				return
			}
			if gotSig != tc.windowsSig {
				t.Errorf("signature = %q, want %q", gotSig, tc.windowsSig)
			}
			if !reflect.DeepEqual(gotKinds, tc.windowsKinds) {
				t.Errorf("kinds = %v, want %v", gotKinds, tc.windowsKinds)
			}
		})
	}
}

// TestNarrowCallbackValue covers every kind narrowCallbackValue knows how to
// narrow, plus its two fallback paths: a kind it does not recognize (the
// value passes through unchanged) and a value that is not actually the
// uintptr the ABI promised (treated as the zero value rather than panicking).
func TestNarrowCallbackValue(t *testing.T) {
	cases := []struct {
		name string
		kind ffibridge.Kind
		in   any
		want any
	}{
		{"i8 reinterprets the low byte as signed", ffibridge.KindI8, uintptr(0xFF), int8(-1)},
		{"u8 keeps the low byte", ffibridge.KindU8, uintptr(0x1FF), uint8(0xFF)},
		{"i16 reinterprets the low 16 bits as signed", ffibridge.KindI16, uintptr(0xFFFF), int16(-1)},
		{"u16 keeps the low 16 bits", ffibridge.KindU16, uintptr(0x1FFFF), uint16(0xFFFF)},
		{"i32 reinterprets the low 32 bits as signed", ffibridge.KindI32, uintptr(0xFFFFFFFF), int32(-1)},
		{"u32 keeps the low 32 bits", ffibridge.KindU32, uintptr(0xFFFFFFFF), uint32(0xFFFFFFFF)},
		{"bool true", ffibridge.KindBool, uintptr(1), true},
		{"bool false", ffibridge.KindBool, uintptr(0), false},
		{"a kind that was never widened passes through unchanged", ffibridge.KindI64, uintptr(42), uintptr(42)},
		{"a non-uintptr value for a narrow kind is treated as zero", ffibridge.KindI8, "not-a-pointer", int8(0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := narrowCallbackValue(tc.kind, tc.in); got != tc.want {
				t.Errorf("narrowCallbackValue(%v, %#v) = %#v, want %#v", tc.kind, tc.in, got, tc.want)
			}
		})
	}
}
