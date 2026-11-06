package filegroups

import "testing"

func TestPreviewApplyCancel(t *testing.T) {
	groups := []Group{{ID: "x", Fields: map[string]string{"Name": "committed"}}}
	count := 0
	r, err := NewRuntime(groups, func(*Snapshot) { count++ })
	if err != nil {
		t.Fatal(err)
	}
	groups[0].Fields["Name"] = "preview"
	if err := r.Preview(groups); err != nil {
		t.Fatal(err)
	}
	if r.Committed().Groups()[0].Label() != "committed" || r.Appearance().Groups()[0].Label() != "preview" {
		t.Fatal("preview leaked into commands")
	}
	r.CancelPreview()
	if r.Appearance().Groups()[0].Label() != "committed" {
		t.Fatal("cancel failed")
	}
	if err := r.Apply(groups); err != nil {
		t.Fatal(err)
	}
	groups[0].Fields["Name"] = "unsaved"
	if err := r.Preview(groups); err != nil {
		t.Fatal(err)
	}
	r.CancelPreview()
	if r.Appearance().Groups()[0].Label() != "preview" {
		t.Fatal("Cancel rolled back Apply")
	}
	before := r.Appearance().Revision()
	groups[0].Fields["Mask"] = "["
	if err := r.Preview(groups); err == nil || r.Appearance().Revision() != before {
		t.Fatal("invalid preview replaced snapshot")
	}
	if count != 5 {
		t.Fatalf("notifications=%d want 5", count)
	}
}

func TestPreviewOwnershipAndFactoryBooleanInheritance(t *testing.T) {
	groups := []Group{{ID: "g", Fields: map[string]string{"Name": "Original"}}}
	r, err := NewRuntime(groups, nil)
	if err != nil {
		t.Fatal(err)
	}
	first, second := r.BeginPreview(), r.BeginPreview()
	groups[0].Fields["Name"] = "Preview"
	snapshot, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	first.Publish(snapshot)
	second.Publish(snapshot)
	first.Cancel()
	if r.Appearance().Groups()[0].Label() != "Preview" {
		t.Fatal("older editor cancelled newer preview")
	}
	second.Cancel()
	if r.Appearance().Groups()[0].Label() != "Original" {
		t.Fatal("owner rollback failed")
	}
	base := []Group{{ID: "g", Fields: map[string]string{"Name": "Original"}}}
	edited := []Group{{ID: "g", Fields: map[string]string{"Name": "Original", "Enabled": "true", "Icons.Enabled": "false"}, Commands: []CommandSet{}}}
	patch := Differences(base, edited)
	if len(patch[0].Fields) != 0 || patch[0].Commands != nil {
		t.Fatalf("synthetic editor defaults became overrides: %+v", patch)
	}
}
