package luaplug

import lua "github.com/yuin/gopher-lua"

// luaState keeps the test bodies readable without importing gopher-lua into
// every test file.
type luaState = lua.LState
