package envman

import "testing"

func threeProfileConfig() Config {
	return Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "a", Variables: []string{"A=1"}},
			{Kind: KindProfile, Name: "b", Variables: []string{"B=1"}},
			{Kind: KindProfile, Name: "c", Variables: []string{"C=1"}},
		},
	}
}

func TestSetEntryEnabledValidatesIndexAndSkipsNonProfiles(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "p0", Enabled: false},
			{Kind: KindSeparator},
		},
	}

	if _, err := setEntryEnabled(config, -1, true); err == nil {
		t.Fatal("setEntryEnabled accepted a negative index")
	}
	if _, err := setEntryEnabled(config, len(config.Entries), true); err == nil {
		t.Fatal("setEntryEnabled accepted an out-of-range index")
	}

	unchanged, err := setEntryEnabled(config, 1, true)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Entries[1].Kind != KindSeparator {
		t.Fatalf("setEntryEnabled mutated a separator entry: %#v", unchanged.Entries[1])
	}

	next, err := setEntryEnabled(config, 0, true)
	if err != nil {
		t.Fatal(err)
	}
	if !next.Entries[0].Enabled {
		t.Fatal("setEntryEnabled did not enable the profile")
	}
	if config.Entries[0].Enabled {
		t.Fatal("setEntryEnabled mutated the original config")
	}
}

func TestToggleEntryFlipsProfilesAndValidatesIndex(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "p0", Enabled: false},
			{Kind: KindSeparator},
		},
	}

	if _, err := toggleEntry(config, -1); err == nil {
		t.Fatal("toggleEntry accepted a negative index")
	}
	if _, err := toggleEntry(config, len(config.Entries)); err == nil {
		t.Fatal("toggleEntry accepted an out-of-range index")
	}

	unchanged, err := toggleEntry(config, 1)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Entries[1].Kind != KindSeparator {
		t.Fatalf("toggleEntry mutated a separator entry: %#v", unchanged.Entries[1])
	}

	toggled, err := toggleEntry(config, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !toggled.Entries[0].Enabled {
		t.Fatal("toggleEntry did not flip a disabled profile on")
	}
	toggledAgain, err := toggleEntry(toggled, 0)
	if err != nil {
		t.Fatal(err)
	}
	if toggledAgain.Entries[0].Enabled {
		t.Fatal("toggleEntry did not flip an enabled profile off")
	}
}

func TestReplaceEntryValidatesIndexAndClonesWithoutMutatingOriginal(t *testing.T) {
	config := threeProfileConfig()

	if _, err := replaceEntry(config, -1, Entry{Kind: KindProfile, Name: "x"}); err == nil {
		t.Fatal("replaceEntry accepted a negative index")
	}
	if _, err := replaceEntry(config, len(config.Entries), Entry{Kind: KindProfile, Name: "x"}); err == nil {
		t.Fatal("replaceEntry accepted an out-of-range index")
	}

	next, err := replaceEntry(config, 1, Entry{Kind: KindProfile, Name: "B2", Variables: []string{"X=1"}})
	if err != nil {
		t.Fatal(err)
	}
	if next.Entries[1].Name != "B2" {
		t.Fatalf("replaceEntry name = %q, want B2", next.Entries[1].Name)
	}
	if config.Entries[1].Name != "b" {
		t.Fatalf("replaceEntry mutated the original config: %q", config.Entries[1].Name)
	}
}

func TestDeleteEntryValidatesIndexAndRemovesWithoutMutatingOriginal(t *testing.T) {
	config := threeProfileConfig()

	if _, _, err := deleteEntry(config, -1); err == nil {
		t.Fatal("deleteEntry accepted a negative index")
	}
	if _, _, err := deleteEntry(config, len(config.Entries)); err == nil {
		t.Fatal("deleteEntry accepted an out-of-range index")
	}

	next, removed, err := deleteEntry(config, 1)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Name != "b" {
		t.Fatalf("deleteEntry removed = %q, want b", removed.Name)
	}
	if len(next.Entries) != 2 || next.Entries[0].Name != "a" || next.Entries[1].Name != "c" {
		t.Fatalf("deleteEntry result = %#v", next.Entries)
	}
	if len(config.Entries) != 3 {
		t.Fatalf("deleteEntry mutated the original config: %#v", config.Entries)
	}
}

