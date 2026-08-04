package luaplug

import (
	"github.com/unxed/ffibridge"
	lua "github.com/yuin/gopher-lua"
)

// openFFI installs the f4ffi module.
//
// The module is deliberately not called ffi. LuaJIT's ffi means cdef, ffi.new
// and a C declaration parser; ours means signature strings and raw addresses.
// Claiming the name would make ported LuaJIT code fail late and confusingly
// instead of immediately and clearly. Once a cdef front end exists, it can take
// the name honestly.
func (r *Runtime) openFFI(L *lua.LState) {
	bridge := r.opts.FFI

	mod := L.NewTable()
	L.SetField(mod, "supported", lua.LBool(bridge != nil && ffibridge.Supported))

	if bridge != nil {
		L.SetFuncs(mod, map[string]lua.LGFunction{
			"open":     ffiOpen(bridge),
			"openlibc": ffiOpenLibC(bridge),
			"close":    ffiCloseLib(bridge),
			"sym":      ffiSym(bridge),
			"call":     ffiCall(bridge),
			"callsym":  ffiCallSym(bridge),
			"alloc":    ffiAlloc(bridge),
			"free":     ffiFree(bridge),
			"cstring":  ffiCString(bridge),
			"tostring": ffiToString(bridge),
			"read":     ffiRead(bridge),
			"write":    ffiWrite(bridge),
			"peek":     ffiPeek(bridge),
			"poke":     ffiPoke(bridge),
			"callback": r.ffiCallback(bridge),
		})
	}

	L.PreloadModule("f4ffi", func(L *lua.LState) int {
		L.Push(mod)
		return 1
	})
	L.SetGlobal("f4ffi", mod)
}

// checkAddr reads an address argument. Addresses travel as Lua numbers, which
// are doubles: exact below 2^53, and every address on every platform f4 targets
// is far below that.
func checkAddr(L *lua.LState, index int) uintptr {
	return uintptr(int64(L.CheckNumber(index)))
}

// collectArgs turns the trailing arguments of a call into host values.
func collectArgs(L *lua.LState, first int) []any {
	top := L.GetTop()
	if top < first {
		return nil
	}
	args := make([]any, 0, top-first+1)
	for i := first; i <= top; i++ {
		args = append(args, fromLua(L.Get(i)))
	}
	return args
}

func ffiOpen(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		handle, err := b.Open(L.CheckString(1))
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(lua.LNumber(handle))
		return 1
	}
}

func ffiOpenLibC(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		handle, err := b.OpenLibC()
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(lua.LNumber(handle))
		return 1
	}
}

func ffiCloseLib(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		if err := b.CloseLib(checkAddr(L, 1)); err != nil {
			L.RaiseError("%s", err)
		}
		return 0
	}
}

func ffiSym(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		addr, err := b.Sym(checkAddr(L, 1), L.CheckString(2))
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(lua.LNumber(addr))
		return 1
	}
}

func ffiCall(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		result, err := b.Call(checkAddr(L, 1), L.CheckString(2), collectArgs(L, 3)...)
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(toLua(L, result))
		return 1
	}
}

func ffiCallSym(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		result, err := b.CallSym(checkAddr(L, 1), L.CheckString(2), L.CheckString(3), collectArgs(L, 4)...)
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(toLua(L, result))
		return 1
	}
}

func ffiAlloc(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		addr, err := b.Alloc(L.CheckInt(1))
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(lua.LNumber(addr))
		return 1
	}
}

func ffiFree(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		if err := b.Free(checkAddr(L, 1)); err != nil {
			L.RaiseError("%s", err)
		}
		return 0
	}
}

func ffiCString(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		addr, err := b.CString(L.CheckString(1))
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(lua.LNumber(addr))
		return 1
	}
}

func ffiToString(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		text, err := b.GoStringAt(checkAddr(L, 1))
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(lua.LString(text))
		return 1
	}
}

func ffiRead(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		data, err := b.Read(checkAddr(L, 1), L.CheckInt(2), L.CheckInt(3))
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(lua.LString(data))
		return 1
	}
}

func ffiWrite(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		if err := b.Write(checkAddr(L, 1), L.CheckInt(2), []byte(L.CheckString(3))); err != nil {
			L.RaiseError("%s", err)
		}
		return 0
	}
}

func ffiPeek(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		data, err := b.Peek(checkAddr(L, 1), L.CheckInt(2))
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}
		L.Push(lua.LString(data))
		return 1
	}
}

func ffiPoke(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		if err := b.Poke(checkAddr(L, 1), []byte(L.CheckString(2))); err != nil {
			L.RaiseError("%s", err)
		}
		return 0
	}
}

// ffiCallback wraps a Lua function as a native function pointer. Native code
// may invoke it on the runtime's own worker goroutine, which is why the body
// goes through Do rather than touching the state directly.
func (r *Runtime) ffiCallback(b *ffibridge.Bridge) lua.LGFunction {
	return func(L *lua.LState) int {
		signature := L.CheckString(1)
		handler := L.CheckFunction(2)

		addr, err := b.NewCallback(signature, func(args []any) (any, error) {
			var result any
			err := r.Do(func(L *lua.LState) error {
				L.Push(handler)
				for _, arg := range args {
					L.Push(toLua(L, arg))
				}
				if err := L.PCall(len(args), 1, nil); err != nil {
					return err
				}
				result = fromLua(L.Get(-1))
				L.Pop(1)
				return nil
			})
			return result, err
		})
		if err != nil {
			L.RaiseError("%s", err)
			return 0
		}

		L.Push(lua.LNumber(addr))
		return 1
	}
}
