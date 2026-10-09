package dialog

import (
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestAboutLinesAlignLabelsAndHideEmptyRows(t *testing.T) {
	rows := []aboutRow{
		{label: "f4 version", value: "v1"},
		{label: "PID", value: ""},
		{divider: true},
		{label: "TERM", value: ""},
		{divider: true},
		{label: "Number of plugins", value: "0"},
	}

	got := aboutLines(rows, true)
	want := []aboutLine{
		{text: "       f4 version: v1"},
		{divider: true},
		{text: "Number of plugins: 0"},
	}
	if len(got) != len(want) {
		t.Fatalf("hidden lines = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d = %#v, want %#v", i, got[i], want[i])
		}
	}

	if all := aboutLines(rows, false); len(all) != 6 {
		t.Fatalf("unhidden lines = %d, want 6: %#v", len(all), all)
	}
}

func TestAboutTextCopiesHiddenRowsToo(t *testing.T) {
	rows := []aboutRow{
		{label: "Version", value: "v1"},
		{label: "PID", value: ""},
		{divider: true},
		{label: "TERM", value: "xterm"},
	}
	want := "Version: v1\n    PID:\n\n   TERM: xterm"
	if got := aboutText(rows); got != want {
		t.Fatalf("aboutText = %q, want %q", got, want)
	}
}

func TestAboutMenuItemsEscapeAmpersandsAndKeepDividers(t *testing.T) {
	items := aboutMenuItems([]aboutLine{{text: "Shell: \"a&b\""}, {divider: true}}, 80)
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	if items[0].Text != "Shell: \"a&&b\"" {
		t.Fatalf("text = %q, want the ampersand doubled", items[0].Text)
	}
	if !items[1].Separator {
		t.Fatalf("divider did not become a separator: %#v", items[1])
	}
}

func TestParseOSReleasePrettyName(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{"NAME=Debian\nPRETTY_NAME=\"Debian GNU/Linux 13 (trixie)\"\n", "Debian GNU/Linux 13 (trixie)"},
		{"PRETTY_NAME=Alpine\n", "Alpine"},
		{"PRETTY_NAME='Void Linux'\n", "Void Linux"},
		{"NAME=Nothing\n", ""},
	} {
		if got := parseOSReleasePrettyName(strings.NewReader(tc.data)); got != tc.want {
			t.Errorf("parseOSReleasePrettyName(%q) = %q, want %q", tc.data, got, tc.want)
		}
	}
}

func TestCollectAboutRowsCarriesTheFactsItIsGiven(t *testing.T) {
	text := aboutText(collectAboutRows(AboutFacts{
		Version:         "v9.9.9",
		Plugins:         []string{"NetFox", "SQLite"},
		CommandPrefixes: []AboutCommandPrefix{{Prefix: "f4", Owner: "f4"}},
	}))
	for _, want := range []string{
		"f4 version: v9.9.9",
		"Platform: " + runtime.GOOS + "/" + runtime.GOARCH,
		"Admin: -",
		"Command prefix: f4: (f4)",
		"Number of plugins: 2",
		"Plugin #01: NetFox",
		"Plugin #02: SQLite",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("about text lacks %q:\n%s", want, text)
		}
	}
}

func TestCollectAboutRowsListsBackendDetails(t *testing.T) {
	previousName, previousDetails := vtui.ActiveBackend(), vtui.BackendDetails()
	t.Cleanup(func() { vtui.SetActiveBackend(previousName, previousDetails...) })
	vtui.SetActiveBackend("win32", "cell 8x16, font \"Consolas\"")

	text := aboutText(collectAboutRows(AboutFacts{}))
	if !strings.Contains(text, "Backend detail: cell 8x16, font \"Consolas\"") {
		t.Fatalf("about text lacks the backend detail:\n%s", text)
	}
}

func TestAboutTitleMarksHiddenRows(t *testing.T) {
	plain, marked := aboutTitle(false), aboutTitle(true)
	if marked != plain+" *" {
		t.Fatalf("aboutTitle(true) = %q, want %q", marked, plain+" *")
	}
}
