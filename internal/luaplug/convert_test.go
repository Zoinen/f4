package luaplug

import (
	lua "github.com/yuin/gopher-lua"
	"reflect"
	"testing"
)

type sampleItem struct {
	Name   string
	Size   int64
	IsDir  bool
	hidden string
}

func TestToLuaScalars(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	cases := []struct {
		in   any
		want lua.LValue
	}{
		{nil, lua.LNil},
		{true, lua.LBool(true)},
		{"text", lua.LString("text")},
		{[]byte("bytes"), lua.LString("bytes")},
		{int(3), lua.LNumber(3)},
		{int8(-4), lua.LNumber(-4)},
		{int16(-5), lua.LNumber(-5)},
		{int32(-6), lua.LNumber(-6)},
		{int64(7), lua.LNumber(7)},
		{uint(1), lua.LNumber(1)},
		{uint8(2), lua.LNumber(2)},
		{uint16(3), lua.LNumber(3)},
		{uint32(9), lua.LNumber(9)},
		{uint64(11), lua.LNumber(11)},
		{uintptr(0x40), lua.LNumber(0x40)},
		{float32(1.25), lua.LNumber(float32(1.25))},
		{2.5, lua.LNumber(2.5)},
		{lua.LString("already lua"), lua.LString("already lua")},
	}
	for _, tc := range cases {
		if got := toLua(L, tc.in); got != tc.want {
			t.Errorf("toLua(%#v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// Named types share an underlying kind with a builtin but are a distinct Go
// type, so they miss every case in toLuaDepth's type switch and must fall
// through to the reflect-based conversion below it.
type namedBool bool
type namedString string
type namedUint uint16
type namedFloat float32

func TestToLuaNamedTypesUseReflectKind(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	cases := []struct {
		in   any
		want lua.LValue
	}{
		{namedBool(true), lua.LBool(true)},
		{namedString("hi"), lua.LString("hi")},
		{namedUint(7), lua.LNumber(7)},
		{namedFloat(1.5), lua.LNumber(float32(1.5))},
	}
	for _, tc := range cases {
		if got := toLua(L, tc.in); got != tc.want {
			t.Errorf("toLua(%#v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestToLuaPointerIndirection(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	var nilPtr *int
	if got := toLua(L, nilPtr); got != lua.LNil {
		t.Errorf("toLua(nil *int) = %v, want LNil", got)
	}

	value := 99
	if got := toLua(L, &value); got != lua.LNumber(99) {
		t.Errorf("toLua(&value) = %v, want 99", got)
	}
}

func TestToLuaUnconvertibleKind(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	// A channel has no representation in the protocol; toLuaDepth must fall
	// back to nil instead of panicking on an unhandled reflect.Kind.
	if got := toLua(L, make(chan int)); got != lua.LNil {
		t.Errorf("toLua(chan int) = %v, want LNil", got)
	}
}

func TestToLuaStruct(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	tbl, ok := toLua(L, sampleItem{Name: "a.txt", Size: 12, IsDir: false, hidden: "x"}).(*lua.LTable)
	if !ok {
		t.Fatal("struct did not convert to a table")
	}
	if got := tbl.RawGetString("Name"); got != lua.LString("a.txt") {
		t.Errorf("Name = %v, want a.txt", got)
	}
	if got := tbl.RawGetString("Size"); got != lua.LNumber(12) {
		t.Errorf("Size = %v, want 12", got)
	}
	if got := tbl.RawGetString("hidden"); got != lua.LNil {
		t.Errorf("unexported field leaked into Lua: %v", got)
	}
}

func TestToLuaSliceOfStructs(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	tbl, ok := toLua(L, []sampleItem{{Name: "a"}, {Name: "b"}}).(*lua.LTable)
	if !ok {
		t.Fatal("slice did not convert to a table")
	}
	if tbl.Len() != 2 {
		t.Fatalf("table length = %d, want 2", tbl.Len())
	}
	first, ok := tbl.RawGetInt(1).(*lua.LTable)
	if !ok {
		t.Fatal("element 1 is not a table")
	}
	if got := first.RawGetString("Name"); got != lua.LString("a") {
		t.Errorf("element 1 Name = %v, want a", got)
	}
}

func TestFromLuaTables(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	array := L.NewTable()
	array.Append(lua.LString("x"))
	array.Append(lua.LString("y"))
	if got := fromLua(array); !reflect.DeepEqual(got, []any{"x", "y"}) {
		t.Errorf("dense table = %#v, want a slice", got)
	}

	record := L.NewTable()
	record.RawSetString("Name", lua.LString("a.txt"))
	record.RawSetString("Size", lua.LNumber(12))
	want := map[string]any{"Name": "a.txt", "Size": int64(12)}
	if got := fromLua(record); !reflect.DeepEqual(got, want) {
		t.Errorf("keyed table = %#v, want %#v", got, want)
	}

	empty := L.NewTable()
	if got := fromLua(empty); !reflect.DeepEqual(got, []any{}) {
		t.Errorf("empty table = %#v, want an empty slice", got)
	}
}

func TestFromLuaNumbers(t *testing.T) {
	if got := fromLua(lua.LNumber(42)); got != int64(42) {
		t.Errorf("whole number = %#v, want int64(42)", got)
	}
	if got := fromLua(lua.LNumber(2.5)); got != 2.5 {
		t.Errorf("fractional number = %#v, want 2.5", got)
	}
}

func TestFromLuaDepthLimit(t *testing.T) {
	// fromLua always starts at depth 0, so the cutoff itself is only
	// reachable by calling fromLuaDepth directly with a depth already past
	// maxConvertDepth, the same way a runaway cyclic structure would reach it.
	if got := fromLuaDepth(lua.LNumber(5), maxConvertDepth+1); got != nil {
		t.Errorf("fromLuaDepth beyond max depth = %#v, want nil", got)
	}
}

func TestFromLuaUnhandledType(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	fn := L.NewFunction(func(L *lua.LState) int { return 0 })
	if got := fromLua(fn); got != nil {
		t.Errorf("fromLua(function) = %#v, want nil", got)
	}
}

func TestFromLuaSparseTableBecomesMap(t *testing.T) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	tbl := L.NewTable()
	tbl.RawSetInt(1, lua.LString("a"))
	// A key far outside the dense-array range forces gopher-lua to store it
	// in the table's hash part; tableToGo must notice the sequence is not
	// actually 1..N dense and fall back to a string-keyed map instead of
	// treating it as an array.
	tbl.RawSetInt(100000000, lua.LString("b"))

	want := map[string]any{"1": "a", "100000000": "b"}
	if got := fromLua(tbl); !reflect.DeepEqual(got, want) {
		t.Errorf("sparse table = %#v, want %#v", got, want)
	}
}
