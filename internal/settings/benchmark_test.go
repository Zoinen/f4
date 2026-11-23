package settings

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func BenchmarkFileGroupEditorOpen(b *testing.B) {
	settingsProfileSetup(b)
	store, runtime := fileGroupStore, fileGroupRuntime
	b.Cleanup(func() { ConfigureFileGroups(store, runtime) })
	ConfigureFileGroups(filegroups.Store{
		Path:     filepath.Join(config.GetF4ConfigDir(), "highlight.ini"),
		Defaults: theme.FileGroupDefaults(), ColorDefaults: filegroups.FactoryColorThemes(),
		ColorStyle: "Modern",
	}, nil)
	provider := fileGroupsProvider{}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(draft.Close)
	center := newSettingsCenter([]*settingsSession{{catalog: provider.Catalog(), draft: draft}})
	center.selectCategory("filegroups")
	overview := center.fileGroupOverview
	if len(overview.rows) < 2 {
		b.Fatalf("expected factory groups, got %d", len(overview.rows))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		editor := overview.childEditor(0)
		editor.SetPosition(0, 0, 145, 49)
		_ = editor.SemanticNode(&vtui.SemanticContext{Width: 160, Height: 60})
		b.StopTimer()
		editor.sessions[0].draft.Close()
		b.StartTimer()
	}
}

// Include provider snapshots, catalog discovery, layout and the first paint.
// No PanelsFrame is needed: opening settings must also work in standalone editors.
func BenchmarkSettingsOpen(b *testing.B) {
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(160, 50)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sessions, err := beginSettingsSessions(context.Background())
		if err != nil {
			b.Fatal(err)
		}
		c := newSettingsCenter(sessions)
		c.ResizeConsole(160, 50)
		c.Show(scr)
		for _, s := range sessions {
			s.draft.Close()
		}
	}
}

func BenchmarkSettingsHoverSearch(b *testing.B) {
	for _, query := range []string{"", "editor"} {
		name := query
		if name == "" {
			name = "empty"
		}
		b.Run(name, func(b *testing.B) {
			sessions, err := beginSettingsSessions(context.Background())
			if err != nil {
				b.Fatal(err)
			}
			defer func() {
				for _, s := range sessions {
					s.draft.Close()
				}
			}()
			c := newSettingsCenter(sessions)
			c.selectCategory("editor")
			c.SetPosition(0, 0, 159, 49)
			scr := vtui.NewSilentScreenBuf()
			scr.AllocBuf(160, 50)
			c.search.SetText(query)
			c.search.OnTextChange(query)
			c.Show(scr)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: testutil.Int16(c.page.X1 + 5), MouseY: testutil.Int16(c.page.Y1 + 1 + i%8), MouseEventFlags: vtinput.MouseMoved})
				c.Show(scr)
			}
		})
	}
}