func TestDuplicateEntryValidatesIndex(t *testing.T) {
	config := threeProfileConfig()

	if _, _, err := duplicateEntry(config, -1); err == nil {
		t.Fatal("duplicateEntry accepted a negative index")
	}
	if _, _, err := duplicateEntry(config, len(config.Entries)); err == nil {
		t.Fatal("duplicateEntry accepted an out-of-range index")
	}
}

func TestInsertEntryClampsOutOfRangeIndexesAndClonesEntry(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "a"},
			{Kind: KindProfile, Name: "b"},
		},
	}

	front := insertEntry(config, -5, Entry{Kind: KindProfile, Name: "new"})
	if len(front.Entries) != 3 || front.Entries[0].Name != "new" {
		t.Fatalf("insertEntry(-5) = %#v", front.Entries)
	}

	back := insertEntry(config, 100, Entry{Kind: KindProfile, Name: "end"})
	if len(back.Entries) != 3 || back.Entries[2].Name != "end" {
		t.Fatalf("insertEntry(100) = %#v", back.Entries)
	}

	variables := []string{"X=1"}
	entry := Entry{Kind: KindProfile, Name: "mid", Variables: variables}
	middle := insertEntry(config, 1, entry)
	variables[0] = "MUTATED"
	if middle.Entries[1].Name != "mid" {
		t.Fatalf("insertEntry(1) = %#v", middle.Entries)
	}
	if middle.Entries[1].Variables[0] != "X=1" {
		t.Fatalf("insertEntry did not clone Variables: got %q", middle.Entries[1].Variables[0])
	}
}

func TestMoveEntryValidatesIndex(t *testing.T) {
	config := threeProfileConfig()
	if _, _, err := moveEntry(config, -1, 1); err == nil {
		t.Fatal("moveEntry accepted a negative index")
	}
	if _, _, err := moveEntry(config, len(config.Entries), 1); err == nil {
		t.Fatal("moveEntry accepted an out-of-range index")
	}
}

func TestMoveEntrySwapsWithinBounds(t *testing.T) {
	config := threeProfileConfig()
	next, selected, err := moveEntry(config, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if selected != 1 {
		t.Fatalf("moveEntry selected = %d, want 1", selected)
	}
	names := []string{next.Entries[0].Name, next.Entries[1].Name, next.Entries[2].Name}
	if names[0] != "b" || names[1] != "a" || names[2] != "c" {
		t.Fatalf("moveEntry order = %v, want [b a c]", names)
	}
}

func TestMoveEntryPastRightEdgeInsertsSeparator(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "a"},
			{Kind: KindProfile, Name: "b"},
		},
	}
	next, selected, err := moveEntry(config, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Entries) != 3 {
		t.Fatalf("moveEntry(right edge) entries = %#v", next.Entries)
	}
	if next.Entries[0].Name != "a" || next.Entries[1].Kind != KindSeparator || next.Entries[2].Name != "b" {
		t.Fatalf("moveEntry(right edge) = %#v", next.Entries)
	}
	if selected != 2 {
		t.Fatalf("moveEntry(right edge) selected = %d, want 2", selected)
	}
}

func TestTrimSeparatorsCollapsesLeadingSeparators(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindSeparator},
			{Kind: KindSeparator},
			{Kind: KindProfile, Name: "a"},
		},
	}
	next, selected := trimSeparators(config, 2)
	if len(next.Entries) != 1 || next.Entries[0].Name != "a" {
		t.Fatalf("trimSeparators(leading) = %#v", next.Entries)
	}
	if selected != 0 {
		t.Fatalf("trimSeparators(leading) selected = %d, want 0", selected)
	}
}

