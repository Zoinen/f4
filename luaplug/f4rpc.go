package luaplug

import (
	"fmt"
	"sort"

	lua "github.com/yuin/gopher-lua"
)

// openF4RPC installs the module a plugin gets from require('f4rpc'). It carries
// the same three functions as the standalone SDK in sdk/lua, so a plugin
// written against the subprocess transport runs here unmodified: require finds
// the preloaded module and never reaches the file that would drag in a
// MessagePack rock.
func (r *Runtime) openF4RPC(L *lua.LState) {
	mod := L.NewTable()
	L.SetFuncs(mod, map[string]lua.LGFunction{
		"register": r.luaRegister,
		"call":     r.luaHostCall,
		"serve":    luaServe,
	})

	// Modifier masks, matching sdk/lua/f4rpc.lua.
	L.SetField(mod, "SHIFT", lua.LNumber(16))
	L.SetField(mod, "CTRL", lua.LNumber(8))
	L.SetField(mod, "ALT", lua.LNumber(2))

	L.PreloadModule("f4rpc", func(L *lua.LState) int {
		L.Push(mod)
		return 1
	})
	L.SetGlobal("f4rpc", mod)
}

func (r *Runtime) luaRegister(L *lua.LState) int {
	method := L.CheckString(1)
	handler := L.CheckFunction(2)
	r.handlers[method] = handler
	return 0
}

func (r *Runtime) luaHostCall(L *lua.LState) int {
	method := L.CheckString(1)
	if r.opts.Host == nil {
		L.RaiseError("f4rpc.call(%q): this runtime has no host attached", method)
		return 0
	}

	var params any
	if L.GetTop() >= 2 {
		params = fromLua(L.Get(2))
	}

	result, err := r.opts.Host.Call(method, params)
	if err != nil {
		L.RaiseError("f4rpc.call(%q): %s", method, err)
		return 0
	}

	L.Push(toLua(L, result))
	return 1
}

// luaServe exists so that an unmodified plugin can end with f4rpc.serve().
// Out of process that call is the stdio read loop; embedded there is no loop to
// enter, the host dispatches straight into the handler table.
func luaServe(L *lua.LState) int {
	return 0
}

// Dispatch invokes a handler the plugin registered, which is how the host makes
// a Plugin.* or VFS.* request.
func (r *Runtime) Dispatch(method string, params any) (any, error) {
	var result any
	err := r.Do(func(L *lua.LState) error {
		handler, ok := r.handlers[method]
		if !ok {
			return fmt.Errorf("luaplug: %s has no handler for %q", r.opts.Name, method)
		}

		L.Push(handler)
		if params == nil {
			L.Push(lua.LNil)
		} else {
			L.Push(toLua(L, params))
		}
		if err := L.PCall(1, 1, nil); err != nil {
			return err
		}

		result = fromLua(L.Get(-1))
		L.Pop(1)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// HasHandler reports whether the plugin registered the given method.
func (r *Runtime) HasHandler(method string) bool {
	found := false
	_ = r.Do(func(L *lua.LState) error {
		_, found = r.handlers[method]
		return nil
	})
	return found
}

// Methods lists the registered handlers, sorted.
func (r *Runtime) Methods() []string {
	var names []string
	_ = r.Do(func(L *lua.LState) error {
		names = make([]string, 0, len(r.handlers))
		for name := range r.handlers {
			names = append(names, name)
		}
		return nil
	})
	sort.Strings(names)
	return names
}
