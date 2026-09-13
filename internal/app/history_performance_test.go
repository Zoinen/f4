package app

import (
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

func TestHistoryNativeKeyHints(t *testing.T) {
	for _, key := range []string{"History.CommandsHint", "History.FoldersHint", "History.ViewEditHint"} {
		t.Run(key, func(t *testing.T) {
			screen := vtui.NewSilentScreenBuf()
			screen.AllocBuf(100, 40)
			vtui.FrameManager.Init(screen)
			menu := vtui.NewVMenu("History")
			hint := i18n.Msg(key)
			s := newHistorySearch(menu, []history.HistoryRecord{{Name: "command"}}, hint)
			defer s.cleanup()
			vtui.FrameManager.PushMenu(menu)
			defer vtui.FrameManager.RemoveFrame(menu)
			var previous map[string]any
			for _, query := range []string{"", "", "no matches"} {
				if query != "" {
					s.query = []rune(query)
					s.applyFilter()
				}
				next, _ := nativeui.BuildAppMenuState(nil, previous)
				if got := semantic.AppMapSlice(next["menus"])[0]["bottomHint"]; got != hint {
					t.Fatalf("native bottom hint = %q, want %q", got, hint)
				}
				previous = next
			}
		})
	}
}

func TestHistoryNativeRowsRetainedUntilFilterChanges(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(100, 40)
	vtui.FrameManager.Init(screen)
	menu := vtui.NewVMenu("History")
	menu.SetPosition(0, 0, 80, 20)
	s := newHistorySearch(menu, []history.HistoryRecord{
		{Name: "new command", Timestamp: time.Now()}, {Name: "old command"},
	}, "")
	menu.SemanticPresentation = "fullWidth"
	defer s.cleanup()
	vtui.FrameManager.PushMenu(menu)
	defer vtui.FrameManager.RemoveFrame(menu)
	previous, _ := nativeui.BuildAppMenuState(nil)
	if got := semantic.AppMapSlice(previous["menus"])[0]["presentation"]; got != "fullWidth" {
		t.Fatalf("native presentation = %q", got)
	}
	menu.SetSelectPos(0)
	next, _ := nativeui.BuildAppMenuState(nil, previous)
	wire := semantic.CompactMenuRows(previous, next)
	if !semantic.Bool(semantic.AppMapSlice(wire["menus"])[0]["itemsUnchanged"]) {
		t.Fatal("navigation retransmitted history")
	}
	s.query = []rune("new")
	s.applyFilter()
	next, _ = nativeui.BuildAppMenuState(nil, previous)
	if title := semantic.AppMapSlice(next["menus"])[0]["title"]; title != "History [new]" {
		t.Fatalf("native search title = %q", title)
	}
	rows := semantic.AppMapSlice(semantic.AppMapSlice(next["menus"])[0]["items"])
	if len(rows) != 1 {
		t.Fatalf("stale filtered rows: %v", rows)
	}
	if semantic.Bool(semantic.AppMapSlice(semantic.CompactMenuRows(previous, next)["menus"])[0]["itemsUnchanged"]) {
		t.Fatal("filter did not replace rows")
	}
	s.query = []rune("no matches")
	s.prefixOnly = true
	s.applyFilter()
	if menu.GetTitle() != "History [no matches*]" || len(menu.Items) != 0 {
		t.Fatal("empty results lost the query or prefix indicator")
	}
	s.query, s.prefixOnly = nil, false
	s.applyFilter()
	if menu.GetTitle() != "History" {
		t.Fatal("clearing the query did not restore the title")
	}
}

func TestHistoryUnfilteredDetailsOmitSearchMasks(t *testing.T) {
	s := &historySearch{showDirPrefix: true}
	details := s.semanticDetails(history.HistoryRecord{Name: "echo <text>"})
	for _, key := range []string{"primaryMatches", "pathMatches", "dateMatches"} {
		if _, exists := details[key]; exists {
			t.Fatal("unfiltered mask", key)
		}
	}
}

func TestHistoryCommandPrefixColor(t *testing.T) {
	for _, name := range []string{"echo argument", "команда аргумент", "single", " leading", "\"quoted path\" arg"} {
		t.Run(name, func(t *testing.T) {
			screen := vtui.NewSilentScreenBuf()
			screen.AllocBuf(100, 10)
			vtui.FrameManager.Init(screen)
			menu := vtui.NewVMenu("Commands")
			menu.SetPosition(1, 1, 95, 8)
			s := newHistorySearch(menu, []history.HistoryRecord{{Name: name}}, "")
			defer s.cleanup()
			s.showDirPrefix = true
			s.dirPrefixLen = 4
			s.applyFilter()
			for _, query := range []string{"", "a"} {
				s.query = []rune(query)
				menu.Show(screen)
				s.draw(screen)
				base := vtui.Palette[menu.ColorSelectedTextIdx]
				text := s.displayText(s.all[0])
				_, matches := historySearchMatch(text, s.query, false)
				prefix := true
				for i, r := range []rune(name) {
					if r == ' ' {
						prefix = false
					}
					want := base
					if prefix {
						want = vtui.SetRGBFore(base, 0x75d977)
					}
					if matches[i+5] {
						want = vtui.Palette[menu.ColorSelectedHighlightIdx]
					}
					if got := screen.GetCell(menu.X1+1+5+i, menu.Y1+1).Attributes; got != want {
						t.Fatalf("rune %d: color %x, want %x", i, got, want)
					}
				}
			}
		})
	}
}

func TestHistoryASCIIBoundsMatchUnicodeTruncation(t *testing.T) {
	menu := vtui.NewVMenu("History")
	menu.SetPosition(0, 0, 30, 20)
	s := &historySearch{menu: menu}
	for _, text := range []string{"", "short", "a very long command --argument with spaces", "команда --файл", "界界界界界界界界界界界界界界界界", "echo\ttext", "e\u0301 combining"} {
		got := s.defaultMenuText(text)
		if got != runewidth.Truncate(text, menu.X2-menu.X1-2, "…") {
			t.Fatalf("%q truncated incorrectly: %q", text, got)
		}
	}
}
