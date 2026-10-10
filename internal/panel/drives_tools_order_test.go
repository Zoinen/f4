package panel

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/sysinfo"
)

func toolEntries(names ...string) []sysinfo.DriveEntry {
	entries := make([]sysinfo.DriveEntry, len(names))
	for i, name := range names {
		entries[i] = sysinfo.DriveEntry{Name: name}
	}
	return entries
}

func toolNames(entries []sysinfo.DriveEntry) []string {
	names := make([]string, len(entries))
	for i, entry := range entries {
		names[i] = entry.Name
	}
	return names
}

func TestOrderDriveToolsPutsStoredOrderFirstAndKeepsTheRest(t *testing.T) {
	entries := toolEntries("AI", "Android", "Network", "iOS")
	got := toolNames(orderDriveTools(entries, []string{"iOS", "Gone", "AI"}))
	want := []string{"iOS", "AI", "Android", "Network"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if got := toolNames(orderDriveTools(entries, nil)); !reflect.DeepEqual(got, []string{"AI", "Android", "Network", "iOS"}) {
		t.Fatalf("no stored order changed the registry order: %v", got)
	}
}

func TestMoveDriveToolStopsAtTheEnds(t *testing.T) {
	names := []string{"AI", "Android", "Network"}
	if moved, ok := moveDriveTool(names, "Android", -1); !ok || !reflect.DeepEqual(moved, []string{"Android", "AI", "Network"}) {
		t.Fatalf("up: %v %v", moved, ok)
	}
	if _, ok := moveDriveTool(names, "AI", -1); ok {
		t.Fatal("the first tool moved up")
	}
	if _, ok := moveDriveTool(names, "Network", 1); ok {
		t.Fatal("the last tool moved down")
	}
	if _, ok := moveDriveTool(names, "Missing", 1); ok {
		t.Fatal("a tool that is not there moved")
	}
	if !reflect.DeepEqual(names, []string{"AI", "Android", "Network"}) {
		t.Fatalf("the caller's slice was changed: %v", names)
	}
}

func TestDriveToolsOrderRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "drive-tools-order.txt")
	if names, err := LoadDriveToolsOrder(path); err != nil || names != nil {
		t.Fatalf("a missing file read as %v, %v; want an empty order", names, err)
	}
	if err := SaveDriveToolsOrder(path, []string{"Network", " ", "AI"}); err != nil {
		t.Fatal(err)
	}
	names, err := LoadDriveToolsOrder(path)
	if err != nil || !reflect.DeepEqual(names, []string{"Network", "AI"}) {
		t.Fatalf("stored order read back as %v, %v", names, err)
	}
}
