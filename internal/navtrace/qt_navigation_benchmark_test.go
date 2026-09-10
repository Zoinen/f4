package navtrace

import (
	"bytes"
	"encoding/json"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	NavigationBenchmarkEmit("stderr-current", "writer.current", "go.test")
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

	closeOutput := NavigationBenchmarkConfigureOutput()
	NavigationBenchmarkEmit("dedicated-output", "writer.dedicated", "go.test")
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

func TestNavigationBenchmarkTracesEditorRouteKeys(t *testing.T) {
	for _, tc := range []struct {
		name   string
		vk     uint16
		action string
		phase  string
	}{
		{name: "f3", vk: vtinput.VK_F3, action: "key.f3", phase: "f3-dispatch"},
		{name: "f4", vk: vtinput.VK_F4, action: "key.f4", phase: "f4-dispatch"},
		{name: "escape", vk: vtinput.VK_ESCAPE, action: "key.escape", phase: "escape-dispatch"},
		{name: "right", vk: vtinput.VK_RIGHT, action: "key.right", phase: "right-dispatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := captureNavigationBenchmark(t)
			message := map[string]any{
				"type": "key", "down": true, "vk": uint64(tc.vk),
				"keySequence": uint64(7), "benchmarkTraceId": "qt:editor:7",
			}
			trace := NavigationBenchmarkTraceForKey(message, nil)
			if trace == nil || trace.Action != tc.action {
				t.Fatalf("trace = %#v, want action %q", trace, tc.action)
			}
			event := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
				VirtualKeyCode: tc.vk}
			NavigationBenchmarkInputQueueBegin(event, trace, 7, 0, 4)
			NavigationBenchmarkInputQueueEnd(event, true, 1)
			navigationBenchmarkInputDispatchBegin(event)
			navigationBenchmarkInputDispatchEnd(event)

			events := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))
			if got := events["go.scene.phase.published"]["phase"]; got != tc.phase {
				t.Fatalf("phase = %#v, want %q", got, tc.phase)
			}
		})
	}
}
