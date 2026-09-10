package navtrace

import (
	"encoding/json"
	"fmt"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
		currentUI    *NavigationBenchmarkTrace
		currentScene *navigationBenchmarkSceneMarker
		renderScene  *navigationBenchmarkSceneMarker
		nextSceneSeq uint64
	}

	navigationBenchmarkInputEvents sync.Map
)

type NavigationBenchmarkReadTiming struct {
	ReadStartNs   int64
	HeaderDoneNs  int64
	PayloadDoneNs int64
	DecodeStartNs int64
	DecodeDoneNs  int64
	PayloadBytes  int
}

type NavigationBenchmarkTrace struct {
	mu sync.Mutex

	Id           string
	Action       string
	side         int
	fromPath     string
	toPath       string
	direction    string
	nextPhaseSeq uint64
}

type navigationBenchmarkSceneMarker struct {
	Trace         *NavigationBenchmarkTrace
	phase         string
	phaseSequence uint64
	sceneSequence uint64
	sent          bool
}

type NavigationBenchmarkMessage struct {
	TraceID       string
	Phase         string
	PhaseSequence uint64
	SceneSequence uint64
	MessageType   string
}

type navigationBenchmarkInputEvent struct {
	trace            *NavigationBenchmarkTrace
	queuedNs         int64
	previousTrace    *NavigationBenchmarkTrace
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
		if trace := NavigationBenchmarkCurrentUI(); trace != nil {
			trace.Event(event, "go.ui", fields...)
		}
	}
}

func navigationBenchmarkFrameManagerEvent(event string, fields ...any) {
	if !NavigationBenchmarkIsEnabled() {
		return
	}
	traceID := ""
	if marker := NavigationBenchmarkRenderMarker(); marker != nil && marker.Trace != nil {
		traceID = marker.Trace.Id
		fields = append(fields, NavigationBenchmarkMarkerFields(marker)...)
	} else if trace := NavigationBenchmarkCurrentUI(); trace != nil {
		traceID = trace.Id
	}
	NavigationBenchmarkEmit(traceID, "frame_manager."+event, "go.ui", fields...)
}

func NavigationBenchmarkIsEnabled() bool {
	return navigationBenchmarkEnabled.Load()
}

