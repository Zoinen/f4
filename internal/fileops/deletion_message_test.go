package fileops

import (
	vtui "github.com/unxed/vtui"
	strings "strings"
	testing "testing"
)

func TestDeletionErrorListPreservesUnwrappedSemanticMessages(t *testing.T) {
	message := "Skipped 'photo & notes.png': " + strings.Repeat("long path segment ", 12)
	list := &deletionErrorList{ListBox: vtui.NewListBox(0, 0, 56, 9, vtui.WrapText(message, 54)), messages: []string{message}}
	if len(list.Items) < 2 {
		t.Fatal("fixture did not wrap console rows")
	}
	node := list.SemanticNode(&vtui.SemanticContext{})
	items := node["items"].([]string)
	if len(items) != 1 || items[0] != message {
		t.Fatalf("console wrapping leaked into semantic data: %#v", items)
	}
}
