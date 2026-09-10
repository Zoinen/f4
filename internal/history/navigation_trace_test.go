package history

import (
	"encoding/json"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/testutil"
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
func TestNavigationBenchmarkCapturesFolderHistoryPersistenceStages(t *testing.T) {
	if !testutil.RunWithEnvironment(t, "F4_NAV_BENCHMARK_TRACE", "1") {
		return
	}
	output := captureNavigationBenchmark(t)
	history := &F4HistoryProvider{
		path: filepath.Join(t.TempDir(), "history.json"),
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	oldProvider := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = history
	t.Cleanup(func() { vtui.GlobalHistoryProvider = oldProvider })

	trace := navtrace.NavigationBenchmarkTraceForAction(map[string]any{"benchmarkTraceId": "qt:key:history:1"}, map[string]any{"action": "panel.open", "side": 0}, nil)
	previous := navtrace.NavigationBenchmarkSetCurrentUI(trace)
	AddFolderHistory(filepath.Join(t.TempDir(), "child"))
	navtrace.NavigationBenchmarkSetCurrentUI(previous)
	if err := history.Flush(); err != nil {
		t.Fatalf("flush history trace: %v", err)
	}
	t.Cleanup(func() { _ = history.Close() })

	events := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))
	for _, event := range []string{
		"go.history.load_plain.begin", "go.history.load_plain.end",
		"go.history.load_rich.begin", "go.history.load_rich.end",
		"go.history.update.begin", "go.history.update.end",
		"go.history.save_rich.begin", "go.history.save_rich.end",
		"go.history.save_plain.begin", "go.history.save_plain.end",
		"go.history.marshal.begin", "go.history.marshal.end",
		"go.history.write.begin", "go.history.write.end",
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
