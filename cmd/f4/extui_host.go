package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	extUiProtocolVersion                = 4
	extUiMaxMessageSize                 = 64 * 1024 * 1024
	extUiMaxDimension                   = 1<<15 - 1
	extUiPanelCatalogMetadataCapability = "panelCatalogMetadataV1"
	extUiPanelCatalogRowsCapability     = "panelCatalogRowsV1"
)

// Deferred panel metadata is a required extension of the lockstep protocol. Keep the
// process default conservative; RunExternalUI enables it only after the client
// advertises the exact capability in its hello. Tests which exercise the native
// model directly opt in from TestMain.
var extUiPanelCatalogMetadataEnabled atomic.Bool
var extUiPanelCatalogRowsEnabled atomic.Bool

func setExtUiPanelCatalogMetadataEnabled(enabled bool) bool {
	return extUiPanelCatalogMetadataEnabled.Swap(enabled)
}

func extUiPanelCatalogMetadataIsEnabled() bool {
	return extUiPanelCatalogMetadataEnabled.Load()
}

func setExtUiPanelCatalogRowsEnabled(enabled bool) bool {
	return extUiPanelCatalogRowsEnabled.Swap(enabled)
}

func extUiPanelCatalogRowsIsEnabled() bool {
	return extUiPanelCatalogRowsEnabled.Load()
}

func extUiHelloCapability(hello map[string]any, name string) bool {
	capabilities, ok := hello["capabilities"].(map[string]any)
	return ok && extUiAnyBool(capabilities[name])
}

func extUiNewNonce() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf[:]), nil
}

func extUiSendMessage(w io.Writer, msg map[string]any) error {
	return extUiSendMessageWithBenchmark(w, msg, nil)
}

func extUiSendMessageWithBenchmark(w io.Writer, msg map[string]any, benchmark *navigationBenchmarkMessage) error {
	if benchmark != nil {
		navigationBenchmarkEmit(benchmark.traceID, "transport.marshal.begin", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType)
	}
	payload, err := msgpack.Marshal(msg)
	if err != nil {
		if benchmark != nil {
			navigationBenchmarkEmit(benchmark.traceID, "transport.marshal.end", "go.transport",
				"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
				"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType,
				"ok", false, "error", err.Error())
		}
		return err
	}
	if benchmark != nil {
		navigationBenchmarkEmit(benchmark.traceID, "transport.marshal.end", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType,
			"ok", true, "payloadBytes", len(payload))
	}
	if len(payload) > extUiMaxMessageSize {
		return fmt.Errorf("extui message too large: %d bytes", len(payload))
	}

	var hdr [4]byte
	// #nosec G115 -- payload was limited to 64 MiB above, well inside uint32.
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if benchmark != nil {
		navigationBenchmarkEmit(benchmark.traceID, "transport.header_write.begin", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType,
			"bytes", len(hdr))
	}
	if _, err := w.Write(hdr[:]); err != nil {
		if benchmark != nil {
			navigationBenchmarkEmit(benchmark.traceID, "transport.header_write.end", "go.transport",
				"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
				"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType,
				"ok", false, "error", err.Error())
		}
		return err
	}
	if benchmark != nil {
		navigationBenchmarkEmit(benchmark.traceID, "transport.header_write.end", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType,
			"ok", true, "bytes", len(hdr))
		navigationBenchmarkEmit(benchmark.traceID, "transport.payload_write.begin", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType,
			"payloadBytes", len(payload))
	}
	_, err = w.Write(payload)
	if benchmark != nil {
		fields := []any{
			"phase", benchmark.phase,
			"phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence,
			"messageType", benchmark.messageType,
			"payloadBytes", len(payload),
			"ok", err == nil,
		}
		if err != nil {
			fields = append(fields, "error", err.Error())
		}
		navigationBenchmarkEmit(benchmark.traceID, "transport.payload_write.end", "go.transport", fields...)
	}
	return err
}

type extUiMessageSender struct {
	mu                   sync.Mutex
	w                    io.Writer
	nextSemanticSequence uint64
	streamRevisions      map[string]uint64
}

type extUiSemanticDispatch struct {
	streamID string
	kind     string
	payload  map[string]any
}

func (s *extUiMessageSender) Send(msg map[string]any) error {
	return s.SendWithBenchmark(msg, navigationBenchmarkMessageFromMap(msg))
}

func (s *extUiMessageSender) SendWithBenchmark(msg map[string]any, benchmark *navigationBenchmarkMessage) error {
	dispatches := extUiSemanticDispatches(msg)
	if len(dispatches) == 0 {
		return s.sendWithBenchmark(msg, benchmark, "", "", false)
	}
	if len(dispatches) == 1 {
		dispatch := dispatches[0]
		return s.sendWithBenchmark(dispatch.payload, benchmark,
			dispatch.streamID, dispatch.kind, true)
	}
	return s.sendSemanticBatchWithBenchmark(dispatches, benchmark)
}

func (s *extUiMessageSender) SendSemanticSnapshot(streamID string,
	payload map[string]any,
) error {
	return s.sendWithBenchmark(payload,
		navigationBenchmarkMessageFromMap(payload), streamID,
		extui.KindSnapshot, true)
}

func (s *extUiMessageSender) sendWithBenchmark(msg map[string]any,
	benchmark *navigationBenchmarkMessage, streamID, kind string,
	semantic bool,
) error {
	if benchmark != nil {
		navigationBenchmarkEmit(benchmark.traceID, "transport.send_lock.wait", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType)
	}
	s.mu.Lock()
	wireMessage := msg
	if semantic {
		if streamID == "" || kind == "" {
			s.mu.Unlock()
			err := fmt.Errorf("invalid semantic stream envelope")
			navigationBenchmarkMessageSent(benchmark, err)
			return err
		}
		if s.streamRevisions == nil {
			s.streamRevisions = make(map[string]uint64)
		}
		s.nextSemanticSequence++
		baseRevision := s.streamRevisions[streamID]
		revision := baseRevision + 1
		envelope := extui.Envelope{
			Sequence: s.nextSemanticSequence,
			StreamID: streamID,
			Revision: revision,
			Kind:     kind,
			Payload:  msg,
		}
		if kind != extui.KindSnapshot {
			envelope.BaseRevision = extui.Revision(baseRevision)
		}
		wireMessage = envelope.ToMap()
		s.streamRevisions[streamID] = revision
	}
	if benchmark != nil {
		navigationBenchmarkEmit(benchmark.traceID, "transport.send_lock.acquired", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType)
	}
	err := extUiSendMessageWithBenchmark(s.w, wireMessage, benchmark)
	s.mu.Unlock()
	navigationBenchmarkMessageSent(benchmark, err)
	return err
}

func (s *extUiMessageSender) sendSemanticBatchWithBenchmark(
	dispatches []extUiSemanticDispatch,
	benchmark *navigationBenchmarkMessage,
) error {
	if benchmark != nil {
		navigationBenchmarkEmit(benchmark.traceID, "transport.send_lock.wait", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType)
	}
	s.mu.Lock()
	if s.streamRevisions == nil {
		s.streamRevisions = make(map[string]uint64)
	}
	if benchmark != nil {
		navigationBenchmarkEmit(benchmark.traceID, "transport.send_lock.acquired", "go.transport",
			"phase", benchmark.phase, "phaseSequence", benchmark.phaseSequence,
			"sceneSequence", benchmark.sceneSequence, "messageType", benchmark.messageType)
	}
	var sendErr error
	for _, dispatch := range dispatches {
		if dispatch.streamID == "" || dispatch.kind == "" || dispatch.payload == nil {
			sendErr = fmt.Errorf("invalid semantic stream batch")
			break
		}
		s.nextSemanticSequence++
		baseRevision := s.streamRevisions[dispatch.streamID]
		revision := baseRevision + 1
		envelope := extui.Envelope{
			Sequence:     s.nextSemanticSequence,
			StreamID:     dispatch.streamID,
			Revision:     revision,
			BaseRevision: extui.Revision(baseRevision),
			Kind:         dispatch.kind,
			Payload:      dispatch.payload,
		}
		if dispatch.kind == extui.KindSnapshot {
			envelope.BaseRevision = nil
		}
		if sendErr = extUiSendMessageWithBenchmark(
			s.w, envelope.ToMap(), benchmark); sendErr != nil {
			break
		}
		s.streamRevisions[dispatch.streamID] = revision
	}
	s.mu.Unlock()
	navigationBenchmarkMessageSent(benchmark, sendErr)
	return sendErr
}

func extUiSemanticDispatches(msg map[string]any) []extUiSemanticDispatch {
	if extUiString(msg, "type") == "semantic_stream_snapshot" {
		streamID := extUiString(msg, "streamId")
		payload, _ := msg["payload"].(map[string]any)
		if streamID == "" || payload == nil {
			return nil
		}
		return []extUiSemanticDispatch{{
			streamID: streamID,
			kind:     extui.KindSnapshot,
			payload:  payload,
		}}
	}
	if extUiString(msg, "type") == "scene" {
		return extUiSplitSceneSnapshot(msg)
	}
	if extUiString(msg, "type") == "scene_patch" {
		if dispatches := extUiSplitScenePatch(msg); len(dispatches) > 0 {
			return dispatches
		}
	}
	streamID, kind, semantic := extUiSemanticStream(msg)
	if !semantic {
		return nil
	}
	return []extUiSemanticDispatch{{
		streamID: streamID,
		kind:     kind,
		payload:  msg,
	}}
}

// extUiChangedSceneSnapshotMessages is the fallback reconciliation path for
// state changes which cannot be represented by one of the smaller intent-
// specific patches. It compares independent stream projections and queues a
// snapshot only for streams whose owned state actually changed. The complete
// scene remains Go-local authoritative state and is never handed to the wire
// encoder from this path.
func extUiChangedSceneSnapshotMessages(previous, current map[string]any) []map[string]any {
	// A v4 scene is split into typed stream snapshots before it reaches the
	// sender. Assign the logical scene sequence while the complete benchmark
	// metadata is still available, then copy it into each delivered payload.
	navigationBenchmarkPrepareSceneMessage(current)
	if previous == nil {
		dispatches := extUiSplitSceneSnapshot(current)
		messages := make([]map[string]any, 0, len(dispatches))
		for _, dispatch := range dispatches {
			message := map[string]any{
				"type":     "semantic_stream_snapshot",
				"streamId": dispatch.streamID,
				"payload":  dispatch.payload,
			}
			for _, key := range []string{"benchmarkTraceId", "benchmark"} {
				if value, present := current[key]; present {
					dispatch.payload[key] = value
					message[key] = value
				}
			}
			messages = append(messages, message)
		}
		if len(messages) > 0 {
			messages[len(messages)-1]["benchmarkSceneFinal"] = true
		}
		return messages
	}
	streamIDs := []string{
		"chrome", "workspaces", "menus", "dialogs", "operations",
		"command-line",
	}
	appendPanelStreams := func(scene map[string]any) {
		if panels, ok := semanticScenePanelMaps(scene); ok {
			for side := range panels {
				streamIDs = append(streamIDs, "panel/"+strconv.Itoa(side))
			}
		}
	}
	appendDocumentStream := func(scene map[string]any) {
		surface, _ := scene["surface"].(map[string]any)
		if surface == nil {
			return
		}
		id := semanticString(surface["id"])
		if id == "" {
			id = "active"
		}
		streamIDs = append(streamIDs, "document/"+id)
	}
	appendPanelStreams(previous)
	appendPanelStreams(current)
	appendDocumentStream(previous)
	appendDocumentStream(current)
	// Shell is deliberately last: it exposes the composed surface only after
	// every changed catalog/document model in this transaction is installed.
	streamIDs = append(streamIDs, "shell")

	seen := make(map[string]struct{}, len(streamIDs))
	messages := make([]map[string]any, 0, len(streamIDs))
	for _, streamID := range streamIDs {
		if _, duplicate := seen[streamID]; duplicate {
			continue
		}
		seen[streamID] = struct{}{}
		nextPayload, nextOK := semanticStreamSnapshot(current, streamID)
		previousPayload, previousOK := semanticStreamSnapshot(previous, streamID)
		if !nextOK {
			// A document stream can disappear. An empty typed document snapshot
			// removes only that stream's surface in Qt.
			if strings.HasPrefix(streamID, "document/") && previousOK {
				nextPayload = map[string]any{
					"type":  semanticStreamSnapshotPayloadType(streamID),
					"state": map[string]any{},
				}
				nextOK = true
			}
		}
		if !nextOK {
			continue
		}
		if previousOK && reflect.DeepEqual(
			previousPayload["state"], nextPayload["state"]) {
			continue
		}
		for _, key := range []string{"benchmarkTraceId", "benchmark"} {
			if value, present := current[key]; present {
				nextPayload[key] = value
			}
		}
		message := map[string]any{
			"type":     "semantic_stream_snapshot",
			"streamId": streamID,
			"payload":  nextPayload,
		}
		for _, key := range []string{"benchmarkTraceId", "benchmark"} {
			if value, present := current[key]; present {
				message[key] = value
			}
		}
		messages = append(messages, message)
	}
	if len(messages) > 0 {
		messages[len(messages)-1]["benchmarkSceneFinal"] = true
	}
	return messages
}

// extUiSplitSceneSnapshot performs the v4 bootstrap as independent typed
// stream snapshots. In particular, a panel catalog is never serialized with
// menus, documents, or the other panel. The shell snapshot is emitted last so
// the Qt host cannot expose a stable panels surface before both catalog models
// have received their bounded initial windows.
func extUiSplitSceneSnapshot(scene map[string]any) []extUiSemanticDispatch {
	if scene == nil {
		return nil
	}
	streamIDs := []string{
		"chrome", "workspaces", "menus", "dialogs", "operations",
		"command-line",
	}
	if panels, ok := semanticScenePanelMaps(scene); ok {
		for _, panel := range panels {
			streamIDs = append(streamIDs,
				"panel/"+strconv.Itoa(extUiInt(panel, "side")))
		}
	}
	if surface, ok := scene["surface"].(map[string]any); ok {
		id := semanticString(surface["id"])
		if id == "" {
			id = "active"
		}
		streamIDs = append(streamIDs, "document/"+id)
	}
	streamIDs = append(streamIDs, "shell")

	dispatches := make([]extUiSemanticDispatch, 0, len(streamIDs))
	seen := make(map[string]struct{}, len(streamIDs))
	for _, streamID := range streamIDs {
		if _, duplicate := seen[streamID]; duplicate {
			continue
		}
		seen[streamID] = struct{}{}
		payload, ok := semanticStreamSnapshot(scene, streamID)
		if !ok {
			continue
		}
		state, _ := payload["state"].(map[string]any)
		if len(state) == 0 {
			continue
		}
		dispatches = append(dispatches, extUiSemanticDispatch{
			streamID: streamID,
			kind:     extui.KindSnapshot,
			payload:  payload,
		})
	}
	return dispatches
}

func extUiSplitScenePatch(msg map[string]any) []extUiSemanticDispatch {
	var dispatches []extUiSemanticDispatch
	appendGroupedMapPatch := func(location string, value any,
		route func(string) string,
	) {
		patch, ok := value.(map[string]any)
		if !ok {
			return
		}
		groups := extUiGroupMapPatch(patch, route)
		streams := make([]string, 0, len(groups))
		for streamID := range groups {
			streams = append(streams, streamID)
		}
		sort.Strings(streams)
		for _, streamID := range streams {
			payload := extUiScenePatchPayloadBase(msg)
			payload[location] = groups[streamID]
			dispatches = append(dispatches, extUiSemanticDispatch{
				streamID: streamID,
				kind:     extui.KindPatch,
				payload:  payload,
			})
		}
	}

	if shell, ok := msg["shell"].(map[string]any); ok {
		panelValues := extUiAnyMapSlice(shell["panels"])
		for index, panel := range panelValues {
			side := extUiInt(panel, "side")
			payload := extUiScenePatchPayloadBase(msg)
			panelShell := map[string]any{
				"panels": []map[string]any{panel},
			}
			// Shell title/prompt changes are derived from this catalog
			// transition. Keep them with the first affected panel so the
			// frontend commits one visible folder-entry transaction, while
			// unrelated root streams remain physically separate.
			if index == 0 {
				for _, key := range []string{"set", "clear"} {
					if value, present := shell[key]; present {
						panelShell[key] = value
					}
				}
			}
			payload["shell"] = panelShell
			dispatches = append(dispatches, extUiSemanticDispatch{
				streamID: "panel/" + strconv.Itoa(side),
				kind:     extui.KindPatch,
				payload:  payload,
			})
		}
		if len(panelValues) == 0 {
			appendGroupedMapPatch("shell", shell, func(key string) string {
				if key == "commandLine" {
					return "command-line"
				}
				return "shell"
			})
		}
	}
	rootRoute := extUiRootFieldStream
	if root, ok := msg["root"].(map[string]any); ok &&
		extUiMapPatchContainsKey(root, "menus") &&
		msg["shell"] == nil && msg["surface"] == nil {
		// Popup lifecycle is one bounded visual transaction. Contextual keybar
		// and workspace-title changes belong to that menu transaction too; a
		// second envelope would expose an avoidable intermediate frame.
		rootRoute = func(string) string { return "menus" }
	}
	appendGroupedMapPatch("root", msg["root"], rootRoute)
	if surface, ok := msg["surface"].(map[string]any); ok {
		id := extUiString(surface, "id")
		if id == "" {
			id = "active"
		}
		payload := extUiScenePatchPayloadBase(msg)
		payload["surface"] = surface
		dispatches = append(dispatches, extUiSemanticDispatch{
			streamID: "document/" + id,
			kind:     extui.KindPatch,
			payload:  payload,
		})
	}
	return dispatches
}

func extUiScenePatchPayloadBase(msg map[string]any) map[string]any {
	payload := map[string]any{
		"type":    "scene_patch",
		"schema":  msg["schema"],
		"version": msg["version"],
	}
	for _, key := range []string{
		"baseRevision", "revision", "benchmarkTraceId", "benchmark",
	} {
		if value, present := msg[key]; present {
			payload[key] = value
		}
	}
	return payload
}

func extUiGroupMapPatch(patch map[string]any,
	route func(string) string,
) map[string]map[string]any {
	groups := map[string]map[string]any{}
	ensure := func(streamID string) map[string]any {
		group := groups[streamID]
		if group == nil {
			group = map[string]any{}
			groups[streamID] = group
		}
		return group
	}
	if set, ok := patch["set"].(map[string]any); ok {
		for key, value := range set {
			streamID := route(key)
			group := ensure(streamID)
			groupSet, _ := group["set"].(map[string]any)
			if groupSet == nil {
				groupSet = map[string]any{}
				group["set"] = groupSet
			}
			groupSet[key] = value
		}
	}
	for _, key := range extUiAnyStringSlice(patch["clear"]) {
		streamID := route(key)
		group := ensure(streamID)
		clear, _ := group["clear"].([]string)
		group["clear"] = append(clear, key)
	}
	return groups
}

func extUiAnyMapSlice(value any) []map[string]any {
	switch values := value.(type) {
	case []map[string]any:
		return values
	case []any:
		result := make([]map[string]any, 0, len(values))
		for _, item := range values {
			if mapped, ok := item.(map[string]any); ok {
				result = append(result, mapped)
			}
		}
		return result
	default:
		return nil
	}
}

func extUiAnyStringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, item := range values {
			if text, ok := item.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func extUiMapPatchContainsKey(patch map[string]any, key string) bool {
	if set, ok := patch["set"].(map[string]any); ok {
		if _, present := set[key]; present {
			return true
		}
	}
	for _, candidate := range extUiAnyStringSlice(patch["clear"]) {
		if candidate == key {
			return true
		}
	}
	return false
}

func extUiSemanticStream(msg map[string]any) (string, string, bool) {
	switch extUiString(msg, "type") {
	case "scene_patch":
		// A well-formed ScenePatch is split above. Keep malformed/empty payloads
		// on a bounded canonical stream so the receiver can reject them without
		// reviving the former cross-stream "transaction" escape hatch.
		return "chrome", extui.KindPatch, true
	case "panel_catalog":
		return "panel/" + strconv.Itoa(extUiInt(msg, "side")),
			extui.KindReset, true
	case "panel_chrome", "panel_activation":
		return "shell", extui.KindPatch, true
	case "command_line":
		return "command-line", extui.KindPatch, true
	case "panel_catalog_metadata", "panel_catalog_metadata_rejected":
		return extUiPanelMessageStream(msg), extui.KindMetadata, true
	case "panel_catalog_rows", "panel_catalog_rows_rejected":
		return extUiPanelMessageStream(msg), extui.KindRows, true
	}
	return "", "", false
}

func extUiPanelMessageStream(msg map[string]any) string {
	if side, present := msg["side"]; present {
		return "panel/" + strconv.Itoa(extUiAnyInt(side))
	}
	if panelID := extUiString(msg, "panelId"); panelID != "" {
		return "panel-id/" + panelID
	}
	return "panel/unknown"
}

func extUiRootFieldStream(key string) string {
	switch key {
	case "workspaceTabs", "workspaceCount", "activeScreen":
		return "workspaces"
	case "menuBar", "menus":
		return "menus"
	case "dialogs":
		return "dialogs"
	case "operationsQueue":
		return "operations"
	case "surface":
		return "document/active"
	case "shell":
		return "shell"
	default:
		return "chrome"
	}
}

func extUiReadMessage(r io.Reader) (map[string]any, error) {
	return extUiReadMessageWithBenchmark(r, nil)
}

func extUiReadMessageWithBenchmark(r io.Reader, timing *navigationBenchmarkReadTiming) (map[string]any, error) {
	msg, err := extUiReadWireMessageWithBenchmark(r, timing)
	if err != nil {
		return nil, err
	}
	// Existing Go-side reducer tests consume the typed payload, while the Qt
	// peer validates the envelope itself. Application code never receives a
	// semantic envelope from Qt.
	if extUiString(msg, "type") == extui.EnvelopeType {
		if extUiInt(msg, "version") != extui.EnvelopeVersion {
			return nil, fmt.Errorf("unsupported extui semantic envelope")
		}
		payloadMap, ok := msg["payload"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid extui semantic payload")
		}
		return payloadMap, nil
	}
	return msg, nil
}

func extUiReadWireMessage(r io.Reader) (map[string]any, error) {
	return extUiReadWireMessageWithBenchmark(r, nil)
}

func extUiReadWireMessageWithBenchmark(r io.Reader,
	timing *navigationBenchmarkReadTiming,
) (map[string]any, error) {
	if timing != nil {
		timing.readStartNs = navigationBenchmarkMonotonicNs()
	}
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	if timing != nil {
		timing.headerDoneNs = navigationBenchmarkMonotonicNs()
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 {
		return nil, fmt.Errorf("empty extui message")
	}
	if n > extUiMaxMessageSize {
		return nil, fmt.Errorf("extui message too large: %d bytes", n)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	if timing != nil {
		timing.payloadDoneNs = navigationBenchmarkMonotonicNs()
		timing.payloadBytes = len(payload)
		timing.decodeStartNs = navigationBenchmarkMonotonicNs()
	}

	var msg map[string]any
	if err := msgpack.Unmarshal(payload, &msg); err != nil {
		return nil, err
	}
	if timing != nil {
		timing.decodeDoneNs = navigationBenchmarkMonotonicNs()
	}
	return msg, nil
}

func extUiString(msg map[string]any, key string) string {
	if v, ok := msg[key].(string); ok {
		return v
	}
	return ""
}

func extUiBool(msg map[string]any, key string) bool {
	return extUiAnyBool(msg[key])
}

func extUiAnyBool(value any) bool {
	result, _ := value.(bool)
	return result
}

func extUiInt(msg map[string]any, key string) int {
	return extUiAnyInt(msg[key])
}

func extUiInt64(msg map[string]any, key string) int64 {
	switch n := msg[key].(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		if n <= uint64(^uint64(0)>>1) {
			return int64(n)
		}
	}
	return 0
}

func extUiAnyInt(v any) int {
	value, _ := extUiAnyIntOK(v)
	return value
}

func extUiAnyIntOK(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int8:
		return int(n), true
	case int16:
		return int(n), true
	case int32:
		return int(n), true
	case int64:
		return boundedInt64ToInt(n)
	case uint:
		return boundedUint64ToInt(uint64(n))
	case uint8:
		return int(n), true
	case uint16:
		return int(n), true
	case uint32:
		return boundedUint64ToInt(uint64(n))
	case uint64:
		return boundedUint64ToInt(n)
	}
	return 0, false
}

func extUiDimension(msg map[string]any, key string) (int, bool) {
	value, ok := extUiAnyIntOK(msg[key])
	return value, ok && value > 0 && value <= extUiMaxDimension
}

func extUiInt16(msg map[string]any, key string) (int16, bool) {
	value, ok := extUiAnyIntOK(msg[key])
	if !ok {
		return 0, false
	}
	return boundedInt16(value)
}

func extUiUint16(msg map[string]any, key string) (uint16, bool) {
	value, ok := extUiAnyIntOK(msg[key])
	if !ok {
		return 0, false
	}
	return boundedUint16(value)
}

func extUiUint32(msg map[string]any, key string) (uint32, bool) {
	value, ok := extUiAnyIntOK(msg[key])
	if !ok {
		return 0, false
	}
	return boundedUint32(value)
}

func extUiRune(msg map[string]any, key string) (rune, bool) {
	value, ok := extUiAnyIntOK(msg[key])
	if !ok {
		return 0, false
	}
	return boundedRune(value)
}

type ExtUiRenderer struct {
	mu   sync.Mutex
	conn net.Conn
	send *extUiMessageSender

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      vtui.CursorShape
	cursorDirty      bool

	palette      [256]uint32
	paletteValid bool

	pendingPalette              []uint32
	pendingFrame                map[string]any
	pendingScene                map[string]any
	pendingCommandLine          map[string]any
	pendingCommandLineScene     map[string]any
	pendingPanelCatalog         map[string]any
	pendingPanelCatalogScene    map[string]any
	pendingPanelActivation      map[string]any
	pendingPanelActivationScene map[string]any
	pendingScenePatch           map[string]any
	lastScene                   map[string]any
	lastCompactScene            map[string]any
	sceneRevision               uint64
	queuedPanelActivationSide   int
	panelActivationQueued       bool
	nextPanelActivationRevision uint64
	suppressSemanticExport      bool
	deferSemanticRender         bool
	deferSemanticRenderBound    bool
	deferSemanticRenderGen      uint64
	semanticUpdateOpen          bool
	semanticUpdateHandled       bool
	semanticUpdateTouched       bool
	semanticUpdateCheckpoint    bool
	semanticUpdatePreviousBound bool
	semanticFastPathUnsafe      bool
	panelActivationProjected    bool
	directPanelCatalog          map[string]any
	// The semantic Qt presentation fully owns native app surfaces. Its cell
	// grid remains instantiated only as a fallback/input sink, so serializing
	// the hidden TUI buffer on every panel mutation wastes the latency budget.
	nativeSemanticSurfaceEnabled bool
	nativeCellFrameSuppressed    bool
	forceNextCellFrame           bool
	fallbackRevealPending        bool
	lastWindowTitle              string
	windowTitleValid             bool
	closed                       bool
}

func NewExtUiRenderer(conn net.Conn, sender *extUiMessageSender) *ExtUiRenderer {
	return &ExtUiRenderer{
		conn: conn, send: sender, cursorDirty: true,
	}
}

// SendStreamSnapshot answers a revision-gap request without serializing an
// unrelated application scene. The renderer's lastScene is the authoritative
// immutable semantic snapshot; each projection below contains only one
// protocol stream's state.
func (r *ExtUiRenderer) SendStreamSnapshot(streamID string) bool {
	if r == nil || streamID == "" {
		return false
	}
	r.mu.Lock()
	if r.closed || r.send == nil {
		r.mu.Unlock()
		return false
	}
	payload, ok := semanticStreamSnapshot(r.lastScene, streamID)
	r.mu.Unlock()
	if !ok {
		return false
	}
	if err := r.send.SendSemanticSnapshot(streamID, payload); err != nil {
		vtui.DebugLog("EXTUI_RENDERER: stream snapshot send failed: %v", err)
		r.mu.Lock()
		r.closed = true
		r.mu.Unlock()
		return false
	}
	return true
}

// BeginSemanticSceneUpdate starts an input/task mutation boundary. Unless a
// handler queues a proven compact update, the boundary makes the next render
// perform a normal semantic export.
func (r *ExtUiRenderer) BeginSemanticSceneUpdate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.semanticUpdateOpen {
		r.semanticFastPathUnsafe = true
		r.suppressSemanticExport = false
		r.deferSemanticRender = false
		r.semanticUpdateCheckpoint = false
		r.semanticUpdateTouched = true
	} else {
		r.semanticUpdateCheckpoint = true
		r.semanticUpdatePreviousBound = r.deferSemanticRenderBound
		r.semanticUpdateTouched = false
	}
	r.semanticUpdateOpen = true
	r.semanticUpdateHandled = false
	r.deferSemanticRenderBound = false
}

// EndSemanticSceneUpdate conservatively invalidates a queued direct update
// when some other input or UI task was processed in the same render batch.
func (r *ExtUiRenderer) EndSemanticSceneUpdate() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.semanticUpdateOpen && !r.semanticUpdateHandled {
		r.semanticFastPathUnsafe = true
		r.suppressSemanticExport = false
		r.deferSemanticRender = false
	}
	r.semanticUpdateOpen = false
	r.semanticUpdateHandled = false
	r.semanticUpdateTouched = false
	r.semanticUpdateCheckpoint = false
}

// EndSemanticSceneUpdateUnchanged closes a task/input boundary whose caller
// proved that it did not change semantic state. The renderer accepts that proof
// when none of its semantic/direct-update entry points were touched inside the
// boundary. This proof is presentation-independent: a redraw which was already
// pending still renders, while a standalone proven no-op does not manufacture
// a redraw merely because the client currently uses the compatibility grid.
func (r *ExtUiRenderer) EndSemanticSceneUpdateUnchanged() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.semanticUpdateOpen || !r.semanticUpdateCheckpoint ||
		r.semanticUpdateTouched || r.semanticUpdateHandled {
		return false
	}
	r.semanticUpdateOpen = false
	r.semanticUpdateHandled = false
	r.semanticUpdateTouched = false
	r.semanticUpdateCheckpoint = false
	r.deferSemanticRenderBound = r.semanticUpdatePreviousBound
	return true
}

// ConsumeSemanticSceneExportSuppression consumes the one-render permit armed
// by QueuePanelActivation. Cell rendering and Flush still run normally.
func (r *ExtUiRenderer) ConsumeSemanticSceneExportSuppression() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.suppressSemanticExport || r.semanticFastPathUnsafe {
		reason := "no_permit"
		if r.semanticFastPathUnsafe {
			reason = "unsafe_boundary"
		}
		navigationBenchmarkRenderEvent("scene.suppression.rejected",
			"reason", reason, "hadPermit", r.suppressSemanticExport)
		r.suppressSemanticExport = false
		r.deferSemanticRender = false
		return false
	}
	navigationBenchmarkRenderEvent("scene.suppression.accepted")
	r.suppressSemanticExport = false
	r.deferSemanticRender = false
	r.deferSemanticRenderBound = false
	r.panelActivationQueued = false
	return true
}

// BindSemanticRenderPhaseDeferral ties a direct update to the redraw state at
// the end of its complete FrameManager mutation boundary. A redraw requested
// afterwards changes the generation and forces an ordinary render.
func (r *ExtUiRenderer) BindSemanticRenderPhaseDeferral(generation uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deferSemanticRenderBound = r.deferSemanticRender &&
		!r.semanticUpdateOpen && !r.semanticFastPathUnsafe
	if r.deferSemanticRenderBound {
		r.deferSemanticRenderGen = generation
	}
}

func (r *ExtUiRenderer) SemanticRenderPhaseDeferralBound(generation uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deferSemanticRender && r.deferSemanticRenderBound &&
		r.deferSemanticRenderGen == generation && !r.closed &&
		!r.semanticUpdateOpen && !r.semanticFastPathUnsafe &&
		r.nativeCellFrameSuppressed
}

// ConsumeSemanticRenderPhaseDeferral consumes the one-render permit armed by
// a compact update that has already crossed the wire. The whole render can be
// omitted only while Qt owns the visible semantic surface and no unverified
// mutation or pending transport state needs Show/Flush. Cursor changes do not
// block this path: the hidden cell grid retains them until fallback is shown.
func (r *ExtUiRenderer) ConsumeSemanticRenderPhaseDeferral(generation uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.deferSemanticRender {
		return false
	}
	r.deferSemanticRender = false
	bound := r.deferSemanticRenderBound
	r.deferSemanticRenderBound = false
	if !bound || r.deferSemanticRenderGen != generation ||
		r.closed || r.semanticUpdateOpen || r.semanticFastPathUnsafe ||
		!r.nativeCellFrameSuppressed || r.directPanelCatalog != nil ||
		r.forceNextCellFrame || r.pendingPalette != nil || r.pendingFrame != nil ||
		r.pendingScene != nil || r.pendingCommandLine != nil ||
		r.pendingPanelCatalog != nil || r.pendingPanelActivation != nil {
		if bound {
			// A newer redraw may represent semantic state outside the direct
			// activation. Do not let the narrower export permit hide it.
			r.suppressSemanticExport = false
			r.panelActivationQueued = false
		}
		return false
	}
	// No export will run in this phase, so consume the narrower permit too and
	// discard activation bookkeeping that it would otherwise have cleared.
	r.suppressSemanticExport = false
	r.panelActivationQueued = false
	return true
}

// QueuePanelActivation immediately validates the delivered semantic snapshot
// and, when safe, prepares its small copy-on-write successor. That lets the
// next render omit the complete catalog traversal. shellTitle is optional for
// compatibility with callers that do not expose an active-panel title.
func (r *ExtUiRenderer) QueuePanelActivation(side int, shellTitle ...string) {
	title := ""
	if len(shellTitle) > 0 {
		title = shellTitle[0]
	}
	r.queuePanelActivation(side, title, nil)
}

// QueuePanelActivationState is the application-aware activation path. The
// command-line prompt depends on the active panel path, so it travels beside
// the activation while the large catalogs remain shared and untouched.
func (r *ExtUiRenderer) QueuePanelActivationState(side int, shellTitle string,
	commandLine map[string]any,
) {
	r.queuePanelActivation(side, shellTitle, commandLine)
}

