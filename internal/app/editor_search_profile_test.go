package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/textsearch"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Opt-in diagnostic: repeats real editor searches without editing the source.
func TestEditorRepeatedSearchProfile(t *testing.T) {
	path, output := os.Getenv("F4_SEARCH_PROFILE_FILE"), os.Getenv("F4_SEARCH_PROFILE_DIR")
	if path == "" || output == "" {
		t.Skip("opt-in editor search profile")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(300, 50)
	vtui.FrameManager.Init(screen)
	previousAdapter := vtui.AppSceneAdapter
	t.Cleanup(func() { vtui.AppSceneAdapter = previousAdapter })
	vtui.AppSceneAdapter = nativeui.BuildAppSceneFromLegacy
	v := editor.NewEditorView(piecetable.New(data), vfs.NewOSVFS(filepath.Dir(path)), path)
	t.Cleanup(v.Close)
	vtui.FrameManager.Push(v)
	v.SetPosition(0, 0, 299, 48)
	semantic.ApplyNativeDocumentViewport(v, semantic.NativeDocumentGeometry{Columns: 300, Rows: 46, Revision: 1})
	oldPattern, oldCase, oldReverse := editor.LastEditorSearch, editor.LastEditorSearchCase, editor.LastEditorSearchReverse
	oldRegex, oldWord, oldHex := editor.LastEditorSearchRegexp, editor.LastEditorSearchWholeWord, editor.LastEditorSearchHex
	t.Cleanup(func() {
		editor.LastEditorSearch = oldPattern
		editor.LastEditorSearchCase = oldCase
		editor.LastEditorSearchReverse = oldReverse
		editor.LastEditorSearchRegexp = oldRegex
		editor.LastEditorSearchWholeWord = oldWord
		editor.LastEditorSearchHex = oldHex
	})
	editor.LastEditorSearch = "mp4"
	editor.LastEditorSearchCase = false
	editor.LastEditorSearchReverse = false
	editor.LastEditorSearchRegexp = false
	editor.LastEditorSearchWholeWord = false
	editor.LastEditorSearchHex = false
	if err = os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	export := func(name string) {
		bytes, err := json.Marshal(vtui.FrameManager.ExportSemanticScene())
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, name+".json"), bytes, 0600); err != nil {
			t.Fatal(err)
		}
	}
	_ = v.SemanticNode(nil)
	for cycle := 0; cycle < 10; cycle++ {
		v.CursorLine = 0
		v.CursorPos = 0
		v.SelActive = false
		v.ScrollTopRow = 0
		if cycle == 0 {
			export("initial")
		}
		for hit, start := 0, 0; hit < 39; hit++ {
			scanStart := time.Now()
			expected, length, err := textsearch.FindMatch(data, "mp4", false, false, false, false, false, start)
			scan := time.Since(scanStart)
			if err != nil {
				t.Fatal(err)
			}
			began := time.Now()
			v.Search("mp4", false, false, false, false, true)
			var callback, exportTime time.Duration
			tasks := 0
			deadline := time.NewTimer(3 * time.Second)
			for {
				select {
				case task := <-vtui.FrameManager.TaskChan:
					ts := time.Now()
					task()
					callback += time.Since(ts)
					tasks++
					vtui.FrameManager.Step(0)
					if cycle == 0 && tasks == 1 {
						ts = time.Now()
						export(fmt.Sprintf("progress-%02d", hit))
						exportTime += time.Since(ts)
					}
				case <-deadline.C:
					t.Fatal("editor search did not finish")
				}
				if expected < 0 {
					if vtui.FrameManager.GetTopFrame() != v {
						break
					}
				} else if v.SelActive {
					a, b := v.GetSelectionRange()
					if a == expected && b == expected+length {
						break
					}
				}
			}
			deadline.Stop()
			elapsed := time.Since(began) - exportTime
			projectionStart := time.Now()
			_ = v.SemanticNode(nil)
			projection := time.Since(projectionStart)
			t.Logf("EDITOR cycle=%d hit=%d scan_us=%.3f callback_us=%.3f action_us=%.3f projection_us=%.3f tasks=%d", cycle, hit, float64(scan.Nanoseconds())/1e3, float64(callback.Nanoseconds())/1e3, float64(elapsed.Nanoseconds())/1e3, float64(projection.Nanoseconds())/1e3, tasks)
			if cycle == 0 {
				export(fmt.Sprintf("hit-%02d", hit))
			}
			if expected < 0 {
				vtui.FrameManager.GetTopFrame().(*vtui.Window).Close()
				vtui.FrameManager.Step(0)
				break
			}
			if vtui.FrameManager.GetTopFrame() != v {
				t.Fatal("quick editor search opened progress")
			}
			start = expected + length
		}
	}
}
