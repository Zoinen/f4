package vfs

import "testing"

func TestVFSItemPresentationDefaultsAndMetadata(t *testing.T) {
	regular := VFSItem{Name: "report.txt"}
	if got := regular.PresentationName(); got != "report.txt" {
		t.Fatalf("default presentation name = %q, want canonical name", got)
	}
	if regular.Kind != VFSItemRegular {
		t.Fatalf("zero row kind = %d, want regular", regular.Kind)
	}

	separator := VFSItem{
		Name:               "git-status-separator",
		DisplayName:        "── Unstaged ──",
		Kind:               VFSItemSeparator,
		ExtendedAttributes: map[string]string{"git.section": "unstaged"},
	}
	if got := separator.PresentationName(); got != "── Unstaged ──" {
		t.Fatalf("presentation name = %q", got)
	}
	if separator.Name != "git-status-separator" {
		t.Fatalf("display metadata changed canonical name to %q", separator.Name)
	}
	if separator.Kind != VFSItemSeparator || separator.ExtendedAttributes["git.section"] != "unstaged" {
		t.Fatalf("presentation metadata = %#v", separator)
	}

	action := VFSItem{Kind: VFSItemAction}
	if action.Kind != VFSItemAction {
		t.Fatalf("action row kind = %d", action.Kind)
	}
}