func (r *ExtUiRenderer) queuePanelActivation(side int, title string,
	commandLine map[string]any,
) {
	if side < 0 || side > 1 {
		navigationBenchmarkUIEvent("panel.activate.direct_rejected",
			"reason", "invalid_side", "side", side)
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.semanticUpdateOpen {
		r.semanticUpdateTouched = true
	}
	r.queuedPanelActivationSide = side
	r.panelActivationQueued = true
	rejection := ""
	switch {
	case r.closed:
		rejection = "closed"
	case r.semanticFastPathUnsafe:
		rejection = "unsafe_boundary"
	case r.pendingScene != nil:
		rejection = "pending_scene"
	case r.pendingCommandLine != nil:
		rejection = "pending_command_line"
	case r.pendingPanelCatalog != nil:
		rejection = "pending_panel_catalog"
	case r.directPanelCatalog != nil:
		rejection = "direct_panel_catalog"
	}
	if rejection != "" {
		navigationBenchmarkUIEvent("panel.activate.direct_rejected",
			"reason", rejection, "side", side,
			"sceneRevision", r.sceneRevision)
		return
	}

	basis := r.lastScene
	if r.pendingPanelActivationScene != nil {
		basis = r.pendingPanelActivationScene
	}
	patched, rejection := semanticSceneWithPanelActivation(
		basis, side, title, commandLine)
	if rejection != "" {
		navigationBenchmarkUIEvent("panel.activate.direct_rejected",
			"reason", rejection, "side", side,
			"sceneRevision", r.sceneRevision)
		return
	}

	// An even number of coalesced Tab events returns to the last delivered
	// side. Detect that from the scalar fields plus the bounded command-line
	// model instead of comparing the scene, which would walk every shared
	// catalog row and defeat the purpose of this fast path.
	deliveredSide, deliveredSideOK := semanticSceneActivePanel(r.lastScene)
	deliveredShell, _ := r.lastScene["shell"].(map[string]any)
	deliveredTitle := semanticString(deliveredShell["title"])
	deliveredCommandLineMatches := commandLine == nil ||
		reflect.DeepEqual(commandLine, deliveredShell["commandLine"])
	if deliveredSideOK && side == deliveredSide &&
		(title == "" || title == deliveredTitle) && deliveredCommandLineMatches {
		r.pendingPanelActivation = nil
		r.pendingPanelActivationScene = nil
		r.suppressSemanticExport = true
		r.deferSemanticRender = r.nativeCellFrameSuppressed
		r.deferSemanticRenderBound = false
		if r.semanticUpdateOpen {
			r.semanticUpdateHandled = true
		}
		navigationBenchmarkUIEvent("panel.activate.direct_accepted",
			"result", "unchanged", "side", side,
			"sceneRevision", r.sceneRevision)
		return
	}
	if deliveredSideOK && side == deliveredSide {
		// A title mutation while returning to the delivered side cannot have
		// come from panel activation alone. Preserve the regular full-export
		// fallback for that unexpected state.
		r.semanticFastPathUnsafe = true
		r.suppressSemanticExport = false
		navigationBenchmarkUIEvent("panel.activate.direct_rejected",
			"reason", "same_side_title_changed", "side", side,
			"sceneRevision", r.sceneRevision)
		return
	}

	revision := uint64(0)
	if r.pendingPanelActivation != nil {
		revision = uint64(extUiAnyInt(r.pendingPanelActivation["revision"]))
	}
	if revision == 0 {
		r.nextPanelActivationRevision++
		revision = r.nextPanelActivationRevision
	}
	patch := map[string]any{
		"type":        "panel_activation",
		"activePanel": side,
		"revision":    revision,
	}
	if title != "" {
		patch["shellTitle"] = title
	}
	if commandLine != nil {
		patch["commandLine"] = commandLine
	}
	if trace := navigationBenchmarkCurrentUI(); trace != nil {
		patch["benchmarkTraceId"] = trace.id
	}
	r.suppressSemanticExport = true

	// Tab is an input-latency path, not a render path.  Waiting for Flush here
	// used to add the complete TUI Show() cost (20-30 ms on a large panel) before
	// this catalog-free patch even entered the socket.  The sender is serialized
	// independently, so publish the already validated authoritative transition
	// now and advance the logical scene only after the write succeeds.
	if r.send != nil {
		benchmark := navigationBenchmarkPrepareImmediateMessage(patch)
		if err := r.send.SendWithBenchmark(patch, benchmark); err != nil {
			vtui.DebugLog("EXTUI_RENDERER: direct activation send failed: %v", err)
			r.closed = true
			r.suppressSemanticExport = false
			r.deferSemanticRender = false
			return
		}
		r.lastScene = patched
		// panel_activation and scene_patch advance two independent wire
		// sequences, but both are projections of the same logical app scene.
		// Keep the row-free snapshot in lockstep so the next menu/header patch
		// is based on the state Qt already displays.
		r.lastCompactScene = compactAppSemanticScene(patched)
		r.panelActivationProjected = true
		r.pendingPanelActivation = nil
		r.pendingPanelActivationScene = nil
		r.deferSemanticRender = r.nativeCellFrameSuppressed
		r.deferSemanticRenderBound = false
		if r.semanticUpdateOpen {
			r.semanticUpdateHandled = true
		}
		navigationBenchmarkUIEvent("panel.activate.direct_accepted",
			"result", "sent", "side", side, "revision", revision,
			"sceneRevision", r.sceneRevision)
		return
	}

	// A nil sender is useful to embedders and unit tests that drive Flush
	// manually. Preserve the queued form as a conservative fallback.
	r.pendingPanelActivation = patch
	r.pendingPanelActivationScene = patched
	if r.semanticUpdateOpen {
		r.semanticUpdateHandled = true
	}
	navigationBenchmarkUIEvent("panel.activate.direct_accepted",
		"result", "queued", "side", side, "revision", revision,
		"sceneRevision", r.sceneRevision)
}

// SetSemanticMenuState publishes a root-only scene patch from an input
// transaction which FrameManager has proved changed only popup/global-menu
// presentation. The projection is intentionally independent of panel header
// caches, so opening or moving in a menu can never fall back merely because a
// directory with thousands of entries is between semantic revisions.
func (r *ExtUiRenderer) SetSemanticMenuState(ctx *vtui.SemanticContext) bool {
	current, supported := BuildAppMenuState(ctx)
	if !supported {
		navigationBenchmarkUIEvent("menu_state.direct_rejected",
			"reason", "projection_unsupported")
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.semanticUpdateOpen {
		navigationBenchmarkUIEvent("menu_state.direct_rejected",
			"reason", "outside_boundary")
		return false
	}
	r.semanticUpdateTouched = true
	rejection := ""
	switch {
	case r.semanticUpdateHandled:
		rejection = "boundary_already_handled"
	case r.closed:
		rejection = "closed"
	case r.send == nil:
		rejection = "no_sender"
	case r.semanticFastPathUnsafe:
		rejection = "unsafe_boundary"
	case r.sceneRevision == 0:
		rejection = "no_scene_revision"
	case r.lastScene == nil:
		rejection = "no_scene_snapshot"
	case r.lastCompactScene == nil:
		rejection = "no_compact_snapshot"
	case r.pendingScene != nil:
		rejection = "pending_scene"
	case r.pendingScenePatch != nil:
		rejection = "pending_scene_patch"
	case r.pendingCommandLine != nil:
		rejection = "pending_command_line"
	case r.pendingPanelCatalog != nil:
		rejection = "pending_panel_catalog"
	case r.pendingPanelActivation != nil:
		rejection = "pending_panel_activation"
	case r.directPanelCatalog != nil:
		rejection = "direct_panel_catalog"
	}
	if rejection != "" {
		navigationBenchmarkUIEvent("menu_state.direct_rejected",
			"reason", rejection, "sceneRevision", r.sceneRevision)
		return false
	}

	rootSet, rootClear := semanticPatchChangedKeys(
		r.lastCompactScene, current, semanticMenuStateRootPatchKeys)
	armDirectResult := func() {
		r.suppressSemanticExport = true
		r.deferSemanticRender = r.nativeCellFrameSuppressed
		r.deferSemanticRenderBound = false
		r.semanticUpdateHandled = true
		r.panelActivationQueued = false
	}
	if len(rootSet) == 0 && len(rootClear) == 0 {
		// Key-up and modifier events still need an explicit no-change proof;
		// otherwise they would invalidate a menu patch queued earlier in the
		// same drained input batch and resurrect the full exporter.
		armDirectResult()
		navigationBenchmarkUIEvent("menu_state.direct_accepted",
			"result", "unchanged", "sceneRevision", r.sceneRevision)
		return true
	}

	patch := extui.ScenePatch{
		BaseRevision: r.sceneRevision,
		Revision:     r.sceneRevision + 1,
		Root: &extui.MapPatch{
			Set: rootSet, Clear: rootClear,
		},
	}
	wire := patch.ToMap()
	if trace := navigationBenchmarkCurrentUI(); trace != nil {
		wire["benchmarkTraceId"] = trace.id
	}
	benchmark := navigationBenchmarkPrepareImmediateMessage(wire)
	if err := r.send.SendWithBenchmark(wire, benchmark); err != nil {
		vtui.DebugLog("EXTUI_RENDERER: direct menu state send failed: %v", err)
		r.closed = true
		r.suppressSemanticExport = false
		r.deferSemanticRender = false
		return false
	}

	r.sceneRevision = patch.Revision
	r.lastScene = semanticSceneStructuralMapCopy(r.lastScene)
	applyAppScenePatchToSnapshot(r.lastScene, patch)
	if r.lastScene != nil {
		r.lastScene["version"] = extui.SceneVersion
	}
	r.lastCompactScene = compactAppSemanticScene(r.lastScene)
	armDirectResult()
	navigationBenchmarkUIEvent("menu_state.direct_accepted",
		"result", "sent", "baseRevision", patch.BaseRevision,
		"revision", patch.Revision, "setKeys", len(rootSet),
		"clearKeys", len(rootClear))
	return true
}

var semanticEditorSurfaceStateKeys = []string{
	"cursorLine",
	"cursorPos",
	"cursorVisualRow",
	"cursorVisualColumn",
	"cursorVisible",
	"cursorShape",
	"cursorAbsoluteRow",
	"topBarRight",
}

func semanticEditorSurfaceStateValid(state map[string]any) bool {
	if len(state) != len(semanticEditorSurfaceStateKeys) {
		return false
	}
	integer := func(value any, nonNegative bool) bool {
		if value == nil {
			return false
		}
		reflected := reflect.ValueOf(value)
		switch reflected.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return !nonNegative || reflected.Int() >= 0
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return reflected.Uint() <= uint64(^uint64(0)>>1)
		default:
			return false
		}
	}
	for _, key := range semanticEditorSurfaceStateKeys {
		value, present := state[key]
		if !present {
			return false
		}
		switch key {
		case "cursorVisible":
			if _, ok := value.(bool); !ok {
				return false
			}
		case "cursorShape":
			shape, ok := value.(string)
			if !ok || (shape != "underline" && shape != "block") {
				return false
			}
		case "topBarRight":
			if _, ok := value.(string); !ok {
				return false
			}
		case "cursorLine", "cursorPos", "cursorAbsoluteRow":
			if !integer(value, true) {
				return false
			}
		default:
			if !integer(value, false) {
				return false
			}
		}
	}
	return true
}

// QueueSurfaceState publishes only the scalar cursor state and right status
// string of the currently displayed editor. The caller has already proved that
// the key changed no document, selection, viewport, mode, or autocomplete
// state. Surface identity and the exact scene revision keep late key repeats
// from touching a replacement document.
func (r *ExtUiRenderer) QueueSurfaceState(surfaceID string,
	state map[string]any,
) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.semanticUpdateOpen {
		r.semanticUpdateTouched = true
	}
	rejection := ""
	switch {
	case !r.semanticUpdateOpen:
		rejection = "outside_boundary"
	case r.semanticUpdateHandled:
		rejection = "boundary_already_handled"
	case r.closed:
		rejection = "closed"
	case r.send == nil:
		rejection = "no_sender"
	case r.semanticFastPathUnsafe:
		rejection = "unsafe_boundary"
	case r.sceneRevision == 0:
		rejection = "no_scene_revision"
	case r.lastScene == nil || r.lastCompactScene == nil:
		rejection = "no_scene_snapshot"
	case r.pendingScene != nil:
		rejection = "pending_scene"
	case r.pendingScenePatch != nil:
		rejection = "pending_scene_patch"
	case r.pendingCommandLine != nil:
		rejection = "pending_command_line"
	case r.pendingPanelCatalog != nil:
		rejection = "pending_panel_catalog"
	case r.pendingPanelActivation != nil:
		rejection = "pending_panel_activation"
	case r.directPanelCatalog != nil:
		rejection = "direct_panel_catalog"
	case surfaceID == "" || !semanticEditorSurfaceStateValid(state):
		rejection = "invalid_state"
	}
	surface, surfaceOK := r.lastScene["surface"].(map[string]any)
	compactSurface, compactSurfaceOK := r.lastCompactScene["surface"].(map[string]any)
	if rejection == "" && (!surfaceOK || !compactSurfaceOK ||
		semanticString(surface["id"]) != surfaceID ||
		semanticString(compactSurface["id"]) != surfaceID ||
		semanticString(surface["kind"]) != "editor" ||
		semanticString(compactSurface["kind"]) != "editor") {
		rejection = "surface_identity_mismatch"
	}
	if rejection != "" {
		navigationBenchmarkUIEvent("surface_state.direct_rejected",
			"reason", rejection, "surfaceId", surfaceID,
			"sceneRevision", r.sceneRevision)
		return false
	}

	set, clear := semanticPatchChangedKeys(surface, state,
		semanticEditorSurfaceStateKeys)
	armDirectResult := func() {
		r.suppressSemanticExport = true
		r.deferSemanticRender = r.nativeCellFrameSuppressed
		r.deferSemanticRenderBound = false
		r.semanticUpdateHandled = true
		r.panelActivationQueued = false
		r.panelActivationProjected = false
	}
	if len(set) == 0 && len(clear) == 0 {
		armDirectResult()
		navigationBenchmarkUIEvent("surface_state.direct_accepted",
			"result", "unchanged", "surfaceId", surfaceID,
			"sceneRevision", r.sceneRevision)
		return true
	}

	patch := extui.ScenePatch{
		BaseRevision: r.sceneRevision,
		Revision:     r.sceneRevision + 1,
		Surface: &extui.SurfacePatch{
			SurfaceID: surfaceID,
			MapPatch:  extui.MapPatch{Set: set},
		},
	}
	wire := patch.ToMap()
	if trace := navigationBenchmarkCurrentOrPublishedTrace(); trace != nil {
		wire["benchmarkTraceId"] = trace.id
	}
	benchmark := navigationBenchmarkPrepareImmediateMessage(wire)
	if err := r.send.SendWithBenchmark(wire, benchmark); err != nil {
		vtui.DebugLog("EXTUI_RENDERER: direct surface state send failed: %v", err)
		r.closed = true
		r.suppressSemanticExport = false
		r.deferSemanticRender = false
		return false
	}

	r.sceneRevision = patch.Revision
	r.lastScene = semanticSceneStructuralMapCopy(r.lastScene)
	applyAppScenePatchToSnapshot(r.lastScene, patch)
	r.lastCompactScene = semanticSceneStructuralMapCopy(r.lastCompactScene)
	applyAppScenePatchToSnapshot(r.lastCompactScene, patch)
	if r.lastScene != nil {
		r.lastScene["version"] = extui.SceneVersion
	}
	if r.lastCompactScene != nil {
		r.lastCompactScene["version"] = extui.SceneVersion
	}
	armDirectResult()
	navigationBenchmarkUIEvent("surface_state.direct_accepted",
		"result", "sent", "surfaceId", surfaceID,
		"baseRevision", patch.BaseRevision, "revision", patch.Revision,
		"setKeys", len(set))
	return true
}

// A catalog replacement is intentionally atomic. Streaming a complete cold
// directory in hundreds of catalog_append patches repeatedly detached the
// growing QVariant scene and made QML recompute layout after every insertion,
// turning a linear 30k-row transfer into O(N²) work. MessagePack's protocol
// limit comfortably holds the minimal deferred catalog; QML still creates
// delegates only for the visible viewport and its reuse margin.
const semanticCatalogStreamChunkSize = 512

func semanticPanelCatalogStreamPrefix(panel map[string]any,
	entries []map[string]any, total int) map[string]any {
	out := make(map[string]any, len(panel)+1)
	for key, value := range panel {
		out[key] = value
	}
	out["entries"] = append([]map[string]any(nil), entries...)
	out["catalogProvisional"] = true
	out["totalCount"] = total
	// Fast-find matches are keyed by entry ID. A prefix must not claim matches
	// for rows which have not reached Qt yet; the final state will restore the
	// complete bounded match map through the normal row-free update.
	if matches, ok := panel["fastFindMatches"].(map[string]any); ok {
		filtered := make(map[string]any)
		for _, entry := range entries {
			if entryID := semanticString(entry["entryId"]); entryID != "" {
				if match, present := matches[entryID]; present {
					filtered[entryID] = match
				}
			}
		}
		if len(filtered) == 0 {
			delete(out, "fastFindMatches")
		} else {
			out["fastFindMatches"] = filtered
		}
	}
	return out
}

// QueuePanelCatalogModelState converts the bounded typed catalog directly.
// Panel rows are paged from Go on demand; the protocol never retains or
// reuses a complete panel snapshot.
func (r *ExtUiRenderer) QueuePanelCatalogModelState(side int,
	model extui.PanelModel, shellTitle, traceID string,
) bool {
	if side < 0 || side > 1 {
		return false
	}
	return r.queuePanelCatalogState(
		side, map[string]any(model.ToMap()), shellTitle, nil, false, traceID)
}

// QueuePanelCatalogModelStateWithCommandLine publishes every shell field that
// directory navigation can change without running the hidden TUI Show pass.
// Because the panel, title, and prompt advance in one revision, the renderer
// can prove that the following native-frame render is redundant.
func (r *ExtUiRenderer) QueuePanelCatalogModelStateWithCommandLine(side int,
	model extui.PanelModel, shellTitle string, commandLine map[string]any,
	traceID string,
) bool {
	if side < 0 || side > 1 {
		return false
	}
	return r.queuePanelCatalogState(side, map[string]any(model.ToMap()),
		shellTitle, commandLine, true, traceID)
}

// QueuePanelCatalogState publishes a minimal catalog in bounded immutable
// chunks. The first chunk is a real interactive model, not a spinner-only
// placeholder; later chunks use rowsInserted and never reset or renormalize
// the rows that are already visible. Paged catalogs carry only a viewport-
// sized slice and Qt asks Go for other slices as scrolling requires them.
func (r *ExtUiRenderer) QueuePanelCatalogState(side int, panel map[string]any,
	shellTitle, traceID string,
) bool {
	return r.queuePanelCatalogState(
		side, panel, shellTitle, nil, false, traceID)
}

func (r *ExtUiRenderer) queuePanelCatalogState(side int, panel map[string]any,
	shellTitle string, commandLine map[string]any,
	presentationComplete bool, traceID string,
) bool {
	if side < 0 || side > 1 || panel == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.semanticUpdateOpen {
		r.semanticUpdateTouched = true
	}
	if r.closed || r.send == nil || r.sceneRevision == 0 ||
		r.pendingScenePatch != nil || r.directPanelCatalog != nil ||
		r.pendingScene != nil ||
		r.pendingCommandLine != nil || r.pendingPanelCatalog != nil ||
		r.pendingPanelActivation != nil {
		return false
	}
	activeSide, activeOK := semanticSceneActivePanel(r.lastScene)
	panels, panelsOK := semanticScenePanelMaps(r.lastScene)
	if !activeOK || activeSide != side || !panelsOK || side >= len(panels) ||
		!semanticPanelCatalogTransitionSafe(panels[side], panel, side) {
		return false
	}
	baseCatalogRevision := semanticInt64(panels[side]["catalogRevision"])
	catalogRevision := semanticInt64(panel["catalogRevision"])
	if catalogRevision <= baseCatalogRevision {
		return false
	}
	entries := appMapSlice(panel["entries"])
	totalCount := extUiAnyInt(panel["totalCount"])
	if totalCount <= 0 {
		totalCount = len(entries)
	}
	// An authoritative catalog shorter than totalCount is necessarily a sparse
	// page. Preserve that invariant even when the snapshot originated before
	// the rows capability handshake completed and therefore omitted the flag.
	// A streamed legacy prefix is marked provisional below and remains a
	// different protocol shape.
	if totalCount > len(entries) && !extUiBool(panel, "catalogProvisional") &&
		!extUiBool(panel, "catalogRowsDeferred") {
		copyPanel := make(map[string]any, len(panel)+1)
		for key, value := range panel {
			copyPanel[key] = value
		}
		copyPanel["catalogRowsDeferred"] = true
		panel = copyPanel
	}
	streamCatalog := extUiBool(panel, "metadataDeferred")
	streamCatalog = streamCatalog && !extUiBool(panel, "catalogProvisional") &&
		totalCount == len(entries) &&
		len(entries) > semanticCatalogStreamChunkSize
	wirePanel := panel
	if streamCatalog {
		wirePanel = semanticPanelCatalogStreamPrefix(
			panel, entries[:semanticCatalogStreamChunkSize], totalCount)
	}
	shellPatch := &extui.ShellPatch{Panels: []extui.PanelPatch{{
		Op: "catalog_replace", Side: side,
		PanelID:             semanticString(panel["id"]),
		BaseCatalogRevision: baseCatalogRevision,
		CatalogRevision:     catalogRevision,
		Panel:               panel,
	}}}
	if shell, ok := r.lastScene["shell"].(map[string]any); ok {
		shellSet := extui.M{}
		if shellTitle != "" && shellTitle != semanticString(shell["title"]) {
			shellSet["title"] = shellTitle
		}
		if commandLine != nil &&
			!reflect.DeepEqual(commandLine, shell["commandLine"]) {
			shellSet["commandLine"] = commandLine
		}
		if len(shellSet) != 0 {
			shellPatch.Set = shellSet
		}
	}
	patch := extui.ScenePatch{
		BaseRevision: r.sceneRevision,
		Revision:     r.sceneRevision + 1,
		Shell:        shellPatch,
	}
	// The panel stream has a dedicated reset payload in v4. Sending the
	// catalog as a generic scene patch made Qt rebuild an isolated map scene,
	// derive the same panel descriptor again, and notify QML in a second stage.
	// Keep ScenePatch only as the Go-local authoritative state transition.
	wire := map[string]any{
		"type":        "panel_catalog",
		"activePanel": activeSide,
		"side":        side,
		"panel":       wirePanel,
	}
	if title, present := shellPatch.Set["title"]; present {
		wire["shellTitle"] = title
	}
	if prompt, present := shellPatch.Set["commandLine"]; present {
		wire["commandLine"] = prompt
	}
	if traceID == "" {
		if trace := navigationBenchmarkCurrentUI(); trace != nil {
			traceID = trace.id
		}
	}
	if traceID != "" {
		wire["benchmarkTraceId"] = traceID
	}
	sendImmediate := func(message map[string]any) bool {
		benchmark := navigationBenchmarkPrepareImmediateMessage(message)
		if err := r.send.SendWithBenchmark(message, benchmark); err != nil {
			vtui.DebugLog("EXTUI_RENDERER: direct panel catalog send failed: %v", err)
			r.closed = true
			r.deferSemanticRender = false
			return false
		}
		return true
	}
	if !sendImmediate(wire) {
		return false
	}
	r.sceneRevision = patch.Revision
	if streamCatalog {
		for offset := semanticCatalogStreamChunkSize; offset < len(entries); offset += semanticCatalogStreamChunkSize {
			end := offset + semanticCatalogStreamChunkSize
			if end > len(entries) {
				end = len(entries)
			}
			appendPatch := extui.ScenePatch{
				BaseRevision: r.sceneRevision,
				Revision:     r.sceneRevision + 1,
				Shell: &extui.ShellPatch{Panels: []extui.PanelPatch{{
					Op:              "catalog_append",
					Side:            side,
					PanelID:         semanticString(panel["id"]),
					CatalogRevision: catalogRevision,
					CatalogOffset:   offset,
					CatalogTotal:    totalCount,
					CatalogFinal:    end == len(entries),
					Entries:         entries[offset:end],
				}}},
			}
			appendWire := appendPatch.ToMap()
			if traceID != "" {
				appendWire["benchmarkTraceId"] = traceID
			}
			if !sendImmediate(appendWire) {
				return false
			}
			r.sceneRevision = appendPatch.Revision
		}
	}
	// The initial wire operation may have contained only a prefix, but the
	// snapshot patch deliberately contains the complete panel. Applying it
	// here avoids appending every chunk to the Go-side authoritative scene.
	r.lastScene = semanticSceneStructuralMapCopy(r.lastScene)
	applyAppScenePatchToSnapshot(r.lastScene, patch)
	if r.lastScene != nil {
		r.lastScene["version"] = extui.SceneVersion
	}
	r.lastCompactScene = compactAppSemanticScene(r.lastScene)
	r.directPanelCatalog = nil
	// Command-line prompt, workspace title and menu state may be derived from
	// the new path. Keep the next render, but let its row-free exporter send
	// only those bounded follow-up fields.
	if presentationComplete {
		r.deferSemanticRender = r.nativeCellFrameSuppressed
		r.deferSemanticRenderBound = false
		r.suppressSemanticExport = true
	} else {
		r.deferSemanticRender = false
		r.deferSemanticRenderBound = false
		r.suppressSemanticExport = false
	}
	if r.semanticUpdateOpen {
		r.semanticUpdateHandled = true
	}
	return true
}

// WantsPeriodicRedraw reports that cursor blinking and other idle presentation
// effects are owned by the native host. Actual model changes still reach the
// renderer through vtui's event, task, resize, and explicit redraw paths.
func (r *ExtUiRenderer) WantsPeriodicRedraw() bool {
	return false
}

// VirtualizePanelTableRows reports that the native panel owns presentation.
// Go still keeps the authoritative entries and cursor metrics, but the hidden
// compatibility table need only materialize rows if it is actually painted.
func (r *ExtUiRenderer) VirtualizePanelTableRows() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nativeCellFrameSuppressed
}

// WantsEditorSyntaxFade reports whether the hidden cell-grid editor should
// start its 25 ms fade ticker. Native Qt document surfaces render the final
// syntax colours themselves, so those redraws only compete with input.
func (r *ExtUiRenderer) WantsEditorSyntaxFade() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.nativeSemanticSurfaceEnabled
}

