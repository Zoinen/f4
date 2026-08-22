package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func captureNavigationBenchmark(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	oldEnabled := navigationBenchmarkEnabled.Load()
	oldHooks := vtui.SemanticSceneBenchmarkHooks
	oldInputHooks := vtui.InputEventBenchmarkHooks
	oldOSVFSHook := vfs.OSVFSSetPathBenchmarkHook
	navigationBenchmarkOutput.Lock()
	oldWriter := navigationBenchmarkOutput.writer
	navigationBenchmarkOutput.writer = &output
	navigationBenchmarkOutput.Unlock()

	navigationBenchmarkState.Lock()
	oldCurrentUI := navigationBenchmarkState.currentUI
	oldCurrentScene := navigationBenchmarkState.currentScene
	oldRenderScene := navigationBenchmarkState.renderScene
	oldNextSceneSeq := navigationBenchmarkState.nextSceneSeq
	navigationBenchmarkState.currentUI = nil
	navigationBenchmarkState.currentScene = nil
	navigationBenchmarkState.renderScene = nil
	navigationBenchmarkState.nextSceneSeq = 0
	navigationBenchmarkState.Unlock()

	navigationBenchmarkEnabled.Store(true)
	navigationBenchmarkInstallHooks()
	t.Cleanup(func() {
		navigationBenchmarkEnabled.Store(oldEnabled)
		vtui.SemanticSceneBenchmarkHooks = oldHooks
		vtui.InputEventBenchmarkHooks = oldInputHooks
		vfs.OSVFSSetPathBenchmarkHook = oldOSVFSHook
		navigationBenchmarkOutput.Lock()
		navigationBenchmarkOutput.writer = oldWriter
		navigationBenchmarkOutput.Unlock()
		navigationBenchmarkState.Lock()
		navigationBenchmarkState.currentUI = oldCurrentUI
		navigationBenchmarkState.currentScene = oldCurrentScene
		navigationBenchmarkState.renderScene = oldRenderScene
		navigationBenchmarkState.nextSceneSeq = oldNextSceneSeq
		navigationBenchmarkState.Unlock()
		navigationBenchmarkInputEvents.Range(func(key, _ any) bool {
			navigationBenchmarkInputEvents.Delete(key)
			return true
		})
	})
	return &output
}

func TestNavigationBenchmarkUsesCurrentStderr(t *testing.T) {
	stderrFile, err := os.CreateTemp(t.TempDir(), "navigation-stderr-*.log")
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	oldStderr := os.Stderr
	oldEnabled := navigationBenchmarkEnabled.Load()
	navigationBenchmarkOutput.Lock()
	oldWriter := navigationBenchmarkOutput.writer
	navigationBenchmarkOutput.writer = nil
	navigationBenchmarkOutput.Unlock()
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		os.Stderr = oldStderr
		navigationBenchmarkEnabled.Store(oldEnabled)
		navigationBenchmarkOutput.Lock()
		navigationBenchmarkOutput.writer = oldWriter
		navigationBenchmarkOutput.Unlock()
	}
	t.Cleanup(restore)

	os.Stderr = stderrFile
	navigationBenchmarkEnabled.Store(true)
	navigationBenchmarkEmit("stderr-current", "writer.current", "go.test")
	if err := stderrFile.Close(); err != nil {
		t.Fatalf("close stderr capture: %v", err)
	}
	restore()

	payload, err := os.ReadFile(stderrFile.Name())
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	if !strings.Contains(string(payload), `"benchmarkTraceId":"stderr-current"`) ||
		!strings.Contains(string(payload), `"event":"go.writer.current"`) {
		t.Fatalf("dynamic stderr did not receive benchmark record: %q", payload)
	}
}

func TestNavigationBenchmarkUsesDedicatedGoOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go-navigation.jsonl")
	t.Setenv(navigationBenchmarkOutputEnv, path)
	oldEnabled := navigationBenchmarkEnabled.Load()
	navigationBenchmarkEnabled.Store(true)
	t.Cleanup(func() { navigationBenchmarkEnabled.Store(oldEnabled) })

	closeOutput := navigationBenchmarkConfigureOutput()
	navigationBenchmarkEmit("dedicated-output", "writer.dedicated", "go.test")
	closeOutput()

	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read dedicated output: %v", err)
	}
	if !strings.Contains(string(payload), `"benchmarkTraceId":"dedicated-output"`) ||
		!strings.Contains(string(payload), `"event":"go.writer.dedicated"`) {
		t.Fatalf("dedicated output did not receive benchmark record: %q", payload)
	}
}

