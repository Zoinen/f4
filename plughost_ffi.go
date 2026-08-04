package main

import (
	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/ffibridge"
	"github.com/vmihailenco/msgpack/v5"
)

// FFIReq is the request shape for every Host.FFI.* method.
//
// One struct rather than a dozen, because the guest side of this is often a
// hand written binding in a language with no code generation, and a single
// well known record is far less to get wrong than fourteen near identical
// ones. Unused fields cost a byte each in MessagePack.
type FFIReq struct {
	Name string
	Sig  string
	Lib  uint64
	Fn   uint64
	Addr uint64
	Off  int
	Len  int
	Size int
	Text string
	Data []byte
	Args []any
	ID   uint64
}

// FFIRes is the reply shape for every Host.FFI.* method.
type FFIRes struct {
	Handle uint64
	Addr   uint64
	Value  any
	Text   string
	Data   []byte
	OK     bool
}

// newFFIHostMethods projects the FFI broker onto the plugin protocol.
//
// This is what makes FFI available to a sandboxed guest that cannot load a
// library itself. It works because ffibridge deals only in integers and
// strings: handles, addresses and signatures all survive MessagePack
// unchanged, so nothing has to reconcile a guest offset with a host address.
// Copying is what the protocol does anyway.
func newFFIHostMethods(bridge *ffibridge.Bridge, back PluginTransport) map[string]f4rpc.Handler {
	decode := func(data msgpack.RawMessage) FFIReq {
		var req FFIReq
		msgpack.Unmarshal(data, &req)
		return req
	}

	methods := make(map[string]f4rpc.Handler)

	methods["Host.FFI.Supported"] = func(data msgpack.RawMessage) (any, error) {
		return FFIRes{OK: ffibridge.Supported}, nil
	}

	methods["Host.FFI.Open"] = func(data msgpack.RawMessage) (any, error) {
		handle, err := bridge.Open(decode(data).Name)
		if err != nil {
			return nil, err
		}
		return FFIRes{Handle: uint64(handle)}, nil
	}

	methods["Host.FFI.OpenLibC"] = func(data msgpack.RawMessage) (any, error) {
		handle, err := bridge.OpenLibC()
		if err != nil {
			return nil, err
		}
		return FFIRes{Handle: uint64(handle)}, nil
	}

	methods["Host.FFI.Close"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		if err := bridge.CloseLib(uintptr(req.Lib)); err != nil {
			return nil, err
		}
		return FFIRes{OK: true}, nil
	}

	methods["Host.FFI.Sym"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		addr, err := bridge.Sym(uintptr(req.Lib), req.Name)
		if err != nil {
			return nil, err
		}
		return FFIRes{Addr: uint64(addr)}, nil
	}

	methods["Host.FFI.Call"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		result, err := bridge.Call(uintptr(req.Fn), req.Sig, req.Args...)
		if err != nil {
			return nil, err
		}
		return FFIRes{Value: ffiWireValue(result)}, nil
	}

	methods["Host.FFI.CallSym"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		result, err := bridge.CallSym(uintptr(req.Lib), req.Name, req.Sig, req.Args...)
		if err != nil {
			return nil, err
		}
		return FFIRes{Value: ffiWireValue(result)}, nil
	}

	methods["Host.FFI.Alloc"] = func(data msgpack.RawMessage) (any, error) {
		addr, err := bridge.Alloc(decode(data).Size)
		if err != nil {
			return nil, err
		}
		return FFIRes{Addr: uint64(addr)}, nil
	}

	methods["Host.FFI.Free"] = func(data msgpack.RawMessage) (any, error) {
		if err := bridge.Free(uintptr(decode(data).Addr)); err != nil {
			return nil, err
		}
		return FFIRes{OK: true}, nil
	}

	methods["Host.FFI.CString"] = func(data msgpack.RawMessage) (any, error) {
		addr, err := bridge.CString(decode(data).Text)
		if err != nil {
			return nil, err
		}
		return FFIRes{Addr: uint64(addr)}, nil
	}

	methods["Host.FFI.GoString"] = func(data msgpack.RawMessage) (any, error) {
		text, err := bridge.GoStringAt(uintptr(decode(data).Addr))
		if err != nil {
			return nil, err
		}
		return FFIRes{Text: text}, nil
	}

	methods["Host.FFI.Read"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		out, err := bridge.Read(uintptr(req.Addr), req.Off, req.Len)
		if err != nil {
			return nil, err
		}
		return FFIRes{Data: out}, nil
	}

	methods["Host.FFI.Write"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		if err := bridge.Write(uintptr(req.Addr), req.Off, req.Data); err != nil {
			return nil, err
		}
		return FFIRes{OK: true}, nil
	}

	methods["Host.FFI.Peek"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		out, err := bridge.Peek(uintptr(req.Addr), req.Len)
		if err != nil {
			return nil, err
		}
		return FFIRes{Data: out}, nil
	}

	methods["Host.FFI.Poke"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		if err := bridge.Poke(uintptr(req.Addr), req.Data); err != nil {
			return nil, err
		}
		return FFIRes{OK: true}, nil
	}

	// The trampoline handed to native code calls back into the plugin, so a
	// guest gets real C callbacks without ever holding a host pointer.
	methods["Host.FFI.Callback"] = func(data msgpack.RawMessage) (any, error) {
		req := decode(data)
		id := req.ID

		addr, err := bridge.NewCallback(req.Sig, func(args []any) (any, error) {
			wire := make([]any, len(args))
			for i, arg := range args {
				wire[i] = ffiWireValue(arg)
			}
			var res FFIRes
			if err := back.Call("Plugin.OnFFICallback", FFIReq{ID: id, Args: wire}, &res); err != nil {
				return nil, err
			}
			return res.Value, nil
		})
		if err != nil {
			return nil, err
		}
		return FFIRes{Addr: uint64(addr)}, nil
	}

	return methods
}

// ffiWireValue makes a bridge result encodable. Only uintptr needs help: it is
// how the bridge spells an address, and MessagePack has no such type.
func ffiWireValue(value any) any {
	if addr, ok := value.(uintptr); ok {
		return uint64(addr)
	}
	return value
}