// CanDeferCoveredTerminalRedraw proves that raw shell bytes changed only a
// terminal which the last delivered app scene completely covers with both file
// panels. The terminal parser keeps the new rows in memory; an input/task that
// exposes the terminal still performs an ordinary render and publishes the
// latest state. A negotiated cell fallback is covered too: it paints those
// same panels from the grid and therefore does not expose background PTY rows.
func (r *ExtUiRenderer) CanDeferCoveredTerminalRedraw() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.lastScene == nil {
		return false
	}
	shell, ok := r.lastScene["shell"].(map[string]any)
	if !ok || shell == nil {
		return false
	}
	negotiatedAppSurface := r.nativeCellFrameSuppressed ||
		(r.nativeSemanticSurfaceEnabled &&
			semanticString(r.lastScene["schema"]) == extui.Schema &&
			semanticString(r.lastScene["presentation"]) != "text")
	if !negotiatedAppSurface {
		return false
	}
	_, covered := semanticCoveredTerminalID(shell, shell)
	return covered
}

func (r *ExtUiRenderer) SetPalette(pal *[256]uint32) {
	if pal == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	changed := !r.paletteValid
	if !changed {
		for i, c := range pal {
			if r.palette[i] != c {
				changed = true
				break
			}
		}
	}
	if !changed {
		return
	}

	r.palette = *pal
	r.paletteValid = true
	colors := make([]uint32, len(pal))
	copy(colors, pal[:])
	r.pendingPalette = colors
}

func (r *ExtUiRenderer) SetCursor(x, y int, visible bool, shape vtui.CursorShape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cursorX == x && r.cursorY == y && r.cursorVis == visible && r.cursorShape == shape {
		return
	}
	r.cursorX, r.cursorY = x, y
	r.cursorVis = visible
	r.cursorShape = shape
	r.cursorDirty = true
}
func (r *ExtUiRenderer) SetWindowTitle(title string) {
	r.mu.Lock()
	if r.windowTitleValid && r.lastWindowTitle == title {
		r.mu.Unlock()
		return
	}
	r.lastWindowTitle = title
	r.windowTitleValid = true
	r.mu.Unlock()
	if err := r.send.Send(map[string]any{
		"type":  "title",
		"title": title,
	}); err != nil {
		r.mu.Lock()
		if r.lastWindowTitle == title {
			r.windowTitleValid = false
		}
		r.mu.Unlock()
	}
}

func (r *ExtUiRenderer) Render(buf, shadow []vtui.CharInfo, width, height int, forceRedraw bool) {
	if width <= 0 || height <= 0 || len(buf) == 0 {
		return
	}
	r.mu.Lock()
	if r.nativeCellFrameSuppressed {
		r.pendingFrame = nil
		r.mu.Unlock()
		return
	}
	forceRedraw = forceRedraw || r.forceNextCellFrame
	r.mu.Unlock()

	needsRedraw := forceRedraw
	if !needsRedraw {
		for i := 0; i < width*height && i < len(buf) && i < len(shadow); i++ {
			if buf[i] != shadow[i] {
				needsRedraw = true
				break
			}
		}
	}
	if !needsRedraw {
		return
	}

	limit := width * height
	if limit > len(buf) {
		limit = len(buf)
	}
	cells := make([][3]uint64, 0, limit)
	if forceRedraw || len(shadow) < limit {
		for i := 0; i < limit; i++ {
			c := buf[i]
			cells = append(cells, [3]uint64{uint64(i), c.Char, c.Attributes})
		}
	} else {
		for i := 0; i < limit; i++ {
			c := buf[i]
			if c != shadow[i] {
				cells = append(cells, [3]uint64{uint64(i), c.Char, c.Attributes})
			}
		}
	}

	r.mu.Lock()
	if r.nativeCellFrameSuppressed {
		r.pendingFrame = nil
		r.mu.Unlock()
		return
	}
	r.pendingFrame = map[string]any{
		"type":   "frame",
		"width":  width,
		"height": height,
		"full":   forceRedraw || len(shadow) < limit,
		"cells":  cells,
	}
	r.forceNextCellFrame = false
	r.mu.Unlock()
}

// setNativeCellFrameSuppression mirrors the external host's presentation
// contract at the transport boundary. App-schema scenes are rendered from the
// semantic model unless the user explicitly selected text presentation or the
// active model declares a fallback node. Non-app producers and fallback
// surfaces continue to receive the complete cell protocol unchanged.
//
// The caller holds r.mu.
func (r *ExtUiRenderer) setNativeCellFrameSuppression(scene map[string]any) {
	suppress := r.nativeSemanticSurfaceEnabled && semanticSceneOwnsNativeSurface(scene)
	if r.nativeCellFrameSuppressed && !suppress {
		// The hidden grid may never have received a frame. Force a complete
		// snapshot (and the latest cursor) before revealing it again. Flush
		// keeps the fallback scene behind both messages so QML cannot expose a
		// stale retained texture between independently decoded protocol frames.
		r.forceNextCellFrame = true
		r.cursorDirty = true
		r.fallbackRevealPending = true
	}
	r.nativeCellFrameSuppressed = suppress
	if suppress {
		r.pendingFrame = nil
		r.fallbackRevealPending = false
	} else {
		r.deferSemanticRender = false
		r.deferSemanticRenderBound = false
	}
}

func semanticSceneOwnsNativeSurface(scene map[string]any) bool {
	if scene == nil || semanticString(scene["schema"]) != "app" ||
		semanticString(scene["presentation"]) == "text" {
		return false
	}
	if queue, ok := scene["operationsQueue"].(map[string]any); ok && queue != nil {
		return !semanticContainsFallback(queue)
	}
	shell, hasShell := scene["shell"].(map[string]any)
	if hasShell && shell != nil && extUiAnyBool(shell["fallback"]) {
		return false
	}
	if surface, ok := scene["surface"].(map[string]any); ok && surface != nil {
		return !semanticContainsFallback(surface)
	}
	if hasShell && shell != nil {
		return !semanticContainsFallback(shell)
	}
	return false
}

func semanticContainsFallback(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		kind := semanticString(typed["kind"])
		if extUiAnyBool(typed["fallback"]) || kind == "fallback" || kind == "fallbackWidget" {
			return true
		}
		// Native fallback is explicitly structural. Do not recursively walk
		// catalog entries or text rows on this latency-sensitive decision.
		return semanticContainsFallback(typed["children"])
	case []map[string]any:
		for _, child := range typed {
			if semanticContainsFallback(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if semanticContainsFallback(child) {
				return true
			}
		}
	}
	return false
}

func (r *ExtUiRenderer) queueFullSemanticSceneLocked(scene map[string]any) {
	if scene == nil {
		return
	}
	wire := semanticShallowMapCopy(scene)
	if semanticString(wire["schema"]) == extui.Schema {
		// Coalescing replaces an unsent full scene/patch at the same wire
		// revision. Advancing again would create a gap the strict frontend must
		// reject because it never received the superseded message.
		if r.pendingScene == nil && r.pendingScenePatch == nil {
			r.sceneRevision++
		}
		wire["version"] = extui.SceneVersion
		wire["revision"] = r.sceneRevision
		r.lastCompactScene = compactAppSemanticScene(wire)
	}
	r.pendingScenePatch = nil
	r.pendingScene = wire
}

// SetSemanticSceneIncremental is called before FrameManager considers the
// complete semantic exporter. Its success path never visits file entries.
func (r *ExtUiRenderer) SetSemanticSceneIncremental(ctx *vtui.SemanticContext) bool {
	return r.setSemanticSceneIncremental(ctx, false)
}

// SetSemanticSceneTransition is called inside the input transaction after
// FrameManager has proved that the non-menu frame stack changed. Publishing
// the bounded projection here avoids both Frame.Show and the complete semantic
// exporter for document open/close transitions.
func (r *ExtUiRenderer) SetSemanticSceneTransition(ctx *vtui.SemanticContext) bool {
	return r.setSemanticSceneIncremental(ctx, true)
}

// SetSemanticInputUnchanged handles an explicit proof from an input action
// which only started asynchronous work. No protocol message is necessary: Qt
// already displays this exact revision. Arming the one-render permit prevents
// the hidden panels from being rebuilt while the future completion task is
// opening the requested document.
func (r *ExtUiRenderer) SetSemanticInputUnchanged() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	rejection := ""
	switch {
	case !r.semanticUpdateOpen:
		rejection = "outside_boundary"
	case r.semanticUpdateHandled:
		rejection = "boundary_already_handled"
	case r.semanticUpdateTouched:
		rejection = "boundary_already_touched"
	case r.closed:
		rejection = "closed"
	case r.semanticFastPathUnsafe:
		rejection = "unsafe_boundary"
	case r.sceneRevision == 0 || r.lastScene == nil || r.lastCompactScene == nil:
		rejection = "no_scene_snapshot"
	case r.pendingScene != nil:
		rejection = "pending_scene"
	case r.pendingScenePatch != nil:
		rejection = "pending_scene_patch"
	case r.pendingCommandLine != nil:
		rejection = "pending_command_line"
	case r.pendingPanelCatalog != nil:
		rejection = "pending_panel_catalog"
	case r.pendingPanelActivation != nil:
		rejection = "pending_panel_activation"
	case r.directPanelCatalog != nil:
		rejection = "direct_panel_catalog"
	}
	if rejection != "" {
		navigationBenchmarkUIEvent("input_unchanged.direct_rejected",
			"reason", rejection, "sceneRevision", r.sceneRevision)
		return false
	}
	r.semanticUpdateTouched = true
	r.semanticUpdateHandled = true
	r.suppressSemanticExport = true
	r.deferSemanticRender = r.nativeCellFrameSuppressed
	r.deferSemanticRenderBound = false
	r.panelActivationQueued = false
	r.panelActivationProjected = false
	navigationBenchmarkUIEvent("input_unchanged.direct_accepted",
		"sceneRevision", r.sceneRevision)
	return true
}

func (r *ExtUiRenderer) setSemanticSceneIncremental(ctx *vtui.SemanticContext,
	direct bool,
) bool {
	r.mu.Lock()
	if direct && r.semanticUpdateOpen {
		r.semanticUpdateTouched = true
	}
	rejection := ""
	switch {
	case direct && !r.semanticUpdateOpen:
		rejection = "outside_boundary"
	case direct && r.semanticUpdateHandled:
		rejection = "boundary_already_handled"
	case r.closed:
		rejection = "closed"
	case direct && r.send == nil:
		rejection = "no_sender"
	case direct && r.semanticFastPathUnsafe:
		rejection = "unsafe_boundary"
	case r.sceneRevision == 0:
		rejection = "no_scene_revision"
	case r.lastCompactScene == nil:
		rejection = "no_compact_scene"
	case r.pendingScene != nil:
		rejection = "pending_scene"
	case r.pendingScenePatch != nil:
		rejection = "pending_scene_patch"
	case r.pendingPanelCatalog != nil:
		rejection = "pending_panel_catalog"
	case r.pendingPanelActivation != nil:
		rejection = "pending_panel_activation"
	case r.pendingCommandLine != nil:
		rejection = "pending_command_line"
	case r.directPanelCatalog != nil:
		rejection = "direct_panel_catalog"
	}
	if rejection != "" {
		navigationBenchmarkIncrementalEvent("scene.incremental.rejected",
			"reason", rejection, "sceneRevision", r.sceneRevision)
		r.mu.Unlock()
		return false
	}
	baseRevision := r.sceneRevision
	previous := r.lastCompactScene
	r.mu.Unlock()

	current, supported := BuildAppIncrementalScene(ctx)
	if !supported {
		navigationBenchmarkIncrementalEvent("scene.incremental.rejected",
			"reason", "unsupported_projection", "sceneRevision", baseRevision)
		return false
	}
	patch, acknowledgements, valid := buildAppScenePatch(previous, current)
	if !valid {
		navigationBenchmarkIncrementalEvent("scene.incremental.rejected",
			"reason", "invalid_patch", "sceneRevision", baseRevision)
		return false
	}
	empty := scenePatchEmpty(patch)

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.sceneRevision != baseRevision || r.pendingScene != nil ||
		r.pendingScenePatch != nil || r.pendingPanelCatalog != nil ||
		r.pendingPanelActivation != nil || r.pendingCommandLine != nil ||
		r.directPanelCatalog != nil ||
		(direct && (!r.semanticUpdateOpen || r.semanticUpdateHandled ||
			r.send == nil || r.semanticFastPathUnsafe)) {
		navigationBenchmarkIncrementalEvent("scene.incremental.rejected",
			"reason", "state_changed_during_projection",
			"baseRevision", baseRevision, "sceneRevision", r.sceneRevision)
		return false
	}
	armDirectResult := func() {
		r.semanticFastPathUnsafe = false
		r.suppressSemanticExport = true
		r.deferSemanticRender = r.nativeCellFrameSuppressed
		r.deferSemanticRenderBound = false
		r.semanticUpdateHandled = true
		r.panelActivationQueued = false
		r.panelActivationProjected = false
	}
	if !direct {
		// The bounded projection is a complete authoritative reconciliation of
		// every native app field (catalog rows are guarded by their revisions).
		// Therefore it closes the same one-render uncertainty as SetSemanticScene:
		// an unrelated input/task in the preceding boundary must not leave direct
		// menu or activation paths permanently disabled after this proof succeeds.
		r.semanticFastPathUnsafe = false
		r.suppressSemanticExport = false
		r.deferSemanticRender = false
		r.deferSemanticRenderBound = false
		r.panelActivationQueued = false
		r.panelActivationProjected = false
	}
	if empty {
		if direct {
			armDirectResult()
			navigationBenchmarkIncrementalEvent("scene.transition.direct_accepted",
				"result", "unchanged", "sceneRevision", r.sceneRevision)
		}
		return true
	}
	patch.BaseRevision = baseRevision
	patch.Revision = baseRevision + 1
	wire := patch.ToMap()
	if direct {
		if trace := navigationBenchmarkCurrentOrPublishedTrace(); trace != nil {
			wire["benchmarkTraceId"] = trace.id
		}
		benchmark := navigationBenchmarkPrepareImmediateMessage(wire)
		if err := r.send.SendWithBenchmark(wire, benchmark); err != nil {
			vtui.DebugLog("EXTUI_RENDERER: direct scene transition send failed: %v", err)
			r.closed = true
			r.suppressSemanticExport = false
			r.deferSemanticRender = false
			return false
		}
	}
	r.sceneRevision = patch.Revision
	if !direct {
		r.pendingScenePatch = wire
	}
	r.lastCompactScene = current.Scene
	r.lastScene = semanticSceneStructuralMapCopy(r.lastScene)
	applyAppScenePatchToSnapshot(r.lastScene, patch)
	if r.lastScene != nil {
		r.lastScene["version"] = extui.SceneVersion
	}
	for _, acknowledgement := range acknowledgements {
		acknowledgement.panel.acknowledgeSemanticSelection(acknowledgement.revision)
	}
	if r.lastScene != nil {
		r.setNativeCellFrameSuppression(r.lastScene)
	}
	if direct {
		armDirectResult()
		navigationBenchmarkIncrementalEvent("scene.transition.direct_accepted",
			"result", "sent", "baseRevision", patch.BaseRevision,
			"revision", patch.Revision)
	}
	return true
}

