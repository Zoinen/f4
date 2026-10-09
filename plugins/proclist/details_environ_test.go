//go:build linux || windows || darwin

package proclist

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

func TestSplitWindowsEnvironmentBlock(t *testing.T) {
	block := func(strs ...string) []uint16 {
		var units []uint16
		for _, s := range strs {
			units = append(units, utf16.Encode([]rune(s))...)
			units = append(units, 0)
		}
		return units
	}
	cases := []struct {
		name  string
		units []uint16
		want  []string
	}{
		{"terminated block", append(block("A=1", "Path=C:\\x;D:\\y"), 0), []string{"A=1", "Path=C:\\x;D:\\y"}},
		{"stops at the empty string", append(block("A=1", "", "B=2"), 0), []string{"A=1"}},
		{"hidden drive entries are left out", append(block("=C:=C:\\dir", "A=1"), 0), []string{"A=1"}},
		{"cut short keeps the last string", []uint16{'A', '=', '1', 0, 'B', '=', '2'}, []string{"A=1", "B=2"}},
		{"non-ASCII", append(block("Имя=значение"), 0), []string{"Имя=значение"}},
		{"empty", nil, nil},
	}
	for _, c := range cases {
		if got := splitWindowsEnvironmentBlock(c.units); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
