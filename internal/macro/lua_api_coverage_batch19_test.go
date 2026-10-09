package macro

import (
	"reflect"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

func TestMacroValueFromLuaScalars(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	tests := []struct {
		name  string
		value lua.LValue
		want  any
	}{
		{name: "nil", value: lua.LNil, want: nil},
		{name: "bool", value: lua.LTrue, want: true},
		{name: "string", value: lua.LString("hello"), want: "hello"},
		{name: "integer", value: lua.LNumber(42), want: int64(42)},
		{name: "fraction", value: lua.LNumber(1.5), want: 1.5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := macroValueFromLua(tc.value, 0)
			if err != nil {
				t.Fatalf("macroValueFromLua: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v (%T), want %#v (%T)", got, got, tc.want, tc.want)
			}
		})
	}
}

func TestMacroValueFromLuaDenseArray(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	table := L.NewTable()
	table.Append(lua.LString("one"))
	table.Append(lua.LNumber(2))
	got, err := macroValueFromLua(table, 0)
	if err != nil {
		t.Fatalf("macroValueFromLua: %v", err)
	}
	want := []any{"one", int64(2)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestMacroValueFromLuaRejectsInvalidValues(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	sparse := L.NewTable()
	sparse.RawSetInt(1, lua.LString("one"))
	sparse.RawSetInt(3, lua.LString("three"))
	for _, tc := range []struct {
		name  string
		value lua.LValue
	}{
		{name: "sparse array", value: sparse},
		{name: "function", value: L.NewFunction(func(*lua.LState) int { return 0 })},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := macroValueFromLua(tc.value, 0); err == nil {
				t.Fatal("macroValueFromLua accepted an invalid value")
			}
		})
	}
}

func TestMacroValueFromLuaRejectsDeepNesting(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	root := L.NewTable()
	current := root
	for i := 0; i < 34; i++ {
		next := L.NewTable()
		current.Append(next)
		current = next
	}
	if _, err := macroValueFromLua(root, 0); err == nil {
		t.Fatal("macroValueFromLua accepted nesting deeper than 32 levels")
	}
}

func TestMacroValueToLuaScalarsAndBytes(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	values := []struct {
		name  string
		value any
		want  lua.LValue
	}{
		{name: "bool", value: true, want: lua.LTrue},
		{name: "string", value: "hello", want: lua.LString("hello")},
		{name: "bytes", value: []byte("hello"), want: lua.LString("hello")},
		{name: "integer", value: int32(7), want: lua.LNumber(7)},
	}
	for _, tc := range values {
		t.Run(tc.name, func(t *testing.T) {
			got, err := macroValueToLua(L, tc.value, 0)
			if err != nil {
				t.Fatalf("macroValueToLua: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMacroValueToLuaSlices(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	got, err := macroValueToLua(L, []string{"one", "two"}, 0)
	if err != nil {
		t.Fatalf("macroValueToLua: %v", err)
	}
	table, ok := got.(*lua.LTable)
	if !ok {
		t.Fatalf("got %T, want *lua.LTable", got)
	}
	if table.Len() != 2 || table.RawGetInt(1) != lua.LString("one") || table.RawGetInt(2) != lua.LString("two") {
		t.Fatalf("converted table = %v, want [one, two]", table)
	}
}

func TestMacroValueToLuaRejectsUnsupportedValues(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	if _, err := macroValueToLua(L, struct{}{}, 0); err == nil {
		t.Fatal("macroValueToLua accepted an unsupported struct")
	}
	if _, err := macroValueToLua(L, uint64(1<<53+1), 0); err == nil {
		t.Fatal("macroValueToLua accepted an imprecise uint64")
	}
}

func TestMacroValueToLuaRejectsDeepNesting(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	var value any = []any{}
	for i := 0; i < 34; i++ {
		value = []any{value}
	}
	if _, err := macroValueToLua(L, value, 0); err == nil {
		t.Fatal("macroValueToLua accepted nesting deeper than 32 levels")
	}
}

func TestMacroStringHelpersCoverEdgeCases(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), "")
	tests := []struct {
		expression string
		want       string
	}{
		{`mf.substr("abcdef", -3, -1)`, "de"},
		{`tostring(mf.index("Hello", "HE", true))`, "0"},
		{`tostring(mf.rindex("Hello hello", "HE", true))`, "6"},
		{`mf.replace("a-b-c", "-", "+", 1)`, "a+b-c"},
		{`mf.replace("abc", "", "x")`, "abc"},
	}
	for _, tc := range tests {
		var got string
		err := Engine.rt.Do(func(L *lua.LState) error {
			if err := L.DoString("__result = " + tc.expression); err != nil {
				return err
			}
			got = lua.LVAsString(L.GetGlobal("__result"))
			return nil
		})
		if err != nil {
			t.Errorf("%s: %v", tc.expression, err)
			continue
		}
		if got != tc.want {
			t.Errorf("%s = %q, want %q", tc.expression, got, tc.want)
		}
	}
}

func TestNewBitTableRejectsInvalidShifts(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	table := newBitTable(L)

	for _, name := range []string{"lshift", "rshift"} {
		fn := table.RawGetString(name)
		if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, lua.LNumber(1), lua.LNumber(-1)); err != nil {
			t.Fatalf("%s(-1): %v", name, err)
		}
		if got := L.Get(-1); got != lua.LNumber(0) {
			t.Errorf("%s(1, -1) = %v, want 0", name, got)
		}
		L.Pop(1)

		if err := L.CallByParam(lua.P{Fn: fn, NRet: 1, Protect: true}, lua.LNumber(1), lua.LNumber(64)); err != nil {
			t.Fatalf("%s(64): %v", name, err)
		}
		if got := L.Get(-1); got != lua.LNumber(0) {
			t.Errorf("%s(1, 64) = %v, want 0", name, got)
		}
		L.Pop(1)
	}

	if _, err := macroValueToLua(L, ^uint64(0), 0); err == nil {
		t.Fatal("macroValueToLua accepted math.MaxUint64")
	}
}
