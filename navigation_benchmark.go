package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/unxed/vtui"
)

const (
	navigationBenchmarkEnv       = "F4_NAV_BENCHMARK_TRACE"
	navigationBenchmarkLogPrefix = "F4_NAV_BENCHMARK_TRACE "
	navigationBenchmarkSchema    = "f4.navigation.v1"
)

var (
	navigationBenchmarkEnabled atomic.Bool
	navigationBenchmarkNextID  atomic.Uint64

	navigationBenchmarkOutput = struct {
		sync.Mutex
		writer io.Writer
	}{writer: os.Stderr}

	navigationBenchmarkState struct {
		sync.Mutex
		currentUI    *navigationBenchmarkTrace
		currentScene *navigationBenchmarkSceneMarker
		renderScene  *navigationBenchmarkSceneMarker
		nextSceneSeq uint64
	}
)

type navigationBenchmarkReadTiming struct {
	readStartNs   int64
	headerDoneNs  int64
	payloadDoneNs int64
	decodeStartNs int64
	decodeDoneNs  int64
	payloadBytes  int
}

type navigationBenchmarkTrace struct {
	mu sync.Mutex

	id           string
	action       string
	side         int
	fromPath     string
	toPath       string
	direction    string
	nextPhaseSeq uint64
}

type navigationBenchmarkSceneMarker struct {
	trace         *navigationBenchmarkTrace
	phase         string
	phaseSequence uint64
	sceneSequence uint64
	sent          bool
}

type navigationBenchmarkMessage struct {
	traceID       string
	phase         string
	phaseSequence uint64
	sceneSequence uint64
	messageType   string
}

func init() {
	_, enabled := os.LookupEnv(navigationBenchmarkEnv)
	navigationBenchmarkEnabled.Store(enabled)
	if enabled {
		navigationBenchmarkInstallHooks()
	}
}

func navigationBenchmarkInstallHooks() {
	vtui.SemanticSceneBenchmarkHooks = &vtui.SemanticBenchmarkHooks{
		RenderBegin: navigationBenchmarkRenderBegin,
		ExportBegin: navigationBenchmarkExportBegin,
		ExportEnd:   navigationBenchmarkExportEnd,
		RenderEnd:   navigationBenchmarkRenderEnd,
	}
}

func navigationBenchmarkIsEnabled() bool {
	return navigationBenchmarkEnabled.Load()
}

func navigationBenchmarkEmit(traceID, event, thread string, fields ...any) {
	if !navigationBenchmarkIsEnabled() {
		return
	}
	navigationBenchmarkEmitAt(traceID, event, thread, navigationBenchmarkMonotonicNs(), fields...)
}

func navigationBenchmarkEmitAt(traceID, event, thread string, monotonicNs int64, fields ...any) {
	if !navigationBenchmarkIsEnabled() || monotonicNs == 0 {
		return
	}
	if !strings.HasPrefix(event, "go.") {
		event = "go." + event
	}
	record := make(map[string]any, 4+len(fields)/2)
	record["event"] = event
	record["monotonicNs"] = monotonicNs
	record["pid"] = os.Getpid()
	record["thread"] = thread
	if traceID != "" {
		record["benchmarkTraceId"] = traceID
	}
	for i := 0; i+1 < len(fields); i += 2 {
		key, ok := fields[i].(string)
		if ok && key != "" {
			record[key] = fields[i+1]
		}
	}
	payload, err := json.Marshal(record)
	if err != nil {
		return
	}
	line := make([]byte, 0, len(navigationBenchmarkLogPrefix)+len(payload)+1)
	line = append(line, navigationBenchmarkLogPrefix...)
	line = append(line, payload...)
	line = append(line, '\n')

	navigationBenchmarkOutput.Lock()
	_, _ = navigationBenchmarkOutput.writer.Write(line)
	navigationBenchmarkOutput.Unlock()
}

