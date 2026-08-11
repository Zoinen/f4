package main

import (
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// installAPI puts the Far macro vocabulary into the interpreter.
//
// The dialect implemented is far2m's, which is the same luafar lineage as
// Far3's but already free of Windows specifics. What is here is the subset a
// typical user macro touches; the shape is Far's, so the rest can be filled in
// without moving anything.
func (e *LuaMacroEngine) installAPI(L *lua.LState) {
	L.SetGlobal("Macro", L.NewFunction(e.luaMacro))
	L.SetGlobal("Keys", L.NewFunction(e.luaKeys))
	L.SetGlobal("akey", L.NewFunction(e.luaAKey))
	L.SetGlobal("exit", L.NewFunction(luaMacroExit))
	L.SetGlobal("msgbox", L.NewFunction(e.luaMsgBox))

	L.SetGlobal("Actions", e.newActionsTable(L))
	L.SetGlobal("Plugin", e.newPluginTable(L))

	// Declarations f4 does not implement yet. They are accepted and ignored so
	// that a script mixing them with Macro{} still contributes its macros
	// instead of failing to load entirely.
	for _, name := range []string{"Event", "MenuItem", "CommandLine"} {
		declaration := name
		L.SetGlobal(name, L.NewFunction(func(L *lua.LState) int {
			e.host.Log("MACRO: %s{} is not supported yet, ignored (%s)", declaration, L.Where(1))
			return 0
		}))
	}

	L.SetGlobal("Area", e.newAreaTable(L))
	L.SetGlobal("APanel", e.newPanelTable(L, true))
	L.SetGlobal("PPanel", e.newPanelTable(L, false))
	L.SetGlobal("CmdLine", e.newCmdLineTable(L))
	L.SetGlobal("Far", e.newFarTable(L))
	L.SetGlobal("mf", e.newMFTable(L))
	L.SetGlobal("far", e.newFarNamespace(L))

	bits := newBitTable(L)
	L.SetGlobal("bit", bits)
	L.SetGlobal("bit64", bits)
}

func (e *LuaMacroEngine) newPluginTable(L *lua.LState) *lua.LTable {
	table := L.NewTable()
	L.SetFuncs(table, map[string]lua.LGFunction{
		"Call": e.luaPluginCall,
	})
	return table
}

func (e *LuaMacroEngine) luaPluginCall(L *lua.LState) int {
	id := L.CheckString(1)
	args := make([]any, 0, max(0, L.GetTop()-1))
	for index := 2; index <= L.GetTop(); index++ {
		value, err := macroValueFromLua(L.Get(index), 0)
		if err != nil {
			L.RaiseError("Plugin.Call argument %d: %v", index-1, err)
			return 0
		}
		args = append(args, value)
	}

	results, err := e.host.CallPlugin(L.Context(), id, args)
	if err != nil {
		e.host.Log("MACRO: Plugin.Call(%q): %v", id, err)
		L.Push(lua.LNil)
		return 1
	}
	converted := make([]lua.LValue, len(results))
	for index, result := range results {
		value, convertErr := macroValueToLua(L, result, 0)
		if convertErr != nil {
			e.host.Log("MACRO: Plugin.Call(%q) result %d: %v", id, index+1, convertErr)
			L.Push(lua.LNil)
			return 1
		}
		converted[index] = value
	}
	for _, value := range converted {
		L.Push(value)
	}
	return len(results)
}

func macroValueFromLua(value lua.LValue, depth int) (any, error) {
	if depth > 32 {
		return nil, fmt.Errorf("value nesting exceeds 32 levels")
	}
	switch value := value.(type) {
	case *lua.LNilType:
		return nil, nil
	case lua.LBool:
		return bool(value), nil
	case lua.LString:
		return string(value), nil
	case lua.LNumber:
		number := float64(value)
		if number == math.Trunc(number) && math.Abs(number) <= 1<<53 {
			return int64(number), nil
		}
		return number, nil
	case *lua.LTable:
		length := value.Len()
		count := 0
		valid := true
		value.ForEach(func(key, _ lua.LValue) {
			count++
			number, ok := key.(lua.LNumber)
			if !ok || float64(number) != math.Trunc(float64(number)) || int(number) < 1 || int(number) > length {
				valid = false
			}
		})
		if !valid || count != length {
			return nil, fmt.Errorf("tables must be dense arrays")
		}
		result := make([]any, length)
		for index := 1; index <= length; index++ {
			item, err := macroValueFromLua(value.RawGetInt(index), depth+1)
			if err != nil {
				return nil, err
			}
			result[index-1] = item
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported Lua value %s", value.Type())
	}
}

func macroValueToLua(L *lua.LState, value any, depth int) (lua.LValue, error) {
	if value == nil {
		return lua.LNil, nil
	}
	if depth > 32 {
		return nil, fmt.Errorf("value nesting exceeds 32 levels")
	}
	switch value := value.(type) {
	case bool:
		return lua.LBool(value), nil
	case string:
		return lua.LString(value), nil
	case []byte:
		return lua.LString(value), nil
	case int:
		return lua.LNumber(value), nil
	case int8:
		return lua.LNumber(value), nil
	case int16:
		return lua.LNumber(value), nil
	case int32:
		return lua.LNumber(value), nil
	case int64:
		return lua.LNumber(value), nil
	case uint:
		return lua.LNumber(value), nil
	case uint8:
		return lua.LNumber(value), nil
	case uint16:
		return lua.LNumber(value), nil
	case uint32:
		return lua.LNumber(value), nil
	case uint64:
		if value > 1<<53 {
			return nil, fmt.Errorf("integer %d cannot be represented exactly in Lua", value)
		}
		return lua.LNumber(value), nil
	case float32:
		return lua.LNumber(value), nil
	case float64:
		return lua.LNumber(value), nil
	}

	reflected := reflect.ValueOf(value)
	if reflected.Kind() != reflect.Slice && reflected.Kind() != reflect.Array {
		return nil, fmt.Errorf("unsupported Go value %T", value)
	}
	table := L.NewTable()
	for index := 0; index < reflected.Len(); index++ {
		item, err := macroValueToLua(L, reflected.Index(index).Interface(), depth+1)
		if err != nil {
			return nil, err
		}
		table.Append(item)
	}
	return table, nil
}

// dynamicTable builds a table whose fields are computed on access, which is
// how Far's APanel, Area and friends behave: reading a field asks for the
// current state, it does not read a snapshot taken at startup.
func dynamicTable(L *lua.LState, index func(key string) lua.LValue) *lua.LTable {
	table := L.NewTable()
	meta := L.NewTable()
	L.SetField(meta, "__index", L.NewFunction(func(L *lua.LState) int {
		L.Push(index(L.CheckString(2)))
		return 1
	}))
	L.SetMetatable(table, meta)
	return table
}

func (e *LuaMacroEngine) luaMacro(L *lua.LState) int {
	spec := L.CheckTable(1)

	action, _ := spec.RawGetString("action").(*lua.LFunction)
	if action == nil {
		L.RaiseError("Macro: action must be a function")
		return 0
	}

	key := lua.LVAsString(spec.RawGetString("key"))
	if strings.TrimSpace(key) == "" {
		L.RaiseError("Macro: key is required")
		return 0
	}

	area := lua.LVAsString(spec.RawGetString("area"))
	if strings.TrimSpace(area) == "" {
		area = "Common"
	}

	condition, _ := spec.RawGetString("condition").(*lua.LFunction)

	e.add(&LuaMacro{
		Areas:       splitMacroList(area),
		Keys:        splitMacroList(key),
		Description: lua.LVAsString(spec.RawGetString("description")),
		Source:      L.Where(1),
		action:      action,
		condition:   condition,
	})
	return 0
}

// luaKeys queues keys for injection.
//
// Far's Keys() is synchronous: the macro pauses while the keys are processed.
// Reproducing that would mean re-entering the input loop from inside the
// interpreter. Until then the keys are collected and injected as one batch
// when the macro returns, which is indistinguishable for a macro that is a
// sequence of keystrokes and visible for one that inspects state in between.
func (e *LuaMacroEngine) luaKeys(L *lua.LState) int {
	for i := 1; i <= L.GetTop(); i++ {
		spec := lua.LVAsString(L.Get(i))
		events := parseMacroKeys(spec)
		if len(events) == 0 && strings.TrimSpace(spec) != "" {
			e.host.Log("MACRO: Keys(%q) produced no keys", spec)
		}
		e.pendingKeys = append(e.pendingKeys, events...)
	}
	return 0
}

// luaAKey reports the key that started the macro, as Far's akey() does.
func (e *LuaMacroEngine) luaAKey(L *lua.LState) int {
	L.Push(lua.LString(e.invokedKey))
	return 1
}

func luaMacroExit(L *lua.LState) int {
	L.RaiseError("%s", macroExitSentinel)
	return 0
}

func (e *LuaMacroEngine) luaMsgBox(L *lua.LState) int {
	text := lua.LVAsString(L.Get(1))
	title := " Macro "
	if L.GetTop() >= 2 {
		if given := strings.TrimSpace(lua.LVAsString(L.Get(2))); given != "" {
			title = given
		}
	}
	e.host.Message(title, text)
	return 0
}

func (e *LuaMacroEngine) newAreaTable(L *lua.LState) *lua.LTable {
	return dynamicTable(L, func(key string) lua.LValue {
		area := e.host.CurrentArea()
		if strings.EqualFold(key, "Current") {
			return lua.LString(area)
		}
		if alias, ok := macroAreaAliases[strings.ToLower(area)]; ok && strings.EqualFold(key, alias) {
			return lua.LTrue
		}
		return lua.LBool(strings.EqualFold(key, area))
	})
}

func (e *LuaMacroEngine) newPanelTable(L *lua.LState, active bool) *lua.LTable {
	return dynamicTable(L, func(key string) lua.LValue {
		info := e.host.Panel(active)
		switch strings.ToLower(key) {
		case "path", "path0":
			return lua.LString(info.Path)
		case "current":
			return lua.LString(info.Current)
		case "itemcount":
			return lua.LNumber(info.ItemCount)
		case "selcount":
			return lua.LNumber(info.SelCount)
		case "curpos":
			return lua.LNumber(info.CurPos)
		case "toppos":
			return lua.LNumber(info.TopPos)
		case "type":
			return lua.LNumber(info.Type)
		case "folder":
			return lua.LBool(info.IsFolder)
		case "empty":
			return lua.LBool(info.Empty)
		case "left":
			return lua.LBool(info.Left)
		case "visible":
			return lua.LBool(info.Visible)
		case "root":
			return lua.LBool(info.Root)
		case "bof":
			return lua.LBool(info.Bof)
		case "eof":
			return lua.LBool(info.Eof)
		}
		return lua.LNil
	})
}

func (e *LuaMacroEngine) newCmdLineTable(L *lua.LState) *lua.LTable {
	return dynamicTable(L, func(key string) lua.LValue {
		value := e.host.CommandLine()
		switch strings.ToLower(key) {
		case "value":
			return lua.LString(value)
		case "size":
			return lua.LNumber(len([]rune(value)))
		case "empty":
			return lua.LBool(value == "")
		case "selected":
			return lua.LString("")
		case "bof":
			return lua.LBool(value == "")
		case "eof":
			return lua.LTrue
		}
		return lua.LNil
	})
}
func (e *LuaMacroEngine) newActionsTable(L *lua.LState) *lua.LTable {
	table := L.NewTable()
	L.SetFuncs(table, map[string]lua.LGFunction{
		"Run": func(L *lua.LState) int {
			name := L.CheckString(1)
			ok := e.host.RunAction(name)
			L.Push(lua.LBool(ok))
			return 1
		},
	})
	return table
}

func (e *LuaMacroEngine) newFarTable(L *lua.LState) *lua.LTable {
	return dynamicTable(L, func(key string) lua.LValue {
		switch strings.ToLower(key) {
		case "width":
			width, _ := e.host.ScreenSize()
			return lua.LNumber(width)
		case "height":
			_, height := e.host.ScreenSize()
			return lua.LNumber(height)
		case "version":
			return lua.LString(e.host.Version())
		case "title":
			return lua.LString("f4")
		case "fullscreen", "isuseradmin":
			return lua.LFalse
		}
		return lua.LNil
	})
}

// newFarNamespace provides the lowercase far.* namespace, of which only the
// pieces a macro realistically reaches for are implemented.
func (e *LuaMacroEngine) newFarNamespace(L *lua.LState) *lua.LTable {
	namespace := L.NewTable()
	L.SetFuncs(namespace, map[string]lua.LGFunction{
		"Message": func(L *lua.LState) int {
			text := lua.LVAsString(L.Get(1))
			title := " Macro "
			if given := strings.TrimSpace(lua.LVAsString(L.Get(2))); given != "" {
				title = given
			}
			e.host.Message(title, text)
			return 0
		},
		"GetMsg": func(L *lua.LState) int {
			L.Push(lua.LString(""))
			return 1
		},
	})
	return namespace
}

func newBitTable(L *lua.LState) *lua.LTable {
	table := L.NewTable()
	binary := func(op func(a, b int64) int64) lua.LGFunction {
		return func(L *lua.LState) int {
			result := int64(L.CheckNumber(1))
			for i := 2; i <= L.GetTop(); i++ {
				result = op(result, int64(L.CheckNumber(i)))
			}
			L.Push(lua.LNumber(result))
			return 1
		}
	}
	L.SetFuncs(table, map[string]lua.LGFunction{
		"band": binary(func(a, b int64) int64 { return a & b }),
		"bor":  binary(func(a, b int64) int64 { return a | b }),
		"bxor": binary(func(a, b int64) int64 { return a ^ b }),
		"bnot": func(L *lua.LState) int {
			L.Push(lua.LNumber(^int64(L.CheckNumber(1))))
			return 1
		},
		"lshift": binary(func(a, b int64) int64 { return a << uint(b) }),
		"rshift": binary(func(a, b int64) int64 { return int64(uint64(a) >> uint(b)) }),
	})
	return table
}

// newMFTable implements Far's mf.* helpers. String positions are zero based
// and index returns -1 when not found, as they do in Far, not as Lua's own
// string library would have it.
func (e *LuaMacroEngine) newMFTable(L *lua.LState) *lua.LTable {
	table := L.NewTable()

	L.SetFuncs(table, map[string]lua.LGFunction{
		"iif": func(L *lua.LState) int {
			if lua.LVAsBool(L.Get(1)) {
				L.Push(L.Get(2))
			} else {
				L.Push(L.Get(3))
			}
			return 1
		},
		"abs": func(L *lua.LState) int {
			L.Push(lua.LNumber(math.Abs(float64(L.CheckNumber(1)))))
			return 1
		},
		"max": func(L *lua.LState) int {
			L.Push(lua.LNumber(math.Max(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
			return 1
		},
		"min": func(L *lua.LState) int {
			L.Push(lua.LNumber(math.Min(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
			return 1
		},
		"int": func(L *lua.LState) int {
			L.Push(lua.LNumber(math.Trunc(float64(L.CheckNumber(1)))))
			return 1
		},
		"float": func(L *lua.LState) int {
			L.Push(lua.LNumber(L.CheckNumber(1)))
			return 1
		},
		"string": func(L *lua.LState) int {
			L.Push(lua.LString(L.ToStringMeta(L.Get(1)).String()))
			return 1
		},
		"len": func(L *lua.LState) int {
			L.Push(lua.LNumber(len([]rune(L.CheckString(1)))))
			return 1
		},
		"lcase": func(L *lua.LState) int {
			L.Push(lua.LString(strings.ToLower(L.CheckString(1))))
			return 1
		},
		"ucase": func(L *lua.LState) int {
			L.Push(lua.LString(strings.ToUpper(L.CheckString(1))))
			return 1
		},
		"trim": func(L *lua.LState) int {
			L.Push(lua.LString(strings.TrimSpace(L.CheckString(1))))
			return 1
		},
		"substr":  macroSubstr,
		"index":   macroIndex,
		"rindex":  macroRIndex,
		"replace": macroReplace,
		"asc": func(L *lua.LState) int {
			runes := []rune(L.CheckString(1))
			if len(runes) == 0 {
				L.Push(lua.LNumber(0))
			} else {
				L.Push(lua.LNumber(runes[0]))
			}
			return 1
		},
		"chr": func(L *lua.LState) int {
			L.Push(lua.LString(string(rune(int(L.CheckNumber(1))))))
			return 1
		},
		"env": func(L *lua.LState) int {
			L.Push(lua.LString(os.Getenv(L.CheckString(1))))
			return 1
		},
		"fexist": func(L *lua.LState) int {
			_, err := os.Stat(L.CheckString(1))
			L.Push(lua.LBool(err == nil))
			return 1
		},
		"fattr": func(L *lua.LState) int {
			info, err := os.Stat(L.CheckString(1))
			if err != nil {
				L.Push(lua.LNumber(-1))
				return 1
			}
			if info.IsDir() {
				L.Push(lua.LNumber(1))
			} else {
				L.Push(lua.LNumber(0))
			}
			return 1
		},
		"sleep": func(L *lua.LState) int {
			time.Sleep(time.Duration(L.CheckInt(1)) * time.Millisecond)
			return 0
		},
		"beep": func(L *lua.LState) int { return 0 },
		"print": func(L *lua.LState) int {
			e.host.Log("MACRO: %s", lua.LVAsString(L.Get(1)))
			return 0
		},
		"exit":   luaMacroExit,
		"msgbox": e.luaMsgBox,
		"Keys":   e.luaKeys,
		"akey":   e.luaAKey,
	})
	return table
}

func macroSubstr(L *lua.LState) int {
	runes := []rune(L.CheckString(1))
	start := L.CheckInt(2)
	if start < 0 {
		start = len(runes) + start
	}
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		L.Push(lua.LString(""))
		return 1
	}

	end := len(runes)
	if L.GetTop() >= 3 {
		length := L.CheckInt(3)
		if length < 0 {
			end = len(runes) + length
		} else {
			end = start + length
		}
	}
	if end > len(runes) {
		end = len(runes)
	}
	if end < start {
		end = start
	}
	L.Push(lua.LString(string(runes[start:end])))
	return 1
}

func macroIndex(L *lua.LState) int {
	haystack, needle := L.CheckString(1), L.CheckString(2)
	if L.GetTop() >= 3 && lua.LVAsBool(L.Get(3)) {
		haystack, needle = strings.ToLower(haystack), strings.ToLower(needle)
	}
	L.Push(lua.LNumber(runeIndex(haystack, strings.Index(haystack, needle))))
	return 1
}

func macroRIndex(L *lua.LState) int {
	haystack, needle := L.CheckString(1), L.CheckString(2)
	if L.GetTop() >= 3 && lua.LVAsBool(L.Get(3)) {
		haystack, needle = strings.ToLower(haystack), strings.ToLower(needle)
	}
	L.Push(lua.LNumber(runeIndex(haystack, strings.LastIndex(haystack, needle))))
	return 1
}

// runeIndex converts a byte offset into a rune offset, keeping -1 as -1.
func runeIndex(s string, byteOffset int) int {
	if byteOffset < 0 {
		return -1
	}
	return len([]rune(s[:byteOffset]))
}

func macroReplace(L *lua.LState) int {
	source, find, replace := L.CheckString(1), L.CheckString(2), L.CheckString(3)
	count := -1
	if L.GetTop() >= 4 {
		count = L.CheckInt(4)
	}
	if find == "" {
		L.Push(lua.LString(source))
		return 1
	}
	L.Push(lua.LString(strings.Replace(source, find, replace, count)))
	return 1
}