// navigationBenchmarkConfigureOutput gives the Go process its own JSONL sink
// when requested. On Windows the Qt child inherits stderr as a separate
// process handle; concurrent append positions are not reliable enough for a
// lossless combined trace. Both streams use the same monotonic clock and can
// be merged by timestamp afterwards.
func NavigationBenchmarkConfigureOutput() func() {
	path := strings.TrimSpace(os.Getenv(navigationBenchmarkOutputEnv))
	if !NavigationBenchmarkIsEnabled() || path == "" {
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

func NavigationBenchmarkEmit(traceID, event, thread string, fields ...any) {
	if !NavigationBenchmarkIsEnabled() {
		return
	}
	NavigationBenchmarkEmitAt(traceID, event, thread, NavigationBenchmarkMonotonicNs(), fields...)
}

func NavigationBenchmarkEmitAt(traceID, event, thread string, monotonicNs int64, fields ...any) {
	if !NavigationBenchmarkIsEnabled() || monotonicNs == 0 {
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

func NavigationBenchmarkString(value any) string {
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
				if id := NavigationBenchmarkString(source[key]); id != "" {
					return id
				}
			}
		}
	}
	return ""
}

func navigationBenchmarkIsNavigationAction(action string) bool {
	switch action {
	case "panel.open", "panel_open", "panel.navigatePath", "panel_navigate_path", "panel.refresh", "panel_refresh", "editor.mouse":
		return true
	default:
		return false
	}
}

func NavigationBenchmarkTraceForAction(outer, action map[string]any, timing *NavigationBenchmarkReadTiming) *NavigationBenchmarkTrace {
	if !NavigationBenchmarkIsEnabled() || action == nil {
		return nil
	}
	actionName := semantic.String(action["action"])
	if !navigationBenchmarkIsNavigationAction(actionName) {
		return nil
	}
	id := navigationBenchmarkTraceID(outer, action)
	if id == "" {
		id = fmt.Sprintf("go:%d:%d", os.Getpid(), navigationBenchmarkNextID.Add(1))
	}
	action["benchmarkTraceId"] = id
	trace := &NavigationBenchmarkTrace{Id: id, Action: actionName, side: -1}
	if _, present := action["side"]; present {
		trace.side = semantic.Int(action["side"])
	}

	navigationBenchmarkEmitIPCTiming(id, timing)
	trace.Event("ui_action.received", "go.ipc", "action", actionName, "side", trace.side)
	return trace
}

func navigationBenchmarkEmitIPCTiming(traceID string, timing *NavigationBenchmarkReadTiming) {
	if timing == nil {
		return
	}
	NavigationBenchmarkEmitAt(traceID, "ipc.read.begin", "go.ipc", timing.ReadStartNs)
	NavigationBenchmarkEmitAt(traceID, "ipc.header.done", "go.ipc", timing.HeaderDoneNs)
	NavigationBenchmarkEmitAt(traceID, "ipc.payload.done", "go.ipc", timing.PayloadDoneNs,
		"payloadBytes", timing.PayloadBytes)
	NavigationBenchmarkEmitAt(traceID, "ipc.decode.begin", "go.ipc", timing.DecodeStartNs,
		"payloadBytes", timing.PayloadBytes)
	NavigationBenchmarkEmitAt(traceID, "ipc.decode.done", "go.ipc", timing.DecodeDoneNs,
		"payloadBytes", timing.PayloadBytes)
}

func NavigationBenchmarkTraceForKey(message map[string]any, timing *NavigationBenchmarkReadTiming) *NavigationBenchmarkTrace {
	if !NavigationBenchmarkIsEnabled() || !semantic.Bool(message["down"]) {
		return nil
	}
	vk := uint16(semantic.Int(message["vk"]))
	action := ""
	switch vk {
	case vtinput.VK_RETURN:
		action = "key.enter"
	case vtinput.VK_TAB:
		action = "key.tab"
	case vtinput.VK_F4:
		action = "key.f4"
	case vtinput.VK_F3:
		action = "key.f3"
	case vtinput.VK_ESCAPE:
		action = "key.escape"
	case vtinput.VK_RIGHT:
		action = "key.right"
	default:
		return nil
	}
	id := navigationBenchmarkTraceID(message, nil)
	if id == "" {
		id = fmt.Sprintf("go:%d:%d", os.Getpid(), navigationBenchmarkNextID.Add(1))
	}
	message["benchmarkTraceId"] = id
	trace := &NavigationBenchmarkTrace{Id: id, Action: action, side: -1}
	sequence := 0
	if _, present := message["keySequence"]; present {
		sequence = semantic.Int(message["keySequence"])
	}
	navigationBenchmarkEmitIPCTiming(id, timing)
	trace.Event("key.received", "go.ipc",
		"action", action,
		"vk", vk,
		"char", semantic.Int(message["char"]),
		"mods", semantic.Int(message["mods"]),
		"repeat", semantic.Bool(message["repeat"]),
		"keySequence", sequence)
	return trace
}

func NavigationBenchmarkInputQueueBegin(ev *vtinput.InputEvent, trace *NavigationBenchmarkTrace, keySequence, depth, capacity int) {
	if trace == nil || ev == nil {
		return
	}
	queuedNs := NavigationBenchmarkMonotonicNs()
	navigationBenchmarkInputEvents.Store(ev, &navigationBenchmarkInputEvent{
		trace:            trace,
		queuedNs:         queuedNs,
		keySequence:      keySequence,
		queueDepthBefore: depth,
		queueCapacity:    capacity,
	})
	trace.EventAt("input_queue.send.begin", "go.ipc", queuedNs,
		"action", trace.Action, "keySequence", keySequence,
		"queueDepth", depth, "queueCapacity", capacity)
}

func NavigationBenchmarkInputQueueEnd(ev *vtinput.InputEvent, sent bool, depth int) {
	value, ok := navigationBenchmarkInputEvents.Load(ev)
	if !ok {
		return
	}
	input := value.(*navigationBenchmarkInputEvent)
	endedNs := NavigationBenchmarkMonotonicNs()
	input.trace.EventAt("input_queue.send.end", "go.ipc", endedNs,
		"action", input.trace.Action, "keySequence", input.keySequence,
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
	startedNs := NavigationBenchmarkMonotonicNs()
	input.previousTrace = NavigationBenchmarkSetCurrentUI(input.trace)
	input.trace.EventAt("input.dispatch.begin", "go.ui", startedNs,
		"action", input.trace.Action, "keySequence", input.keySequence,
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
	input.trace.Event("input.dispatch.end", "go.ui",
		"action", input.trace.Action, "keySequence", input.keySequence)
	phase := strings.TrimPrefix(input.trace.Action, "key.") + "-dispatch"
	NavigationBenchmarkPublishScene(input.trace, phase)
	NavigationBenchmarkSetCurrentUI(input.previousTrace)
}

func (t *NavigationBenchmarkTrace) Event(event, thread string, fields ...any) {
	if t == nil {
		return
	}
	NavigationBenchmarkEmit(t.Id, event, thread, fields...)
}

func (t *NavigationBenchmarkTrace) EventAt(event, thread string, monotonicNs int64, fields ...any) {
	if t == nil {
		return
	}
	NavigationBenchmarkEmitAt(t.Id, event, thread, monotonicNs, fields...)
}

func NavigationBenchmarkTraceName(trace *NavigationBenchmarkTrace) string {
	if trace == nil {
		return ""
	}
	return trace.Id
}

func (t *NavigationBenchmarkTrace) SetSide(side int) {
	if t == nil || side < 0 {
		return
	}
	t.mu.Lock()
	t.side = side
	t.mu.Unlock()
}

func (t *NavigationBenchmarkTrace) SetPaths(fromPath, toPath, direction string) {
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

func (t *NavigationBenchmarkTrace) PathFields() (int, string, string, string) {
	if t == nil {
		return -1, "", "", ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.side, t.fromPath, t.toPath, t.direction
}

func NavigationBenchmarkSetCurrentUI(trace *NavigationBenchmarkTrace) *NavigationBenchmarkTrace {
	if !NavigationBenchmarkIsEnabled() {
		return nil
	}
	navigationBenchmarkState.Lock()
	previous := navigationBenchmarkState.currentUI
	navigationBenchmarkState.currentUI = trace
	navigationBenchmarkState.Unlock()
	return previous
}

func NavigationBenchmarkCurrentUI() *NavigationBenchmarkTrace {
	if !NavigationBenchmarkIsEnabled() {
		return nil
	}
	navigationBenchmarkState.Lock()
	trace := navigationBenchmarkState.currentUI
	navigationBenchmarkState.Unlock()
	return trace
}

func NavigationBenchmarkCurrentOrPublishedTrace() *NavigationBenchmarkTrace {
	if trace := NavigationBenchmarkCurrentUI(); trace != nil {
		return trace
	}
	navigationBenchmarkState.Lock()
	marker := navigationBenchmarkState.currentScene
	navigationBenchmarkState.Unlock()
	if marker != nil {
		return marker.Trace
	}
	return nil
}

func NavigationBenchmarkPublishScene(trace *NavigationBenchmarkTrace, phase string) {
	if trace == nil || phase == "" {
		return
	}
	trace.mu.Lock()
	trace.nextPhaseSeq++
	phaseSequence := trace.nextPhaseSeq
	trace.mu.Unlock()
	marker := &navigationBenchmarkSceneMarker{
		Trace:         trace,
		phase:         phase,
		phaseSequence: phaseSequence,
	}
	navigationBenchmarkState.Lock()
	navigationBenchmarkState.currentScene = marker
	navigationBenchmarkState.Unlock()
	trace.Event("scene.phase.published", "go.ui", "phase", phase, "phaseSequence", phaseSequence)
}

func NavigationBenchmarkRenderMarker() *navigationBenchmarkSceneMarker {
	navigationBenchmarkState.Lock()
	marker := navigationBenchmarkState.renderScene
	navigationBenchmarkState.Unlock()
	return marker
}

func NavigationBenchmarkMarkerFields(marker *navigationBenchmarkSceneMarker) []any {
	if marker == nil || marker.Trace == nil {
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
func NavigationBenchmarkRenderEvent(event string, fields ...any) {
	marker := NavigationBenchmarkRenderMarker()
	if marker == nil || marker.Trace == nil {
		return
	}
	fields = append(fields, NavigationBenchmarkMarkerFields(marker)...)
	marker.Trace.Event(event, "go.render", fields...)
}

// navigationBenchmarkIncrementalEvent records rejection diagnostics even for
// uncorrelated mouse/task renders. Those are exactly the cases where a silent
// fallback to a full semantic scene is otherwise hardest to explain. It stays
// completely disabled outside an explicitly requested navigation trace.
func NavigationBenchmarkIncrementalEvent(event string, fields ...any) {
	if !NavigationBenchmarkIsEnabled() {
		return
	}
	traceID := ""
	if marker := NavigationBenchmarkRenderMarker(); marker != nil && marker.Trace != nil {
		traceID = marker.Trace.Id
		fields = append(fields, NavigationBenchmarkMarkerFields(marker)...)
	}
	NavigationBenchmarkEmit(traceID, event, "go.render", fields...)
}

// navigationBenchmarkUIEvent records direct semantic decisions made before a
// render marker exists. It remains a no-op unless live navigation tracing was
// explicitly enabled.
func NavigationBenchmarkUIEvent(event string, fields ...any) {
	if !NavigationBenchmarkIsEnabled() {
		return
	}
	if trace := NavigationBenchmarkCurrentUI(); trace != nil {
		trace.Event(event, "go.ui", fields...)
		return
	}
	NavigationBenchmarkEmit("", event, "go.ui", fields...)
}

func navigationBenchmarkRenderBegin() {
	if !NavigationBenchmarkIsEnabled() {
		return
	}
	navigationBenchmarkState.Lock()
	marker := navigationBenchmarkState.currentScene
	navigationBenchmarkState.renderScene = marker
	navigationBenchmarkState.Unlock()
	if marker != nil && marker.Trace != nil {
		marker.Trace.Event("render.begin", "go.render", NavigationBenchmarkMarkerFields(marker)...)
	}
}

func navigationBenchmarkExportBegin() {
	marker := NavigationBenchmarkRenderMarker()
	if marker != nil && marker.Trace != nil {
		marker.Trace.Event("scene.export.begin", "go.render", NavigationBenchmarkMarkerFields(marker)...)
	}
}

func navigationBenchmarkExportEnd(scene map[string]any) {
	marker := NavigationBenchmarkRenderMarker()
	if marker == nil || marker.Trace == nil || scene == nil {
		return
	}
	side, fromPath, toPath, direction := marker.Trace.PathFields()
	meta := map[string]any{
		"schema":           navigationBenchmarkSchema,
		"benchmarkTraceId": marker.Trace.Id,
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
	scene["benchmarkTraceId"] = marker.Trace.Id
	scene["benchmark"] = meta
	marker.Trace.Event("scene.export.end", "go.render", NavigationBenchmarkMarkerFields(marker)...)
}

func navigationBenchmarkRenderEnd() {
	marker := NavigationBenchmarkRenderMarker()
	if marker != nil && marker.Trace != nil {
		marker.Trace.Event("render.end", "go.render", NavigationBenchmarkMarkerFields(marker)...)
	}
	navigationBenchmarkState.Lock()
	navigationBenchmarkState.renderScene = nil
	navigationBenchmarkState.Unlock()
}

func navigationBenchmarkSceneMessage(scene map[string]any) *NavigationBenchmarkMessage {
	if !NavigationBenchmarkIsEnabled() || scene == nil {
		return nil
	}
	id := NavigationBenchmarkString(scene["benchmarkTraceId"])
	meta, _ := scene["benchmark"].(map[string]any)
	if id == "" && meta != nil {
		id = NavigationBenchmarkString(meta["benchmarkTraceId"])
	}
	if id == "" {
		return nil
	}
	return &NavigationBenchmarkMessage{
		TraceID:       id,
		Phase:         NavigationBenchmarkString(meta["phase"]),
		PhaseSequence: uint64(semantic.Int(meta["phaseSequence"])),
		SceneSequence: uint64(semantic.Int(meta["sceneSequence"])),
		MessageType:   NavigationBenchmarkString(scene["type"]),
	}
}

func NavigationBenchmarkSceneCompareBegin(scene map[string]any) *NavigationBenchmarkMessage {
	message := navigationBenchmarkSceneMessage(scene)
	if message != nil {
		NavigationBenchmarkEmit(message.TraceID, "scene.compare.begin", "go.render",
			"phase", message.Phase, "phaseSequence", message.PhaseSequence)
	}
	return message
}

func NavigationBenchmarkSceneCompareEnd(message *NavigationBenchmarkMessage, result string) {
	if message == nil {
		return
	}
	NavigationBenchmarkEmit(message.TraceID, "scene.compare.end", "go.render",
		"phase", message.Phase, "phaseSequence", message.PhaseSequence, "result", result)
}

func NavigationBenchmarkPrepareSceneMessage(scene map[string]any) *NavigationBenchmarkMessage {
	message := navigationBenchmarkSceneMessage(scene)
	if message == nil || message.MessageType != "scene" {
		return message
	}
	navigationBenchmarkState.Lock()
	marker := navigationBenchmarkState.currentScene
	if marker != nil && marker.Trace != nil && marker.Trace.Id == message.TraceID &&
		marker.phaseSequence == message.PhaseSequence {
		if marker.sceneSequence == 0 {
			navigationBenchmarkState.nextSceneSeq++
			marker.sceneSequence = navigationBenchmarkState.nextSceneSeq
		}
		message.SceneSequence = marker.sceneSequence
	}
	navigationBenchmarkState.Unlock()
	if message.SceneSequence == 0 {
		navigationBenchmarkState.Lock()
		navigationBenchmarkState.nextSceneSeq++
		message.SceneSequence = navigationBenchmarkState.nextSceneSeq
		navigationBenchmarkState.Unlock()
	}
	if meta, ok := scene["benchmark"].(map[string]any); ok {
		meta["sceneSequence"] = message.SceneSequence
	}
	NavigationBenchmarkEmit(message.TraceID, "scene.send.queued", "go.render",
		"phase", message.Phase, "phaseSequence", message.PhaseSequence,
		"sceneSequence", message.SceneSequence)
	return message
}

func NavigationBenchmarkPrepareRenderMessage(messageMap map[string]any) *NavigationBenchmarkMessage {
	if NavigationBenchmarkString(messageMap["type"]) == "scene" {
		return NavigationBenchmarkPrepareSceneMessage(messageMap)
	}
	marker := NavigationBenchmarkRenderMarker()
	if marker == nil || marker.Trace == nil {
		return nil
	}
	navigationBenchmarkState.Lock()
	if marker.sceneSequence == 0 {
		navigationBenchmarkState.nextSceneSeq++
		marker.sceneSequence = navigationBenchmarkState.nextSceneSeq
	}
	sequence := marker.sceneSequence
	navigationBenchmarkState.Unlock()
	if semantic.Bool(messageMap["benchmarkSceneFinal"]) {
		// Protocol v4 transports one logical scene as independent stream
		// snapshots. Attribute completion to the final stream so the trace still
		// measures when the complete scene, rather than its first fragment, lands.
		return &NavigationBenchmarkMessage{
			TraceID:       marker.Trace.Id,
			Phase:         marker.phase,
			PhaseSequence: marker.phaseSequence,
			SceneSequence: marker.sceneSequence,
			MessageType:   "scene",
		}
	}
	message := &NavigationBenchmarkMessage{
		TraceID:       marker.Trace.Id,
		Phase:         marker.phase,
		PhaseSequence: marker.phaseSequence,
		SceneSequence: sequence,
		MessageType:   NavigationBenchmarkString(messageMap["type"]),
	}
	// v4 scene exports are split into typed stream envelopes. Their benchmark
	// metadata lives in the payload, so keep the assigned scene sequence on the
	// payload as well as in the transport trace used by Flush.
	if payload, ok := messageMap["payload"].(map[string]any); ok {
		if meta, ok := payload["benchmark"].(map[string]any); ok {
			meta["sceneSequence"] = sequence
		}
	}
	// Compact render messages do not pass through SemanticBenchmarkHooks, so
	// add the correlation field here in trace mode. The strict Qt envelope
	// explicitly permits benchmark-prefixed diagnostics.
	messageMap["benchmarkTraceId"] = marker.Trace.Id
	event := "message.send.queued"
	if navigationBenchmarkIsSceneMessageType(message.MessageType) {
		event = "scene.send.queued"
	}
	NavigationBenchmarkEmit(message.TraceID, event, "go.render",
		"phase", message.Phase, "phaseSequence", message.PhaseSequence,
		"sceneSequence", message.SceneSequence, "messageType", message.MessageType)
	return message
}

func navigationBenchmarkIsSceneMessageType(messageType string) bool {
	return messageType == "scene" || messageType == "semantic_stream_snapshot" ||
		messageType == "scene_patch"
}

// navigationBenchmarkPrepareImmediateMessage gives compact state messages
// emitted directly from input dispatch the same transport timing coverage as
// messages emitted by the later render/Flush pass.  There is deliberately no
// render marker here: the whole point of the direct path is to reach the host
// before cell rendering and semantic export begin.
func NavigationBenchmarkPrepareImmediateMessage(messageMap map[string]any) *NavigationBenchmarkMessage {
	if !NavigationBenchmarkIsEnabled() || messageMap == nil {
		return nil
	}
	traceID := NavigationBenchmarkString(messageMap["benchmarkTraceId"])
	messageType := NavigationBenchmarkString(messageMap["type"])
	if traceID == "" || messageType == "" {
		return nil
	}
	navigationBenchmarkState.Lock()
	navigationBenchmarkState.nextSceneSeq++
	sequence := navigationBenchmarkState.nextSceneSeq
	navigationBenchmarkState.Unlock()
	message := &NavigationBenchmarkMessage{
		TraceID:       traceID,
		Phase:         "input-direct",
		PhaseSequence: 1,
		SceneSequence: sequence,
		MessageType:   messageType,
	}
	NavigationBenchmarkEmit(message.TraceID, "message.send.queued", "go.ui",
		"phase", message.Phase, "phaseSequence", message.PhaseSequence,
		"sceneSequence", message.SceneSequence, "messageType", message.MessageType)
	return message
}

func NavigationBenchmarkMessageFromMap(msg map[string]any) *NavigationBenchmarkMessage {
	if NavigationBenchmarkString(msg["type"]) != "scene" {
		return nil
	}
	return navigationBenchmarkSceneMessage(msg)
}

func NavigationBenchmarkMessageSent(message *NavigationBenchmarkMessage, err error) {
	if message == nil {
		return
	}
	if err == nil && navigationBenchmarkIsSceneMessageType(message.MessageType) {
		navigationBenchmarkState.Lock()
		marker := navigationBenchmarkState.currentScene
		if marker != nil && marker.Trace != nil && marker.Trace.Id == message.TraceID &&
			marker.phaseSequence == message.PhaseSequence {
			marker.sent = true
		}
		navigationBenchmarkState.Unlock()
	}
	fields := []any{
		"phase", message.Phase,
		"phaseSequence", message.PhaseSequence,
		"sceneSequence", message.SceneSequence,
		"messageType", message.MessageType,
		"ok", err == nil,
	}
	if err != nil {
		fields = append(fields, "error", err.Error())
	}
	event := "message.send.done"
	if navigationBenchmarkIsSceneMessageType(message.MessageType) {
		event = "scene.send.done"
	}
	NavigationBenchmarkEmit(message.TraceID, event, "go.transport", fields...)
}