func navigationBenchmarkString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}

func navigationBenchmarkTraceID(outer, action map[string]any) string {
	for _, source := range []map[string]any{action, outer} {
		for _, key := range []string{"benchmarkTraceId", "navigationId", "traceId"} {
			if source != nil {
				if id := navigationBenchmarkString(source[key]); id != "" {
					return id
				}
			}
		}
	}
	return ""
}

func navigationBenchmarkIsNavigationAction(action string) bool {
	switch action {
	case "panel.open", "panel_open", "panel.navigatePath", "panel_navigate_path", "panel.refresh", "panel_refresh":
		return true
	default:
		return false
	}
}

func navigationBenchmarkTraceForAction(outer, action map[string]any, timing *navigationBenchmarkReadTiming) *navigationBenchmarkTrace {
	if !navigationBenchmarkIsEnabled() || action == nil {
		return nil
	}
	actionName := semanticString(action["action"])
	if !navigationBenchmarkIsNavigationAction(actionName) {
		return nil
	}
	id := navigationBenchmarkTraceID(outer, action)
	if id == "" {
		id = fmt.Sprintf("go:%d:%d", os.Getpid(), navigationBenchmarkNextID.Add(1))
	}
	action["benchmarkTraceId"] = id
	trace := &navigationBenchmarkTrace{id: id, action: actionName, side: -1}
	if _, present := action["side"]; present {
		trace.side = semanticInt(action["side"])
	}

	if timing != nil {
		navigationBenchmarkEmitAt(id, "ipc.read.begin", "go.ipc", timing.readStartNs)
		navigationBenchmarkEmitAt(id, "ipc.header.done", "go.ipc", timing.headerDoneNs)
		navigationBenchmarkEmitAt(id, "ipc.payload.done", "go.ipc", timing.payloadDoneNs,
			"payloadBytes", timing.payloadBytes)
		navigationBenchmarkEmitAt(id, "ipc.decode.begin", "go.ipc", timing.decodeStartNs,
			"payloadBytes", timing.payloadBytes)
		navigationBenchmarkEmitAt(id, "ipc.decode.done", "go.ipc", timing.decodeDoneNs,
			"payloadBytes", timing.payloadBytes)
	}
	trace.event("ui_action.received", "go.ipc", "action", actionName, "side", trace.side)
	return trace
}

func (t *navigationBenchmarkTrace) event(event, thread string, fields ...any) {
	if t == nil {
		return
	}
	navigationBenchmarkEmit(t.id, event, thread, fields...)
}

func (t *navigationBenchmarkTrace) eventAt(event, thread string, monotonicNs int64, fields ...any) {
	if t == nil {
		return
	}
	navigationBenchmarkEmitAt(t.id, event, thread, monotonicNs, fields...)
}

func navigationBenchmarkTraceName(trace *navigationBenchmarkTrace) string {
	if trace == nil {
		return ""
	}
	return trace.id
}

func (t *navigationBenchmarkTrace) setSide(side int) {
	if t == nil || side < 0 {
		return
	}
	t.mu.Lock()
	t.side = side
	t.mu.Unlock()
}

func (t *navigationBenchmarkTrace) setPaths(fromPath, toPath, direction string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	if fromPath != "" {
		t.fromPath = fromPath
	}
	if toPath != "" {
		t.toPath = toPath
	}
	if direction != "" {
		t.direction = direction
	}
	t.mu.Unlock()
}

