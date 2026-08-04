package luaplug

import (
	"reflect"
	"testing"

	lua "github.com/yuin/gopher-lua"
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
		{int64(7), lua.LNumber(7)},
		{uint32(9), lua.LNumber(9)},
		{uintptr(0x40), lua.LNumber(0x40)},
		{2.5, lua.LNumber(2.5)},
	}
	for _, tc := range cases {
		if got := toLua(L, tc.in); got != tc.want {
			t.Errorf("toLua(%#v) = %v, want %v", tc.in, got, tc.want)
		}
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
