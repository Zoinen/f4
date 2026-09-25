package extui

import (
	"math"
	"reflect"
	"testing"
)

func TestExifFileFieldDisplay(t *testing.T) {
	tests := []struct {
		name  string
		value FileFieldValue
		want  string
	}{
		{"shutter fraction", FileFieldValue{State: FileFieldKnown, Kind: FileFieldNumber, Number: 1.0 / 125, Unit: "s"}, "1/125 с"},
		{"shutter seconds", FileFieldValue{State: FileFieldKnown, Kind: FileFieldNumber, Number: 1.3, Unit: "s"}, "1.3 с"},
		{"aperture", FileFieldValue{State: FileFieldKnown, Kind: FileFieldNumber, Number: 2.8, Unit: "f"}, "f/2.8"},
		{"iso", FileFieldValue{State: FileFieldKnown, Kind: FileFieldInteger, Integer: 800}, "800"},
		{"35mm focal length", FileFieldValue{State: FileFieldKnown, Kind: FileFieldNumber, Number: 75, Unit: "mm"}, "75 мм"},
		{"zoom range", FileFieldValue{State: FileFieldKnown, Kind: FileFieldRange, Min: 24, Max: 70}, "24–70 мм"},
		{"fixed range", FileFieldValue{State: FileFieldKnown, Kind: FileFieldRange, Min: 50, Max: 50}, "50 мм"},
		{"unread", FileFieldValue{State: FileFieldUnread, Kind: FileFieldNumber, Number: 12, Unit: "mm"}, ""},
		{"missing", FileFieldValue{State: FileFieldMissing, Kind: FileFieldNumber}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.value.Display(); got != test.want {
				t.Fatalf("Display() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestFileFieldDisplayDoesNotInventInvalidNumericValues(t *testing.T) {
	for _, value := range []FileFieldValue{
		{State: FileFieldKnown, Kind: FileFieldNumber, Unit: "mm", Number: math.NaN()},
		{State: FileFieldKnown, Kind: FileFieldNumber, Unit: "f", Number: math.Inf(1)},
		{State: FileFieldKnown, Kind: FileFieldInteger, Integer: 0},
		{State: FileFieldKnown, Kind: FileFieldRange, Min: 24, Max: 0},
	} {
		if display := value.Display(); display != "" {
			t.Errorf("invalid value %#v displayed as %q", value, display)
		}
	}
}

func TestExifFileFieldDescriptorsAreDefensiveAndTyped(t *testing.T) {
	first := ExifFileFieldDescriptors()
	if len(first) != 7 {
		t.Fatalf("got %d descriptors, want 7", len(first))
	}
	if first[0].ID != "exif.exposure_time" || first[4].Kind != FileFieldRange {
		t.Fatalf("unexpected initial schema: %#v", first)
	}
	first[0].Operations[0] = "corrupted"
	second := ExifFileFieldDescriptors()
	if second[0].Operations[0] != "eq" {
		t.Fatalf("descriptor operation slice was shared: %#v", second[0].Operations)
	}
}

func TestFileFieldDescriptorRegistryAcceptsAdditionalTypedFields(t *testing.T) {
	descriptor := FileFieldDescriptor{
		ID: "test.custom_distance", Title: "Distance", Kind: FileFieldNumber,
		Unit: "km", Format: "quantity", Precision: 2,
		Operations: []string{"eq", "gt", "has", "missing"},
	}
	if err := RegisterFileFieldDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	if err := RegisterFileFieldDescriptor(descriptor); err != nil {
		t.Fatalf("identical registration should be idempotent: %v", err)
	}
	registered, ok := FileFieldDescriptorByID(descriptor.ID)
	if !ok {
		t.Fatal("registered descriptor missing")
	}
	if got := (FileFieldValue{
		State: FileFieldKnown, Kind: registered.Kind, Unit: registered.Unit,
		Format: registered.Format, Precision: registered.Precision, Number: 2.5,
	}).Display(); got != "2.50 km" {
		t.Fatalf("registered formatter produced %q, want %q", got, "2.50 km")
	}
	registered.Operations[0] = "changed"
	again, _ := FileFieldDescriptorByID(descriptor.ID)
	if again.Operations[0] != "eq" {
		t.Fatalf("lookup shared mutable operations: %#v", again.Operations)
	}
	conflicting := descriptor
	conflicting.Title = "Different title"
	if err := RegisterFileFieldDescriptor(conflicting); err == nil {
		t.Fatal("conflicting descriptor re-registration was accepted")
	}
}

func TestFileFieldUpdateModelToMap(t *testing.T) {
	update := FileFieldUpdateModel{
		PanelID: "left", Generation: 9, SourceKey: "source-key", SourceVersion: "v2",
		Complete: true,
		Values: map[string]FileFieldValue{
			"exif.iso": {State: FileFieldKnown, Kind: FileFieldInteger, Integer: 800},
		},
	}
	want := M{
		"type": "panel_file_fields_update", "panelId": "left", "generation": int64(9),
		"sourceKey": "source-key", "sourceVersion": "v2", "complete": true,
		"values": M{"exif.iso": M{"state": "known", "kind": "integer", "integer": int64(800)}},
	}
	if got := update.ToMap(); !reflect.DeepEqual(got, want) {
		t.Fatalf("ToMap() = %#v, want %#v", got, want)
	}
}