func (t *navigationBenchmarkTrace) pathFields() (int, string, string, string) {
	if t == nil {
		return -1, "", "", ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.side, t.fromPath, t.toPath, t.direction
}

func navigationBenchmarkSetCurrentUI(trace *navigationBenchmarkTrace) *navigationBenchmarkTrace {
	if !navigationBenchmarkIsEnabled() {
		return nil
	}
	navigationBenchmarkState.Lock()
	previous := navigationBenchmarkState.currentUI
	navigationBenchmarkState.currentUI = trace
	navigationBenchmarkState.Unlock()
	return previous
}

func navigationBenchmarkCurrentUI() *navigationBenchmarkTrace {
	if !navigationBenchmarkIsEnabled() {
		return nil
	}
	navigationBenchmarkState.Lock()
	trace := navigationBenchmarkState.currentUI
	navigationBenchmarkState.Unlock()
	return trace
}

func navigationBenchmarkPublishScene(trace *navigationBenchmarkTrace, phase string) {
	if trace == nil || phase == "" {
		return
	}
	trace.mu.Lock()
	trace.nextPhaseSeq++
	phaseSequence := trace.nextPhaseSeq
	trace.mu.Unlock()
	marker := &navigationBenchmarkSceneMarker{
		trace:         trace,
		phase:         phase,
		phaseSequence: phaseSequence,
	}
	navigationBenchmarkState.Lock()
	navigationBenchmarkState.currentScene = marker
	navigationBenchmarkState.Unlock()
	trace.event("scene.phase.published", "go.ui", "phase", phase, "phaseSequence", phaseSequence)
}

func navigationBenchmarkRenderMarker() *navigationBenchmarkSceneMarker {
	navigationBenchmarkState.Lock()
	marker := navigationBenchmarkState.renderScene
	navigationBenchmarkState.Unlock()
	return marker
}

func navigationBenchmarkMarkerFields(marker *navigationBenchmarkSceneMarker) []any {
	if marker == nil || marker.trace == nil {
		return nil
	}
	return []any{
		"phase", marker.phase,
		"phaseSequence", marker.phaseSequence,
		"sceneSequence", marker.sceneSequence,
	}
}

func navigationBenchmarkRenderBegin() {
	if !navigationBenchmarkIsEnabled() {
		return
	}
	navigationBenchmarkState.Lock()
	marker := navigationBenchmarkState.currentScene
	navigationBenchmarkState.renderScene = marker
	navigationBenchmarkState.Unlock()
	if marker != nil && marker.trace != nil {
		marker.trace.event("render.begin", "go.render", navigationBenchmarkMarkerFields(marker)...)
	}
}

func navigationBenchmarkExportBegin() {
	marker := navigationBenchmarkRenderMarker()
	if marker != nil && marker.trace != nil {
		marker.trace.event("scene.export.begin", "go.render", navigationBenchmarkMarkerFields(marker)...)
	}
}

func navigationBenchmarkExportEnd(scene map[string]any) {
	marker := navigationBenchmarkRenderMarker()
	if marker == nil || marker.trace == nil || scene == nil {
		return
	}
	side, fromPath, toPath, direction := marker.trace.pathFields()
	meta := map[string]any{
		"schema":           navigationBenchmarkSchema,
		"benchmarkTraceId": marker.trace.id,
		"phase":            marker.phase,
		"phaseSequence":    marker.phaseSequence,
		"side":             side,
		"fromPath":         fromPath,
		"toPath":           toPath,
		"direction":        direction,
	}
	if marker.sceneSequence != 0 {
		meta["sceneSequence"] = marker.sceneSequence
	}
	// Keep the direct ID for the native decoder's cheap correlation path and a
	// richer, versioned map for benchmark consumers.
	scene["benchmarkTraceId"] = marker.trace.id
	scene["benchmark"] = meta
	marker.trace.event("scene.export.end", "go.render", navigationBenchmarkMarkerFields(marker)...)
}

func navigationBenchmarkRenderEnd() {
	marker := navigationBenchmarkRenderMarker()
	if marker != nil && marker.trace != nil {
		marker.trace.event("render.end", "go.render", navigationBenchmarkMarkerFields(marker)...)
	}
	navigationBenchmarkState.Lock()
	navigationBenchmarkState.renderScene = nil
	navigationBenchmarkState.Unlock()
}

func navigationBenchmarkSceneMessage(scene map[string]any) *navigationBenchmarkMessage {
	if !navigationBenchmarkIsEnabled() || scene == nil {
		return nil
	}
	id := navigationBenchmarkString(scene["benchmarkTraceId"])
	meta, _ := scene["benchmark"].(map[string]any)
	if id == "" && meta != nil {
		id = navigationBenchmarkString(meta["benchmarkTraceId"])
	}
	if id == "" {
		return nil
	}
	return &navigationBenchmarkMessage{
		traceID:       id,
		phase:         navigationBenchmarkString(meta["phase"]),
		phaseSequence: uint64(extUiAnyInt(meta["phaseSequence"])),
		sceneSequence: uint64(extUiAnyInt(meta["sceneSequence"])),
		messageType:   navigationBenchmarkString(scene["type"]),
	}
}

func navigationBenchmarkSceneCompareBegin(scene map[string]any) *navigationBenchmarkMessage {
	message := navigationBenchmarkSceneMessage(scene)
	if message != nil {
		navigationBenchmarkEmit(message.traceID, "scene.compare.begin", "go.render",
			"phase", message.phase, "phaseSequence", message.phaseSequence)
	}
	return message
}

func navigationBenchmarkSceneCompareEnd(message *navigationBenchmarkMessage, result string) {
	if message == nil {
		return
	}
	navigationBenchmarkEmit(message.traceID, "scene.compare.end", "go.render",
		"phase", message.phase, "phaseSequence", message.phaseSequence, "result", result)
}

func navigationBenchmarkPrepareSceneMessage(scene map[string]any) *navigationBenchmarkMessage {
	message := navigationBenchmarkSceneMessage(scene)
	if message == nil || message.messageType != "scene" {
		return message
	}
	navigationBenchmarkState.Lock()
	marker := navigationBenchmarkState.currentScene
	if marker != nil && marker.trace != nil && marker.trace.id == message.traceID &&
		marker.phaseSequence == message.phaseSequence {
		if marker.sceneSequence == 0 {
			navigationBenchmarkState.nextSceneSeq++
			marker.sceneSequence = navigationBenchmarkState.nextSceneSeq
		}
		message.sceneSequence = marker.sceneSequence
	}
	navigationBenchmarkState.Unlock()
	if message.sceneSequence == 0 {
		navigationBenchmarkState.Lock()
		navigationBenchmarkState.nextSceneSeq++
		message.sceneSequence = navigationBenchmarkState.nextSceneSeq
		navigationBenchmarkState.Unlock()
	}
	if meta, ok := scene["benchmark"].(map[string]any); ok {
		meta["sceneSequence"] = message.sceneSequence
	}
	navigationBenchmarkEmit(message.traceID, "scene.send.queued", "go.render",
		"phase", message.phase, "phaseSequence", message.phaseSequence,
		"sceneSequence", message.sceneSequence)
	return message
}

func navigationBenchmarkMessageFromMap(msg map[string]any) *navigationBenchmarkMessage {
	if navigationBenchmarkString(msg["type"]) != "scene" {
		return nil
	}
	return navigationBenchmarkSceneMessage(msg)
}

func navigationBenchmarkMessageSent(message *navigationBenchmarkMessage, err error) {
	if message == nil {
		return
	}
	if err == nil {
		navigationBenchmarkState.Lock()
		marker := navigationBenchmarkState.currentScene
		if marker != nil && marker.trace != nil && marker.trace.id == message.traceID &&
			marker.phaseSequence == message.phaseSequence {
			marker.sent = true
		}
		navigationBenchmarkState.Unlock()
	}
	fields := []any{
		"phase", message.phase,
		"phaseSequence", message.phaseSequence,
		"sceneSequence", message.sceneSequence,
		"ok", err == nil,
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}
	navigationBenchmarkEmit(message.traceID, "scene.send.done", "go.transport", fields...)
}
