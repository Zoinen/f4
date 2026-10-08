package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Opt-in diagnostic: measures the real source and exports its presentation.
func TestViewerRepeatedSearchProfile(t *testing.T) {
	path, output := os.Getenv("F4_SEARCH_PROFILE_FILE"), os.Getenv("F4_SEARCH_PROFILE_DIR")
	if path == "" || output == "" {
		t.Skip("opt-in viewer search profile")
	}
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(300, 50)
	vtui.FrameManager.Init(screen)
	previousAdapter := vtui.AppSceneAdapter
	t.Cleanup(func() { vtui.AppSceneAdapter = previousAdapter })
	vtui.AppSceneAdapter = nativeui.BuildAppSceneFromLegacy
	v, err := viewer.NewViewerView(context.Background(), vfs.NewOSVFS(filepath.Dir(path)), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	vtui.FrameManager.Push(v)
	v.SetPosition(0, 0, 299, 48)
	semantic.ApplyNativeDocumentViewport(v, semantic.NativeDocumentGeometry{Columns: 300, Rows: 46, Revision: 1})
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	export := func(name string) {
		data, err := json.Marshal(vtui.FrameManager.ExportSemanticScene())
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(output, name+".json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 10; i++ {
		v.TopOffset = 0
		v.LastSearchFound = false
		for start, n := int64(0), 0; ; n++ {
			begin := time.Now()
			off, length, err := viewer.SearchMatch(context.Background(), v.Backend, "mp4", start, viewer.SearchOptions{}, nil)
			scan := time.Since(begin)
			if err != nil {
				t.Fatal(err)
			}
			if off < 0 {
				break
			}
			begin = time.Now()
			v.SelectSearchMatch(off, length)
			reveal := time.Since(begin)
			begin = time.Now()
			_ = v.SemanticNode(nil)
			projection := time.Since(begin)
			t.Logf("STAGE cycle=%d hit=%d scan_us=%.3f reveal_us=%.3f projection_us=%.3f", i, n, float64(scan.Nanoseconds())/1e3, float64(reveal.Nanoseconds())/1e3, float64(projection.Nanoseconds())/1e3)
			start = off + 1
		}
	}
	v.TopOffset = 0
	v.LastSearchFound = false
	v.LastSearch = "mp4"
	export("initial")
	for n := 0; n < 39; n++ {
		previous := v.LastSearchOffset
		began := time.Now()
		runViewerSearch(v, "mp4", false)
		tasks := 0
		var callback time.Duration
		var exportTime time.Duration
		deadline := time.After(3 * time.Second)
		for {
			select {
			case task := <-vtui.FrameManager.TaskChan:
				start := time.Now()
				task()
				callback += time.Since(start)
				tasks++
				vtui.FrameManager.Step(0)
				if tasks == 1 {
					start := time.Now()
					export(fmt.Sprintf("progress-%02d", n))
					exportTime += time.Since(start)
				}
			case <-deadline:
				t.Fatal("search did not finish")
			}
			if v.LastSearchFound && (n == 0 || v.LastSearchOffset != previous) {
				break
			}
			if vtui.FrameManager.GetTopFrame().GetTitle() == " Search " {
				break
			}
		}
		t.Logf("ACTION hit=%d total_us=%.3f callback_us=%.3f tasks=%d", n, float64((time.Since(began)-exportTime).Nanoseconds())/1e3, float64(callback.Nanoseconds())/1e3, tasks)
		export(fmt.Sprintf("hit-%02d", n))
	}
}
