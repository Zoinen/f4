package editor

import (
	"testing"

	colorer "github.com/unxed/colorer4go"
)

func TestManageColorerOutlineTree(t *testing.T) {
	var stack []int
	var got []int
	for _, level := range []int{1, 3, 3, 5, 2, 1} {
		var tree int
		stack, tree = manageColorerOutlineTree(stack, level)
		got = append(got, tree)
	}
	want := []int{0, 1, 1, 2, 1, 0}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tree levels %v, want %v", got, want)
		}
	}
}

func TestColorerOutlineRow(t *testing.T) {
	entry := colorerOutlineEntry{line: 41, item: colorer.OutlineItem{Region: "def:Function"}, label: "alpha"}
	if got := colorerOutlineRow(entry, 2, false, ""); got != "  42     f alpha" {
		t.Errorf("row %q", got)
	}
	if got := colorerOutlineRow(entry, 2, true, "int alpha(void)\n"); got != "int alpha(void)" {
		t.Errorf("old row %q", got)
	}
}

func TestColorerWordAtAndPick(t *testing.T) {
	for _, c := range []struct {
		text string
		pos  int
		want string
	}{{"alpha(beta)", 0, "alpha"}, {"alpha(beta)", 8, "beta"}, {"x_1", 2, "x_1"}, {"a b", 1, ""}, {"Жук", 2, "Жук"}} {
		if got := colorerWordAt(c.text, c.pos); got != c.want {
			t.Errorf("colorerWordAt(%q, %d) = %q, want %q", c.text, c.pos, got, c.want)
		}
	}
	entries := []colorerOutlineEntry{{line: 1, label: "Beta"}, {line: 5, label: "betamax"}, {line: 9, label: "beta"}}
	if got, ok := pickColorerFunction(entries, "BETA", 9); !ok || got.line != 5 {
		t.Errorf("pick = %+v, %v; want line 5, the last one off the cursor line", got, ok)
	}
	if got, ok := pickColorerFunction(entries[2:], "beta", 9); !ok || got.line != 9 {
		t.Errorf("pick on the cursor line only = %+v, %v", got, ok)
	}
	if _, ok := pickColorerFunction(entries, "gamma", 0); ok {
		t.Error("found a function that is not there")
	}
}
