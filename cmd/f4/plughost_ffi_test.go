package main

import (
	"strings"
	"testing"

	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/ffibridge"
	"github.com/vmihailenco/msgpack/v5"
)

// ffiTestTransport answers the calls the host makes back into a plugin.
type ffiTestTransport struct {
	handler func(method string, params any, result any) error
}

func (t *ffiTestTransport) Call(method string, params any, result any) error {
	if t.handler == nil {
		return nil
	}
	return t.handler(method, params, result)
}

func newFFITestMethods(t *testing.T, back PluginTransport) (map[string]f4rpc.Handler, *ffibridge.Bridge) {
	t.Helper()
	if !ffibridge.Supported {
		t.Skip("ffibridge: FFI is disabled in this build")
	}

	bridge := ffibridge.New(ffibridge.Options{})
	if _, err := bridge.OpenLibC(); err != nil {
		_ = bridge.Close() // Cleanup is secondary when the host library is unavailable.
		t.Skipf("no system C library available: %v", err)
	}
	t.Cleanup(func() {
		if err := bridge.Close(); err != nil {
			t.Errorf("close FFI bridge: %v", err)
		}
	})

	return newFFIHostMethods(bridge, back), bridge
}

// callFFI invokes one Host.FFI.* method the way a plugin would.
func callFFI(t *testing.T, methods map[string]f4rpc.Handler, method string, req FFIReq) (FFIRes, error) {
	t.Helper()

	handler, ok := methods[method]
	if !ok {
		t.Fatalf("method %q is not registered", method)
	}

	encoded, err := msgpack.Marshal(req)
	if err != nil {
		t.Fatalf("encoding request: %v", err)
	}

	raw, callErr := handler(encoded)
	if callErr != nil {
		return FFIRes{}, callErr
	}

	// Round trip through MessagePack so the test sees exactly what a plugin
	// would, including any type that fails to encode.
	wire, err := msgpack.Marshal(raw)
	if err != nil {
		t.Fatalf("encoding response: %v", err)
	}
	var res FFIRes
	if err := msgpack.Unmarshal(wire, &res); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	return res, nil
}

// ffiNumber reads a number out of a wire value. MessagePack encodes small
// integers compactly, so a result the bridge produced as int64 comes back as
// int8; asserting on the Go type would test the encoder rather than the
// bridge.
func ffiNumber(t *testing.T, value any) int64 {
	t.Helper()
	switch n := value.(type) {
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		return testInt64Uint64(n)
	case uint:
		return testInt64Uint(n)
	case float64:
		return int64(n)
	}
	t.Fatalf("expected a number, got %T", value)
	return 0
}

func TestFFIMethodsAbsentWithoutABridge(t *testing.T) {
	methods := newHostMethods(newLuaTestHostAPI(), &ffiTestTransport{}, "test", nil)
	for method := range methods {
		if strings.HasPrefix(method, "Host.FFI.") {
			t.Fatalf("%s was registered without a bridge", method)
		}
	}
	if _, ok := methods["Host.Log"]; !ok {
		t.Error("the ordinary host methods went missing")
	}
}

func TestFFIMethodsPresentWithABridge(t *testing.T) {
	if !ffibridge.Supported {
		t.Skip("ffibridge: FFI is disabled in this build")
	}
	bridge := ffibridge.New(ffibridge.Options{})
	t.Cleanup(func() {
		if err := bridge.Close(); err != nil {
			t.Errorf("close FFI bridge: %v", err)
		}
	})

	methods := newHostMethods(newLuaTestHostAPI(), &ffiTestTransport{}, "test", bridge)
	for _, method := range []string{"Host.FFI.Open", "Host.FFI.Call", "Host.FFI.Alloc"} {
		if _, ok := methods[method]; !ok {
			t.Errorf("%s was not registered", method)
		}
	}
}

func TestFFICallOverTheProtocol(t *testing.T) {
	methods, _ := newFFITestMethods(t, &ffiTestTransport{})

	lib, err := callFFI(t, methods, "Host.FFI.OpenLibC", FFIReq{})
	if err != nil {
		t.Fatalf("OpenLibC: %v", err)
	}
	if lib.Handle == 0 {
		t.Fatal("OpenLibC returned a null handle")
	}

	res, err := callFFI(t, methods, "Host.FFI.CallSym", FFIReq{
		Lib:  lib.Handle,
		Name: "strlen",
		Sig:  "i64(str)",
		Args: []any{"hello"},
	})
	if err != nil {
		t.Fatalf("strlen: %v", err)
	}
	if got := ffiNumber(t, res.Value); got != 5 {
		t.Fatalf("strlen returned %#v, want 5", res.Value)
	}
}

