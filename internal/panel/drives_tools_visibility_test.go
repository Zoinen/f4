package panel

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/sysinfo"
)

func TestDriveToolsVisibilityRoundTripAndFilter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "drive-tools-visibility.txt")
	if names, err := LoadDisabledDriveTools(path); err != nil || names != nil {
		t.Fatalf("a missing file read as %v, %v; want no disabled tools", names, err)
	}
	if err := SaveDisabledDriveTools(path, []string{"Network", " ", "AI", "Network"}); err != nil {
		t.Fatal(err)
	}
	names, err := LoadDisabledDriveTools(path)
	if err != nil || !reflect.DeepEqual(names, []string{"Network", "AI"}) {
		t.Fatalf("stored visibility read back as %v, %v", names, err)
	}
	entries := []sysinfo.DriveEntry{{Name: "AI"}, {Name: "Android"}, {Name: "Network"}}
	visible := FilterVisibleDriveTools(entries, names)
	if got := []string{visible[0].Name}; !reflect.DeepEqual(got, []string{"Android"}) {
		t.Fatalf("visible tools = %v, want Android", got)
	}
	if got := FilterVisibleDriveTools(entries, nil); !reflect.DeepEqual(got, entries) {
		t.Fatalf("empty disabled list changed entries: %#v", got)
	}
}
