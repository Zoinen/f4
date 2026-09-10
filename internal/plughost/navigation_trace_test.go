package plughost

import (
	"bytes"
	"encoding/json"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
func TestNavigationBenchmarkPropagatesActionTraceAndIPCTimestamps(t *testing.T) {
	if !testutil.RunWithEnvironment(t, "F4_NAV_BENCHMARK_TRACE", "1") {
		return
	}
	output := captureNavigationBenchmark(t)

	wantID := "qt:held-enter:17"
	var wire bytes.Buffer
	if err := extUiSendMessage(&wire, map[string]any{
		"benchmarkTraceId": wantID,
		"type":             "ui_action",
		"action": map[string]any{
			"action":  "panel.open",
			"side":    1,
			"entryId": "local:/tmp/child",
		},
	}); err != nil {
		t.Fatalf("encode action: %v", err)
	}
	timing := &navtrace.NavigationBenchmarkReadTiming{}
	message, err := extUiReadMessageWithBenchmark(&wire, timing)
	if err != nil {
		t.Fatalf("decode action: %v", err)
	}
	action, ok := message["action"].(map[string]any)
	if !ok {
		t.Fatalf("nested action = %#v", message["action"])
	}
	trace := navtrace.NavigationBenchmarkTraceForAction(message, action, timing)
	if trace == nil || trace.Id != wantID {
		t.Fatalf("trace ID = %#v, want %q", trace, wantID)
	}
	if got := semantic.String(action["benchmarkTraceId"]); got != wantID {
		t.Fatalf("action trace ID = %q, want %q", got, wantID)
	}
	if !(timing.ReadStartNs <= timing.HeaderDoneNs &&
		timing.HeaderDoneNs <= timing.PayloadDoneNs &&
		timing.PayloadDoneNs <= timing.DecodeStartNs &&
		timing.DecodeStartNs <= timing.DecodeDoneNs) {
		t.Fatalf("IPC timestamps are out of order: %#v", timing)
	}

	events := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))
	for _, event := range []string{
		"go.ipc.read.begin", "go.ipc.header.done", "go.ipc.payload.done",
		"go.ipc.decode.begin", "go.ipc.decode.done", "go.ui_action.received",
	} {
		record, ok := events[event]
		if !ok {
			t.Fatalf("missing %s event; got %v", event, events)
		}
		if got := record["benchmarkTraceId"]; got != wantID {
			t.Fatalf("%s trace ID = %#v, want %q", event, got, wantID)
		}
	}
}

func TestNavigationBenchmarkCapturesUIActionQueue(t *testing.T) {
	if !testutil.RunWithEnvironment(t, "F4_NAV_BENCHMARK_TRACE", "1") {
		return
	}
	output := captureNavigationBenchmark(t)
	previousApp := App
	recorder := &semanticActionRecorder{}
	App = recorder
	t.Cleanup(func() { App = previousApp })
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	host := &ExtUiHost{}
	host.handleMessageWithBenchmark(map[string]any{
		"type":             "ui_action",
		"action":           "panel.refresh",
		"side":             0,
		"benchmarkTraceId": "qt:queue:9",
	}, nil)

	select {
	case task := <-vtui.FrameManager.PriorityTaskChan:
		task()
	case <-time.After(2 * time.Second):
		t.Fatal("semantic action did not reach the UI task queue")
	}
	if recorder.action["action"] != "panel.refresh" {
		t.Fatal("UI task did not dispatch semantic action")
	}
	events := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))
	for _, event := range []string{
		"go.ui_action.received", "go.ui_task.queued", "go.ui_task.started",
		"go.semantic_action.begin", "go.semantic_action.end",
	} {
		record, ok := events[event]
		if !ok {
			t.Fatalf("missing %s event; got %v", event, events)
		}
		if got := record["benchmarkTraceId"]; got != "qt:queue:9" {
			t.Fatalf("%s trace ID = %#v", event, got)
		}
	}
}

