package vfs

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// #875, second round: on a Windows machine set to a UTF-8 system codepage the
// system aliases both carry id 65001. They must not push the real UTF-8
// entry out of the list, and 65001 must still read "UTF-8".
func TestCodepages_SystemAliasNeverShadowsUTF8(t *testing.T) {
	list := uniqueCodepages([]Codepage{
		{ID: 65001, Name: "System ANSI (65001)", Enc: unicode.UTF8, group: codepageSystem},
		{ID: 65001, Name: "System OEM (65001)", Enc: unicode.UTF8, group: codepageSystem},
		{ID: 65001, Name: "UTF-8", Enc: unicode.UTF8, group: codepageUnicode},
		{ID: 1200, Name: "UTF-16 (Little endian)", Enc: unicode.UTF16(unicode.LittleEndian, unicode.UseBOM), group: codepageUnicode},
		{ID: 1251, Name: "Windows-1251 (Cyrillic)", Enc: charmap.Windows1251, group: codepageOther},
	})
	var names []string
	for _, cp := range list {
		names = append(names, cp.Name)
	}
	got := strings.Join(names, "|")
	if strings.Contains(got, "System ANSI") || strings.Contains(got, "System OEM") {
		t.Fatalf("a system alias with a Unicode id survived: %s", got)
	}
	if !strings.Contains(got, "UTF-8") {
		t.Fatalf("UTF-8 was lost to deduplication: %s", got)
	}
	for _, cp := range list {
		if cp.ID == 65001 && cp.group != codepageUnicode {
			t.Fatalf("65001 kept as %q (group %d), want the Unicode entry", cp.Name, cp.group)
		}
	}
}

// The normal case is untouched: a system alias with a legacy id stays and
// wins over a later entry with the same id.
func TestCodepages_SystemAliasStillWinsOverLegacyDuplicate(t *testing.T) {
	list := uniqueCodepages([]Codepage{
		{ID: 1251, Name: "System ANSI (1251)", Enc: charmap.Windows1251, group: codepageSystem},
		{ID: 65001, Name: "UTF-8", Enc: unicode.UTF8, group: codepageUnicode},
		{ID: 1251, Name: "Windows-1251 (Cyrillic)", Enc: charmap.Windows1251, group: codepageOther},
	})
	if len(list) != 2 {
		t.Fatalf("got %d entries, want 2", len(list))
	}
	if list[0].Name != "System ANSI (1251)" {
		t.Fatalf("system alias lost: %q", list[0].Name)
	}
}

func TestCodepages_DisplayNameUTF8BeatsSystemAlias(t *testing.T) {
	oldANSI, oldOEM := systemANSI, systemOEM
	systemANSI, systemOEM = 65001, 65001
	defer func() { systemANSI, systemOEM = oldANSI, oldOEM }()

	if got := DisplayCodepageName(65001); got != "UTF-8" {
		t.Fatalf("DisplayCodepageName(65001) on a UTF-8 system = %q, want UTF-8", got)
	}
}

// With no system entries, the menu must not open with an empty " System "
// header; the current codepage is marked even while Auto-detect is ticked.
func TestCodepages_MenuSkipsEmptyGroupAndMarksCurrent(t *testing.T) {
	oldList := AvailableCodepages
	AvailableCodepages = []Codepage{
		{ID: 65001, Name: "UTF-8", Enc: unicode.UTF8, group: codepageUnicode},
		{ID: 1251, Name: "Windows-1251 (Cyrillic)", Enc: charmap.Windows1251, group: codepageOther},
	}
	defer func() { AvailableCodepages = oldList }()

	items, currIdx := BuildCodepageMenuItems(65001, true)
	var texts []string
	for _, it := range items {
		texts = append(texts, it.Text)
	}
	joined := strings.Join(texts, "\n")
	if strings.Contains(joined, " System ") {
		t.Fatalf("empty System header rendered:\n%s", joined)
	}
	if !strings.Contains(joined, "√ Auto-detect") {
		t.Fatalf("Auto-detect not ticked:\n%s", joined)
	}
	if !strings.Contains(joined, "√ 65001  UTF-8") {
		t.Fatalf("current codepage not marked while Auto-detect is on:\n%s", joined)
	}
	if currIdx != 0 {
		t.Fatalf("cursor should rest on Auto-detect when it is on, got %d", currIdx)
	}
}
