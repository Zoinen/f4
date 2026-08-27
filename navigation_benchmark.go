package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

const (
	navigationBenchmarkEnv       = "F4_NAV_BENCHMARK_TRACE"
	navigationBenchmarkOutputEnv = "F4_NAV_BENCHMARK_GO_OUTPUT"
	navigationBenchmarkLogPrefix = "F4_NAV_BENCHMARK_TRACE "
	navigationBenchmarkSchema    = "f4.navigation.v1"
)

var (
	navigationBenchmarkEnabled atomic.Bool
	navigationBenchmarkNextID  atomic.Uint64

	navigationBenchmarkOutput = struct {
		sync.Mutex
		writer io.Writer
	}{}

	navigationBenchmarkState struct {
		sync.Mutex
		currentUI    *navigationBenchmarkTrace
		currentScene *navigationBenchmarkSceneMarker
		renderScene  *navigationBenchmarkSceneMarker
		nextSceneSeq uint64
	}

	navigationBenchmarkInputEvents sync.Map
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

type navigationBenchmarkInputEvent struct {
	trace            *navigationBenchmarkTrace
	queuedNs         int64
	previousTrace    *navigationBenchmarkTrace
	keySequence      int
	queueDepthBefore int
	queueCapacity    int
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
	vtui.InputEventBenchmarkHooks = &vtui.InputBenchmarkHooks{
		DispatchBegin: navigationBenchmarkInputDispatchBegin,
		DispatchEnd:   navigationBenchmarkInputDispatchEnd,
	}
	vtui.FrameManagerLifecycleBenchmarkHooks = &vtui.FrameManagerBenchmarkHooks{
		Event: navigationBenchmarkFrameManagerEvent,
	}
	vfs.OSVFSSetPathBenchmarkHook = func(event string, fields ...any) {
		if trace := navigationBenchmarkCurrentUI(); trace != nil {
			trace.event(event, "go.ui", fields...)
		}
	}
}

func navigationBenchmarkFrameManagerEvent(event string, fields ...any) {
	if !navigationBenchmarkIsEnabled() {
		return
	}
	traceID := ""
	if marker := navigationBenchmarkRenderMarker(); marker != nil && marker.trace != nil {
		traceID = marker.trace.id
		fields = append(fields, navigationBenchmarkMarkerFields(marker)...)
	} else if trace := navigationBenchmarkCurrentUI(); trace != nil {
		traceID = trace.id
	}
	navigationBenchmarkEmit(traceID, "frame_manager."+event, "go.ui", fields...)
}

func navigationBenchmarkIsEnabled() bool {
	return navigationBenchmarkEnabled.Load()
}

// navigationBenchmarkConfigureOutput gives the Go process its own JSONL sink
// when requested. On Windows the Qt child inherits stderr as a separate
// process handle; concurrent append positions are not reliable enough for a
// lossless combined trace. Both streams use the same monotonic clock and can
// be merged by timestamp afterwards.
func navigationBenchmarkConfigureOutput() func() {
	path := strings.TrimSpace(os.Getenv(navigationBenchmarkOutputEnv))
	if !navigationBenchmarkIsEnabled() || path == "" {
		return func() {}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return func() {}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return func() {}
	}
	navigationBenchmarkOutput.Lock()
	previous := navigationBenchmarkOutput.writer
	navigationBenchmarkOutput.writer = file
	navigationBenchmarkOutput.Unlock()
	return func() {
		navigationBenchmarkOutput.Lock()
		if navigationBenchmarkOutput.writer == file {
			navigationBenchmarkOutput.writer = previous
		}
		navigationBenchmarkOutput.Unlock()
		_ = file.Close()
	}
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
	writer := navigationBenchmarkOutput.writer
	if writer == nil {
		// SetupStderrLog replaces os.Stderr after package initialization on
		// Windows. Resolve it at emission time so Go and the child Qt host write
		// into the same redirected diagnostic stream.
		writer = os.Stderr
	}
	_, _ = writer.Write(line)
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

	navigationBenchmarkEmitIPCTiming(id, timing)
	trace.event("ui_action.received", "go.ipc", "action", actionName, "side", trace.side)
	return trace
}

func navigationBenchmarkEmitIPCTiming(traceID string, timing *navigationBenchmarkReadTiming) {
	if timing == nil {
		return
	}
	navigationBenchmarkEmitAt(traceID, "ipc.read.begin", "go.ipc", timing.readStartNs)
	navigationBenchmarkEmitAt(traceID, "ipc.header.done", "go.ipc", timing.headerDoneNs)
	navigationBenchmarkEmitAt(traceID, "ipc.payload.done", "go.ipc", timing.payloadDoneNs,
		"payloadBytes", timing.payloadBytes)
	navigationBenchmarkEmitAt(traceID, "ipc.decode.begin", "go.ipc", timing.decodeStartNs,
		"payloadBytes", timing.payloadBytes)
	navigationBenchmarkEmitAt(traceID, "ipc.decode.done", "go.ipc", timing.decodeDoneNs,
		"payloadBytes", timing.payloadBytes)
}

func navigationBenchmarkTraceForKey(message map[string]any, timing *navigationBenchmarkReadTiming) *navigationBenchmarkTrace {
	if !navigationBenchmarkIsEnabled() || !extUiBool(message, "down") {
		return nil
	}
	vk := uint16(extUiInt(message, "vk"))
	action := ""
	switch vk {
	case vtinput.VK_RETURN:
		action = "key.enter"
	case vtinput.VK_TAB:
		action = "key.tab"
	default:
		return nil
	}
	id := navigationBenchmarkTraceID(message, nil)
	if id == "" {
		id = fmt.Sprintf("go:%d:%d", os.Getpid(), navigationBenchmarkNextID.Add(1))
	}
	message["benchmarkTraceId"] = id
	trace := &navigationBenchmarkTrace{id: id, action: action, side: -1}
	sequence := 0
	if _, present := message["keySequence"]; present {
		sequence = extUiInt(message, "keySequence")
	}
	navigationBenchmarkEmitIPCTiming(id, timing)
	trace.event("key.received", "go.ipc",
		"action", action,
		"vk", vk,
		"char", extUiInt(message, "char"),
		"mods", extUiInt(message, "mods"),
		"repeat", extUiBool(message, "repeat"),
		"keySequence", sequence)
	return trace
}

func navigationBenchmarkInputQueueBegin(ev *vtinput.InputEvent, trace *navigationBenchmarkTrace, keySequence, depth, capacity int) {
	if trace == nil || ev == nil {
		return
	}
	queuedNs := navigationBenchmarkMonotonicNs()
	navigationBenchmarkInputEvents.Store(ev, &navigationBenchmarkInputEvent{
		trace:            trace,
		queuedNs:         queuedNs,
		keySequence:      keySequence,
		queueDepthBefore: depth,
		queueCapacity:    capacity,
	})
	trace.eventAt("input_queue.send.begin", "go.ipc", queuedNs,
		"action", trace.action, "keySequence", keySequence,
		"queueDepth", depth, "queueCapacity", capacity)
}

func navigationBenchmarkInputQueueEnd(ev *vtinput.InputEvent, sent bool, depth int) {
	value, ok := navigationBenchmarkInputEvents.Load(ev)
	if !ok {
		return
	}
	input := value.(*navigationBenchmarkInputEvent)
	endedNs := navigationBenchmarkMonotonicNs()
	input.trace.eventAt("input_queue.send.end", "go.ipc", endedNs,
		"action", input.trace.action, "keySequence", input.keySequence,
		"queueNs", endedNs-input.queuedNs, "queueDepth", depth,
		"queueCapacity", input.queueCapacity, "sent", sent)
	if !sent {
		navigationBenchmarkInputEvents.Delete(ev)
	}
}

func navigationBenchmarkInputDispatchBegin(ev *vtinput.InputEvent) {
	value, ok := navigationBenchmarkInputEvents.Load(ev)
	if !ok {
		return
	}
	input := value.(*navigationBenchmarkInputEvent)
	startedNs := navigationBenchmarkMonotonicNs()
	input.previousTrace = navigationBenchmarkSetCurrentUI(input.trace)
	input.trace.eventAt("input.dispatch.begin", "go.ui", startedNs,
		"action", input.trace.action, "keySequence", input.keySequence,
		"queueWaitNs", startedNs-input.queuedNs,
		"queueDepthAtSend", input.queueDepthBefore,
		"queueCapacity", input.queueCapacity)
}

func navigationBenchmarkInputDispatchEnd(ev *vtinput.InputEvent) {
	value, ok := navigationBenchmarkInputEvents.LoadAndDelete(ev)
	if !ok {
		return
	}
	input := value.(*navigationBenchmarkInputEvent)
	input.trace.event("input.dispatch.end", "go.ui",
		"action", input.trace.action, "keySequence", input.keySequence)
	if input.trace.action == "key.tab" {
		navigationBenchmarkPublishScene(input.trace, "tab-dispatch")
	}
	navigationBenchmarkSetCurrentUI(input.previousTrace)
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

// navigationBenchmarkRenderEvent attributes fine-grained semantic export work
// to the scene marker currently being rendered. It is a no-op outside a
// traced render, keeping normal semantic exports free of tracing work.
func navigationBenchmarkRenderEvent(event string, fields ...any) {
	marker := navigationBenchmarkRenderMarker()
	if marker == nil || marker.trace == nil {
		return
	}
	fields = append(fields, navigationBenchmarkMarkerFields(marker)...)
	marker.trace.event(event, "go.render", fields...)
}

// navigationBenchmarkIncrementalEvent records rejection diagnostics even for
// uncorrelated mouse/task renders. Those are exactly the cases where a silent
// fallback to a full semantic scene is otherwise hardest to explain. It stays
// completely disabled outside an explicitly requested navigation trace.
func navigationBenchmarkIncrementalEvent(event string, fields ...any) {
	if !navigationBenchmarkIsEnabled() {
		return
	}
	traceID := ""
	if marker := navigationBenchmarkRenderMarker(); marker != nil && marker.trace != nil {
		traceID = marker.trace.id
		fields = append(fields, navigationBenchmarkMarkerFields(marker)...)
	}
	navigationBenchmarkEmit(traceID, event, "go.render", fields...)
}

// navigationBenchmarkUIEvent records direct semantic decisions made before a
// render marker exists. It remains a no-op unless live navigation tracing was
// explicitly enabled.
func navigationBenchmarkUIEvent(event string, fields ...any) {
	if !navigationBenchmarkIsEnabled() {
		return
	}
	if trace := navigationBenchmarkCurrentUI(); trace != nil {
		trace.event(event, "go.ui", fields...)
		return
	}
	navigationBenchmarkEmit("", event, "go.ui", fields...)
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

func navigationBenchmarkPrepareRenderMessage(messageMap map[string]any) *navigationBenchmarkMessage {
	if navigationBenchmarkString(messageMap["type"]) == "scene" {
		return navigationBenchmarkPrepareSceneMessage(messageMap)
	}
	marker := navigationBenchmarkRenderMarker()
	if marker == nil || marker.trace == nil {
		return nil
	}
	message := &navigationBenchmarkMessage{
		traceID:       marker.trace.id,
		phase:         marker.phase,
		phaseSequence: marker.phaseSequence,
		sceneSequence: marker.sceneSequence,
		messageType:   navigationBenchmarkString(messageMap["type"]),
	}
	navigationBenchmarkEmit(message.traceID, "message.send.queued", "go.render",
		"phase", message.phase, "phaseSequence", message.phaseSequence,
		"sceneSequence", message.sceneSequence, "messageType", message.messageType)
	return message
}

// navigationBenchmarkPrepareImmediateMessage gives compact state messages
// emitted directly from input dispatch the same transport timing coverage as
// messages emitted by the later render/Flush pass.  There is deliberately no
// render marker here: the whole point of the direct path is to reach the host
// before cell rendering and semantic export begin.
func navigationBenchmarkPrepareImmediateMessage(messageMap map[string]any) *navigationBenchmarkMessage {
	if !navigationBenchmarkIsEnabled() || messageMap == nil {
		return nil
	}
	traceID := navigationBenchmarkString(messageMap["benchmarkTraceId"])
	messageType := navigationBenchmarkString(messageMap["type"])
	if traceID == "" || messageType == "" {
		return nil
	}
	navigationBenchmarkState.Lock()
	navigationBenchmarkState.nextSceneSeq++
	sequence := navigationBenchmarkState.nextSceneSeq
	navigationBenchmarkState.Unlock()
	message := &navigationBenchmarkMessage{
		traceID:       traceID,
		phase:         "input-direct",
		phaseSequence: 1,
		sceneSequence: sequence,
		messageType:   messageType,
	}
	navigationBenchmarkEmit(message.traceID, "message.send.queued", "go.ui",
		"phase", message.phase, "phaseSequence", message.phaseSequence,
		"sceneSequence", message.sceneSequence, "messageType", message.messageType)
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
	if err == nil && message.messageType == "scene" {
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
		"messageType", message.messageType,
		"ok", err == nil,
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}
	event := "message.send.done"
	if message.messageType == "scene" {
		event = "scene.send.done"
	}
	navigationBenchmarkEmit(message.traceID, event, "go.transport", fields...)
}