func TestNavigationBenchmarkCorrelatesRawTabThroughDispatch(t *testing.T) {
	if !testutil.RunWithEnvironment(t, "F4_NAV_BENCHMARK_TRACE", "1") {
		return
	}
	output := captureNavigationBenchmark(t)
	reader := &vtinput.Reader{EventChan: make(chan *vtinput.InputEvent, 4)}
	host := &ExtUiHost{reader: reader}

	var wire bytes.Buffer
	if err := extUiSendMessage(&wire, map[string]any{
		"type":             "key",
		"benchmarkTraceId": "qt:key:77:41",
		"keySequence":      uint64(41),
		"down":             true,
		"repeat":           true,
		"vk":               uint64(vtinput.VK_TAB),
	}); err != nil {
		t.Fatalf("encode key: %v", err)
	}
	timing := &navtrace.NavigationBenchmarkReadTiming{}
	message, err := extUiReadMessageWithBenchmark(&wire, timing)
	if err != nil {
		t.Fatalf("decode key: %v", err)
	}
	host.handleMessageWithBenchmark(message, timing)

	var event *vtinput.InputEvent
	select {
	case event = <-reader.EventChan:
	case <-time.After(time.Second):
		t.Fatal("raw key did not reach input queue")
	}
	if event.RepeatCount != 2 {
		t.Fatalf("repeat count = %d, want 2", event.RepeatCount)
	}
	vtui.InputEventBenchmarkHooks.DispatchBegin(event)
	if current := navtrace.NavigationBenchmarkCurrentUI(); current == nil || current.Id != "qt:key:77:41" {
		t.Fatalf("dispatch trace = %#v", current)
	}
	vtui.InputEventBenchmarkHooks.DispatchEnd(event)
	if current := navtrace.NavigationBenchmarkCurrentUI(); current != nil {
		t.Fatalf("dispatch left current trace installed: %#v", current)
	}

	events := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))
	for _, name := range []string{
		"go.ipc.read.begin", "go.ipc.decode.done", "go.key.received",
		"go.input_queue.send.begin", "go.input_queue.send.end",
		"go.input.dispatch.begin", "go.input.dispatch.end",
		"go.scene.phase.published",
	} {
		record, ok := events[name]
		if !ok {
			t.Fatalf("missing %s event; got %v", name, events)
		}
		if got := record["benchmarkTraceId"]; got != "qt:key:77:41" {
			t.Fatalf("%s trace ID = %#v", name, got)
		}
	}
	if got := events["go.key.received"]["keySequence"]; got != float64(41) {
		t.Fatalf("key sequence = %#v, want 41", got)
	}
	if got := events["go.scene.phase.published"]["phase"]; got != "tab-dispatch" {
		t.Fatalf("published phase = %#v", got)
	}
}

func TestNavigationBenchmarkSceneMetadataAndTransportStages(t *testing.T) {
	if !testutil.RunWithEnvironment(t, "F4_NAV_BENCHMARK_TRACE", "1") {
		return
	}
	output := captureNavigationBenchmark(t)
	trace := navtrace.NavigationBenchmarkTraceForAction(map[string]any{"benchmarkTraceId": "qt:navigation:42"}, map[string]any{"action": "panel.open", "side": 0}, nil)
	trace.SetPaths("/tmp", "/tmp/child", "child")
	navtrace.NavigationBenchmarkPublishScene(trace, "cached")

	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
	scene := map[string]any{"type": "scene", "shell": map[string]any{"activePanel": 0}}
	vtui.SemanticSceneBenchmarkHooks.RenderBegin()
	vtui.SemanticSceneBenchmarkHooks.ExportBegin()
	vtui.SemanticSceneBenchmarkHooks.ExportEnd(scene)
	renderer.SetSemanticScene(scene)
	renderer.Flush()
	vtui.SemanticSceneBenchmarkHooks.RenderEnd()

	got, err := extUiReadMessage(&wire)
	if err != nil {
		t.Fatalf("read benchmark scene: %v", err)
	}
	if got["benchmarkTraceId"] != trace.Id {
		t.Fatalf("direct scene trace ID = %#v, want %q; message=%#v", got["benchmarkTraceId"], trace.Id, got)
	}
	meta, ok := got["benchmark"].(map[string]any)
	if !ok {
		t.Fatalf("scene benchmark metadata = %#v", got["benchmark"])
	}
	if meta["schema"] != "f4.navigation.v1" || meta["phase"] != "cached" ||
		meta["fromPath"] != "/tmp" || meta["toPath"] != "/tmp/child" ||
		meta["direction"] != "child" || semantic.Int(meta["sceneSequence"]) <= 0 {
		t.Fatalf("unexpected scene benchmark metadata: %#v", meta)
	}

	// Re-exporting the same phase must carry the assigned scene sequence and
	// remain equal to the delivered scene, rather than benchmarking itself into
	// an extra full-scene transmission.
	equalScene := map[string]any{"type": "scene", "shell": map[string]any{"activePanel": 0}}
	vtui.SemanticSceneBenchmarkHooks.RenderBegin()
	vtui.SemanticSceneBenchmarkHooks.ExportBegin()
	vtui.SemanticSceneBenchmarkHooks.ExportEnd(equalScene)
	renderer.SetSemanticScene(equalScene)
	renderer.Flush()
	vtui.SemanticSceneBenchmarkHooks.RenderEnd()
	if wire.Len() != 0 {
		t.Fatalf("stable benchmark metadata caused a duplicate scene (%d bytes)", wire.Len())
	}

	events := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))
	for _, event := range []string{
		"go.render.begin", "go.scene.export.begin", "go.scene.export.end",
		"go.scene.compare.begin", "go.scene.compare.end", "go.scene.send.queued",
		"go.transport.send_lock.wait", "go.transport.send_lock.acquired",
		"go.transport.marshal.begin", "go.transport.marshal.end",
		"go.transport.header_write.begin", "go.transport.header_write.end",
		"go.transport.payload_write.begin", "go.transport.payload_write.end",
		"go.scene.send.done", "go.render.end",
	} {
		if _, ok := events[event]; !ok {
			t.Fatalf("missing %s event; got %v", event, events)
		}
	}
	if result := events["go.scene.compare.end"]["result"]; result != "equal_last" {
		t.Fatalf("last compare result = %#v, want equal_last", result)
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

type semanticActionRecorder struct {
	Application
	action map[string]any
}

func (r *semanticActionRecorder) RunSemanticAction(action map[string]any) bool {
	r.action = action
	return false
}
