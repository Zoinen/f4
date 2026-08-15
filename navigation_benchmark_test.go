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
	"github.com/unxed/vtui"
)

func captureNavigationBenchmark(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	oldEnabled := navigationBenchmarkEnabled.Load()
	oldHooks := vtui.SemanticSceneBenchmarkHooks
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
		navigationBenchmarkOutput.Lock()
		navigationBenchmarkOutput.writer = oldWriter
		navigationBenchmarkOutput.Unlock()
		navigationBenchmarkState.Lock()
		navigationBenchmarkState.currentUI = oldCurrentUI
		navigationBenchmarkState.currentScene = oldCurrentScene
		navigationBenchmarkState.renderScene = oldRenderScene
		navigationBenchmarkState.nextSceneSeq = oldNextSceneSeq
		navigationBenchmarkState.Unlock()
	})
	return &output
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
