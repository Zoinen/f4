package app

import (
	"github.com/unxed/f4/internal/action"
	"os"
	"strings"
	"testing"
)

// The golden order lives with the table that produces it: actionMenuOrder in
// internal/action decides where a known action sits, and action_table.go is
// what registers them. This is the test that makes a silent menu reorder loud.

func TestActionOrderIsStable(t *testing.T) {
	data, err := os.ReadFile("testdata/action_order.golden")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields(string(data))

	var got []string
	for _, action := range action.All() {
		got = append(got, action.Name)
	}

	for index := 0; index < len(want) && index < len(got); index++ {
		if got[index] != want[index] {
			t.Fatalf("action %d = %q, want %q\n"+
				"the menu the user sees has been rearranged; if that is intended, "+
				"move the entry in actionMenuOrder and regenerate "+
				"testdata/action_order.golden", index, got[index], want[index])
		}
	}
	if len(got) != len(want) {
		t.Fatalf("presented actions = %d, want %d: %v", len(got), len(want), symmetricDifference(got, want))
	}
}

func symmetricDifference(got, want []string) []string {
	inWant := make(map[string]bool, len(want))
	for _, name := range want {
		inWant[name] = true
	}
	inGot := make(map[string]bool, len(got))
	for _, name := range got {
		inGot[name] = true
	}
	var only []string
	for _, name := range got {
		if !inWant[name] {
			only = append(only, "+"+name)
		}
	}
	for _, name := range want {
		if !inGot[name] {
			only = append(only, "-"+name)
		}
	}
	return only
}
