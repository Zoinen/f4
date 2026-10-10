//go:build !extralite

package macro

import (
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestMacroNumberAndTrimHelpers(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), "")
	tests := []struct {
		expression string
		want       string
	}{
		{`mf.itoa(255)`, "255"},
		{`mf.itoa(255, 16)`, "ff"},
		{`mf.itoa(-5, 2)`, "-101"},
		{`mf.itoa(7, 0)`, "7"},
		{`tostring(mf.atoi("  42abc"))`, "42"},
		{`tostring(mf.atoi("-17"))`, "-17"},
		{`tostring(mf.atoi("+9"))`, "9"},
		{`tostring(mf.atoi("0x1F"))`, "31"},
		{`tostring(mf.atoi("017"))`, "15"},
		{`tostring(mf.atoi("ff", 16))`, "255"},
		{`tostring(mf.atoi("0xff", 16))`, "255"},
		{`tostring(mf.atoi("z", 36))`, "35"},
		{`tostring(mf.atoi("xyz"))`, "0"},
		{`tostring(mf.atoi(""))`, "0"},
		{`tostring(mf.mod(7, 3))`, "1"},
		{`tostring(mf.mod(-7, 3))`, "-1"},
		{`tostring(mf.mod(7, 0))`, "0"},
		{`mf.trim("  x  ", 1) .. "|"`, "x  |"},
		{`"|" .. mf.trim("  x  ", 2)`, "|  x"},
		{`mf.trim("  x  ", 0)`, "x"},
		{`mf.trim("  x  ", 5)`, "  x  "},
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

func TestMacroNumberHelpersRejectBadRadix(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), "")
	for _, expression := range []string{`mf.itoa(1, 1)`, `mf.itoa(1, 37)`, `mf.atoi("1", 1)`, `mf.atoi("1", 40)`, `mf.atoi("1", -2)`} {
		err := Engine.rt.Do(func(L *lua.LState) error { return L.DoString("__r = " + expression) })
		if err == nil {
			t.Errorf("%s accepted a bad radix", expression)
		}
	}
}

func TestParseIntPrefixOverflow(t *testing.T) {
	if got := parseIntPrefix("99999999999999999999", 10); got != 1<<63-1 {
		t.Errorf("overflow = %d", got)
	}
	if got := parseIntPrefix("-99999999999999999999", 10); got != -1<<63 {
		t.Errorf("negative overflow = %d", got)
	}
}

func TestStrftime(t *testing.T) {
	at := time.Date(2026, time.March, 5, 14, 7, 9, 0, time.FixedZone("TST", 3600))
	for _, tc := range []struct{ format, want string }{
		{"%Y-%m-%d %H:%M:%S", "2026-03-05 14:07:09"},
		{"%a %A %b %B %h", "Thu Thursday Mar March Mar"},
		{"%e|%d|%y|%j", " 5|05|26|064"},
		{"%I:%M %p", "02:07 PM"},
		{"%D %T %R", "03/05/26 14:07:09 14:07"},
		{"%Z %z", "TST +0100"},
		{"100%%", "100%"},
		{"a%nb%tc", "a\nb\tc"},
		{"%Q end%", "%Q end%"},
		{"", "Thu Mar 05 14:07:09 2026"},
	} {
		if got := strftime(tc.format, at); got != tc.want {
			t.Errorf("strftime(%q) = %q, want %q", tc.format, got, tc.want)
		}
	}
}

func TestMacroDateUsesTheClock(t *testing.T) {
	Engine := newTestMacroEngine(t, newFakeMacroHost(), "")
	var got string
	err := Engine.rt.Do(func(L *lua.LState) error {
		if err := L.DoString(`__result = mf.date("%Y")`); err != nil {
			return err
		}
		got = lua.LVAsString(L.GetGlobal("__result"))
		return nil
	})
	if err != nil || len(got) != 4 || !strings.HasPrefix(got, "20") {
		t.Errorf("mf.date(%%Y) = %q, %v", got, err)
	}
	if err := Engine.rt.Do(func(L *lua.LState) error { return L.DoString(`__result = mf.date()`) }); err != nil {
		t.Errorf("mf.date() failed: %v", err)
	}
}