func (r *ExtUiRenderer) SetSemanticScene(scene map[string]any) {
	scene = semanticNormalizeSparsePanelCatalogs(scene)
	benchmark := navigationBenchmarkSceneCompareBegin(scene)
	compareResult := "full_scene"
	if benchmark != nil {
		defer func() {
			navigationBenchmarkSceneCompareEnd(benchmark, compareResult)
		}()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.semanticUpdateOpen {
		r.semanticUpdateTouched = true
	}
	r.setNativeCellFrameSuppression(scene)
	r.suppressSemanticExport = false
	r.deferSemanticRender = false
	r.deferSemanticRenderBound = false
	r.semanticFastPathUnsafe = false
	queuedActivationSide := r.queuedPanelActivationSide
	activationQueued := r.panelActivationQueued
	r.panelActivationQueued = false

	// A plain Tab changes only shell.activePanel and the two panel.active
	// flags. PanelsFrame explicitly marks that transition; validate the fresh
	// authoritative export against the last delivered scene before replacing
	// a megabyte-scale catalog with this revisioned acknowledgement.
	if activationQueued && r.pendingScene == nil &&
		r.pendingCommandLine == nil &&
		semanticSceneHasPanelActivation(scene, queuedActivationSide) &&
		semanticScenesEqualExceptPanelActivation(r.lastScene, scene) {
		previousSide, previousOK := semanticSceneActivePanel(r.lastScene)
		if previousOK && previousSide != queuedActivationSide {
			compareResult = "panel_activation_patch"
			revision := uint64(0)
			if r.pendingPanelActivation != nil {
				revision = uint64(extUiAnyInt(r.pendingPanelActivation["revision"]))
			}
			if revision == 0 {
				r.nextPanelActivationRevision++
				revision = r.nextPanelActivationRevision
			}
			patch := map[string]any{
				"type":        "panel_activation",
				"activePanel": queuedActivationSide,
				"revision":    revision,
			}
			// Preserve benchmark correlation without making the production patch
			// carry any catalog or presentation data.
			if traceID, present := scene["benchmarkTraceId"]; present {
				patch["benchmarkTraceId"] = traceID
			}
			if benchmarkMeta, present := scene["benchmark"]; present {
				patch["benchmark"] = benchmarkMeta
			}
			r.pendingPanelCatalog = nil
			r.pendingPanelCatalogScene = nil
			r.pendingPanelActivation = patch
			r.pendingPanelActivationScene = scene
			return
		}
	}
	// A direct catalog is already visible in the client while lastScene still
	// describes the pre-navigation state. Skip the generic whole-scene walk in
	// that case: the strict catalog/remainder proof below both adopts an exact
	// projection and forces a full correction if the UI mutation rolled back.
	if r.directPanelCatalog == nil && semanticScenesEqual(scene, r.lastScene) {
		compareResult = "equal_last"
		// Benchmark correlation belongs to the transport observation, not to the
		// client's logical scene. Keep the newest authoritative snapshot so a
		// trace-id or scene-sequence annotation can never accumulate stale state.
		r.lastScene = scene
		r.panelActivationProjected = false
		// A transient full scene or command-line patch returned to the state the
		// native client already owns before Flush.
		r.pendingScene = nil
		r.pendingCommandLine = nil
		r.pendingCommandLineScene = nil
		r.pendingPanelCatalog = nil
		r.pendingPanelCatalogScene = nil
		r.pendingPanelActivation = nil
		r.pendingPanelActivationScene = nil
		return
	}
	// QueuePanelActivationState is intentionally captured before Show(), so its
	// prompt and edit model are authoritative while the cell-derived runs still
	// describe the preceding render. The immediate native patch already carries
	// every interactive value. Once the next export proves that the rest of the
	// scene is unchanged, adopt its rendered command-line snapshot without a
	// redundant full scene or follow-up patch.
	if r.panelActivationProjected {
		if semanticScenesEqualExceptCommandLineRendering(r.lastScene, scene) {
			compareResult = "equal_projected_panel_activation"
			r.lastScene = scene
			r.panelActivationProjected = false
			r.pendingScene = nil
			r.pendingCommandLine = nil
			r.pendingCommandLineScene = nil
			r.pendingPanelCatalog = nil
			r.pendingPanelCatalogScene = nil
			r.pendingPanelActivation = nil
			r.pendingPanelActivationScene = nil
			return
		}
		semanticPanelActivationTraceRejection(r.lastScene, scene)
		r.panelActivationProjected = false
	}
	if semanticScenesEqual(scene, r.pendingScene) {
		compareResult = "equal_pending_scene"
		return
	}
	if semanticScenesEqual(scene, r.pendingCommandLineScene) {
		compareResult = "equal_pending_command_line_scene"
		return
	}
	if semanticScenesEqual(scene, r.pendingPanelCatalogScene) {
		compareResult = "equal_pending_panel_catalog_scene"
		return
	}
	if semanticScenesEqual(scene, r.pendingPanelActivationScene) {
		compareResult = "equal_pending_panel_activation_scene"
		return
	}

	// A directory transition changes one authoritative minimal panel catalog,
	// while the other panel and the rest of the application stay untouched.
	// Transport that one panel (plus the few small values derived from its
	// path) instead of serializing the complete scene and its legacy aliases.
	// The diff routine is intentionally strict: an unknown or simultaneous
	// state change makes this fall through to the complete-scene protocol.
	if r.pendingScene == nil {
		directProjectionPending := r.directPanelCatalog != nil
		if patch, ok := semanticPanelCatalogPatch(r.lastScene, scene); ok {
			if traceID, present := scene["benchmarkTraceId"]; present {
				patch["benchmarkTraceId"] = traceID
			}
			if benchmarkMeta, present := scene["benchmark"]; present {
				patch["benchmark"] = benchmarkMeta
			}
			if r.directPanelCatalog != nil {
				chrome, matches := semanticDirectPanelCatalogRemainder(
					r.directPanelCatalog, patch)
				if !matches && navigationBenchmarkIsEnabled() {
					navigationBenchmarkRenderEvent("scene.panel_catalog.direct_mismatch",
						"firstDifference", semanticFirstDifferencePath(
							r.directPanelCatalog, patch, "$"))
				}
				r.directPanelCatalog = nil
				if matches {
					r.pendingScene = nil
					r.pendingPanelCatalog = nil
					r.pendingPanelCatalogScene = nil
					r.pendingPanelActivation = nil
					r.pendingPanelActivationScene = nil
					if chrome == nil {
						compareResult = "equal_direct_panel_catalog"
						r.pendingCommandLine = nil
						r.pendingCommandLineScene = nil
						r.lastScene = scene
						return
					}
					compareResult = "direct_panel_catalog_chrome"
					r.pendingCommandLine = chrome
					r.pendingCommandLineScene = scene
					return
				}
				// The immediate catalog was visible, but the authoritative scene
				// contains another simultaneous mutation. Correct it immediately
				// instead of allowing a smaller unrelated fast path to adopt a
				// scene which no longer describes what the client displays.
				compareResult = "direct_panel_catalog_correction"
				r.pendingCommandLine = nil
				r.pendingCommandLineScene = nil
				r.pendingPanelCatalog = nil
				r.pendingPanelCatalogScene = nil
				r.pendingPanelActivation = nil
				r.pendingPanelActivationScene = nil
				r.queueFullSemanticSceneLocked(scene)
				return
			} else {
				compareResult = "panel_catalog_patch"
				r.pendingCommandLine = nil
				r.pendingCommandLineScene = nil
				r.pendingPanelActivation = nil
				r.pendingPanelActivationScene = nil
				r.pendingPanelCatalog = patch
				r.pendingPanelCatalogScene = scene
				return
			}
		}
		r.directPanelCatalog = nil
		semanticPanelCatalogTraceRejection(r.lastScene, scene)
		if directProjectionPending {
			// The input-direct projection rolled back or no longer satisfies the
			// catalog-only contract. The client has already painted it, so only a
			// full authoritative scene can safely restore the actual state.
			compareResult = "direct_panel_catalog_correction"
			r.pendingCommandLine = nil
			r.pendingCommandLineScene = nil
			r.pendingPanelCatalog = nil
			r.pendingPanelCatalogScene = nil
			r.pendingPanelActivation = nil
			r.pendingPanelActivationScene = nil
			r.queueFullSemanticSceneLocked(scene)
			return
		}
	}

	// Typing used to serialize and decode the complete panel catalogs for
	// every character. The command line is the only changing subtree in the
	// common case, so retain the authoritative Go model while transporting a
	// tiny patch to the native frontend. A pending full scene cannot be based
	// on the client's last scene and therefore deliberately stays full.
	if r.pendingScene == nil && semanticScenesEqualExceptCommandLine(r.lastScene, scene) {
		if commandLine, ok := semanticSceneCommandLine(scene); ok {
			compareResult = "command_line_patch"
			r.pendingPanelCatalog = nil
			r.pendingPanelCatalogScene = nil
			r.pendingPanelActivation = nil
			r.pendingPanelActivationScene = nil
			r.pendingCommandLine = map[string]any{
				"type":        "command_line",
				"commandLine": commandLine,
			}
			if menus, present := scene["menus"]; present {
				r.pendingCommandLine["menus"] = menus
			}
			r.pendingCommandLineScene = scene
			return
		}
	}
	r.pendingCommandLine = nil
	r.pendingCommandLineScene = nil
	r.pendingPanelCatalog = nil
	r.pendingPanelCatalogScene = nil
	r.pendingPanelActivation = nil
	r.pendingPanelActivationScene = nil
	r.directPanelCatalog = nil
	r.queueFullSemanticSceneLocked(scene)
}

var semanticPanelCatalogMutableKeys = map[string]struct{}{
	"path": {}, "title": {}, "catalogRevision": {}, "selectionRevision": {},
	"cursorEntryId": {}, "cursor": {}, "loading": {}, "catalogProvisional": {},
	"catalogRowsDeferred": {},
	"fastFind":            {}, "fastFindText": {}, "fastFindMatchColor": {},
	"fastFindMatches": {}, "selectedCount": {}, "totalCount": {},
	"metadataRevision": {}, "entries": {}, "highlightStyles": {},
}

var semanticCommandLineNavigationKeys = map[string]struct{}{
	"prompt": {}, "promptRuns": {}, "runs": {}, "inputX": {},
	"cursorPrefixRuns": {}, "cursorX": {},
}

type semanticPanelCatalogDiff struct {
	activePanel       int
	activeScreen      int
	side              int
	panelID           string
	shellID           string
	commandLineID     string
	hiddenTerminalID  string
	panel             map[string]any
	commandLine       any
	commandLineDiff   bool
	shellTitle        any
	shellTitleDiff    bool
	workspaceTabs     any
	workspaceTabsDiff bool
	menus             any
	menusDiff         bool
}

var semanticHiddenTerminalPresentationKeys = map[string]struct{}{
	"cursorX": {}, "cursorY": {}, "cursorAbsoluteRow": {},
	"cursorVisible": {}, "rows": {}, "windowStart": {}, "windowEnd": {},
	"viewportStart": {}, "viewportSpan": {}, "contentExtent": {},
	"viewportRow": {}, "viewportRows": {}, "windowGeneration": {},
	"windowContentKey": {}, "windowRows": {},
}

// semanticHiddenTerminalPresentationTransition recognizes the narrow PTY
// mutation produced by synchronizing the background shell after panel
// navigation. The terminal is completely covered in this state, but the
// private cd command advances its cursor and appends prompt rows. Every
// logical/non-presentation field must remain byte-equivalent; mode, busy,
// alt-screen, title, visibility, and focus changes therefore still force an
// authoritative full scene. Revealing either side also fails the guards below
// and publishes the latest rows before the terminal can become visible.
func semanticCoveredTerminalID(previousShell, currentShell map[string]any) (string, bool) {
	for _, shell := range []map[string]any{previousShell, currentShell} {
		if shell == nil || semanticString(shell["mode"]) != "panels" ||
			!extUiAnyBool(shell["showPanels"]) ||
			!extUiAnyBool(shell["showLeftPanel"]) ||
			!extUiAnyBool(shell["showRightPanel"]) ||
			extUiAnyBool(shell["wide"]) {
			return "", false
		}
		terminalActive, present := shell["terminalActive"].(bool)
		if !present || terminalActive {
			return "", false
		}
		// A shortened panel reveals the terminal beneath it, so terminal rows are
		// no longer a hidden presentation detail and must not be deferred.
		if layout, ok := shell["panelLayout"].(map[string]any); ok {
			if semanticInt(layout["leftBottomInsetRows"]) != 0 ||
				semanticInt(layout["rightBottomInsetRows"]) != 0 {
				return "", false
			}
		}
	}
	previousTerminal, previousOK := previousShell["terminal"].(map[string]any)
	currentTerminal, currentOK := currentShell["terminal"].(map[string]any)
	if !previousOK || !currentOK || previousTerminal == nil || currentTerminal == nil {
		return "", false
	}
	terminalID := semanticString(previousTerminal["id"])
	if terminalID == "" || terminalID != semanticString(currentTerminal["id"]) {
		return "", false
	}
	return terminalID, true
}

func semanticHiddenTerminalPresentationTransition(previousShell, currentShell map[string]any) (string, bool) {
	terminalID, ok := semanticCoveredTerminalID(previousShell, currentShell)
	if !ok {
		return "", false
	}
	previousTerminal := previousShell["terminal"].(map[string]any)
	currentTerminal := currentShell["terminal"].(map[string]any)
	if !semanticMapEqualExceptKeys(previousTerminal, currentTerminal, semanticHiddenTerminalPresentationKeys) {
		return "", false
	}
	return terminalID, true
}

// semanticPanelCatalogPatch returns a lossless native-scene patch only for a
// single deferred file-panel transition. Legacy aliases are checked below as
// well, but are not copied onto the wire because they mirror the typed model.
func semanticPanelCatalogPatch(previous, current map[string]any) (map[string]any, bool) {
	if previous == nil || current == nil {
		return nil, false
	}
	previousShell, previousOK := previous["shell"].(map[string]any)
	currentShell, currentOK := current["shell"].(map[string]any)
	if !previousOK || !currentOK || previousShell == nil || currentShell == nil {
		return nil, false
	}
	previousActive, previousActiveOK := semanticSceneActivePanel(previous)
	currentActive, currentActiveOK := semanticSceneActivePanel(current)
	if !previousActiveOK || !currentActiveOK || previousActive != currentActive {
		return nil, false
	}
	previousPanels, previousOK := semanticScenePanelMaps(previous)
	currentPanels, currentOK := semanticScenePanelMaps(current)
	if !previousOK || !currentOK || len(previousPanels) != len(currentPanels) {
		return nil, false
	}

	changedIndex := -1
	for index := range previousPanels {
		if reflect.DeepEqual(previousPanels[index], currentPanels[index]) {
			continue
		}
		if changedIndex >= 0 || !semanticPanelCatalogTransitionSafe(previousPanels[index], currentPanels[index], index) {
			return nil, false
		}
		changedIndex = index
	}
	if changedIndex < 0 {
		return nil, false
	}
	changedPanel := currentPanels[changedIndex]
	changedSide := changedIndex
	if _, present := changedPanel["side"]; present {
		changedSide = extUiAnyInt(changedPanel["side"])
	}

	previousCommandLine, previousCommandLinePresent := previousShell["commandLine"]
	currentCommandLine, currentCommandLinePresent := currentShell["commandLine"]
	commandLineDiff := previousCommandLinePresent != currentCommandLinePresent ||
		!reflect.DeepEqual(previousCommandLine, currentCommandLine)
	if commandLineDiff {
		previousMap, previousMapOK := previousCommandLine.(map[string]any)
		currentMap, currentMapOK := currentCommandLine.(map[string]any)
		if !previousMapOK || !currentMapOK ||
			!semanticMapEqualExceptKeys(previousMap, currentMap, semanticCommandLineNavigationKeys) {
			return nil, false
		}
	}

	previousTitle, previousTitlePresent := previousShell["title"]
	currentTitle, currentTitlePresent := currentShell["title"]
	shellTitleDiff := previousTitlePresent != currentTitlePresent || !reflect.DeepEqual(previousTitle, currentTitle)
	workspaceTabsDiff := !reflect.DeepEqual(previous["workspaceTabs"], current["workspaceTabs"])
	if workspaceTabsDiff && !semanticWorkspaceTabsNavigationEquivalent(
		previous["workspaceTabs"], current["workspaceTabs"], extUiAnyInt(current["activeScreen"]), changedSide) {
		return nil, false
	}
	// Only the active panel contributes the shell title, prompt and workspace
	// title. Seeing any of those derived changes for an inactive panel signals
	// that this was not a plain catalog transition.
	if changedSide != currentActive && (commandLineDiff || shellTitleDiff || workspaceTabsDiff) {
		return nil, false
	}

	menusDiff := !reflect.DeepEqual(previous["menus"], current["menus"])
	if menusDiff {
		if !commandLineDiff || !reflect.DeepEqual(
			semanticNonAutocompleteMenus(previous["menus"]),
			semanticNonAutocompleteMenus(current["menus"])) {
			return nil, false
		}
	}

	shellIgnored := map[string]struct{}{
		"panels": {}, "commandLine": {}, "title": {},
	}
	hiddenTerminalID, hiddenTerminalPresentationOnly := semanticHiddenTerminalPresentationTransition(
		previousShell, currentShell)
	if hiddenTerminalPresentationOnly {
		// The helper above already proved every visible/logical terminal field equal.
		shellIgnored["terminal"] = struct{}{}
	}
	if !semanticMapEqualExceptKeys(previousShell, currentShell, shellIgnored) {
		return nil, false
	}
	if !semanticMapEqualExceptKeys(previous, current, map[string]struct{}{
		"shell": {}, "workspaceTabs": {}, "menus": {}, "legacy": {},
		"frames": {}, "screens": {}, "benchmarkTraceId": {}, "benchmark": {},
	}) {
		return nil, false
	}

	diff := semanticPanelCatalogDiff{
		activePanel:       currentActive,
		activeScreen:      extUiAnyInt(current["activeScreen"]),
		side:              changedSide,
		panelID:           semanticString(changedPanel["id"]),
		shellID:           semanticString(currentShell["id"]),
		hiddenTerminalID:  hiddenTerminalID,
		panel:             changedPanel,
		commandLine:       currentCommandLine,
		commandLineDiff:   commandLineDiff,
		shellTitle:        currentTitle,
		shellTitleDiff:    shellTitleDiff,
		workspaceTabs:     current["workspaceTabs"],
		workspaceTabsDiff: workspaceTabsDiff,
		menus:             current["menus"],
		menusDiff:         menusDiff,
	}
	if commandLine, ok := currentCommandLine.(map[string]any); ok {
		diff.commandLineID = semanticString(commandLine["id"])
	}
	if !semanticPanelCatalogLegacyEquivalent(previous, current, diff) {
		return nil, false
	}

	patch := map[string]any{
		"type":        "panel_catalog",
		"activePanel": diff.activePanel,
		"side":        diff.side,
		"panel":       diff.panel,
	}
	if diff.commandLineDiff {
		patch["commandLine"] = diff.commandLine
	}
	if diff.shellTitleDiff {
		patch["shellTitle"] = diff.shellTitle
	}
	if diff.workspaceTabsDiff {
		patch["workspaceTabs"] = diff.workspaceTabs
	}
	if diff.menusDiff {
		if diff.menus == nil {
			patch["menus"] = []map[string]any{}
		} else {
			patch["menus"] = diff.menus
		}
	}
	return patch, true
}

var semanticPanelChromeKeys = map[string]struct{}{
	"commandLine": {}, "shellTitle": {}, "workspaceTabs": {}, "menus": {},
}

// semanticDirectPanelCatalogRemainder validates the later strict scene diff
// against the catalog that was already sent directly from the Go mutation.
// Only bounded chrome fields may remain; catalogs are never retransmitted.
func semanticDirectPanelCatalogRemainder(delivered, authoritative map[string]any) (map[string]any, bool) {
	if delivered == nil || authoritative == nil ||
		semanticString(delivered["type"]) != "panel_catalog" ||
		semanticString(authoritative["type"]) != "panel_catalog" ||
		extUiAnyInt(delivered["activePanel"]) != extUiAnyInt(authoritative["activePanel"]) ||
		extUiAnyInt(delivered["side"]) != extUiAnyInt(authoritative["side"]) ||
		!reflect.DeepEqual(delivered["panel"], authoritative["panel"]) {
		return nil, false
	}
	for key := range semanticPanelChromeKeys {
		if value, present := delivered[key]; present &&
			(!reflect.DeepEqual(value, authoritative[key]) || authoritative[key] == nil) {
			return nil, false
		}
	}
	chrome := map[string]any{
		"type":        "panel_chrome",
		"activePanel": authoritative["activePanel"],
	}
	hasChrome := false
	for key := range semanticPanelChromeKeys {
		value, present := authoritative[key]
		if !present || reflect.DeepEqual(value, delivered[key]) {
			continue
		}
		chrome[key] = value
		hasChrome = true
	}
	if !hasChrome {
		return nil, true
	}
	for key, value := range authoritative {
		if strings.HasPrefix(key, "benchmark") {
			chrome[key] = value
		}
	}
	return chrome, true
}

// semanticPanelCatalogTraceRejection explains a conservative fast-path
// fallback only while the opt-in navigation trace is active. It deliberately
// stays off the production hot path: recursive first-difference discovery can
// walk large legacy aliases, but is invaluable when a real scene contains a
// derived field that a synthetic transport test did not model.
func semanticPanelCatalogTraceRejection(previous, current map[string]any) {
	if !navigationBenchmarkIsEnabled() {
		return
	}
	reason, path, relevant := semanticPanelCatalogRejection(previous, current)
	if !relevant {
		return
	}
	fields := []any{"reason", reason}
	if path != "" {
		fields = append(fields, "firstDifference", path)
	}
	navigationBenchmarkRenderEvent("scene.panel_catalog.rejected", fields...)
}

func semanticPanelCatalogRejection(previous, current map[string]any) (string, string, bool) {
	if previous == nil || current == nil {
		return "missing_scene", "$", false
	}
	previousShell, previousOK := previous["shell"].(map[string]any)
	currentShell, currentOK := current["shell"].(map[string]any)
	if !previousOK || !currentOK || previousShell == nil || currentShell == nil {
		return "missing_shell", "$.shell", false
	}
	previousPanels, previousOK := semanticScenePanelMaps(previous)
	currentPanels, currentOK := semanticScenePanelMaps(current)
	if !previousOK || !currentOK || len(previousPanels) != len(currentPanels) {
		return "panel_shape", "$.shell.panels", true
	}
	changed := make([]int, 0, len(previousPanels))
	for index := range previousPanels {
		if !reflect.DeepEqual(previousPanels[index], currentPanels[index]) {
			changed = append(changed, index)
		}
	}
	if len(changed) == 0 {
		return "no_panel_change", "", false
	}
	if len(changed) != 1 {
		return "multiple_panels_changed", "$.shell.panels", true
	}
	index := changed[0]
	previousPanel, currentPanel := previousPanels[index], currentPanels[index]
	panelPath := fmt.Sprintf("$.shell.panels[%d]", index)
	if semanticString(previousPanel["kind"]) != "filePanel" ||
		semanticString(currentPanel["kind"]) != "filePanel" ||
		semanticString(previousPanel["id"]) == "" ||
		semanticString(previousPanel["id"]) != semanticString(currentPanel["id"]) {
		return "panel_identity", panelPath, true
	}
	if previousPanel["metadataDeferred"] != true || currentPanel["metadataDeferred"] != true {
		return "panel_not_deferred", panelPath + ".metadataDeferred", true
	}
	previousSide, currentSide := index, index
	if _, present := previousPanel["side"]; present {
		previousSide = extUiAnyInt(previousPanel["side"])
	}
	if _, present := currentPanel["side"]; present {
		currentSide = extUiAnyInt(currentPanel["side"])
	}
	if previousSide != currentSide || previousSide < 0 || previousSide > 1 {
		return "panel_side", panelPath + ".side", true
	}
	previousStable := semanticMapWithoutKeys(previousPanel, semanticPanelCatalogMutableKeys)
	currentStable := semanticMapWithoutKeys(currentPanel, semanticPanelCatalogMutableKeys)
	if !reflect.DeepEqual(previousStable, currentStable) {
		return "panel_non_catalog_field", semanticFirstDifferencePath(previousStable, currentStable, panelPath), true
	}
	if !reflect.DeepEqual(previousPanel["entries"], currentPanel["entries"]) &&
		extUiAnyInt(previousPanel["catalogRevision"]) == extUiAnyInt(currentPanel["catalogRevision"]) &&
		!semanticPanelEntriesEqualExceptSelection(previousPanel["entries"], currentPanel["entries"]) {
		return "entries_without_catalog_revision", panelPath + ".entries", true
	}

	previousActive, previousActiveOK := semanticSceneActivePanel(previous)
	currentActive, currentActiveOK := semanticSceneActivePanel(current)
	if !previousActiveOK || !currentActiveOK || previousActive != currentActive {
		return "active_panel_changed", "$.shell.activePanel", true
	}
	previousCommandLine, previousCommandLinePresent := previousShell["commandLine"]
	currentCommandLine, currentCommandLinePresent := currentShell["commandLine"]
	commandLineDiff := previousCommandLinePresent != currentCommandLinePresent ||
		!reflect.DeepEqual(previousCommandLine, currentCommandLine)
	if commandLineDiff {
		previousMap, previousMapOK := previousCommandLine.(map[string]any)
		currentMap, currentMapOK := currentCommandLine.(map[string]any)
		if !previousMapOK || !currentMapOK {
			return "command_line_shape", "$.shell.commandLine", true
		}
		previousStable = semanticMapWithoutKeys(previousMap, semanticCommandLineNavigationKeys)
		currentStable = semanticMapWithoutKeys(currentMap, semanticCommandLineNavigationKeys)
		if !reflect.DeepEqual(previousStable, currentStable) {
			return "command_line_non_navigation_field",
				semanticFirstDifferencePath(previousStable, currentStable, "$.shell.commandLine"), true
		}
	}
	previousTitle, previousTitlePresent := previousShell["title"]
	currentTitle, currentTitlePresent := currentShell["title"]
	shellTitleDiff := previousTitlePresent != currentTitlePresent || !reflect.DeepEqual(previousTitle, currentTitle)
	workspaceTabsDiff := !reflect.DeepEqual(previous["workspaceTabs"], current["workspaceTabs"])
	if workspaceTabsDiff && !semanticWorkspaceTabsNavigationEquivalent(
		previous["workspaceTabs"], current["workspaceTabs"], extUiAnyInt(current["activeScreen"]), currentSide) {
		return "workspace_tabs_non_navigation_field",
			semanticFirstDifferencePath(previous["workspaceTabs"], current["workspaceTabs"], "$.workspaceTabs"), true
	}
	if currentSide != currentActive && (commandLineDiff || shellTitleDiff || workspaceTabsDiff) {
		return "inactive_panel_changed_active_derivation", panelPath, true
	}
	menusDiff := !reflect.DeepEqual(previous["menus"], current["menus"])
	if menusDiff && (!commandLineDiff || !reflect.DeepEqual(
		semanticNonAutocompleteMenus(previous["menus"]),
		semanticNonAutocompleteMenus(current["menus"]))) {
		return "menus_non_autocomplete_field",
			semanticFirstDifferencePath(previous["menus"], current["menus"], "$.menus"), true
	}
	previousStable = semanticMapWithoutKeys(previousShell, map[string]struct{}{
		"panels": {}, "commandLine": {}, "title": {},
	})
	currentStable = semanticMapWithoutKeys(currentShell, map[string]struct{}{
		"panels": {}, "commandLine": {}, "title": {},
	})
	if !reflect.DeepEqual(previousStable, currentStable) {
		return "shell_non_catalog_field",
			semanticFirstDifferencePath(previousStable, currentStable, "$.shell"), true
	}
	previousStable = semanticMapWithoutKeys(previous, map[string]struct{}{
		"shell": {}, "workspaceTabs": {}, "menus": {}, "legacy": {},
		"frames": {}, "screens": {}, "benchmarkTraceId": {}, "benchmark": {},
	})
	currentStable = semanticMapWithoutKeys(current, map[string]struct{}{
		"shell": {}, "workspaceTabs": {}, "menus": {}, "legacy": {},
		"frames": {}, "screens": {}, "benchmarkTraceId": {}, "benchmark": {},
	})
	if !reflect.DeepEqual(previousStable, currentStable) {
		return "root_non_catalog_field",
			semanticFirstDifferencePath(previousStable, currentStable, "$"), true
	}

	diff := semanticPanelCatalogDiff{
		activePanel:       currentActive,
		activeScreen:      extUiAnyInt(current["activeScreen"]),
		side:              currentSide,
		panelID:           semanticString(currentPanel["id"]),
		shellID:           semanticString(currentShell["id"]),
		commandLine:       currentCommandLine,
		commandLineDiff:   commandLineDiff,
		shellTitle:        currentTitle,
		shellTitleDiff:    shellTitleDiff,
		workspaceTabs:     current["workspaceTabs"],
		workspaceTabsDiff: workspaceTabsDiff,
		menus:             current["menus"],
		menusDiff:         menusDiff,
	}
	if commandLine, ok := currentCommandLine.(map[string]any); ok {
		diff.commandLineID = semanticString(commandLine["id"])
	}
	for _, key := range []string{"legacy", "frames", "screens"} {
		previousAlias := semanticScrubPanelCatalogAlias(previous[key], diff)
		currentAlias := semanticScrubPanelCatalogAlias(current[key], diff)
		if !reflect.DeepEqual(previousAlias, currentAlias) {
			return "legacy_alias_" + key,
				semanticFirstDifferencePath(previousAlias, currentAlias, "$."+key), true
		}
	}
	return "unknown", "", true
}

func semanticFirstDifferencePath(previous, current any, path string) string {
	if reflect.DeepEqual(previous, current) {
		return ""
	}
	previousMap, previousMapOK := previous.(map[string]any)
	currentMap, currentMapOK := current.(map[string]any)
	if previousMapOK && currentMapOK {
		keys := make(map[string]struct{}, len(previousMap)+len(currentMap))
		for key := range previousMap {
			keys[key] = struct{}{}
		}
		for key := range currentMap {
			keys[key] = struct{}{}
		}
		ordered := make([]string, 0, len(keys))
		for key := range keys {
			ordered = append(ordered, key)
		}
		sort.Strings(ordered)
		for _, key := range ordered {
			previousValue, previousPresent := previousMap[key]
			currentValue, currentPresent := currentMap[key]
			childPath := path + "." + key
			if !previousPresent || !currentPresent {
				return childPath
			}
			if difference := semanticFirstDifferencePath(previousValue, currentValue, childPath); difference != "" {
				return difference
			}
		}
		return path
	}
	previousSlice, previousSliceOK := semanticAnySlice(previous)
	currentSlice, currentSliceOK := semanticAnySlice(current)
	if previousSliceOK && currentSliceOK {
		limit := len(previousSlice)
		if len(currentSlice) < limit {
			limit = len(currentSlice)
		}
		for index := 0; index < limit; index++ {
			childPath := fmt.Sprintf("%s[%d]", path, index)
			if difference := semanticFirstDifferencePath(previousSlice[index], currentSlice[index], childPath); difference != "" {
				return difference
			}
		}
		if len(previousSlice) != len(currentSlice) {
			return path + ".length"
		}
	}
	return path
}

func semanticAnySlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	case []map[string]any:
		out := make([]any, len(typed))
		for index := range typed {
			out[index] = typed[index]
		}
		return out, true
	default:
		return nil, false
	}
}

