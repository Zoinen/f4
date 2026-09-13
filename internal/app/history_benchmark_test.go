package app

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Includes preparation, input dispatch, native projection and JSON encoding.
// Qt presentation has its own matching large-history benchmark.
func BenchmarkLargeHistory(b *testing.B) {
	for _, fixture := range []struct {
		name  string
		count int
		open  func(*panel.PanelsFrame)
	}{
		{"commands", 2432, actionCommandHistory}, {"files", 999, actionViewerEditorHistory}, {"folders", 1294, actionFoldersHistory},
	} {
		b.Run(fixture.name, func(b *testing.B) {
			screen := vtui.NewSilentScreenBuf()
			screen.AllocBuf(160, 50)
			vtui.FrameManager.Init(screen)
			previous := vtui.GlobalHistoryProvider
			hp := history.NewProviderAtPath(filepath.Join(b.TempDir(), "history.json"))
			vtui.GlobalHistoryProvider = hp
			defer func() { _ = hp.Close(); vtui.GlobalHistoryProvider = previous }()
			var source history.Far3History
			for i := 0; i < fixture.count; i++ {
				path := fmt.Sprintf(`C:\Projects\folder-%04d\long filename with spaces %04d.txt`, i, i)
				stamp := time.Unix(1700000000+int64(i), 0)
				source.Commands = append(source.Commands, history.HistoryRecord{Name: "app --input \"" + path + "\" --check", Dir: `C:\Projects`, Timestamp: stamp})
				source.Folders = append(source.Folders, history.HistoryRecord{Name: path, Timestamp: stamp})
				source.Files = append(source.Files, history.ViewerEditorRecord{Path: path, Display: path, Mode: history.HistoryModeView, Local: true, VFSType: "*vfs.OSVFS", Timestamp: stamp})
			}
			hp.MergeFar3History(source)
			pf := panel.NewPanelsFrame()
			defer pf.Close()
			pf.ResizeConsole(160, 50)
			var previousState map[string]any
			project := func(b *testing.B) {
				state, _ := nativeui.BuildAppMenuState(nil, previousState)
				data, err := json.Marshal(semantic.CompactMenuRows(previousState, state))
				previousState = state
				if err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(len(data)), "packet_B")
			}
			closeMenu := func() {
				previousState = nil
				if activeHistorySearch != nil {
					menu := activeHistorySearch.menu
					menu.Close()
					vtui.FrameManager.RemoveFrame(menu)
					activeHistorySearch.cleanup()
				}
			}
			b.Run("open", func(b *testing.B) {
				for b.Loop() {
					fixture.open(pf)
					project(b)
					closeMenu()
				}
			})
			b.Run("page", func(b *testing.B) {
				fixture.open(pf)
				defer closeMenu()
				menu := activeHistorySearch.menu
				previousState, _ = nativeui.BuildAppMenuState(nil)
				event := vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR}
				for b.Loop() {
					if menu.SelectPos < 60 {
						menu.SetSelectPos(len(menu.Items) - 1)
					}
					menu.ProcessKey(&event)
					project(b)
				}
			})
		})
	}
}
