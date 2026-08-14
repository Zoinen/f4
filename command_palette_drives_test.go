package main

import (
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func replaceDriveRegistryForCommandPaletteTest(drives []DriveEntry) func() {
	pluginRegistryMu.Lock()
	previous := append([]DriveEntry(nil), DriveRegistry...)
	DriveRegistry = append([]DriveEntry(nil), drives...)
	pluginRegistryMu.Unlock()
	return func() {
		pluginRegistryMu.Lock()
		DriveRegistry = previous
		pluginRegistryMu.Unlock()
	}
}

func TestCommandPaletteDriveEntriesExposeRegistryNamesForBothPanels(t *testing.T) {
	restore := replaceDriveRegistryForCommandPaletteTest([]DriveEntry{
		{Name: "1. &NetFox", Factory: func() vfs.VFS { return nil }},
		{Name: "", Factory: func() vfs.VFS { return nil }},
		{Name: "Broken", Factory: nil},
	})
	t.Cleanup(restore)

	pf := &PanelsFrame{panels: [2]Panel{&FileSystemPanel{}, &FileSystemPanel{}}}
	setCommandPaletteActivePanelsForTest(t, pf)
	var entries []commandPaletteEntry
	for _, entry := range commandPaletteDriveEntries(pf) {
		if entry.ID == "1. &NetFox" {
			entries = append(entries, entry)
		}
	}
	if len(entries) != 2 {
		t.Fatalf("drive entries = %#v, want left and right NetFox commands", entries)
	}
	if entries[0].Key == entries[1].Key {
		t.Fatalf("left and right drive commands share key %q", entries[0].Key)
	}
	for _, entry := range entries {
		if entry.ID != "1. &NetFox" || !strings.Contains(entry.Label, "NetFox") || strings.Contains(entry.Label, "&") {
			t.Errorf("drive entry did not retain a clean, searchable registry name: %#v", entry)
		}
		if entry.Category != Msg("CommandPalette.CategoryDrive") || entry.run == nil {
			t.Errorf("drive entry lacks category or execution hook: %#v", entry)
		}
	}

	ranked := rankCommandPaletteEntries(entries, "меню дисков левой панели", nil)
	if len(ranked) != 1 || !strings.HasPrefix(ranked[0].Key, "drive:left:") {
		t.Fatalf("translated left-drive query ranked %#v, want only the left entry", commandPaletteTestKeys(ranked))
	}
}

func TestCommandPaletteDriveEntryReResolvesFactoryAndRejectsRemoval(t *testing.T) {
	oldCalls, replacementCalls := 0, 0
	restore := replaceDriveRegistryForCommandPaletteTest([]DriveEntry{{
		Name: "Mutable drive",
		Factory: func() vfs.VFS {
			oldCalls++
			return nil
		},
	}})
	t.Cleanup(restore)

	pf := &PanelsFrame{panels: [2]Panel{&FileSystemPanel{}, &FileSystemPanel{}}}
	setCommandPaletteActivePanelsForTest(t, pf)
	var entries []commandPaletteEntry
	for _, entry := range commandPaletteDriveEntries(pf) {
		if entry.ID == "Mutable drive" {
			entries = append(entries, entry)
		}
	}
	if len(entries) != 2 {
		t.Fatalf("drive entries = %d, want 2", len(entries))
	}
	left := entries[0]
	if !strings.HasPrefix(left.Key, "drive:left:") {
		left = entries[1]
	}

	pluginRegistryMu.Lock()
	DriveRegistry[0].Factory = func() vfs.VFS {
		replacementCalls++
		return nil
	}
	pluginRegistryMu.Unlock()
	if executeCommandPaletteEntry(left) {
		t.Fatal("nil replacement VFS was reported as a successful drive switch")
	}
	if oldCalls != 0 || replacementCalls != 1 {
		t.Fatalf("stale/current factory calls = %d/%d, want 0/1", oldCalls, replacementCalls)
	}

	pluginRegistryMu.Lock()
	DriveRegistry = nil
	pluginRegistryMu.Unlock()
	if executeCommandPaletteEntry(left) {
		t.Fatal("removed drive executed from a stale palette entry")
	}
	if oldCalls != 0 || replacementCalls != 1 {
		t.Fatalf("removed drive called a retained factory: stale/current=%d/%d", oldCalls, replacementCalls)
	}
}

func TestCommandPaletteDriveEntryDoesNotResolveAgainstClosedPanels(t *testing.T) {
	calls := 0
	restore := replaceDriveRegistryForCommandPaletteTest([]DriveEntry{{
		Name: "Closed panels drive",
		Factory: func() vfs.VFS {
			calls++
			return nil
		},
	}})
	t.Cleanup(restore)

	pf := &PanelsFrame{closed: true, panels: [2]Panel{&FileSystemPanel{}, &FileSystemPanel{}}}
	var entries []commandPaletteEntry
	for _, entry := range commandPaletteDriveEntries(pf) {
		if entry.ID == "Closed panels drive" {
			entries = append(entries, entry)
		}
	}
	if len(entries) != 2 {
		t.Fatalf("drive entries = %d, want 2", len(entries))
	}
	if executeCommandPaletteEntry(entries[0]) || calls != 0 {
		t.Fatalf("closed PanelsFrame resolved a drive factory %d times", calls)
	}
}