func semanticPanelCatalogTransitionSafe(previous, current map[string]any, index int) bool {
	if semanticString(previous["kind"]) != "filePanel" || semanticString(current["kind"]) != "filePanel" ||
		semanticString(previous["id"]) == "" || semanticString(previous["id"]) != semanticString(current["id"]) ||
		previous["metadataDeferred"] != true || current["metadataDeferred"] != true {
		return false
	}
	previousSide, currentSide := index, index
	if _, present := previous["side"]; present {
		previousSide = extUiAnyInt(previous["side"])
	}
	if _, present := current["side"]; present {
		currentSide = extUiAnyInt(current["side"])
	}
	if previousSide != currentSide || previousSide < 0 || previousSide > 1 {
		return false
	}
	previousCatalogRevision := semanticInt64(previous["catalogRevision"])
	currentCatalogRevision := semanticInt64(current["catalogRevision"])
	if currentCatalogRevision != previousCatalogRevision &&
		currentCatalogRevision <= previousCatalogRevision {
		return false
	}
	if !semanticMapEqualExceptKeys(previous, current, semanticPanelCatalogMutableKeys) {
		return false
	}
	// A base-entry rewrite without a catalog revision would make an in-flight
	// metadata request ambiguous. Selection-only changes are the sole exception.
	if !reflect.DeepEqual(previous["entries"], current["entries"]) &&
		previousCatalogRevision == currentCatalogRevision &&
		!semanticPanelEntriesEqualExceptSelection(previous["entries"], current["entries"]) {
		return false
	}
	return true
}

func semanticPanelEntriesEqualExceptSelection(previous, current any) bool {
	previousEntries, previousOK := semanticMapSlice(previous)
	currentEntries, currentOK := semanticMapSlice(current)
	if !previousOK || !currentOK || len(previousEntries) != len(currentEntries) {
		return false
	}
	for index := range previousEntries {
		if !semanticMapEqualExceptKeys(previousEntries[index], currentEntries[index], map[string]struct{}{"selected": {}}) {
			return false
		}
	}
	return true
}

func semanticMapSlice(value any) ([]map[string]any, bool) {
	switch typed := value.(type) {
	case []map[string]any:
		return typed, true
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			entry, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			result = append(result, entry)
		}
		return result, true
	default:
		return nil, false
	}
}

func semanticMapEqualExceptKeys(previous, current map[string]any, ignored map[string]struct{}) bool {
	if previous == nil || current == nil {
		return previous == nil && current == nil
	}
	return reflect.DeepEqual(
		semanticMapWithoutKeys(previous, ignored),
		semanticMapWithoutKeys(current, ignored))
}

func semanticMapWithoutKeys(source map[string]any, ignored map[string]struct{}) map[string]any {
	if source == nil {
		return nil
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		if _, skip := ignored[key]; !skip {
			out[key] = value
		}
	}
	return out
}

func semanticWorkspaceTabsNavigationEquivalent(previous, current any, activeScreen, changedSide int) bool {
	previousMap, previousOK := previous.(map[string]any)
	currentMap, currentOK := current.(map[string]any)
	if !previousOK || !currentOK || !semanticMapEqualExceptKeys(previousMap, currentMap, map[string]struct{}{
		"tabs": {}, "newTab": {},
	}) {
		return false
	}
	if !semanticMapValuesEqualExceptKeys(previousMap["newTab"], currentMap["newTab"], map[string]struct{}{
		"x": {}, "y": {}, "w": {}, "h": {},
	}) {
		return false
	}
	previousTabs, previousOK := semanticMapSlice(previousMap["tabs"])
	currentTabs, currentOK := semanticMapSlice(currentMap["tabs"])
	if !previousOK || !currentOK || len(previousTabs) != len(currentTabs) {
		return false
	}
	for index := range previousTabs {
		active := index == activeScreen || extUiAnyInt(currentTabs[index]["index"]) == activeScreen
		if explicit, present := currentTabs[index]["active"].(bool); present {
			active = explicit
		}
		allowed := map[string]struct{}{"x": {}, "y": {}, "w": {}, "h": {}}
		if active {
			allowed["text"] = struct{}{}
			allowed["title"] = struct{}{}
			if changedSide == 0 {
				allowed["tooltipPrimary"] = struct{}{}
			} else {
				allowed["tooltipSecondary"] = struct{}{}
			}
		}
		if !semanticMapEqualExceptKeys(previousTabs[index], currentTabs[index], allowed) {
			return false
		}
	}
	return true
}

func semanticMapValuesEqualExceptKeys(previous, current any, ignored map[string]struct{}) bool {
	if reflect.DeepEqual(previous, current) {
		return true
	}
	previousMap, previousOK := previous.(map[string]any)
	currentMap, currentOK := current.(map[string]any)
	return previousOK && currentOK && semanticMapEqualExceptKeys(previousMap, currentMap, ignored)
}

func semanticPanelCatalogLegacyEquivalent(previous, current map[string]any, diff semanticPanelCatalogDiff) bool {
	for _, key := range []string{"legacy", "frames", "screens"} {
		previousValue := semanticScrubPanelCatalogAlias(previous[key], diff)
		currentValue := semanticScrubPanelCatalogAlias(current[key], diff)
		if !reflect.DeepEqual(previousValue, currentValue) {
			return false
		}
	}
	return true
}

func semanticScrubPanelCatalogAlias(value any, diff semanticPanelCatalogDiff) any {
	switch typed := value.(type) {
	case map[string]any:
		kind := semanticString(typed["kind"])
		id := semanticString(typed["id"])
		ignored := map[string]struct{}{}
		if kind == "filePanel" && id == diff.panelID {
			ignored = semanticPanelCatalogMutableKeys
		}
		if (kind == "panels" || kind == "shell") && id == diff.shellID && diff.shellTitleDiff {
			ignored = map[string]struct{}{"title": {}}
		}
		if kind == "commandLine" && id == diff.commandLineID && diff.commandLineDiff {
			ignored = semanticCommandLineNavigationKeys
		}
		if kind == "terminal" && id == diff.hiddenTerminalID && diff.hiddenTerminalID != "" {
			ignored = semanticHiddenTerminalPresentationKeys
		}
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if _, skip := ignored[key]; skip {
				continue
			}
			if key == "workspaceTabs" && diff.workspaceTabsDiff {
				out[key] = "<panel-catalog-workspace-tabs>"
				continue
			}
			if key == "menus" && diff.menusDiff {
				out[key] = "<panel-catalog-menus>"
				continue
			}
			if key == "title" && (diff.workspaceTabsDiff || diff.shellTitleDiff) {
				if _, isScreen := typed["frames"]; isScreen &&
					(extUiAnyInt(typed["index"]) == diff.activeScreen || typed["active"] == true) {
					continue
				}
			}
			out[key] = semanticScrubPanelCatalogAlias(item, diff)
		}
		return out
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, semanticScrubPanelCatalogAlias(item, diff))
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, semanticScrubPanelCatalogAlias(item, diff))
		}
		return out
	default:
		return value
	}
}

func semanticSceneActivePanel(scene map[string]any) (int, bool) {
	if scene == nil {
		return 0, false
	}
	shell, ok := scene["shell"].(map[string]any)
	if !ok || shell == nil {
		return 0, false
	}
	value, present := shell["activePanel"]
	if !present {
		return 0, false
	}
	side := extUiAnyInt(value)
	return side, side >= 0 && side <= 1
}

func semanticScenePanelMaps(scene map[string]any) ([]map[string]any, bool) {
	if scene == nil {
		return nil, false
	}
	shell, ok := scene["shell"].(map[string]any)
	if !ok || shell == nil {
		return nil, false
	}
	switch panels := shell["panels"].(type) {
	case []map[string]any:
		return panels, len(panels) > 0
	case []any:
		result := make([]map[string]any, 0, len(panels))
		for _, value := range panels {
			panel, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			result = append(result, panel)
		}
		return result, len(result) > 0
	default:
		return nil, false
	}
}

func semanticStreamSnapshot(scene map[string]any,
	streamID string,
) (map[string]any, bool) {
	if scene == nil {
		return nil, false
	}
	state := map[string]any{}
	switch streamID {
	case "chrome":
		for _, key := range []string{
			"schema", "version", "width", "height", "presentation",
			"qmlIconSet", "keyBar", "toast",
		} {
			if value, present := scene[key]; present {
				state[key] = value
			}
		}
	case "workspaces":
		for _, key := range []string{
			"activeScreen", "workspaceCount", "workspaceTabs",
		} {
			if value, present := scene[key]; present {
				state[key] = value
			}
		}
	case "menus":
		for _, key := range []string{"menuBar", "menus"} {
			if value, present := scene[key]; present {
				state[key] = value
			}
		}
	case "dialogs":
		if value, present := scene["dialogs"]; present {
			state["dialogs"] = value
		}
	case "operations":
		if value, present := scene["operationsQueue"]; present {
			state["operationsQueue"] = value
		}
	case "command-line":
		shell, ok := scene["shell"].(map[string]any)
		if !ok {
			return nil, false
		}
		if value, present := shell["commandLine"]; present {
			state["commandLine"] = value
		}
	case "shell":
		shell, ok := scene["shell"].(map[string]any)
		if !ok {
			return nil, false
		}
		projected := semanticShallowMapCopy(shell)
		delete(projected, "commandLine")
		if panels, panelsOK := semanticScenePanelMaps(scene); panelsOK {
			compactPanels := make([]map[string]any, 0, len(panels))
			for _, panel := range panels {
				compact := semanticShallowMapCopy(panel)
				delete(compact, "entries")
				delete(compact, "highlightStyles")
				compactPanels = append(compactPanels, compact)
			}
			projected["panels"] = compactPanels
		}
		state["shell"] = projected
	default:
		if strings.HasPrefix(streamID, "panel/") {
			side, err := strconv.Atoi(strings.TrimPrefix(streamID, "panel/"))
			panels, ok := semanticScenePanelMaps(scene)
			if err != nil || !ok || side < 0 || side >= len(panels) {
				return nil, false
			}
			state["side"] = side
			state["panel"] = panels[side]
		} else if strings.HasPrefix(streamID, "panel-id/") {
			panelID := strings.TrimPrefix(streamID, "panel-id/")
			panels, ok := semanticScenePanelMaps(scene)
			if !ok {
				return nil, false
			}
			for side, panel := range panels {
				if semanticString(panel["id"]) == panelID {
					state["side"] = side
					state["panel"] = panel
					break
				}
			}
			if _, present := state["panel"]; !present {
				return nil, false
			}
		} else if strings.HasPrefix(streamID, "document/") {
			if surface, present := scene["surface"]; present {
				state["surface"] = surface
			}
		} else {
			return nil, false
		}
	}
	return map[string]any{
		"type":  semanticStreamSnapshotPayloadType(streamID),
		"state": state,
	}, true
}

func semanticStreamSnapshotPayloadType(streamID string) string {
	switch {
	case streamID == "chrome":
		return "chrome_snapshot"
	case streamID == "workspaces":
		return "workspaces_snapshot"
	case streamID == "menus":
		return "menus_snapshot"
	case streamID == "dialogs":
		return "dialogs_snapshot"
	case streamID == "operations":
		return "operations_snapshot"
	case streamID == "command-line":
		return "command_line_snapshot"
	case streamID == "shell":
		return "shell_snapshot"
	case strings.HasPrefix(streamID, "panel/") ||
		strings.HasPrefix(streamID, "panel-id/"):
		return "panel_catalog_snapshot"
	case strings.HasPrefix(streamID, "document/"):
		return "document_snapshot"
	default:
		return "unknown_snapshot"
	}
}

// semanticSceneWithPanelActivation builds the exact logical successor for a
// plain split-panel Tab without traversing or copying catalog entries. It
// updates the typed shell and the active-workspace legacy aliases emitted by
// Scene.ToMap, while sharing every unchanged (and potentially large) value.
func semanticSceneWithPanelActivation(scene map[string]any, activeSide int,
	shellTitle string, commandLine map[string]any,
) (map[string]any, string) {
	if scene == nil || activeSide < 0 || activeSide > 1 {
		return nil, "invalid_scene"
	}
	shell, ok := scene["shell"].(map[string]any)
	if !ok || !semanticPanelActivationShellSupported(shell) {
		return nil, "unsupported_shell"
	}
	if commandLine != nil {
		previousCommandLine, ok := shell["commandLine"].(map[string]any)
		if !ok || semanticString(previousCommandLine["id"]) == "" {
			return nil, "command_line_shape"
		}
		if semanticString(previousCommandLine["id"]) != semanticString(commandLine["id"]) {
			return nil, "command_line_identity"
		}
	}
	previousSide, ok := semanticSceneActivePanel(scene)
	if !ok {
		return nil, "active_panel_shape"
	}
	if previousSide == activeSide {
		return nil, "active_panel_already_projected"
	}

	patchedShell, ok := semanticPanelActivationShellCopy(
		shell, previousSide, activeSide, shellTitle, commandLine)
	if !ok {
		return nil, "panel_shape"
	}
	out := semanticShallowMapCopy(scene)
	out["shell"] = patchedShell
	// In app schema v4 the typed root/shell is authoritative. frames, screens
	// and legacy are compatibility aliases: update recognized shapes for an
	// exact retained snapshot, but never reject the transition for an unknown
	// alias which Qt also updates locally. Legacy scenes retain strict aliases.
	strictAliases := semanticString(scene["schema"]) != extui.Schema
	shellID := semanticString(shell["id"])
	activeScreen := extUiAnyInt(scene["activeScreen"])

	if frames, present := scene["frames"]; present {
		patched, ok := semanticPanelActivationFramesCopy(
			frames, shellID, previousSide, activeSide, shellTitle)
		if !ok {
			if strictAliases {
				return nil, "frames_alias"
			}
		} else {
			out["frames"] = patched
		}
	}
	if screens, present := scene["screens"]; present {
		patched, ok := semanticPanelActivationScreensCopy(
			screens, activeScreen, shellID, previousSide, activeSide, shellTitle)
		if !ok {
			if strictAliases {
				return nil, "screens_alias"
			}
		} else {
			out["screens"] = patched
		}
	}

	if legacyValue, present := scene["legacy"]; present {
		legacy, ok := legacyValue.(map[string]any)
		if !ok || legacy == nil {
			if strictAliases {
				return nil, "legacy_alias_shape"
			}
		} else {
			legacyCopy := semanticShallowMapCopy(legacy)
			if frames, present := legacy["frames"]; present {
				patched, valid := semanticPanelActivationFramesCopy(
					frames, shellID, previousSide, activeSide, shellTitle)
				if !valid && strictAliases {
					return nil, "legacy_frames_alias"
				}
				if valid {
					legacyCopy["frames"] = patched
				}
			}
			if screens, present := legacy["screens"]; present {
				patched, valid := semanticPanelActivationScreensCopy(
					screens, activeScreen, shellID, previousSide, activeSide, shellTitle)
				if !valid && strictAliases {
					return nil, "legacy_screens_alias"
				}
				if valid {
					legacyCopy["screens"] = patched
				}
			}
			out["legacy"] = legacyCopy
		}
	}
	return out, ""
}

