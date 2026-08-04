package luaplug

import (
	"math"
	"reflect"

	lua "github.com/yuin/gopher-lua"
)

// maxConvertDepth stops a cyclic table or struct from converting forever.
const maxConvertDepth = 32

// toLua converts a host value into a Lua value. The accepted shapes are the
// ones the F4-RPC protocol carries: scalars, slices, maps and structs.
func toLua(L *lua.LState, v any) lua.LValue {
	return toLuaDepth(L, v, 0)
}

func toLuaDepth(L *lua.LState, v any, depth int) lua.LValue {
	if v == nil || depth > maxConvertDepth {
		return lua.LNil
	}

	switch n := v.(type) {
	case bool:
		return lua.LBool(n)
	case string:
		return lua.LString(n)
	case []byte:
		return lua.LString(n)
	case int:
		return lua.LNumber(n)
	case int8:
		return lua.LNumber(n)
	case int16:
		return lua.LNumber(n)
	case int32:
		return lua.LNumber(n)
	case int64:
		return lua.LNumber(n)
	case uint:
		return lua.LNumber(n)
	case uint8:
		return lua.LNumber(n)
	case uint16:
		return lua.LNumber(n)
	case uint32:
		return lua.LNumber(n)
	case uint64:
		return lua.LNumber(n)
	case uintptr:
		return lua.LNumber(n)
	case float32:
		return lua.LNumber(n)
	case float64:
		return lua.LNumber(n)
	case lua.LValue:
		return n
	}

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return lua.LNil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		tbl := L.NewTable()
		for i := 0; i < rv.Len(); i++ {
			tbl.Append(toLuaDepth(L, rv.Index(i).Interface(), depth+1))
		}
		return tbl

	case reflect.Map:
		tbl := L.NewTable()
		iter := rv.MapRange()
		for iter.Next() {
			key := toLuaDepth(L, iter.Key().Interface(), depth+1)
			tbl.RawSetString(key.String(), toLuaDepth(L, iter.Value().Interface(), depth+1))
		}
		return tbl

	case reflect.Struct:
		tbl := L.NewTable()
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			field := rt.Field(i)
			if !field.IsExported() {
				continue
			}
			tbl.RawSetString(field.Name, toLuaDepth(L, rv.Field(i).Interface(), depth+1))
		}
		return tbl

	case reflect.Bool:
		return lua.LBool(rv.Bool())
	case reflect.String:
		return lua.LString(rv.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return lua.LNumber(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return lua.LNumber(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return lua.LNumber(rv.Float())
	}
	return lua.LNil
}

// fromLua converts a Lua value into a host value. Whole numbers come back as
// int64 so that sizes and handles survive a round trip unchanged.
func fromLua(v lua.LValue) any {
	return fromLuaDepth(v, 0)
}

func fromLuaDepth(v lua.LValue, depth int) any {
	if depth > maxConvertDepth {
		return nil
	}

	switch n := v.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(n)
	case lua.LString:
		return string(n)
	case lua.LNumber:
		f := float64(n)
		if f == math.Trunc(f) && math.Abs(f) <= 1<<53 {
			return int64(f)
		}
		return f
	case *lua.LTable:
		return tableToGo(n, depth)
	}
	return nil
}

// tableToGo turns a Lua table into a slice when it is a dense array and into a
// string keyed map otherwise, which is what the msgpack side of the protocol
// expects.
func tableToGo(t *lua.LTable, depth int) any {
	length := t.Len()
	count := 0
	isArray := true

	t.ForEach(func(k, v lua.LValue) {
		count++
		num, ok := k.(lua.LNumber)
		if !ok {
			isArray = false
			return
		}
		f := float64(num)
		if f != math.Trunc(f) || f < 1 || int(f) > length {
			isArray = false
		}
	})

	if isArray && count == length {
		out := make([]any, 0, length)
		for i := 1; i <= length; i++ {
			out = append(out, fromLuaDepth(t.RawGetInt(i), depth+1))
		}
		return out
	}

	out := make(map[string]any, count)
	t.ForEach(func(k, v lua.LValue) {
		out[k.String()] = fromLuaDepth(v, depth+1)
	})
	return out
}
