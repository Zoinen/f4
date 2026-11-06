package filegroups

import (
	"reflect"
	"strings"
	"testing"
)

func TestDocumentRoundTrip(t *testing.T) {
	source := []byte("# retained header\n[FileGroups]\nVersion=2\nOrder=images\nUnknown=keep\n\n[Vendor]\n# vendor comment\nAnswer = 42\n\n[Group.images]\n# group comment\nName=\"Images\"\nMask=\"*.png\"\nVendor.Extension=\"keep\"\n")
	d, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := d.Groups()
	if err != nil {
		t.Fatal(err)
	}
	command := CommandSet{ID: "app", Name: "Editor"}
	command.Commands[0] = "process \"C:\\Users\\xs\\file.png\"\n  --next 日本語"
	command.Enabled[0] = true
	groups[0].Commands = []CommandSet{command}
	groups[0].Fields["Icon.future-theme"] = "asset:future/test"
	data, err := d.Encode(groups)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# retained header", "# group comment", "# vendor comment", "Answer = 42", "Unknown = keep", "Vendor.Extension"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("lost %q in %s", want, data)
		}
	}
	reloaded, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reloaded.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Commands[0].Commands[0] != command.Commands[0] || got[0].Fields["Icon.future-theme"] != "asset:future/test" {
		t.Fatalf("roundtrip changed values: %#v", got)
	}
}

func TestMergeAndResetKeepNewFactoryValues(t *testing.T) {
	defaults := []Group{{ID: "images", Fields: map[string]string{"Name": "Images", "NormalColor": "blue", "Icon.zoin": "asset:image"}}}
	overrides := []Group{{ID: "images", Fields: map[string]string{"NormalColor": ""}}}
	got := Merge(defaults, overrides)
	if got[0].Fields["NormalColor"] != "" || got[0].Fields["Icon.zoin"] != "asset:image" {
		t.Fatal(got)
	}
	defaults[0].Fields["Icon.zoin"] = "asset:new-image"
	if Merge(defaults, overrides)[0].Fields["Icon.zoin"] != "asset:new-image" {
		t.Fatal("default frozen")
	}
	delete(overrides[0].Fields, "NormalColor")
	if Merge(defaults, overrides)[0].Fields["NormalColor"] != "blue" {
		t.Fatal("reset did not inherit")
	}
}

func TestDocumentOrderAndDeletion(t *testing.T) {
	d, _ := Parse(nil)
	groups := []Group{{ID: "b", Fields: map[string]string{"Name": "B"}}, {ID: "a", Fields: map[string]string{"Name": "A"}}}
	data, err := d.Encode(groups)
	if err != nil {
		t.Fatal(err)
	}
	d, err = Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ID != "b" {
		t.Fatal("order lost", got)
	}
	data, err = d.Encode(got[1:])
	if err != nil {
		t.Fatal(err)
	}
	d, err = Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err = d.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatal("deleted group returned", got)
	}
}

func TestDocumentRejectsInvalidVersionAndEscapes(t *testing.T) {
	for _, source := range []string{"[FileGroups]\nVersion=4\n", "[FileGroups]\nVersion=2\n[Group.x]\nName=\"\\q\"\n", "[Group.x\nMask=*\n"} {
		if _, err := Parse([]byte(source)); err == nil {
			t.Errorf("accepted %q", source)
		}
	}
}

func TestSnapshotDoesNotBorrowEditableMaps(t *testing.T) {
	groups := []Group{{ID: "x", Fields: map[string]string{"Name": "before"}}}
	s, err := Compile(groups)
	if err != nil {
		t.Fatal(err)
	}
	groups[0].Fields["Name"] = "after"
	copy := s.Groups()
	copy[0].Fields["Name"] = "mutated"
	if !reflect.DeepEqual(s.Groups()[0].Fields, map[string]string{"Name": "before"}) {
		t.Fatal("snapshot mutable")
	}
}