func semanticPanelActivationShellSupported(shell map[string]any) bool {
	if semanticString(shell["mode"]) != "panels" ||
		!extUiAnyBool(shell["showPanels"]) ||
		!extUiAnyBool(shell["showLeftPanel"]) ||
		!extUiAnyBool(shell["showRightPanel"]) ||
		extUiAnyBool(shell["wide"]) {
		return false
	}
	for _, key := range []string{"infoPanels", "quickViews"} {
		if values, present := shell[key]; present && semanticCollectionLen(values) != 0 {
			return false
		}
	}
	return true
}

func semanticCollectionLen(value any) int {
	switch values := value.(type) {
	case []map[string]any:
		return len(values)
	case []any:
		return len(values)
	case nil:
		return 0
	default:
		return -1
	}
}

func semanticShallowMapCopy(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func semanticPanelActivationShellCopy(shell map[string]any, previousSide,
	activeSide int, shellTitle string, commandLine map[string]any,
) (map[string]any, bool) {
	if extUiAnyInt(shell["activePanel"]) != previousSide {
		return nil, false
	}
	panels, ok := semanticPanelActivationPanelsCopy(
		shell["panels"], previousSide, activeSide)
	if !ok {
		return nil, false
	}
	out := semanticShallowMapCopy(shell)
	out["activePanel"] = activeSide
	out["panels"] = panels
	if shellTitle != "" {
		out["title"] = shellTitle
	}
	if commandLine != nil {
		out["commandLine"] = commandLine
	}
	return out, true
}

func semanticPanelActivationPanelsCopy(value any, previousSide,
	activeSide int,
) (any, bool) {
	patchPanel := func(panel map[string]any, index int) (map[string]any, int, bool) {
		if panel == nil || semanticString(panel["kind"]) != "filePanel" {
			return nil, 0, false
		}
		side := index
		if _, present := panel["side"]; present {
			side = extUiAnyInt(panel["side"])
		}
		if side < 0 || side > 1 {
			return nil, 0, false
		}
		active, present := panel["active"].(bool)
		if !present || active != (side == previousSide) {
			return nil, 0, false
		}
		out := semanticShallowMapCopy(panel)
		out["active"] = side == activeSide
		return out, side, true
	}

	seen := [2]bool{}
	switch panels := value.(type) {
	case []map[string]any:
		if len(panels) != 2 {
			return nil, false
		}
		out := make([]map[string]any, len(panels))
		for index, panel := range panels {
			patched, side, ok := patchPanel(panel, index)
			if !ok || seen[side] {
				return nil, false
			}
			seen[side] = true
			out[index] = patched
		}
		return out, seen[0] && seen[1]
	case []any:
		if len(panels) != 2 {
			return nil, false
		}
		out := make([]any, len(panels))
		for index, value := range panels {
			panel, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			patched, side, ok := patchPanel(panel, index)
			if !ok || seen[side] {
				return nil, false
			}
			seen[side] = true
			out[index] = patched
		}
		return out, seen[0] && seen[1]
	default:
		return nil, false
	}
}

func semanticPanelActivationFramesCopy(value any, shellID string,
	previousSide, activeSide int, shellTitle string,
) (any, bool) {
	patchFrame := func(frame map[string]any) (map[string]any, bool, bool) {
		kind := semanticString(frame["kind"])
		id := semanticString(frame["id"])
		if kind != "shell" && kind != "panels" && (shellID == "" || id != shellID) {
			return frame, false, true
		}
		patched, ok := semanticPanelActivationShellCopy(
			frame, previousSide, activeSide, shellTitle, nil)
		return patched, true, ok
	}

	matched := 0
	switch frames := value.(type) {
	case []map[string]any:
		out := make([]map[string]any, len(frames))
		for index, frame := range frames {
			patched, changed, ok := patchFrame(frame)
			if !ok {
				return nil, false
			}
			if changed {
				matched++
			}
			out[index] = patched
		}
		return out, matched == 1
	case []any:
		out := make([]any, len(frames))
		for index, value := range frames {
			frame, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			patched, changed, ok := patchFrame(frame)
			if !ok {
				return nil, false
			}
			if changed {
				matched++
			}
			out[index] = patched
		}
		return out, matched == 1
	default:
		return nil, false
	}
}

func semanticPanelActivationScreensCopy(value any, activeScreen int,
	shellID string, previousSide, activeSide int, shellTitle string,
) (any, bool) {
	patchScreen := func(screen map[string]any, index int) (map[string]any, bool) {
		if index != activeScreen {
			return screen, true
		}
		frames, present := screen["frames"]
		if !present {
			return nil, false
		}
		patchedFrames, ok := semanticPanelActivationFramesCopy(
			frames, shellID, previousSide, activeSide, shellTitle)
		if !ok {
			return nil, false
		}
		out := semanticShallowMapCopy(screen)
		out["frames"] = patchedFrames
		if shellTitle != "" {
			out["title"] = shellTitle
		}
		return out, true
	}

	switch screens := value.(type) {
	case []map[string]any:
		if activeScreen < 0 || activeScreen >= len(screens) {
			return nil, false
		}
		out := make([]map[string]any, len(screens))
		for index, screen := range screens {
			patched, ok := patchScreen(screen, index)
			if !ok {
				return nil, false
			}
			out[index] = patched
		}
		return out, true
	case []any:
		if activeScreen < 0 || activeScreen >= len(screens) {
			return nil, false
		}
		out := make([]any, len(screens))
		for index, value := range screens {
			screen, ok := value.(map[string]any)
			if !ok {
				return nil, false
			}
			patched, ok := patchScreen(screen, index)
			if !ok {
				return nil, false
			}
			out[index] = patched
		}
		return out, true
	default:
		return nil, false
	}
}

func semanticSceneHasPanelActivation(scene map[string]any, activeSide int) bool {
	actualSide, ok := semanticSceneActivePanel(scene)
	if !ok || actualSide != activeSide {
		return false
	}
	panels, ok := semanticScenePanelMaps(scene)
	if !ok {
		return false
	}
	foundActive := false
	for index, panel := range panels {
		side := index
		if _, present := panel["side"]; present {
			side = extUiAnyInt(panel["side"])
		}
		active, present := panel["active"].(bool)
		if !present || active != (side == activeSide) {
			return false
		}
		foundActive = foundActive || active
	}
	return foundActive
}

var semanticSceneBenchmarkKeys = map[string]struct{}{
	"benchmarkTraceId": {}, "benchmark": {}, "revision": {},
}

var semanticAppSceneComparisonKeys = map[string]struct{}{
	"benchmarkTraceId": {}, "benchmark": {}, "revision": {},
	// App schema v4 makes the typed root and shell authoritative. These are
	// compatibility aliases of the same data and can lag a sparse patch without
	// meaning that Qt needs another complete scene.
	"legacy": {}, "frames": {}, "screens": {},
}

func semanticSceneComparisonProjection(scene map[string]any) map[string]any {
	if semanticString(scene["schema"]) == extui.Schema {
		return semanticMapWithoutKeys(scene, semanticAppSceneComparisonKeys)
	}
	return semanticMapWithoutKeys(scene, semanticSceneBenchmarkKeys)
}

// semanticScenesEqual compares the native state a client owns. Navigation
// benchmark fields are transport annotations added after scene adaptation;
// they must never turn an otherwise identical redraw into a full-scene send.
func semanticScenesEqual(a, b map[string]any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	aWithoutBenchmark := semanticSceneComparisonProjection(a)
	bWithoutBenchmark := semanticSceneComparisonProjection(b)
	if reflect.DeepEqual(aWithoutBenchmark, bWithoutBenchmark) {
		return true
	}
	aShell, _ := a["shell"].(map[string]any)
	bShell, _ := b["shell"].(map[string]any)
	if terminalID, ok := semanticHiddenTerminalPresentationTransition(aShell, bShell); ok {
		aWithoutBenchmark = semanticSceneWithoutHiddenTerminalPresentation(
			aWithoutBenchmark, terminalID)
		bWithoutBenchmark = semanticSceneWithoutHiddenTerminalPresentation(
			bWithoutBenchmark, terminalID)
		return reflect.DeepEqual(aWithoutBenchmark, bWithoutBenchmark)
	}
	return false
}

var semanticCommandLineRenderedKeys = map[string]struct{}{
	"runs": {}, "cursorPrefixRuns": {}, "cursorX": {},
	"cursorVisible": {}, "cursorShape": {},
}

func semanticSceneWithoutCommandLineRendering(scene map[string]any) (map[string]any, bool) {
	if scene == nil {
		return nil, false
	}
	shell, ok := scene["shell"].(map[string]any)
	if !ok || shell == nil {
		return nil, false
	}
	commandLine, ok := shell["commandLine"].(map[string]any)
	if !ok || commandLine == nil || semanticString(commandLine["id"]) == "" {
		return nil, false
	}
	rootCopy := semanticSceneComparisonProjection(scene)
	shellCopy := semanticShallowMapCopy(shell)
	shellCopy["commandLine"] = semanticMapWithoutKeys(commandLine, semanticCommandLineRenderedKeys)
	rootCopy["shell"] = shellCopy
	return rootCopy, true
}

// semanticScrubHiddenTerminalPresentationAlias updates only the structural
// paths which can contain the active shell's terminal aliases. It deliberately
// does not recurse into catalogs or unrelated semantic surfaces.
func semanticScrubHiddenTerminalPresentationAlias(value any, terminalID string) any {
	switch typed := value.(type) {
	case map[string]any:
		if semanticString(typed["kind"]) == "terminal" &&
			semanticString(typed["id"]) == terminalID {
			return semanticMapWithoutKeys(typed, semanticHiddenTerminalPresentationKeys)
		}
		out := typed
		copied := false
		for _, key := range []string{"shell", "terminal", "frames", "screens"} {
			item, present := typed[key]
			if !present {
				continue
			}
			if !copied {
				// Maps are reference values; allocate lazily before the first write.
				out = semanticShallowMapCopy(typed)
				copied = true
			}
			out[key] = semanticScrubHiddenTerminalPresentationAlias(item, terminalID)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			scrubbed, _ := semanticScrubHiddenTerminalPresentationAlias(item, terminalID).(map[string]any)
			out = append(out, scrubbed)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, semanticScrubHiddenTerminalPresentationAlias(item, terminalID))
		}
		return out
	default:
		return value
	}
}

func semanticSceneWithoutHiddenTerminalPresentation(scene map[string]any, terminalID string) map[string]any {
	if scene == nil || terminalID == "" {
		return scene
	}
	out := semanticShallowMapCopy(scene)
	if shell, ok := scene["shell"].(map[string]any); ok {
		out["shell"] = semanticScrubHiddenTerminalPresentationAlias(shell, terminalID)
	}
	for _, key := range []string{"legacy", "frames", "screens"} {
		if value, present := scene[key]; present {
			out[key] = semanticScrubHiddenTerminalPresentationAlias(value, terminalID)
		}
	}
	return out
}

func semanticScenesEqualExceptCommandLineRendering(a, b map[string]any) bool {
	aWithout, aOK := semanticSceneWithoutCommandLineRendering(a)
	bWithout, bOK := semanticSceneWithoutCommandLineRendering(b)
	if !aOK || !bOK {
		return false
	}
	aShell, _ := a["shell"].(map[string]any)
	bShell, _ := b["shell"].(map[string]any)
	if terminalID, ok := semanticHiddenTerminalPresentationTransition(aShell, bShell); ok {
		aWithout = semanticSceneWithoutHiddenTerminalPresentation(aWithout, terminalID)
		bWithout = semanticSceneWithoutHiddenTerminalPresentation(bWithout, terminalID)
	}
	return reflect.DeepEqual(aWithout, bWithout)
}

func semanticPanelActivationTraceRejection(previous, current map[string]any) {
	if !navigationBenchmarkIsEnabled() {
		return
	}
	previousWithout, previousOK := semanticSceneWithoutCommandLineRendering(previous)
	currentWithout, currentOK := semanticSceneWithoutCommandLineRendering(current)
	if !previousOK || !currentOK {
		navigationBenchmarkRenderEvent("scene.panel_activation.rejected",
			"reason", "command_line_shape")
		return
	}
	previousShell, _ := previous["shell"].(map[string]any)
	currentShell, _ := current["shell"].(map[string]any)
	if terminalID, ok := semanticCoveredTerminalID(previousShell, currentShell); ok {
		previousWithout = semanticSceneWithoutHiddenTerminalPresentation(previousWithout, terminalID)
		currentWithout = semanticSceneWithoutHiddenTerminalPresentation(currentWithout, terminalID)
	}
	path := semanticFirstDifferencePath(previousWithout, currentWithout, "$")
	fields := []any{"reason", "logical_scene_changed"}
	if path != "" {
		fields = append(fields, "firstDifference", path)
	}
	navigationBenchmarkRenderEvent("scene.panel_activation.rejected", fields...)
}

func semanticSceneWithoutPanelActivation(scene map[string]any) (map[string]any, bool) {
	panels, ok := semanticScenePanelMaps(scene)
	if !ok {
		return nil, false
	}
	shell := scene["shell"].(map[string]any)
	rootCopy := semanticSceneComparisonProjection(scene)
	shellCopy := make(map[string]any, len(shell))
	for key, value := range shell {
		if key != "activePanel" && key != "panels" {
			shellCopy[key] = value
		}
	}
	panelCopies := make([]map[string]any, 0, len(panels))
	for _, panel := range panels {
		panelCopy := make(map[string]any, len(panel))
		for key, value := range panel {
			if key != "active" {
				panelCopy[key] = value
			}
		}
		panelCopies = append(panelCopies, panelCopy)
	}
	shellCopy["panels"] = panelCopies
	rootCopy["shell"] = shellCopy
	return rootCopy, true
}

func semanticScenesEqualExceptPanelActivation(a, b map[string]any) bool {
	aWithout, aOK := semanticSceneWithoutPanelActivation(a)
	bWithout, bOK := semanticSceneWithoutPanelActivation(b)
	return aOK && bOK && reflect.DeepEqual(aWithout, bWithout)
}

func semanticSceneCommandLine(scene map[string]any) (map[string]any, bool) {
	if scene == nil {
		return nil, false
	}
	shell, ok := scene["shell"].(map[string]any)
	if !ok || shell == nil {
		return nil, false
	}
	commandLine, ok := shell["commandLine"].(map[string]any)
	return commandLine, ok && commandLine != nil
}

func semanticSceneWithoutCommandLine(scene map[string]any) (map[string]any, bool) {
	if scene == nil {
		return nil, false
	}
	shell, ok := scene["shell"].(map[string]any)
	if !ok || shell == nil {
		return nil, false
	}
	rootCopy := make(map[string]any, len(scene))
	for key, value := range scene {
		if _, traceOnly := semanticSceneBenchmarkKeys[key]; !traceOnly {
			rootCopy[key] = value
		}
	}
	shellCopy := make(map[string]any, len(shell))
	for key, value := range shell {
		if key != "commandLine" {
			shellCopy[key] = value
		}
	}
	rootCopy["shell"] = shellCopy
	delete(rootCopy, "menus")
	if menus, present := scene["menus"]; present {
		if filtered := semanticNonAutocompleteMenus(menus); len(filtered) > 0 {
			rootCopy["menus"] = filtered
		}
	}
	return rootCopy, true
}

func semanticNonAutocompleteMenus(value any) []map[string]any {
	var menus []map[string]any
	switch typed := value.(type) {
	case []map[string]any:
		menus = typed
	case []any:
		menus = make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if menu, ok := item.(map[string]any); ok {
				menus = append(menus, menu)
			}
		}
	default:
		return nil
	}

	filtered := make([]map[string]any, 0, len(menus))
	for _, menu := range menus {
		if semanticString(menu["role"]) != "autocomplete" {
			filtered = append(filtered, menu)
		}
	}
	return filtered
}

func semanticScenesEqualExceptCommandLine(a, b map[string]any) bool {
	if _, ok := semanticSceneCommandLine(a); !ok {
		return false
	}
	if _, ok := semanticSceneCommandLine(b); !ok {
		return false
	}
	aWithout, aOK := semanticSceneWithoutCommandLine(a)
	bWithout, bOK := semanticSceneWithoutCommandLine(b)
	return aOK && bOK && reflect.DeepEqual(aWithout, bWithout)
}

func (r *ExtUiRenderer) Flush() {
	var messages []map[string]any

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	if r.pendingPalette != nil {
		messages = append(messages, map[string]any{
			"type":   "palette",
			"colors": r.pendingPalette,
		})
		r.pendingPalette = nil
	}
	// Ordinary native updates stay semantic-first for latency. Revealing the
	// compatibility grid is the one exception: frame and cursor messages are
	// decoded independently by Qt, and exposing the fallback scene first could
	// paint an old texture for one render tick. Hold either semantic replacement
	// form until Render has produced its forced full snapshot, then make the full
	// scene or bounded scene patch the final reveal.
	fallbackSemanticPending := r.pendingScene != nil || r.pendingScenePatch != nil
	fallbackRevealReady := r.fallbackRevealPending &&
		!r.nativeCellFrameSuppressed && fallbackSemanticPending &&
		r.pendingFrame != nil && extUiBool(r.pendingFrame, "full")
	if fallbackRevealReady {
		messages = append(messages, r.pendingFrame)
		messages = append(messages, map[string]any{
			"type":    "cursor",
			"x":       r.cursorX,
			"y":       r.cursorY,
			"visible": r.cursorVis,
			"shape":   int(r.cursorShape),
		})
		if r.pendingScene != nil {
			messages = append(messages,
				extUiChangedSceneSnapshotMessages(
					r.lastScene, r.pendingScene)...)
			r.lastScene = semanticShallowMapCopy(r.pendingScene)
			delete(r.lastScene, "revision")
			r.pendingScene = nil
		} else {
			// Incremental application already advanced lastScene before Flush;
			// only the exact revisioned wire patch remains to be delivered.
			messages = append(messages, r.pendingScenePatch)
			r.pendingScenePatch = nil
		}
		r.panelActivationProjected = false
		r.pendingFrame = nil
		r.cursorDirty = false
		r.fallbackRevealPending = false
	}
	if !r.fallbackRevealPending && r.pendingScene != nil {
		messages = append(messages,
			extUiChangedSceneSnapshotMessages(
				r.lastScene, r.pendingScene)...)
		// ExportSemanticScene and f4's adapter create a fresh immutable map for
		// every redraw. Remember the last snapshot here so cell-grid redraws and
		// cursor blinking do not repeatedly serialize and deliver the same large
		// semantic catalog to the Qt GUI thread.
		r.lastScene = semanticShallowMapCopy(r.pendingScene)
		delete(r.lastScene, "revision")
		r.panelActivationProjected = false
		r.pendingScene = nil
	}
	if !r.fallbackRevealPending && r.pendingScenePatch != nil {
		messages = append(messages, r.pendingScenePatch)
		r.pendingScenePatch = nil
	}
	if !r.fallbackRevealPending && r.pendingPanelActivation != nil {
		messages = append(messages, r.pendingPanelActivation)
		r.lastScene = r.pendingPanelActivationScene
		r.lastCompactScene = compactAppSemanticScene(r.pendingPanelActivationScene)
		r.panelActivationProjected = true
		r.pendingPanelActivation = nil
		r.pendingPanelActivationScene = nil
	}
	if !r.fallbackRevealPending && r.pendingPanelCatalog != nil {
		messages = append(messages, r.pendingPanelCatalog)
		r.lastScene = r.pendingPanelCatalogScene
		r.lastCompactScene = compactAppSemanticScene(r.pendingPanelCatalogScene)
		r.panelActivationProjected = false
		r.pendingPanelCatalog = nil
		r.pendingPanelCatalogScene = nil
	}
	if !r.fallbackRevealPending && r.pendingCommandLine != nil {
		messages = append(messages, r.pendingCommandLine)
		r.lastScene = r.pendingCommandLineScene
		r.lastCompactScene = compactAppSemanticScene(r.pendingCommandLineScene)
		r.panelActivationProjected = false
		r.pendingCommandLine = nil
		r.pendingCommandLineScene = nil
	}
	// Semantic state drives the native controls and is latency-sensitive. Apart
	// from the atomic fallback reveal above, queue a large cell-grid frame only
	// after the full scene or compact semantic patch for this render.
	if !r.fallbackRevealPending && r.pendingFrame != nil && !r.nativeCellFrameSuppressed {
		messages = append(messages, r.pendingFrame)
	}
	if !r.fallbackRevealPending {
		r.pendingFrame = nil
	}
	if !r.fallbackRevealPending && r.cursorDirty && !r.nativeCellFrameSuppressed {
		messages = append(messages, map[string]any{
			"type":    "cursor",
			"x":       r.cursorX,
			"y":       r.cursorY,
			"visible": r.cursorVis,
			"shape":   int(r.cursorShape),
		})
		r.cursorDirty = false
	}
	r.mu.Unlock()

	// Assign semantic scene sequence numbers before sending any leading palette
	// or cell-frame messages. The gap to transport.send_lock.wait then exposes
	// time spent behind those messages instead of hiding it from the trace.
	var benchmarks []*navigationBenchmarkMessage
	if navigationBenchmarkIsEnabled() {
		benchmarks = make([]*navigationBenchmarkMessage, len(messages))
		// Assign the semantic-scene sequence before annotating any leading
		// palette/cell-frame messages so every message in this render carries
		// the same non-zero sequence.
		for i, msg := range messages {
			if navigationBenchmarkString(msg["type"]) == "scene" {
				benchmarks[i] = navigationBenchmarkPrepareRenderMessage(msg)
			}
		}
		for i, msg := range messages {
			if navigationBenchmarkString(msg["type"]) != "scene" {
				benchmarks[i] = navigationBenchmarkPrepareRenderMessage(msg)
			}
		}
	}
	for i, msg := range messages {
		var benchmark *navigationBenchmarkMessage
		if i < len(benchmarks) {
			benchmark = benchmarks[i]
		}
		if err := r.send.SendWithBenchmark(msg, benchmark); err != nil {
			vtui.DebugLog("EXTUI_RENDERER: send failed: %v", err)
			r.mu.Lock()
			r.closed = true
			r.mu.Unlock()
			return
		}
	}
}

