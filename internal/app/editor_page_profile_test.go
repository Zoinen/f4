package app

import (
	"encoding/json"
	"fmt"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	chromaplugin "github.com/unxed/f4/plugins/chroma"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in, read-only replay of repeated Page Down on an existing file.
func TestEditorPageDownProfile(t *testing.T) {
	path := os.Getenv("F4_EDITOR_PAGE_PROFILE_FILE")
	if path == "" {
		t.Skip("opt-in Page Down profile")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(300, 52)
	vtui.FrameManager.Init(screen)
	previous := vtui.AppSceneAdapter
	vtui.AppSceneAdapter = nativeui.BuildAppSceneFromLegacy
	t.Cleanup(func() { vtui.AppSceneAdapter = previous })
	v := editor.NewEditorView(piecetable.New(data), vfs.NewOSVFS(filepath.Dir(path)), path)
	v.Highlighter = (&chromaplugin.ChromaProvider{}).Create(path, string(data))
	defer v.Close()
	vtui.FrameManager.Push(v)
	v.SetPosition(0, 0, 299, 50)
	semantic.ApplyNativeDocumentViewport(v, semantic.NativeDocumentGeometry{Columns: 300, Rows: 48, Revision: 1})
	output := os.Getenv("F4_EDITOR_PAGE_PROFILE_DIR")
	if output != "" {
		if err := os.MkdirAll(output, 0700); err != nil {
			t.Fatal(err)
		}
		initial, _ := json.Marshal(vtui.FrameManager.ExportSemanticScene())
		if err := os.WriteFile(filepath.Join(output, "initial.json"), initial, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for cycle := 0; cycle < 3; cycle++ {
		v.CursorLine = 0
		v.CursorPos = 0
		v.ScrollTopRow = 0
		for page := 0; page < 120; page++ {
			start := time.Now()
			v.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT})
			key := time.Since(start)
			start = time.Now()
			scene := vtui.FrameManager.ExportSemanticScene()
			project := time.Since(start)
			start = time.Now()
			encoded, err := json.Marshal(scene)
			if err != nil {
				t.Fatal(err)
			}
			marshal := time.Since(start)
			if output != "" && cycle == 0 && page < 39 {
				if err := os.WriteFile(filepath.Join(output, fmt.Sprintf("hit-%02d.json", page)), encoded, 0600); err != nil {
					t.Fatal(err)
				}
			}
			t.Logf("PAGE cycle=%d page=%d line=%d key_us=%.3f scene_us=%.3f json_us=%.3f bytes=%d", cycle, page, v.CursorLine, float64(key.Nanoseconds())/1e3, float64(project.Nanoseconds())/1e3, float64(marshal.Nanoseconds())/1e3, len(encoded))
			drainDocumentBenchmarkTasks()
		}
	}
}
