package filegroups

import "testing"

func TestCommandFieldOverridesFollowFactoryUpgrades(t *testing.T) {
	command := CommandSet{ID: "app", Name: "Factory editor"}
	command.Commands[0] = "open-v1"
	command.Enabled[0] = true
	command.Commands[4] = "edit-v1"
	command.Enabled[4] = true
	defaults := []Group{{ID: "images", Fields: map[string]string{"Associations.Enabled": "true"}, Commands: []CommandSet{command}}}
	groups := []Group{defaults[0].Clone()}
	groups[0].Commands[0].Commands[4] = "my-editor"
	patch := Differences(defaults, groups)
	if len(patch[0].Commands[0].Overrides) != 1 {
		t.Fatalf("command change froze other fields: %+v", patch)
	}
	document, _ := Parse(nil)
	data, err := document.Encode(patch)
	if err != nil {
		t.Fatal(err)
	}
	document, err = Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	patch, err = document.Groups()
	if err != nil {
		t.Fatal(err)
	}
	defaults[0].Commands[0].Commands[0] = "open-v2"
	defaults[0].Commands[0].Name = "Updated editor"
	merged := Merge(defaults, patch)
	if merged[0].Commands[0].Commands[0] != "open-v2" || merged[0].Commands[0].Commands[4] != "my-editor" || merged[0].Commands[0].Name != "Updated editor" {
		t.Fatal(merged)
	}
	groups[0].Commands = nil
	patch = Differences(defaults, groups)
	merged = Merge(defaults, patch)
	if merged[0].Commands[0].Enabled[0] || merged[0].Commands[0].Enabled[4] {
		t.Fatal("deleted factory command returned")
	}
}