type ExtUiHost struct {
	mu                     sync.Mutex
	conn                   net.Conn
	send                   *extUiMessageSender
	reader                 *vtinput.Reader
	cols                   int
	rows                   int
	panelCatalogMetadataV1 bool
	panelCatalogRowsV1     bool
	platformServicesV1     bool
	platform               *platformIPCClient
	renderer               *ExtUiRenderer
}

func RunExternalUI(cols, rows int, execPath string, args []string) error {
	nonce, err := extUiNewNonce()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to create extui listener: %w", err)
	}
	defer listener.Close()
	mediaServer, err := newExtUiMediaServer()
	if err != nil {
		return fmt.Errorf("failed to create extui media listener: %w", err)
	}
	defer mediaServer.Close()
	restoreMediaBroker := setActiveExtUiMediaBroker(mediaServer.broker)
	defer restoreMediaBroker()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmdArgs := append([]string{}, args...)
	cmdArgs = append(cmdArgs,
		"--f4-ext-connect="+listener.Addr().String(),
		"--f4-ext-nonce="+nonce,
		"--f4-ext-cols="+strconv.Itoa(cols),
		"--f4-ext-rows="+strconv.Itoa(rows),
	)

	cmd := exec.CommandContext(ctx, execPath, cmdArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start external UI %q: %w", execPath, err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	var conn net.Conn
	select {
	case conn = <-connCh:
	case err := <-errCh:
		return fmt.Errorf("extui connection failed: %w", err)
	case <-time.After(10 * time.Second):
		return fmt.Errorf("extui did not connect within 10s")
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	hello, err := extUiReadMessage(conn)
	if err != nil {
		return fmt.Errorf("failed to read extui hello: %w", err)
	}
	if extUiString(hello, "type") != "hello" || extUiString(hello, "nonce") != nonce {
		return fmt.Errorf("invalid extui hello")
	}
	panelCatalogMetadataV1 := extUiHelloCapability(
		hello, extUiPanelCatalogMetadataCapability)
	panelCatalogRowsV1 := extUiHelloCapability(
		hello, extUiPanelCatalogRowsCapability)
	platformServicesV1 := runtime.GOOS == "darwin" && extUiHelloCapability(
		hello, extUiPlatformServicesCapability)
	previousPanelCatalogMetadata := setExtUiPanelCatalogMetadataEnabled(
		panelCatalogMetadataV1)
	defer setExtUiPanelCatalogMetadataEnabled(previousPanelCatalogMetadata)
	previousPanelCatalogRows := setExtUiPanelCatalogRowsEnabled(
		panelCatalogRowsV1)
	defer setExtUiPanelCatalogRowsEnabled(previousPanelCatalogRows)

	clientCols := extUiInt(hello, "cols")
	clientRows := extUiInt(hello, "rows")

	pixelW := extUiInt(hello, "pixelWidth")
	pixelH := extUiInt(hello, "pixelHeight")
	cellW := extUiInt(hello, "cellWidth")
	cellH := extUiInt(hello, "cellHeight")

	if pixelW > 0 && cellW > 0 {
		clientCols = pixelW / cellW
	}
	if pixelH > 0 && cellH > 0 {
		clientRows = pixelH / cellH
	}

	if clientCols > 0 && clientCols <= extUiMaxDimension {
		cols = clientCols
	}
	if clientRows > 0 && clientRows <= extUiMaxDimension {
		rows = clientRows
	}

	sender := &extUiMessageSender{w: conn}
	if err := sender.Send(map[string]any{
		"type":              "hello",
		"nonce":             nonce,
		"protocol":          extUiProtocolVersion,
		"cols":              cols,
		"rows":              rows,
		"app":               vtui.AppName,
		"mediaProtocol":     extUiMediaProtocolVersion,
		"mediaEndpoint":     mediaServer.Endpoint(),
		"mediaNonce":        mediaServer.nonce,
		"mediaMaxChunkSize": extUiMediaMaxRangeSize,
	}); err != nil {
		return fmt.Errorf("failed to send extui hello: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return err
	}

	host := &ExtUiHost{
		conn: conn, send: sender, cols: cols, rows: rows,
		panelCatalogMetadataV1: panelCatalogMetadataV1,
		panelCatalogRowsV1:     panelCatalogRowsV1,
		platformServicesV1:     platformServicesV1,
	}
	host.platform = newPlatformIPCClient(sender, platformServicesV1)
	restorePlatformIPC := setActivePlatformIPC(host.platform)
	defer restorePlatformIPC()
	defer host.platform.Close(errors.New("Qt host disconnected"))
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(cols, rows)
	renderer := NewExtUiRenderer(conn, sender)
	host.renderer = renderer
	// The deferred-catalog capability is an exact opt-in from a client which
	// owns native panel semantics. Older protocol-v2 clients keep receiving the
	// complete cell stream even if they tolerate app-schema scene messages.
	renderer.nativeSemanticSurfaceEnabled = panelCatalogMetadataV1
	scr.Renderer = renderer
	vtui.FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	vtui.GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	go host.readLoop()
	SetupUI()
	vtui.FrameManager.Run(reader)
	_ = sender.Send(map[string]any{"type": "quit"})
	return nil
}

func (h *ExtUiHost) readLoop() {
	for {
		var timing *navigationBenchmarkReadTiming
		if navigationBenchmarkIsEnabled() {
			timing = &navigationBenchmarkReadTiming{}
		}
		msg, err := extUiReadMessageWithBenchmark(h.conn, timing)
		if err != nil {
			vtui.DebugLog("EXTUI_HOST: read loop stopped: %v", err)
			if h.platform != nil {
				h.platform.Close(err)
			}
			if h.reader != nil {
				h.reader.Close()
			}
			return
		}
		h.handleMessageWithBenchmark(msg, timing)
	}
}

func (h *ExtUiHost) handleMessage(msg map[string]any) {
	h.handleMessageWithBenchmark(msg, nil)
}

func (h *ExtUiHost) handleMessageWithBenchmark(msg map[string]any, timing *navigationBenchmarkReadTiming) {
	switch extUiString(msg, "type") {
	case "resize":
		cols, colsOK := extUiDimension(msg, "cols")
		rows, rowsOK := extUiDimension(msg, "rows")
		if !colsOK || !rowsOK {
			return
		}
		h.mu.Lock()
		changed := h.cols != cols || h.rows != rows
		h.cols, h.rows = cols, rows
		h.mu.Unlock()
		if changed {
			h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType, InputSource: "extui"})
		}
	case "key":
		repeatCount := uint16(1)
		if extUiBool(msg, "repeat") {
			repeatCount = 2
		}
		event := &vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         extUiBool(msg, "down"),
			VirtualKeyCode:  uint16(extUiInt(msg, "vk")),
			Char:            rune(extUiInt(msg, "char")),
			RepeatCount:     repeatCount,
			ControlKeyState: vtinput.ControlKeyState(uint32(extUiInt(msg, "mods"))),
			InputSource:     "extui",
		}
		benchmark := navigationBenchmarkTraceForKey(msg, timing)
		keySequence := 0
		if _, present := msg["keySequence"]; present {
			keySequence = extUiInt(msg, "keySequence")
		}
		h.sendEventWithBenchmark(event, benchmark, keySequence)
	case "text":
		mods, ok := extUiUint32(msg, "mods")
		if !ok {
			return
		}
		for _, r := range extUiString(msg, "text") {
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				Char:            r,
				ControlKeyState: vtinput.ControlKeyState(mods),
				InputSource:     "extui_text",
			})
		}
	case "mouse":
		x, xOK := extUiInt16(msg, "x")
		y, yOK := extUiInt16(msg, "y")
		button, buttonOK := extUiUint32(msg, "button")
		flags, flagsOK := extUiUint32(msg, "flags")
		mods, modsOK := extUiUint32(msg, "mods")
		if !xOK || !yOK || !buttonOK || !flagsOK || !modsOK {
			return
		}
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          x,
			MouseY:          y,
			ButtonState:     button,
			MouseEventFlags: flags,
			KeyDown:         extUiBool(msg, "down"),
			ControlKeyState: vtinput.ControlKeyState(mods),
			InputSource:     "extui",
		})
	case "wheel":
		x, xOK := extUiInt16(msg, "x")
		y, yOK := extUiInt16(msg, "y")
		mods, modsOK := extUiUint32(msg, "mods")
		if !xOK || !yOK || !modsOK {
			return
		}
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          x,
			MouseY:          y,
			WheelDirection:  extUiInt(msg, "dir"),
			ControlKeyState: vtinput.ControlKeyState(mods),
			InputSource:     "extui",
		})
	case "paste":
		text := extUiString(msg, "text")
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true, InputSource: "extui"})
		for _, r := range text {
			h.sendEvent(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r, InputSource: "extui_paste"})
		}
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: false, InputSource: "extui"})
	case "clipboard_get":
		_ = h.send.Send(map[string]any{
			"type": "clipboard_data",
			"text": vtui.GetClipboard(),
		})
	case "clipboard_set":
		vtui.SetClipboard(extUiString(msg, "text"))
	case "panel_catalog_metadata_request":
		h.queuePanelCatalogMetadata(msg)
	case "panel_catalog_rows_request":
		h.queuePanelCatalogRows(msg)
	case "stream_snapshot_request":
		if h.renderer != nil {
			h.renderer.SendStreamSnapshot(extUiString(msg, "streamId"))
		}
	case "platform_response", "platform_event":
		if h.platform != nil && h.platformServicesV1 {
			h.platform.handleResponse(msg)
		}
	case "ui_action":
		action := msg
		if nested, ok := msg["action"].(map[string]any); ok {
			action = nested
		}
		benchmark := navigationBenchmarkTraceForAction(msg, action, timing)
		queuedNs := int64(0)
		if benchmark != nil {
			queuedNs = navigationBenchmarkMonotonicNs()
			benchmark.eventAt("ui_task.queued", "go.ipc", queuedNs, "action", benchmark.action)
		}
		if vtui.FrameManager != nil {
			vtui.FrameManager.PostPriorityTask(func() {
				if benchmark != nil {
					startedNs := navigationBenchmarkMonotonicNs()
					benchmark.eventAt("ui_task.started", "go.ui", startedNs,
						"action", benchmark.action, "queueNs", startedNs-queuedNs)
					benchmark.event("semantic_action.begin", "go.ui", "action", benchmark.action)
				}
				previousBenchmark := navigationBenchmarkSetCurrentUI(benchmark)
				handled := HandleSemanticAction(action)
				navigationBenchmarkSetCurrentUI(previousBenchmark)
				if benchmark != nil {
					benchmark.event("semantic_action.end", "go.ui", "action", benchmark.action, "handled", handled)
				}
			})
		}
	case "quit":
		if vtui.FrameManager != nil {
			vtui.FrameManager.PostTask(func() {
				vtui.FrameManager.EmitCommand(vtui.CmQuit, nil)
				vtui.FrameManager.Stop()
			})
		}
	}
}

func (h *ExtUiHost) queuePanelCatalogMetadata(msg map[string]any) bool {
	if h == nil || !h.panelCatalogMetadataV1 {
		return false
	}
	// Metadata snapshots are immutable Go-owned values published together
	// with the minimal catalog. Resolve them off the IPC reader so local-path
	// materialization and highlighting never delay the next keyboard event;
	// exact revisions make a response that raced with navigation harmless.
	request := make(map[string]any, len(msg))
	for key, value := range msg {
		request[key] = value
	}
	if h.panelCatalogRowsV1 && vtui.FrameManager != nil {
		vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
			h.serveLivePanelCatalogMetadata(request)
			return false
		})
		return true
	}
	go h.servePanelCatalogMetadata(request)
	return true
}

func (h *ExtUiHost) queuePanelCatalogRows(msg map[string]any) bool {
	if h == nil || !h.panelCatalogRowsV1 || vtui.FrameManager == nil {
		return false
	}
	request := make(map[string]any, len(msg))
	for key, value := range msg {
		request[key] = value
	}
	// Live panel state belongs to the UI thread. A bounded page is converted
	// there, then the immutable response is written from a worker so pipe
	// backpressure can never hold keyboard processing.
	vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
		catalogRevision := extUiInt64(request, "catalogRevision")
		response, ok := BuildLivePanelCatalogRows(
			extUiString(request, "panelId"), extUiString(request, "path"),
			catalogRevision, extUiInt(request, "offset"), extUiInt(request, "limit"))
		if !ok {
			response = map[string]any{
				"type":            "panel_catalog_rows_rejected",
				"panelId":         extUiString(request, "panelId"),
				"path":            extUiString(request, "path"),
				"catalogRevision": catalogRevision,
				"offset":          extUiInt(request, "offset"),
			}
		}
		if traceID := extUiString(request, "benchmarkTraceId"); traceID != "" {
			response["benchmarkTraceId"] = traceID
		}
		go func() {
			if err := h.send.Send(response); err != nil {
				vtui.DebugLog("EXTUI_HOST: catalog rows response failed: %v", err)
			}
		}()
		return false
	})
	return true
}

func (h *ExtUiHost) serveLivePanelCatalogMetadata(request map[string]any) {
	catalogRevision := extUiInt64(request, "catalogRevision")
	metadataRevision := extUiInt64(request, "metadataRevision")
	response, ok := BuildLivePanelCatalogMetadataChunk(
		extUiString(request, "panelId"), extUiString(request, "path"),
		catalogRevision, metadataRevision,
		extUiInt(request, "offset"), extUiInt(request, "limit"))
	if !ok {
		response = map[string]any{
			"type":             "panel_catalog_metadata_rejected",
			"panelId":          extUiString(request, "panelId"),
			"path":             extUiString(request, "path"),
			"catalogRevision":  catalogRevision,
			"metadataRevision": metadataRevision,
			"offset":           extUiInt(request, "offset"),
		}
	}
	if traceID := extUiString(request, "benchmarkTraceId"); traceID != "" {
		response["benchmarkTraceId"] = traceID
	}
	go func() {
		if err := h.send.Send(response); err != nil {
			vtui.DebugLog("EXTUI_HOST: live metadata response failed: %v", err)
		}
	}()
}

func (h *ExtUiHost) servePanelCatalogMetadata(request map[string]any) {
	catalogRevision := extUiInt64(request, "catalogRevision")
	metadataRevision := extUiInt64(request, "metadataRevision")
	response, ok := BuildPanelCatalogMetadataChunk(
		extUiString(request, "panelId"), extUiString(request, "path"),
		catalogRevision, metadataRevision,
		extUiInt(request, "offset"), extUiInt(request, "limit"))
	if !ok {
		response = map[string]any{
			"type":             "panel_catalog_metadata_rejected",
			"panelId":          extUiString(request, "panelId"),
			"path":             extUiString(request, "path"),
			"catalogRevision":  catalogRevision,
			"metadataRevision": metadataRevision,
			"offset":           extUiInt(request, "offset"),
		}
	}
	if traceID := extUiString(request, "benchmarkTraceId"); traceID != "" {
		response["benchmarkTraceId"] = traceID
	}
	if err := h.send.Send(response); err != nil {
		vtui.DebugLog("EXTUI_HOST: metadata response failed: %v", err)
	}
}

func (h *ExtUiHost) sendEvent(ev *vtinput.InputEvent) {
	h.sendEventWithBenchmark(ev, nil, 0)
}

func (h *ExtUiHost) sendEventWithBenchmark(ev *vtinput.InputEvent, benchmark *navigationBenchmarkTrace, keySequence int) {
	if h.reader == nil {
		return
	}
	queue := h.reader.EventChan
	navigationBenchmarkInputQueueBegin(ev, benchmark, keySequence, len(queue), cap(queue))
	select {
	case queue <- ev:
		navigationBenchmarkInputQueueEnd(ev, true, len(queue))
	case <-time.After(500 * time.Millisecond):
		navigationBenchmarkInputQueueEnd(ev, false, len(queue))
		vtui.DebugLog("EXTUI_HOST: dropped event after blocked queue: %s", ev.String())
	}
}

func RunExternalUIWithMapping(backend string) error {
	path, err := findExtUiPath(backend)
	if err != nil {
		return err
	}
	return RunExternalUI(100, 30, path, externalUIBackendArgs(backend))
}

func externalUIBackendArgs(backend string) []string {
	if backend != "qt" {
		return nil
	}
	return []string{
		"--f4-icon-set=" + string(parseQmlIconSetMode(string(AppConfig.QmlIconSet))),
		"--f4-font-family=" + effectiveGuiFont(),
		"--f4-font-size=" + strconv.Itoa(AppConfig.GuiFontSize),
		"--f4-window-geometry-file=" + filepath.Join(
			GetF4ConfigDir(), "window-geometry.ini"),
	}
}

func findExtUiPath(backend string) (string, error) {
	binName := "f4-qt-host"
	if backend != "qt" {
		binName = strings.TrimPrefix(backend, "ext:")
		if binName == "" || strings.Contains(binName, "..") || strings.ContainsAny(binName, `/\`) {
			return "", fmt.Errorf("invalid external UI executable name %q", binName)
		}
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(binName, ".exe") {
		binName += ".exe"
	}

	if env := os.Getenv("F4_EXT_UI_PATH"); env != "" {
		if extUiFileExists(env) {
			return env, nil
		}
	}

	var candidates []string
	appendFromDir := func(dir string) {
		candidates = append(candidates,
			extUiExecutablePaths(dir, binName, runtime.GOOS)...)
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		appendFromDir(exeDir)
		if runtime.GOOS == "darwin" && strings.HasSuffix(exeDir, ".app/Contents/MacOS") {
			appendFromDir(filepath.Join(filepath.Dir(filepath.Dir(exeDir)),
				"Resources"))
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		for _, cfg := range []string{"RelWithDebInfo", "Release", "Debug"} {
			appendFromDir(filepath.Join(cwd, "qt", "host", "build", "bin", cfg))
		}
		appendFromDir(filepath.Join(cwd, "qt", "host", "build", "bin"))
		appendFromDir(filepath.Join(cwd, "qt", "host", "build"))
	}

	for _, path := range candidates {
		if extUiFileExists(path) {
			return path, nil
		}
	}
	if backend == "qt" {
		embeddedPath, err := ensureEmbeddedQtHost()
		if err != nil {
			return "", err
		}
		if embeddedPath != "" {
			return embeddedPath, nil
		}
	}
	return "", fmt.Errorf("external UI executable %q not found", binName)
}

func extUiExecutablePaths(dir, binName, goos string) []string {
	join := filepath.Join
	if goos != "windows" {
		join = path.Join
	}
	paths := make([]string, 0, 2)
	if goos == "darwin" && binName == "f4-qt-host" {
		paths = append(paths, join(dir, "f4-qt-host.app", "Contents",
			"MacOS", "f4-qt-host"))
	}
	return append(paths, join(dir, binName))
}

func extUiFileExists(path string) bool {
	if path == "" {
		return false
	}
	// #nosec G703 -- callers pass either the explicit F4_EXT_UI_PATH override or a path built from a separator-free executable name.
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
