package app

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// This opt-in diagnostic exports the real binary-text projection for the Qt
// presentation benchmark. It never writes the source executable.
func TestDocumentProfilingFixture(t *testing.T) {
	if os.Getenv("F4_DOCUMENT_PROFILE_FIXTURE") != "1" {
		t.Skip("opt-in production document fixture export")
	}
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	viewer, err := viewer.NewViewerView(context.Background(), vfs.NewOSVFS("C:\\Windows"), "C:\\Windows\\pyw.exe")
	if err != nil {
		t.Fatal(err)
	}
	defer viewer.Close()
	viewer.HexMode, viewer.WrapMode = false, true
	viewer.SetPosition(0, 0, 218, 50)
	viewer.SetVisible(true)
	viewer.TopOffset = 15000
	semantic.ApplyNativeDocumentViewport(viewer, semantic.NativeDocumentGeometry{Columns: 310, Rows: 49, Revision: 1})
	deadline := time.Now().Add(3 * time.Second)
	for {
		window := viewer.SemanticNode(nil)
		if window["layoutPending"] != true {
			data, err := json.Marshal(window)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(".diagnostics", "speedup-real-viewer-window.json"), data, 0600); err != nil {
				t.Fatal(err)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("real viewer window did not become ready")
		}
		drainDocumentBenchmarkTasks()
	}
}

// The same benchmark is run against the pristine revision and the changed
// tree. It measures core preparation/projection, not Qt frame presentation.
func BenchmarkDocumentSpeedup(b *testing.B) {
	fixtures := map[string][]byte{
		"ordinary":  []byte(strings.Repeat("alpha beta gamma\t0123456789\n", 4096)),
		"long_tabs": []byte(strings.Repeat(strings.Repeat("123\t世界 x", 800)+"\nshort\n\n", 32)),
	}
	if data, err := os.ReadFile("C:\\Windows\\pyw.exe"); err == nil {
		fixtures["pyw"] = data
	}
	for name, data := range fixtures {
		b.Run(name, func(b *testing.B) {
			b.Run("viewer_open", func(b *testing.B) {
				vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
				dir := b.TempDir()
				path := filepath.Join(dir, "fixture.bin")
				if err := os.WriteFile(path, data, 0600); err != nil {
					b.Fatal(err)
				}
				samples := make([]time.Duration, 0, b.N)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					start := navtrace.NavigationBenchmarkMonotonicNs()
					view, err := viewer.NewViewerView(context.Background(), vfs.NewOSVFS(dir), path)
					if err != nil {
						b.Fatal(err)
					}
					view.SetPosition(0, 0, 119, 30)
					view.SetVisible(true)
					_ = view.SemanticNode(nil)
					view.Close()
					samples = append(samples, time.Duration(navtrace.NavigationBenchmarkMonotonicNs()-start))
					drainDocumentBenchmarkTasks()
				}
				reportDocumentBenchmarkLatency(b, samples)
			})
			for _, wrap := range []bool{false, true} {
				b.Run(fmt.Sprintf("editor_selection_wrap_%t", wrap), func(b *testing.B) {
					vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
					editor := editor.NewEditorView(piecetable.New(data), nil, "fixture.bin")
					defer editor.Close()
					editor.Highlighter = nil
					editor.WordWrap = wrap
					editor.SetPosition(0, 0, 119, 30)
					editor.SetVisible(true)
					editor.EnsureEngineWidth()
					_ = editor.SemanticNode(nil)
					samples := make([]time.Duration, 0, b.N)
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						start := navtrace.NavigationBenchmarkMonotonicNs()
						editor.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType,
							MouseX: int16(20 + i%80), MouseY: int16(2 + i%27),
							ButtonState:     vtinput.FromLeft1stButtonPressed,
							MouseEventFlags: vtinput.MouseMoved})
						_ = editor.SemanticNode(nil)
						samples = append(samples, time.Duration(navtrace.NavigationBenchmarkMonotonicNs()-start))
					}
					reportDocumentBenchmarkLatency(b, samples)
				})
			}
		})
	}
}

func drainDocumentBenchmarkTasks() {
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		default:
			return
		}
	}
}
func reportDocumentBenchmarkLatency(b *testing.B, samples []time.Duration) {
	b.StopTimer()
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	if len(samples) > 0 {
		b.ReportMetric(float64(samples[len(samples)/2].Nanoseconds())/1e6, "p50_ms")
		b.ReportMetric(float64(samples[min(len(samples)-1, len(samples)*95/100)].Nanoseconds())/1e6, "p95_ms")
	}
}