func TestFFIPointerResultSurvivesTheWire(t *testing.T) {
	methods, _ := newFFITestMethods(t, &ffiTestTransport{})

	lib, err := callFFI(t, methods, "Host.FFI.OpenLibC", FFIReq{})
	if err != nil {
		t.Fatalf("OpenLibC: %v", err)
	}

	src, err := callFFI(t, methods, "Host.FFI.CString", FFIReq{Text: "payload"})
	if err != nil {
		t.Fatalf("CString: %v", err)
	}
	dst, err := callFFI(t, methods, "Host.FFI.Alloc", FFIReq{Size: 8})
	if err != nil {
		t.Fatalf("Alloc: %v", err)
	}

	copied, err := callFFI(t, methods, "Host.FFI.CallSym", FFIReq{
		Lib:  lib.Handle,
		Name: "memcpy",
		Sig:  "ptr(ptr,ptr,i64)",
		Args: []any{dst.Addr, src.Addr, 8},
	})
	if err != nil {
		t.Fatalf("memcpy: %v", err)
	}
	// A uintptr has no MessagePack representation; it must arrive as a number.
	if got := ffiNumber(t, copied.Value); got != testInt64Uint64(dst.Addr) {
		t.Fatalf("memcpy returned %#v, want the destination address", copied.Value)
	}

	text, err := callFFI(t, methods, "Host.FFI.GoString", FFIReq{Addr: dst.Addr})
	if err != nil {
		t.Fatalf("GoString: %v", err)
	}
	if text.Text != "payload" {
		t.Fatalf("GoString returned %q", text.Text)
	}
}

func TestFFIMemoryOverTheProtocol(t *testing.T) {
	methods, _ := newFFITestMethods(t, &ffiTestTransport{})

	block, err := callFFI(t, methods, "Host.FFI.Alloc", FFIReq{Size: 16})
	if err != nil {
		t.Fatalf("Alloc: %v", err)
	}

	if _, err := callFFI(t, methods, "Host.FFI.Write", FFIReq{
		Addr: block.Addr, Off: 4, Data: []byte("data"),
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	read, err := callFFI(t, methods, "Host.FFI.Read", FFIReq{Addr: block.Addr, Off: 4, Len: 4})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(read.Data) != "data" {
		t.Fatalf("Read returned %q", read.Data)
	}

	if _, err := callFFI(t, methods, "Host.FFI.Free", FFIReq{Addr: block.Addr}); err != nil {
		t.Fatalf("Free: %v", err)
	}
	if _, err := callFFI(t, methods, "Host.FFI.Free", FFIReq{Addr: block.Addr}); err == nil {
		t.Error("a double free was accepted")
	}
}

func TestFFICallbackReachesThePlugin(t *testing.T) {
	var seen FFIReq
	back := &ffiTestTransport{
		handler: func(method string, params any, result any) error {
			if method != "Plugin.OnFFICallback" {
				t.Errorf("host called %q", method)
				return nil
			}
			req, ok := params.(FFIReq)
			if !ok {
				t.Fatalf("callback params were %T", params)
			}
			seen = req

			var sum int64
			for _, arg := range req.Args {
				sum += ffiNumber(t, arg)
			}
			if res, ok := result.(*FFIRes); ok {
				res.Value = sum
			}
			return nil
		},
	}

	methods, _ := newFFITestMethods(t, back)

	callback, err := callFFI(t, methods, "Host.FFI.Callback", FFIReq{Sig: "i32(i32,i32)", ID: 7})
	if err != nil {
		t.Fatalf("Callback: %v", err)
	}
	if callback.Addr == 0 {
		t.Fatal("Callback returned a null address")
	}

	res, err := callFFI(t, methods, "Host.FFI.Call", FFIReq{
		Fn:   callback.Addr,
		Sig:  "i32(i32,i32)",
		Args: []any{20, 22},
	})
	if err != nil {
		t.Fatalf("calling the trampoline: %v", err)
	}
	if got := ffiNumber(t, res.Value); got != 42 {
		t.Fatalf("the callback returned %#v, want 42", res.Value)
	}
	if seen.ID != 7 {
		t.Errorf("the plugin was told id %d, want 7", seen.ID)
	}
}

func TestFFIErrorsReachThePlugin(t *testing.T) {
	methods, _ := newFFITestMethods(t, &ffiTestTransport{})

	lib, err := callFFI(t, methods, "Host.FFI.OpenLibC", FFIReq{})
	if err != nil {
		t.Fatalf("OpenLibC: %v", err)
	}

	if _, err := callFFI(t, methods, "Host.FFI.Sym", FFIReq{
		Lib: lib.Handle, Name: "f4_no_such_symbol_here",
	}); err == nil {
		t.Error("a missing symbol was reported as success")
	}
	if _, err := callFFI(t, methods, "Host.FFI.Open", FFIReq{Name: "f4-no-such-library.so"}); err == nil {
		t.Error("a missing library was reported as success")
	}
	if _, err := callFFI(t, methods, "Host.FFI.Call", FFIReq{Sig: "i32()"}); err == nil {
		t.Error("a call through a null pointer was accepted")
	}
}