func decodeNavigationBenchmarkRecords(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, navigationBenchmarkLogPrefix) {
			t.Fatalf("benchmark line has wrong prefix: %q", line)
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, navigationBenchmarkLogPrefix)), &record); err != nil {
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
	timing := &navigationBenchmarkReadTiming{}
	message, err := extUiReadMessageWithBenchmark(&wire, timing)
	if err != nil {
		t.Fatalf("decode action: %v", err)
	}
	action, ok := message["action"].(map[string]any)
	if !ok {
		t.Fatalf("nested action = %#v", message["action"])
	}
	trace := navigationBenchmarkTraceForAction(message, action, timing)
	if trace == nil || trace.id != wantID {
		t.Fatalf("trace ID = %#v, want %q", trace, wantID)
	}
	if got := navigationBenchmarkString(action["benchmarkTraceId"]); got != wantID {
		t.Fatalf("action trace ID = %q, want %q", got, wantID)
	}
	if !(timing.readStartNs <= timing.headerDoneNs &&
		timing.headerDoneNs <= timing.payloadDoneNs &&
		timing.payloadDoneNs <= timing.decodeStartNs &&
		timing.decodeStartNs <= timing.decodeDoneNs) {
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
	output := captureNavigationBenchmark(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	host := &ExtUiHost{}
	host.handleMessageWithBenchmark(map[string]any{
		"type":             "ui_action",
		"action":           "panel.refresh",
		"side":             0,
		"benchmarkTraceId": "qt:queue:9",
	}, nil)

	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(2 * time.Second):
		t.Fatal("semantic action did not reach the UI task queue")
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
	timing := &navigationBenchmarkReadTiming{}
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
	if current := navigationBenchmarkCurrentUI(); current == nil || current.id != "qt:key:77:41" {
		t.Fatalf("dispatch trace = %#v", current)
	}
	vtui.InputEventBenchmarkHooks.DispatchEnd(event)
	if current := navigationBenchmarkCurrentUI(); current != nil {
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
	output := captureNavigationBenchmark(t)
	trace := &navigationBenchmarkTrace{id: "qt:navigation:42", action: "panel.open", side: 0}
	trace.setPaths("/tmp", "/tmp/child", "child")
	navigationBenchmarkPublishScene(trace, "cached")

	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
	scene := map[string]any{"type": "scene", "shell": map[string]any{"activePanel": 0}}
	navigationBenchmarkRenderBegin()
	navigationBenchmarkExportBegin()
	navigationBenchmarkExportEnd(scene)
	renderer.SetSemanticScene(scene)
	renderer.Flush()
	navigationBenchmarkRenderEnd()

	got, err := extUiReadMessage(&wire)
	if err != nil {
		t.Fatalf("read benchmark scene: %v", err)
	}
	if got["benchmarkTraceId"] != trace.id {
		t.Fatalf("direct scene trace ID = %#v, want %q", got["benchmarkTraceId"], trace.id)
	}
	meta, ok := got["benchmark"].(map[string]any)
	if !ok {
		t.Fatalf("scene benchmark metadata = %#v", got["benchmark"])
	}
	if meta["schema"] != navigationBenchmarkSchema || meta["phase"] != "cached" ||
		meta["fromPath"] != "/tmp" || meta["toPath"] != "/tmp/child" ||
		meta["direction"] != "child" || extUiAnyInt(meta["sceneSequence"]) <= 0 {
		t.Fatalf("unexpected scene benchmark metadata: %#v", meta)
	}

	// Re-exporting the same phase must carry the assigned scene sequence and
	// remain equal to the delivered scene, rather than benchmarking itself into
	// an extra full-scene transmission.
	equalScene := map[string]any{"type": "scene", "shell": map[string]any{"activePanel": 0}}
	navigationBenchmarkRenderBegin()
	navigationBenchmarkExportBegin()
	navigationBenchmarkExportEnd(equalScene)
	renderer.SetSemanticScene(equalScene)
	renderer.Flush()
	navigationBenchmarkRenderEnd()
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

func TestNavigationBenchmarkCapturesDirectoryPipeline(t *testing.T) {
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
		if panel.cancelLoad != nil {
			panel.cancelLoad()
		}
		panel.stopLoadingAnimation()
	})
	waitForLoad(t, panel)

	trace := &navigationBenchmarkTrace{id: "qt:navigation:pipeline", action: "panel.open", side: 0}
	previous := navigationBenchmarkSetCurrentUI(trace)
	if err := panel.setKnownDirectoryPath(child); err != nil {
		t.Fatalf("navigate to child: %v", err)
	}
	panel.readDirectoryEx(false)
	navigationBenchmarkSetCurrentUI(previous)
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
		if got := record["benchmarkTraceId"]; got != trace.id {
			t.Fatalf("%s trace ID = %#v, want %q", event, got, trace.id)
		}
	}
}

func TestNavigationBenchmarkCapturesFolderHistoryPersistenceStages(t *testing.T) {
	output := captureNavigationBenchmark(t)
	history := &F4HistoryProvider{
		path: filepath.Join(t.TempDir(), "history.json"),
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	oldProvider := vtui.GlobalHistoryProvider
	vtui.GlobalHistoryProvider = history
	t.Cleanup(func() { vtui.GlobalHistoryProvider = oldProvider })

	trace := &navigationBenchmarkTrace{id: "qt:key:history:1", action: "key.enter", side: 0}
	previous := navigationBenchmarkSetCurrentUI(trace)
	AddFolderHistory(filepath.Join(t.TempDir(), "child"))
	navigationBenchmarkSetCurrentUI(previous)
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
		if got := record["benchmarkTraceId"]; got != trace.id {
			t.Fatalf("%s trace ID = %#v, want %q", event, got, trace.id)
		}
	}
}
