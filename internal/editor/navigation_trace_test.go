package editor

import (
	json "encoding/json"
	navtrace "github.com/unxed/f4/internal/navtrace"
	os "os"
	filepath "path/filepath"
	strings "strings"
	testing "testing"
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

type traceCapture struct {
	path   string
	offset int
}

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
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data[c.offset:])
}
