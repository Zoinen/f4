package panel

import (
	"encoding/json"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func decodeNavigationBenchmarkRecords(t *testing.T, output *traceCapture) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output.read(t)), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "F4_NAV_BENCHMARK_TRACE ") {
			t.Fatalf("benchmark line has wrong prefix: %q", line)
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "F4_NAV_BENCHMARK_TRACE ")), &record); err != nil {
			t.Fatalf("invalid benchmark JSON %q: %v", line, err)
		}
		for _, key := range []string{"event", "monotonicNs", "pid", "thread"} {
			if _, ok := record[key]; !ok {
				t.Fatalf("benchmark record lacks %q: %#v", key, record)
			}
		}
		records = append(records, record)
	}
	return records
}

func navigationBenchmarkEvents(records []map[string]any) map[string]map[string]any {
	events := make(map[string]map[string]any, len(records))
	for _, record := range records {
		if event, ok := record["event"].(string); ok {
			events[event] = record
		}
	}
	return events
}
func TestNavigationBenchmarkCapturesDirectoryPipeline(t *testing.T) {
	if !testutil.RunWithEnvironment(t, "F4_NAV_BENCHMARK_TRACE", "1") {
		return
	}
	output := captureNavigationBenchmark(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	dir := t.TempDir()
	child := filepath.Join(dir, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatalf("create child directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(child, "entry.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("create panel entry: %v", err)
	}
	panel := NewFileSystemPanel(0, 0, 40, 12, vfs.NewOSVFS(dir))
	t.Cleanup(func() {
		if panel.CancelLoad != nil {
			panel.CancelLoad()
		}
		panel.StopLoadingAnimation()
	})
	waitForLoad(t, panel)

	trace := navtrace.NavigationBenchmarkTraceForAction(map[string]any{"benchmarkTraceId": "qt:navigation:pipeline"}, map[string]any{"action": "panel.open", "side": 0}, nil)
	previous := navtrace.NavigationBenchmarkSetCurrentUI(trace)
	if err := panel.SetKnownDirectoryPath(child); err != nil {
		t.Fatalf("navigate to child: %v", err)
	}
	panel.readDirectoryEx(false)
	navtrace.NavigationBenchmarkSetCurrentUI(previous)
	waitForLoad(t, panel)

	events := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))
	for _, event := range []string{
		"go.path.set.begin", "go.path.set.end", "go.directory_read.begin",
		"go.history.persist.begin", "go.history.persist.end", "go.model.provisional.ready",
		"go.load.queued", "go.load.worker.started", "go.filesystem.readdir.begin",
		"go.filesystem.readdir.chunk", "go.filesystem.readdir.end",
		"go.filesystem.stat_current.begin", "go.filesystem.stat_current.end",
		"go.model.final.queued", "go.model.final.started", "go.model.final.ready",
		"go.scene.phase.published",
	} {
		record, ok := events[event]
		if !ok {
			t.Fatalf("missing %s event; got %v", event, events)
		}
		if got := record["benchmarkTraceId"]; got != trace.Id {
			t.Fatalf("%s trace ID = %#v, want %q", event, got, trace.Id)
		}
	}
}

type traceCapture struct{ path string }

func captureNavigationBenchmark(t *testing.T) *traceCapture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	t.Setenv("F4_NAV_BENCHMARK_GO_OUTPUT", path)
	t.Cleanup(navtrace.NavigationBenchmarkConfigureOutput())
	return &traceCapture{path: path}
}
func (c *traceCapture) read(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(c.path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