func TestTrimSeparatorsCollapsesTrailingSeparators(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "a"},
			{Kind: KindSeparator},
			{Kind: KindSeparator},
		},
	}
	next, selected := trimSeparators(config, 0)
	if len(next.Entries) != 1 || next.Entries[0].Name != "a" {
		t.Fatalf("trimSeparators(trailing) = %#v", next.Entries)
	}
	if selected != 0 {
		t.Fatalf("trimSeparators(trailing) selected = %d, want 0", selected)
	}
}

func TestTrimSeparatorsCollapsesConsecutiveMiddleSeparatorsAndShiftsSelection(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindProfile, Name: "a"},
			{Kind: KindSeparator},
			{Kind: KindSeparator},
			{Kind: KindProfile, Name: "b"},
		},
	}
	next, selected := trimSeparators(config, 3)
	if len(next.Entries) != 3 {
		t.Fatalf("trimSeparators(middle) entries = %#v", next.Entries)
	}
	if next.Entries[0].Name != "a" || next.Entries[1].Kind != KindSeparator || next.Entries[2].Name != "b" {
		t.Fatalf("trimSeparators(middle) = %#v", next.Entries)
	}
	if selected != 2 {
		t.Fatalf("trimSeparators(middle) selected = %d, want 2", selected)
	}
}

func TestTrimSeparatorsClampsSelectionWhenEverythingCollapses(t *testing.T) {
	config := Config{
		Version: CurrentConfigVersion,
		Entries: []Entry{
			{Kind: KindSeparator},
			{Kind: KindSeparator},
			{Kind: KindSeparator},
		},
	}
	next, selected := trimSeparators(config, 0)
	if len(next.Entries) != 0 {
		t.Fatalf("trimSeparators(all separators) entries = %#v", next.Entries)
	}
	if selected != 0 {
		t.Fatalf("trimSeparators(all separators) selected = %d, want 0", selected)
	}
}

func TestImportDriftEntryAddedChangedAndRemovedOrdering(t *testing.T) {
	options := OptionsForGOOS("linux")
	diff := Diff{
		Added:   []Change{{Name: "NEW", After: "value"}},
		Changed: []Change{{Name: "MID", Before: "old", After: "prefix-old-suffix"}},
		Removed: []Change{{Name: "GONE", Before: "was-there"}},
	}
	entry := importDriftEntry(diff, "Imported", options)
	if entry.Kind != KindProfile || entry.Name != "Imported" || !entry.Enabled {
		t.Fatalf("importDriftEntry header = %#v", entry)
	}
	want := []string{"NEW=value", "MID=prefix-%MID%-suffix", "GONE="}
	if len(entry.Variables) != len(want) {
		t.Fatalf("importDriftEntry variables = %v, want %v", entry.Variables, want)
	}
	for i, w := range want {
		if entry.Variables[i] != w {
			t.Errorf("importDriftEntry variables[%d] = %q, want %q", i, entry.Variables[i], w)
		}
	}
}

func TestImportDriftEntryChangedFallsBackToPlainValueWhenBeforeNotFound(t *testing.T) {
	options := OptionsForGOOS("linux")
	diff := Diff{
		Changed: []Change{{Name: "X", Before: "nomatch", After: "totally-different"}},
	}
	entry := importDriftEntry(diff, "n", options)
	if len(entry.Variables) != 1 || entry.Variables[0] != "X=totally-different" {
		t.Fatalf("importDriftEntry fallback = %v", entry.Variables)
	}
}

func TestEscapeImportedProfileValueEscapesPercentAndOptionalDollar(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		expand bool
		want   string
	}{
		{"percent always escaped", "50%", false, "50%%"},
		{"percent escaped with dollar expansion on", "50%", true, "50%%"},
		{"dollar escaped only when expansion enabled", "$PATH", true, "$$PATH"},
		{"dollar left alone when expansion disabled", "$PATH", false, "$PATH"},
		{"plain value unchanged", "plain", true, "plain"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeImportedProfileValue(tt.value, EngineOptions{ExpandDollarSyntax: tt.expand})
			if got != tt.want {
				t.Fatalf("escapeImportedProfileValue(%q, expand=%v) = %q, want %q", tt.value, tt.expand, got, tt.want)
			}
		})
	}
}
