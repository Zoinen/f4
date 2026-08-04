package luaplug

import (
	"strings"

	lua "github.com/yuin/gopher-lua"
)

type luaLib struct {
	name   string
	opener lua.LGFunction
}

// safeLibs are opened for every plugin. Missing from the list are io and os:
// an in-process plugin that can shell out or write to the terminal is not
// sandboxed in any meaningful sense, and the FFI bridge is the one deliberate,
// permissioned way out.
var safeLibs = []luaLib{
	{lua.LoadLibName, lua.OpenPackage},
	{lua.BaseLibName, lua.OpenBase},
	{lua.TabLibName, lua.OpenTable},
	{lua.StringLibName, lua.OpenString},
	{lua.MathLibName, lua.OpenMath},
	{lua.CoroutineLibName, lua.OpenCoroutine},
	{lua.DebugLibName, lua.OpenDebug},
}

var unsafeLibs = []luaLib{
	{lua.IoLibName, lua.OpenIo},
	{lua.OsLibName, lua.OpenOs},
}

func openLib(L *lua.LState, lib luaLib) {
	L.Push(L.NewFunction(lib.opener))
	L.Push(lua.LString(lib.name))
	L.Call(1, 0)
}

func (r *Runtime) openLibs(L *lua.LState) {
	for _, lib := range safeLibs {
		openLib(L, lib)
	}
	if r.opts.AllowUnsafeStdlib {
		for _, lib := range unsafeLibs {
			openLib(L, lib)
		}
	}
	r.overridePrint(L)
}

// overridePrint keeps plugin output away from the terminal. f4 owns the screen;
// a stray print from a plugin would corrupt the display, so print is routed
// into the host log instead.
func (r *Runtime) overridePrint(L *lua.LState) {
	L.SetGlobal("print", L.NewFunction(func(L *lua.LState) int {
		top := L.GetTop()
		parts := make([]string, 0, top)
		for i := 1; i <= top; i++ {
			parts = append(parts, L.ToStringMeta(L.Get(i)).String())
		}
		r.hostLog(strings.Join(parts, "\t"))
		return 0
	}))
}

func (r *Runtime) hostLog(msg string) {
	if r.opts.Host == nil {
		return
	}
	_, _ = r.opts.Host.Call("Host.Log", msg)
}
