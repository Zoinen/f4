package main

import (
	"bytes"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

type extUiSignalWriter struct {
	writes chan struct{}
}

func extUiScenePatchPanelOperation(message map[string]any, index int) map[string]any {
	shell, _ := message["shell"].(map[string]any)
	panels, _ := shell["panels"].([]any)
	if index < 0 || index >= len(panels) {
		return nil
	}
	operation, _ := panels[index].(map[string]any)
	return operation
}

func (w *extUiSignalWriter) Write(payload []byte) (int, error) {
	select {
	case w.writes <- struct{}{}:
	default:
	}
	return len(payload), nil
}

func TestExtUiProtocolRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := map[string]any{
		"type":   "frame",
		"width":  2,
		"height": 1,
		"full":   true,
		"cells":  [][3]uint64{{0, 'A', 0x010203}, {1, 'B', 0x040506}},
	}
	if err := extUiSendMessage(&buf, want); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	got, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if extUiString(got, "type") != "frame" {
		t.Fatalf("type mismatch: %q", extUiString(got, "type"))
	}
	if extUiInt(got, "width") != 2 || extUiInt(got, "height") != 1 {
		t.Fatalf("size mismatch: %dx%d", extUiInt(got, "width"), extUiInt(got, "height"))
	}
	if !extUiBool(got, "full") {
		t.Fatal("full flag was not preserved")
	}
}

func TestExtUiPanelCatalogMetadataCapabilityRequiresExactBoolean(t *testing.T) {
	const capability = extUiPanelCatalogMetadataCapability
	tests := []struct {
		name  string
		hello map[string]any
		want  bool
	}{
		{name: "missing", hello: map[string]any{}},
		{name: "true", hello: map[string]any{
			"capabilities": map[string]any{capability: true},
		}, want: true},
		{name: "false", hello: map[string]any{
			"capabilities": map[string]any{capability: false},
		}},
		{name: "string is not truthy", hello: map[string]any{
			"capabilities": map[string]any{capability: "true"},
		}},
		{name: "wrong map type", hello: map[string]any{
			"capabilities": map[string]bool{capability: true},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extUiHelloCapability(tc.hello, capability); got != tc.want {
				t.Fatalf("capability = %v, want %v for %#v", got, tc.want, tc.hello)
			}
		})
	}

	// Verify the concrete nested-map type produced by the wire decoder, not
	// only a hand-built Go map.
	var encoded bytes.Buffer
	if err := extUiSendMessage(&encoded, map[string]any{
		"type": "hello",
		"capabilities": map[string]any{
			capability: true,
		},
	}); err != nil {
		t.Fatalf("encode hello: %v", err)
	}
	decoded, err := extUiReadMessage(&encoded)
	if err != nil {
		t.Fatalf("decode hello: %v", err)
	}
	if !extUiHelloCapability(decoded, capability) {
		t.Fatalf("wire-decoded capability was not recognized: %#v", decoded)
	}
}

func TestExtUiHostGatesPanelCatalogMetadataRequests(t *testing.T) {
	request := map[string]any{
		"type":             "panel_catalog_metadata_request",
		"panelId":          "missing-panel",
		"path":             "missing-path",
		"catalogRevision":  int64(1),
		"metadataRevision": int64(1),
	}
	if (&ExtUiHost{}).queuePanelCatalogMetadata(request) {
		t.Fatal("metadata pull was accepted without negotiated capability")
	}

	writer := &extUiSignalWriter{writes: make(chan struct{}, 4)}
	host := &ExtUiHost{
		panelCatalogMetadataV1: true,
		send:                   &extUiMessageSender{w: writer},
	}
	if !host.queuePanelCatalogMetadata(request) {
		t.Fatal("metadata pull was rejected with negotiated capability")
	}
	select {
	case <-writer.writes:
	case <-time.After(time.Second):
		t.Fatal("negotiated metadata pull did not produce a response")
	}
}

func TestExtUiHostPreservesNativeKeyRepeatHint(t *testing.T) {
	reader := vtinput.NewReader(bytes.NewReader(nil), true)
	defer reader.Close()
	host := &ExtUiHost{reader: reader}

	host.handleMessage(map[string]any{
		"type":   "key",
		"vk":     int(vtinput.VK_TAB),
		"down":   true,
		"repeat": true,
	})

	event := <-reader.EventChan
	if event == nil || event.VirtualKeyCode != vtinput.VK_TAB || event.RepeatCount <= 1 {
		t.Fatalf("native autorepeat hint was lost: %#v", event)
	}
}

func TestExtUiRenderer_DisablesTerminalRedrawHeartbeat(t *testing.T) {
	renderer := NewExtUiRenderer(nil, nil)
	if renderer.WantsPeriodicRedraw() {
		t.Fatal("native external UI must own idle cursor animation")
	}
}

func TestExtUiExecutablePathsPreferMacOSQtBundle(t *testing.T) {
	mac := extUiExecutablePaths("/opt/f4/bin", "f4-qt-host", "darwin")
	wantMac := []string{
		"/opt/f4/bin/f4-qt-host.app/Contents/MacOS/f4-qt-host",
		"/opt/f4/bin/f4-qt-host",
	}
	if !reflect.DeepEqual(mac, wantMac) {
		t.Fatalf("macOS Qt paths = %#v, want %#v", mac, wantMac)
	}

	linux := extUiExecutablePaths("/opt/f4/bin", "f4-qt-host", "linux")
	if want := []string{"/opt/f4/bin/f4-qt-host"}; !reflect.DeepEqual(linux, want) {
		t.Fatalf("Linux Qt paths = %#v, want %#v", linux, want)
	}

	custom := extUiExecutablePaths("/opt/f4/bin", "custom-host", "darwin")
	if want := []string{"/opt/f4/bin/custom-host"}; !reflect.DeepEqual(custom, want) {
		t.Fatalf("custom macOS host paths = %#v, want %#v", custom, want)
	}
}

func TestExternalUIBackendArgsScopeQmlIconsToQt(t *testing.T) {
	oldIconSet := AppConfig.QmlIconSet
	oldFont := AppConfig.GuiFont
	oldUseSystemFont := AppConfig.GuiUseSystemMonospace
	oldFontSize := AppConfig.GuiFontSize
	defer func() {
		AppConfig.QmlIconSet = oldIconSet
		AppConfig.GuiFont = oldFont
		AppConfig.GuiUseSystemMonospace = oldUseSystemFont
		AppConfig.GuiFontSize = oldFontSize
	}()

	AppConfig.QmlIconSet = QmlIconSetSystem
	AppConfig.GuiUseSystemMonospace = false
	AppConfig.GuiFont = "JetBrains Mono"
	AppConfig.GuiFontSize = 21
	qtArgs := externalUIBackendArgs("qt")
	want := []string{
		"--f4-icon-set=system",
		"--f4-font-family=JetBrains Mono",
		"--f4-font-size=21",
		"--f4-window-geometry-file=" + filepath.Join(
			GetF4ConfigDir(), "window-geometry.ini"),
	}
	if !reflect.DeepEqual(qtArgs, want) {
		t.Fatalf("Qt backend args = %#v", qtArgs)
	}
	if args := externalUIBackendArgs("ext:custom-host"); len(args) != 0 {
		t.Fatalf("generic external host received QML-only args: %#v", args)
	}

	AppConfig.QmlIconSet = QmlIconSetMode("invalid")
	if args := externalUIBackendArgs("qt"); len(args) != 4 || args[0] != "--f4-icon-set=lucide" {
		t.Fatalf("invalid icon set was not normalized: %#v", args)
	}
}

func TestExtUi_HandshakeScaling(t *testing.T) {
	c, s := net.Pipe()
	defer c.Close()
	defer s.Close()

	nonce := "testnonce"

	go func() {
		msg := map[string]any{
			"type":        "hello",
			"nonce":       nonce,
			"pixelWidth":  1000,
			"pixelHeight": 600,
			"cellWidth":   10,
			"cellHeight":  20,
		}
		extUiSendMessage(c, msg)
	}()

	hello, err := extUiReadMessage(s)
	if err != nil {
		t.Fatalf("failed to read hello: %v", err)
	}

	cols := 100
	rows := 30

	pixelW := extUiInt(hello, "pixelWidth")
	pixelH := extUiInt(hello, "pixelHeight")
	cellW := extUiInt(hello, "cellWidth")
	cellH := extUiInt(hello, "cellHeight")

	if pixelW > 0 && cellW > 0 {
		cols = pixelW / cellW
	}
	if pixelH > 0 && cellH > 0 {
		rows = pixelH / cellH
	}

	if cols != 100 {
		t.Errorf("expected cols 100 (1000/10), got %d", cols)
	}
	if rows != 30 {
		t.Errorf("expected rows 30 (600/20), got %d", rows)
	}
}

func TestExtUiRenderer_DeduplicatesSemanticScenes(t *testing.T) {
	var buf bytes.Buffer
	renderer := &ExtUiRenderer{
		send: &extUiMessageSender{w: &buf},
	}

	first := map[string]any{
		"type": "scene",
		"shell": map[string]any{
			"activePanel": 0,
			"panels": []map[string]any{{
				"catalogRevision": int64(7),
				"entries":         []map[string]any{{"entryId": "photo:1"}},
			}},
		},
	}
	renderer.SetSemanticScene(first)
	renderer.Flush()
	if _, err := extUiReadMessage(&buf); err != nil {
		t.Fatalf("first scene was not sent: %v", err)
	}

	// ExportSemanticScene allocates new maps and slices on each redraw. Equal
	// content must still be suppressed before it reaches MessagePack and Qt.
	equalScene := map[string]any{
		"type": "scene",
		"shell": map[string]any{
			"activePanel": 0,
			"panels": []map[string]any{{
				"catalogRevision": int64(7),
				"entries":         []map[string]any{{"entryId": "photo:1"}},
			}},
		},
	}
	renderer.SetSemanticScene(equalScene)
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("equal semantic scene was sent again (%d bytes)", buf.Len())
	}

	changed := map[string]any{
		"type": "scene",
		"shell": map[string]any{
			"activePanel": 0,
			"panels": []map[string]any{{
				"catalogRevision": int64(7),
				"entries":         []map[string]any{{"entryId": "photo:2"}},
			}},
		},
	}
	renderer.SetSemanticScene(changed)
	// Returning to the last delivered state before Flush cancels the transient
	// pending snapshot instead of sending a redundant scene.
	renderer.SetSemanticScene(equalScene)
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("reverted pending semantic scene was sent (%d bytes)", buf.Len())
	}

	renderer.SetSemanticScene(changed)
	renderer.Flush()
	got, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("changed scene was not sent: %v", err)
	}
	panels := got["shell"].(map[string]any)["panels"].([]any)
	entries := panels[0].(map[string]any)["entries"].([]any)
	if id := entries[0].(map[string]any)["entryId"]; id != "photo:2" {
		t.Fatalf("changed scene entry mismatch: %v", id)
	}
}

func TestExtUiRenderer_CommandLineChangesUseSmallCoalescedPatch(t *testing.T) {
	var buf bytes.Buffer
	renderer := &ExtUiRenderer{
		send: &extUiMessageSender{w: &buf},
	}

	scene := func(text string, cursor int, entryID string) map[string]any {
		result := map[string]any{
			"type": "scene",
			"shell": map[string]any{
				"activePanel": 0,
				"commandLine": map[string]any{
					"text":    text,
					"cursorX": cursor,
				},
				"panels": []map[string]any{{
					"catalogRevision": int64(7),
					"entries": []map[string]any{{
						"entryId": entryID,
						"name":    strings.Repeat("large-catalog-entry", 1024),
					}},
				}},
			},
		}
		if text != "" {
			result["menus"] = []map[string]any{{
				"id":    "command-history",
				"role":  "autocomplete",
				"query": text,
				"items": []map[string]any{{"text": text + "-completion"}},
			}}
		}
		return result
	}

	initial := scene("", 0, "photo:1")
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	initialWireBytes := buf.Len()
	if _, err := extUiReadMessage(&buf); err != nil {
		t.Fatalf("initial scene was not sent: %v", err)
	}

	// Several keystrokes before the renderer flushes must collapse to the most
	// recent authoritative command line, without retransmitting either panel.
	renderer.SetSemanticScene(scene("a", 1, "photo:1"))
	renderer.SetSemanticScene(scene("ab", 2, "photo:1"))
	renderer.SetSemanticScene(scene("abc", 3, "photo:1"))
	renderer.Flush()
	patchWireBytes := buf.Len()
	got, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("command-line patch was not sent: %v", err)
	}
	if got["type"] != "command_line" {
		t.Fatalf("expected command_line patch, got %#v", got["type"])
	}
	commandLine := got["commandLine"].(map[string]any)
	if commandLine["text"] != "abc" || extUiInt(commandLine, "cursorX") != 3 {
		t.Fatalf("latest command line was not coalesced: %#v", commandLine)
	}
	if _, exists := got["shell"]; exists {
		t.Fatalf("command-line patch unexpectedly contains the shell catalog")
	}
	menus := got["menus"].([]any)
	if len(menus) != 1 || menus[0].(map[string]any)["role"] != "autocomplete" {
		t.Fatalf("autocomplete state was not carried by the small patch: %#v", menus)
	}
	if patchWireBytes*20 >= initialWireBytes {
		t.Fatalf("patch is not materially smaller: patch=%d full=%d",
			patchWireBytes, initialWireBytes)
	}

	// The patch advances the delivered logical scene, so an identical redraw
	// is suppressed and returning to it before Flush cancels a transient edit.
	renderer.SetSemanticScene(scene("abc", 3, "photo:1"))
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("identical command line was sent again (%d bytes)", buf.Len())
	}
	renderer.SetSemanticScene(scene("abcd", 4, "photo:1"))
	renderer.SetSemanticScene(scene("abc", 3, "photo:1"))
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("reverted command line was sent (%d bytes)", buf.Len())
	}

	// Any simultaneous non-command-line mutation must fall back to a complete
	// scene so the native client cannot apply a patch to a stale catalog.
	renderer.SetSemanticScene(scene("abcd", 4, "photo:2"))
	renderer.Flush()
	got, err = extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("changed full scene was not sent: %v", err)
	}
	if got["type"] != "scene" {
		t.Fatalf("catalog mutation was incorrectly sent as %#v", got["type"])
	}
}

func TestExtUiRenderer_PanelActivationUsesRevisionedCatalogFreePatch(t *testing.T) {
	var buf bytes.Buffer
	renderer := &ExtUiRenderer{
		send: &extUiMessageSender{w: &buf},
	}

	scene := func(activeSide int, rightEntryID string) map[string]any {
		return map[string]any{
			"type": "scene",
			"shell": map[string]any{
				"activePanel": activeSide,
				"panels": []map[string]any{
					{
						"id":              "left",
						"side":            0,
						"active":          activeSide == 0,
						"catalogRevision": int64(7),
						"entries": []map[string]any{{
							"entryId": "left:1",
							"name":    strings.Repeat("large-left-entry", 1024),
						}},
					},
					{
						"id":              "right",
						"side":            1,
						"active":          activeSide == 1,
						"catalogRevision": int64(11),
						"entries": []map[string]any{{
							"entryId": rightEntryID,
							"name":    strings.Repeat("large-right-entry", 1024),
						}},
					},
				},
			},
		}
	}

	renderer.SetSemanticScene(scene(0, "right:1"))
	renderer.Flush()
	fullWireBytes := buf.Len()
	if _, err := extUiReadMessage(&buf); err != nil {
		t.Fatalf("initial scene was not sent: %v", err)
	}

	renderer.QueuePanelActivation(1)
	renderer.SetSemanticScene(scene(1, "right:1"))
	// A duplicate semantic export in the same redraw must not enqueue a
	// second acknowledgement or fall back to the full catalog.
	renderer.SetSemanticScene(scene(1, "right:1"))
	renderer.Flush()
	patchWireBytes := buf.Len()
	patch, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("activation patch was not sent: %v", err)
	}
	if patch["type"] != "panel_activation" || extUiInt(patch, "activePanel") != 1 {
		t.Fatalf("unexpected activation patch: %#v", patch)
	}
	if extUiInt(patch, "revision") != 1 {
		t.Fatalf("first activation revision = %v, want 1", patch["revision"])
	}
	if _, present := patch["shell"]; present {
		t.Fatalf("activation patch contains semantic shell/catalog: %#v", patch)
	}
	if patchWireBytes >= 96 || patchWireBytes*100 >= fullWireBytes {
		t.Fatalf("activation patch is not tiny: patch=%d full=%d",
			patchWireBytes, fullWireBytes)
	}
	if buf.Len() != 0 {
		t.Fatalf("activation emitted an extra protocol message (%d bytes)", buf.Len())
	}

	// The delivered logical scene advances with the patch and the wire
	// revision stays monotonic across subsequent Tab transitions.
	renderer.QueuePanelActivation(0)
	renderer.SetSemanticScene(scene(0, "right:1"))
	renderer.Flush()
	patch, err = extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("second activation patch was not sent: %v", err)
	}
	if patch["type"] != "panel_activation" || extUiInt(patch, "revision") != 2 {
		t.Fatalf("unexpected second activation patch: %#v", patch)
	}

	// A Tab hint is only an optimization hint. Any simultaneous authoritative
	// catalog mutation must still transmit a complete scene.
	renderer.QueuePanelActivation(1)
	renderer.SetSemanticScene(scene(1, "right:2"))
	renderer.Flush()
	changed, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("changed full scene was not sent: %v", err)
	}
	if changed["type"] != "scene" {
		t.Fatalf("catalog mutation was incorrectly sent as %#v", changed["type"])
	}
}

func panelActivationFastPathScene(activeSide int, shellTitle string) map[string]any {
	panel := func(side int, active bool, prefix string) map[string]any {
		return map[string]any{
			"id": prefix, "kind": "filePanel", "side": side,
			"active": active, "path": prefix + `:\catalog`,
			"catalogRevision":   int64(10 + side),
			"selectionRevision": int64(1), "metadataDeferred": true,
			"metadataRevision": int64(10 + side),
			"entries": []map[string]any{{
				"entryId": prefix + ":entry",
				"name":    strings.Repeat(prefix+"-catalog", 128),
			}},
		}
	}
	shell := func(id string, active int, title string) map[string]any {
		return map[string]any{
			"id": id, "kind": "shell", "mode": "panels", "title": title,
			"activePanel": active, "showPanels": true,
			"showLeftPanel": true, "showRightPanel": true,
			"terminalActive": false, "terminalBusy": false,
			"wide": false, "fallback": false,
			"panels": []map[string]any{
				panel(0, active == 0, id+":left"),
				panel(1, active == 1, id+":right"),
			},
			"terminal": map[string]any{
				"id": id + ":terminal", "kind": "terminal", "title": "Shell",
				"visible": false, "focused": false, "altScreen": false, "busy": false,
				"cursorX": 1, "cursorY": 0,
				"rows": []map[string]any{{"index": 0, "runs": []map[string]any{{"text": "prompt"}}}},
			},
		}
	}
	activeFrame := shell("shell-0", activeSide, shellTitle)
	inactiveFrame := shell("shell-1", 1, "Inactive workspace")
	frames := []map[string]any{activeFrame}
	screens := []map[string]any{
		{"index": 0, "active": true, "title": shellTitle, "frames": frames},
		{"index": 1, "active": false, "title": "Inactive workspace",
			"frames": []map[string]any{inactiveFrame}},
	}
	legacy := map[string]any{
		"activeScreen": 0,
		"frames":       frames,
		"screens":      screens,
	}
	return map[string]any{
		"type": "scene", "schema": "app", "version": extui.SceneVersion,
		"activeScreen": 0,
		"shell":        shell("shell-0", activeSide, shellTitle),
		"frames":       frames, "screens": screens, "legacy": legacy,
	}
}

func TestExtUiRenderer_DirectPanelActivationSkipsOneExportAndAdvancesScene(t *testing.T) {
	var buf bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &buf}}
	initial := panelActivationFastPathScene(0, `Panels: C:\left`)
	initial["shell"].(map[string]any)["commandLine"] = map[string]any{
		"id": "command-line", "kind": "commandLine",
		"promptRuns": []map[string]any{{"text": `C:\left>`}},
		"text":       "", "cursorX": 8,
	}
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	if _, err := extUiReadMessage(&buf); err != nil {
		t.Fatalf("initial scene was not sent: %v", err)
	}

	renderer.BeginSemanticSceneUpdate()
	commandLine := map[string]any{
		"id": "command-line", "kind": "commandLine",
		"promptRuns": []map[string]any{{"text": `D:\right>`}},
		"text":       "", "cursorX": 9,
	}
	renderer.QueuePanelActivationState(
		1, `Panels: D:\right`, commandLine)
	renderer.EndSemanticSceneUpdate()
	if !renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("eligible plain Tab did not suppress its semantic export")
	}
	if renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("semantic export suppression was not one-shot")
	}
	renderer.Flush()
	patch, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("direct activation patch was not sent: %v", err)
	}
	if patch["type"] != "panel_activation" ||
		extUiInt(patch, "activePanel") != 1 ||
		patch["shellTitle"] != `Panels: D:\right` {
		t.Fatalf("unexpected direct activation patch: %#v", patch)
	}
	gotCommandLine, ok := patch["commandLine"].(map[string]any)
	if !ok || gotCommandLine["id"] != "command-line" {
		t.Fatalf("activation command line = %#v", patch["commandLine"])
	}
	gotPromptRuns, ok := gotCommandLine["promptRuns"].([]any)
	if !ok || len(gotPromptRuns) != 1 {
		t.Fatalf("activation prompt runs = %#v", gotCommandLine["promptRuns"])
	}
	gotPromptRun, ok := gotPromptRuns[0].(map[string]any)
	if !ok || gotPromptRun["text"] != `D:\right>` {
		t.Fatalf("activation prompt run = %#v", gotPromptRuns[0])
	}
	if _, present := patch["shell"]; present {
		t.Fatalf("direct activation carried a catalog: %#v", patch)
	}

	want := panelActivationFastPathScene(1, `Panels: D:\right`)
	want["shell"].(map[string]any)["commandLine"] = commandLine
	if !reflect.DeepEqual(renderer.lastScene, want) {
		t.Fatal("direct activation did not advance the renderer's complete logical scene")
	}
	// A later ordinary export must compare cleanly; otherwise the direct path
	// merely defers the same megabyte-scale scene by one render.
	renderer.SetSemanticScene(want)
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("scene matching the direct activation was resent (%d bytes)", buf.Len())
	}
}

func TestExtUiRenderer_NativeDirectActivationDefersOneWholeRender(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		nativeSemanticSurfaceEnabled: true,
	}
	initial := panelActivationFastPathScene(0, `Panels: C:\left`)
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("initial scene was not sent: %v", err)
	}
	if !renderer.nativeCellFrameSuppressed {
		t.Fatal("native app scene did not take ownership of the cell surface")
	}

	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1, `Panels: D:\right`)
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(0)
	patch, err := extUiReadMessage(&wire)
	if err != nil || patch["type"] != "panel_activation" {
		t.Fatalf("direct activation was not sent: %#v, %v", patch, err)
	}
	if !renderer.ConsumeSemanticRenderPhaseDeferral(0) {
		t.Fatal("native direct activation did not defer its redundant render")
	}
	if renderer.ConsumeSemanticRenderPhaseDeferral(0) {
		t.Fatal("whole-render deferral was not one-shot")
	}
	if renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("whole-render deferral left a stale export-suppression permit")
	}

	// A redraw after the boundary changes the generation and restores the full
	// authoritative render/export path.
	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(0, `Panels: C:\left`)
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(7)
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("second direct activation was not sent: %v", err)
	}
	if renderer.ConsumeSemanticRenderPhaseDeferral(8) {
		t.Fatal("newer redraw generation did not cancel whole-render deferral")
	}
	if renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("newer redraw generation left stale export suppression")
	}

	// A later unhandled mutation also invalidates a newly armed permit.
	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1, `Panels: D:\right`)
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(8)
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("third direct activation was not sent: %v", err)
	}
	renderer.BeginSemanticSceneUpdate()
	renderer.EndSemanticSceneUpdate()
	if renderer.ConsumeSemanticRenderPhaseDeferral(8) {
		t.Fatal("unverified mutation did not cancel whole-render deferral")
	}
}

func TestExtUiRenderer_DirectEditorCursorStateIsTinyAndDefersRender(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		nativeSemanticSurfaceEnabled: true,
	}
	rows := make([]map[string]any, 500)
	for index := range rows {
		rows[index] = map[string]any{
			"index": index, "text": strings.Repeat("document-row-", 20),
		}
	}
	initial := map[string]any{
		"type": "scene", "schema": "app", "version": extui.SceneVersion,
		"width": 120, "height": 40, "activeScreen": 0,
		"surface": map[string]any{
			"id": "editor:music.svg", "kind": "editor",
			"cursorLine": 0, "cursorPos": 0,
			"cursorVisualRow": 0, "cursorVisualColumn": 0,
			"cursorVisible": true, "cursorShape": "underline",
			"cursorAbsoluteRow": int64(0), "rows": rows,
		},
	}
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("initial editor scene was not sent: %v", err)
	}
	if !renderer.nativeCellFrameSuppressed {
		t.Fatal("native editor scene did not own the cell surface")
	}

	renderer.BeginSemanticSceneUpdate()
	accepted := renderer.QueueSurfaceState("editor:music.svg", map[string]any{
		"cursorLine": 0, "cursorPos": 1,
		"cursorVisualRow": 0, "cursorVisualColumn": 1,
		"cursorVisible": true, "cursorShape": "underline",
		"cursorAbsoluteRow": int64(0),
	})
	if !accepted {
		t.Fatal("bounded cursor state was rejected")
	}
	message, err := extUiReadMessage(&wire)
	if err != nil {
		t.Fatalf("cursor state was not sent immediately: %v", err)
	}
	if message["type"] != "scene_patch" ||
		semanticInt64(message["baseRevision"]) != 1 ||
		semanticInt64(message["revision"]) != 2 {
		t.Fatalf("unexpected cursor patch envelope: %#v", message)
	}
	surfacePatch, ok := message["surface"].(map[string]any)
	if !ok || surfacePatch["id"] != "editor:music.svg" {
		t.Fatalf("cursor patch lost surface identity: %#v", message["surface"])
	}
	if semanticValueContainsKey(message, "rows") ||
		semanticValueContainsKey(message, "windowRows") {
		t.Fatalf("cursor patch leaked document content: %#v", message)
	}
	payload, err := msgpack.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	if len(payload) > 512 {
		t.Fatalf("cursor patch is %d bytes; expected a tiny scalar packet", len(payload))
	}
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(19)
	if !renderer.ConsumeSemanticRenderPhaseDeferral(19) {
		t.Fatal("direct cursor state did not defer the redundant editor render")
	}
	installed := renderer.lastScene["surface"].(map[string]any)
	if semanticInt(installed["cursorPos"]) != 1 || len(appMapSlice(installed["rows"])) != 500 {
		t.Fatalf("cursor state did not preserve the installed document: %#v", installed)
	}
}

func TestExtUiRenderer_DirectEditorCursorStateRejectsWrongSurface(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		nativeSemanticSurfaceEnabled: true,
	}
	renderer.SetSemanticScene(map[string]any{
		"type": "scene", "schema": "app", "version": extui.SceneVersion,
		"surface": map[string]any{
			"id": "editor:one", "kind": "editor",
			"cursorLine": 0, "cursorPos": 0,
			"cursorVisualRow": 0, "cursorVisualColumn": 0,
			"cursorVisible": true, "cursorShape": "underline",
			"cursorAbsoluteRow": int64(0),
		},
	})
	renderer.Flush()
	_, _ = extUiReadMessage(&wire)
	renderer.BeginSemanticSceneUpdate()
	if renderer.QueueSurfaceState("editor:replacement", map[string]any{
		"cursorLine": 0, "cursorPos": 1,
		"cursorVisualRow": 0, "cursorVisualColumn": 1,
		"cursorVisible": true, "cursorShape": "underline",
		"cursorAbsoluteRow": int64(0),
	}) {
		t.Fatal("cursor state for a replacement surface was accepted")
	}
	if wire.Len() != 0 || renderer.sceneRevision != 1 {
		t.Fatalf("rejected cursor state changed wire/revision: bytes=%d revision=%d",
			wire.Len(), renderer.sceneRevision)
	}
	renderer.EndSemanticSceneUpdate()
}

func TestExtUiRenderer_ExplicitUnchangedInputDefersNativeRender(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		nativeSemanticSurfaceEnabled: true,
	}
	renderer.SetSemanticScene(panelActivationFastPathScene(0, `C:\large`))
	renderer.Flush()
	_, _ = extUiReadMessage(&wire)

	renderer.BeginSemanticSceneUpdate()
	if !renderer.SetSemanticInputUnchanged() {
		t.Fatal("explicit unchanged input proof was rejected")
	}
	renderer.EndSemanticSceneUpdate()
	renderer.BindSemanticRenderPhaseDeferral(31)
	if !renderer.ConsumeSemanticRenderPhaseDeferral(31) {
		t.Fatal("unchanged native input did not defer its intermediate render")
	}
	if wire.Len() != 0 || renderer.sceneRevision != 1 {
		t.Fatalf("unchanged input wrote protocol state: bytes=%d revision=%d",
			wire.Len(), renderer.sceneRevision)
	}
}

func TestExtUiRenderer_DirectActivationSuppressesSceneInCellFallback(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		nativeSemanticSurfaceEnabled: true,
	}
	initial := panelActivationFastPathScene(0, `Panels: C:\left`)
	initial["shell"].(map[string]any)["fallback"] = true
	initial["shell"].(map[string]any)["reason"] = "resized panels"
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("initial fallback scene was not sent: %v", err)
	}
	if renderer.nativeCellFrameSuppressed {
		t.Fatal("cell fallback incorrectly hid its grid")
	}

	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1, `Panels: D:\right`)
	renderer.EndSemanticSceneUpdate()
	activation, err := extUiReadMessage(&wire)
	if err != nil || activation["type"] != "panel_activation" {
		t.Fatalf("fallback activation was not compact: %#v, %v", activation, err)
	}
	renderer.BeginSemanticSceneUpdate()
	if !renderer.EndSemanticSceneUpdateUnchanged() {
		t.Fatal("no-op key release discarded the fallback activation proof")
	}
	if !renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("fallback activation did not suppress the complete semantic scene")
	}
	renderer.BindSemanticRenderPhaseDeferral(42)
	if renderer.ConsumeSemanticRenderPhaseDeferral(42) {
		t.Fatal("cell fallback skipped the grid render it still needs")
	}
}

func TestExtUiRenderer_UnchangedTaskBoundaryPreservesExistingDirectProofs(t *testing.T) {
	t.Run("activation", func(t *testing.T) {
		var wire bytes.Buffer
		renderer := &ExtUiRenderer{
			send:                         &extUiMessageSender{w: &wire},
			nativeSemanticSurfaceEnabled: true,
		}
		renderer.SetSemanticScene(panelActivationFastPathScene(0, `Panels: C:\left`))
		renderer.Flush()
		_, _ = extUiReadMessage(&wire)

		renderer.BeginSemanticSceneUpdate()
		renderer.QueuePanelActivation(1, `Panels: D:\right`)
		renderer.EndSemanticSceneUpdate()
		renderer.BindSemanticRenderPhaseDeferral(17)
		_, _ = extUiReadMessage(&wire)

		renderer.BeginSemanticSceneUpdate()
		if !renderer.EndSemanticSceneUpdateUnchanged() {
			t.Fatal("no-op task boundary was not accepted")
		}
		if !renderer.ConsumeSemanticRenderPhaseDeferral(17) {
			t.Fatal("no-op task boundary discarded the preceding activation permit")
		}
	})

	t.Run("catalog", func(t *testing.T) {
		var wire bytes.Buffer
		renderer := &ExtUiRenderer{
			send:                         &extUiMessageSender{w: &wire},
			nativeSemanticSurfaceEnabled: true,
		}
		renderer.SetSemanticScene(panelActivationFastPathScene(0, `Panels: C:\left`))
		renderer.Flush()
		_, _ = extUiReadMessage(&wire)
		panels, ok := semanticScenePanelMaps(renderer.lastScene)
		if !ok || len(panels) != 2 {
			t.Fatal("initial scene has no panels")
		}
		panel := semanticShallowMapCopy(panels[0])
		panel["path"] = `C:\left\child`
		panel["title"] = `C:\left\child`
		panel["catalogRevision"] = int64(12)
		panel["metadataRevision"] = int64(12)
		panel["entries"] = []map[string]any{{"entryId": "child", "name": "child"}}
		beforeRevision := renderer.sceneRevision
		if !renderer.QueuePanelCatalogState(0, panel, `Panels: C:\left\child`, "") {
			t.Fatal("exact catalog replacement was rejected")
		}
		message, _ := extUiReadMessage(&wire)
		operation := extUiScenePatchPanelOperation(message, 0)
		if message["type"] != "scene_patch" ||
			operation["op"] != "catalog_replace" ||
			renderer.sceneRevision != beforeRevision+1 {
			t.Fatalf("catalog replacement did not advance exact scene state: %#v", message)
		}

		renderer.BeginSemanticSceneUpdate()
		if !renderer.EndSemanticSceneUpdateUnchanged() {
			t.Fatal("no-op task boundary was not accepted")
		}
		if renderer.sceneRevision != beforeRevision+1 {
			t.Fatal("no-op task boundary changed the installed catalog revision")
		}
		if renderer.ConsumeSemanticRenderPhaseDeferral(0) {
			t.Fatal("catalog-only patch hid a later derived-state render")
		}
	})
}

func TestExtUiRenderer_UnchangedTaskBoundaryRejectsTouchedSemanticState(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		nativeSemanticSurfaceEnabled: true,
	}
	renderer.SetSemanticScene(panelActivationFastPathScene(0, `Panels: C:\left`))
	renderer.Flush()
	_, _ = extUiReadMessage(&wire)

	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1, `Panels: D:\right`)
	if renderer.EndSemanticSceneUpdateUnchanged() {
		t.Fatal("task which queued a direct activation was accepted as unchanged")
	}
	renderer.EndSemanticSceneUpdate()
}

func TestExtUiRenderer_UnchangedTaskBoundaryIsPresentationIndependent(t *testing.T) {
	textRenderer := &ExtUiRenderer{nativeSemanticSurfaceEnabled: true}
	textScene := panelActivationFastPathScene(0, `Panels: C:\left`)
	textScene["presentation"] = "text"
	textRenderer.SetSemanticScene(textScene)
	textRenderer.BeginSemanticSceneUpdate()
	if !textRenderer.EndSemanticSceneUpdateUnchanged() {
		t.Fatal("proven no-op depended on the active presentation")
	}
}

func TestExtUiRenderer_RenderDeferralRequiresNegotiatedNativeSurface(t *testing.T) {
	for _, test := range []struct {
		name         string
		native       bool
		presentation string
	}{
		{name: "capability not negotiated"},
		{name: "text fallback", native: true, presentation: "text"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var wire bytes.Buffer
			renderer := &ExtUiRenderer{
				send:                         &extUiMessageSender{w: &wire},
				nativeSemanticSurfaceEnabled: test.native,
			}
			initial := panelActivationFastPathScene(0, `Panels: C:\left`)
			if test.presentation != "" {
				initial["presentation"] = test.presentation
			}
			renderer.SetSemanticScene(initial)
			renderer.Flush()
			if _, err := extUiReadMessage(&wire); err != nil {
				t.Fatalf("initial scene was not sent: %v", err)
			}

			renderer.BeginSemanticSceneUpdate()
			renderer.QueuePanelActivation(1, `Panels: D:\right`)
			renderer.EndSemanticSceneUpdate()
			if _, err := extUiReadMessage(&wire); err != nil {
				t.Fatalf("direct activation was not sent: %v", err)
			}
			if renderer.ConsumeSemanticRenderPhaseDeferral(0) {
				t.Fatal("non-native surface deferred the cell render")
			}
			if !renderer.ConsumeSemanticSceneExportSuppression() {
				t.Fatal("non-native surface lost its narrower export suppression")
			}
		})
	}
}

func TestExtUiRenderer_DirectPanelCatalogCanFollowProjectedActivation(t *testing.T) {
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{
		send:                         &extUiMessageSender{w: &wire},
		nativeSemanticSurfaceEnabled: true,
	}
	initial := panelActivationFastPathScene(0, `Panels: C:\left`)
	renderer.SetSemanticScene(initial)
	renderer.Flush()
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("initial scene was not sent: %v", err)
	}

	renderer.QueuePanelActivation(1, `Panels: D:\right`)
	activation, err := extUiReadMessage(&wire)
	if err != nil || activation["type"] != "panel_activation" {
		t.Fatalf("direct activation was not sent: %#v, %v", activation, err)
	}
	if !renderer.deferSemanticRender {
		t.Fatal("native direct activation did not arm whole-render deferral")
	}
	if !renderer.panelActivationProjected {
		t.Fatal("test did not retain the projected activation state")
	}

	panels, ok := semanticScenePanelMaps(renderer.lastScene)
	if !ok || len(panels) != 2 {
		t.Fatalf("projected scene has no panels: %#v", renderer.lastScene)
	}
	directPanel := semanticShallowMapCopy(panels[1])
	directPanel["path"] = `D:\right\child`
	directPanel["title"] = `D:\right\child`
	directPanel["catalogRevision"] = int64(12)
	directPanel["metadataRevision"] = int64(12)
	directPanel["entries"] = []map[string]any{{
		"entryId": "shell-0:right:child", "name": "child",
	}}
	renderer.QueuePanelCatalogState(1, directPanel, `Panels: D:\right\child`, "")
	catalog, err := extUiReadMessage(&wire)
	operation := extUiScenePatchPanelOperation(catalog, 0)
	if err != nil || catalog["type"] != "scene_patch" ||
		operation["op"] != "catalog_replace" || extUiInt(operation, "side") != 1 {
		t.Fatalf("catalog after projected activation was not immediate: %#v, %v", catalog, err)
	}
	if renderer.ConsumeSemanticRenderPhaseDeferral(0) {
		t.Fatal("direct catalog skipped the render required to validate its projection")
	}

	authoritative := panelActivationFastPathScene(1, `Panels: D:\right\child`)
	var installPanel func(any)
	installPanel = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			if typed["kind"] == "filePanel" && typed["id"] == directPanel["id"] {
				for key, item := range directPanel {
					typed[key] = item
				}
				return
			}
			for _, nested := range typed {
				installPanel(nested)
			}
		case []map[string]any:
			for _, nested := range typed {
				installPanel(nested)
			}
		case []any:
			for _, nested := range typed {
				installPanel(nested)
			}
		}
	}
	installPanel(authoritative)
	firstDifference := semanticFirstDifferencePath(renderer.lastScene, authoritative, "$")
	renderer.SetSemanticScene(authoritative)
	renderer.Flush()
	if wire.Len() != 0 {
		t.Fatalf("authoritative scene retransmitted delivered activation/catalog (%d bytes; first difference %s)",
			wire.Len(), firstDifference)
	}
	if !semanticScenesEqual(renderer.lastScene, authoritative) || renderer.directPanelCatalog != nil {
		t.Fatal("renderer did not reconcile activation followed by direct catalog")
	}
}

func TestExtUiRenderer_DirectPanelActivationPublishesEachInputAndFallsBackWhenUnsafe(t *testing.T) {
	var buf bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &buf}}
	renderer.SetSemanticScene(panelActivationFastPathScene(0, `Panels: C:\left`))
	renderer.Flush()
	if _, err := extUiReadMessage(&buf); err != nil {
		t.Fatalf("initial scene was not sent: %v", err)
	}

	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1, `Panels: D:\right`)
	renderer.EndSemanticSceneUpdate()
	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(0, `Panels: C:\left`)
	renderer.EndSemanticSceneUpdate()
	if !renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("Tab pair did not suppress the redundant export")
	}
	renderer.Flush()
	for index, wantSide := range []int{1, 0} {
		message, err := extUiReadMessage(&buf)
		if err != nil {
			t.Fatalf("direct Tab patch %d was not sent: %v", index, err)
		}
		if message["type"] != "panel_activation" ||
			extUiInt(message, "activePanel") != wantSide {
			t.Fatalf("direct Tab patch %d = %#v, want side %d", index, message, wantSide)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("Tab pair emitted an unexpected trailing message (%d bytes)", buf.Len())
	}

	// An unrelated update in the same batch makes the queued projection
	// untrusted. The renderer must request a real export. Once that export has
	// proven the final state, the existing conservative diff may still choose a
	// compact wire message.
	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelActivation(1, `Panels: D:\right`)
	renderer.EndSemanticSceneUpdate()
	immediate, err := extUiReadMessage(&buf)
	if err != nil || immediate["type"] != "panel_activation" {
		t.Fatalf("unsafe-boundary activation was not published immediately: %#v, %v", immediate, err)
	}
	renderer.BeginSemanticSceneUpdate()
	renderer.EndSemanticSceneUpdate()
	if renderer.ConsumeSemanticSceneExportSuppression() {
		t.Fatal("unverified update did not cancel semantic export suppression")
	}
	renderer.SetSemanticScene(panelActivationFastPathScene(1, `Panels: D:\right`))
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("authoritative scene matching the direct patch was resent (%d bytes)", buf.Len())
	}
}

func TestExtUiRenderer_DirectPanelActivationRejectsUnsupportedShellLayouts(t *testing.T) {
	for _, test := range []struct {
		name  string
		key   string
		value any
	}{
		{name: "wide", key: "wide", value: true},
		{name: "left hidden", key: "showLeftPanel", value: false},
		{name: "right hidden", key: "showRightPanel", value: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			renderer := &ExtUiRenderer{}
			renderer.lastScene = panelActivationFastPathScene(0, "Panels: left")
			renderer.lastScene["shell"].(map[string]any)[test.key] = test.value
			renderer.BeginSemanticSceneUpdate()
			renderer.QueuePanelActivation(1, "Panels: right")
			renderer.EndSemanticSceneUpdate()
			if renderer.ConsumeSemanticSceneExportSuppression() {
				t.Fatal("unsupported layout suppressed the semantic export")
			}
		})
	}
}

func TestExtUiRenderer_PanelCatalogUsesSmallAuthoritativePatch(t *testing.T) {
	var buf bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &buf}}

	scene := func(path string, revision int64, entryID string) map[string]any {
		left := map[string]any{
			"id": "left", "kind": "filePanel", "side": 0, "active": true,
			"path": path, "title": path, "catalogRevision": revision,
			"selectionRevision": revision, "cursorEntryId": entryID, "cursor": 0,
			"loading": false, "catalogProvisional": false,
			"metadataDeferred": true, "metadataRevision": revision,
			"entries": []map[string]any{{
				"index": 0, "entryId": entryID, "name": "..", "path": path + `\..`,
				"isDir": true, "isUp": true, "isImage": false, "selected": false,
			}},
			"sourceKind": "vfs", "sortModeName": "name", "sortReverse": false,
		}
		right := map[string]any{
			"id": "right", "kind": "filePanel", "side": 1, "active": false,
			"path": `D:\large`, "title": `D:\large`, "catalogRevision": int64(44),
			"selectionRevision": int64(1), "cursorEntryId": "right:0", "cursor": 0,
			"loading": false, "catalogProvisional": false,
			"metadataDeferred": true, "metadataRevision": int64(44),
			"entries": []map[string]any{{
				"index": 0, "entryId": "right:0", "name": strings.Repeat("large-unchanged-entry", 8192),
				"path": `D:\large\huge`, "isDir": false, "isUp": false,
				"isImage": false, "selected": false,
			}},
			"sourceKind": "vfs", "sortModeName": "name", "sortReverse": false,
		}
		commandLine := map[string]any{
			"id": "cmd", "kind": "commandLine", "visible": true, "focused": false,
			"prompt": path + ">", "promptRuns": []map[string]any{{"text": path + ">"}},
			"text": "", "cursorPosition": 0, "selectionStart": 0, "selectionEnd": 0,
		}
		shell := map[string]any{
			"id": "shell", "kind": "shell", "title": "Panels: " + path,
			"activePanel": 0, "showPanels": true,
			"panels": []map[string]any{left, right}, "commandLine": commandLine,
		}
		legacyShell := map[string]any{
			"id": "shell", "kind": "panels", "title": "Panels: " + path,
			"activePanel": 0, "showPanels": true,
			"panels": []map[string]any{left, right}, "commandLine": commandLine,
		}
		workspaceTabs := map[string]any{
			"visible": true,
			"tabs": []map[string]any{
				{"index": 0, "active": true, "text": path},
				{"index": 1, "active": false, "text": "other workspace"},
			},
		}
		screen := map[string]any{
			"index": 0, "active": true, "title": path,
			"frames": []map[string]any{legacyShell},
		}
		return map[string]any{
			"type": "scene", "schema": "f4.extui", "version": 3,
			"width": 120, "height": 40, "activeScreen": 0,
			"workspaceTabs": workspaceTabs, "shell": shell,
			"frames":  []map[string]any{legacyShell},
			"screens": []map[string]any{screen},
			"legacy": map[string]any{
				"type": "scene", "activeScreen": 0, "workspaceTabs": workspaceTabs,
				"frames": []map[string]any{legacyShell}, "screens": []map[string]any{screen},
			},
		}
	}

	first := scene(`D:\Code\f4`, 10, "left:old")
	renderer.SetSemanticScene(first)
	renderer.Flush()
	fullWireBytes := buf.Len()
	if _, err := extUiReadMessage(&buf); err != nil {
		t.Fatalf("initial full scene was not sent: %v", err)
	}

	second := scene(`D:\Code\f4\plugins`, 11, "left:new")
	renderer.SetSemanticScene(second)
	renderer.Flush()
	patchWireBytes := buf.Len()
	patchMessage, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("panel catalog patch was not sent: %v", err)
	}
	if patchMessage["type"] != "panel_catalog" || extUiInt(patchMessage, "side") != 0 ||
		extUiInt(patchMessage, "activePanel") != 0 {
		t.Fatalf("unexpected panel catalog envelope: %#v", patchMessage)
	}
	panel := patchMessage["panel"].(map[string]any)
	if panel["id"] != "left" || panel["path"] != `D:\Code\f4\plugins` ||
		extUiInt(panel, "catalogRevision") != 11 {
		t.Fatalf("patch did not carry the latest authoritative panel: %#v", panel)
	}
	if patchMessage["shellTitle"] != `Panels: D:\Code\f4\plugins` {
		t.Fatalf("path-derived shell title was not carried: %#v", patchMessage["shellTitle"])
	}
	if patchMessage["commandLine"].(map[string]any)["prompt"] != `D:\Code\f4\plugins>` {
		t.Fatalf("path-derived command prompt was not carried: %#v", patchMessage["commandLine"])
	}
	if patchMessage["workspaceTabs"].(map[string]any) == nil {
		t.Fatal("path-derived workspace title was not carried")
	}
	for _, forbidden := range []string{"shell", "legacy", "frames", "screens"} {
		if _, present := patchMessage[forbidden]; present {
			t.Fatalf("panel patch duplicated %s: %#v", forbidden, patchMessage)
		}
	}
	if patchWireBytes*20 >= fullWireBytes {
		t.Fatalf("panel patch is not materially smaller: patch=%d full=%d", patchWireBytes, fullWireBytes)
	}
	if buf.Len() != 0 {
		t.Fatalf("panel transition emitted an extra message (%d bytes)", buf.Len())
	}

	// Applying the patch advances the delivered logical scene. Equal redraws
	// disappear, and multiple transitions before Flush collapse to the newest.
	renderer.SetSemanticScene(second)
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("identical post-patch scene was sent again (%d bytes)", buf.Len())
	}
	third := scene(`D:\Code\f4`, 12, "left:third")
	fourth := scene(`D:\Code`, 13, "left:fourth")
	renderer.SetSemanticScene(third)
	renderer.SetSemanticScene(fourth)
	renderer.Flush()
	coalesced, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("coalesced panel patch was not sent: %v", err)
	}
	if coalesced["type"] != "panel_catalog" ||
		coalesced["panel"].(map[string]any)["path"] != `D:\Code` {
		t.Fatalf("panel patch did not coalesce to newest scene: %#v", coalesced)
	}
	fifth := scene(`D:\`, 14, "left:fifth")
	renderer.SetSemanticScene(fifth)
	renderer.SetSemanticScene(fourth)
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("reverted pending panel transition was sent (%d bytes)", buf.Len())
	}
}

func TestExtUiRenderer_PanelCatalogFallsBackForUnsafeCombinedChanges(t *testing.T) {
	baseScene := func() map[string]any {
		panel := func(id string, side int, path string, revision int64) map[string]any {
			return map[string]any{
				"id": id, "kind": "filePanel", "side": side, "active": side == 0,
				"path": path, "title": path, "catalogRevision": revision,
				"selectionRevision": revision, "cursorEntryId": id + ":0", "cursor": 0,
				"loading": false, "metadataDeferred": true, "metadataRevision": revision,
				"entries": []map[string]any{{
					"index": 0, "entryId": id + ":0", "name": "..", "path": path,
					"isDir": true, "isUp": true, "selected": false,
				}},
				"sourceKind": "vfs", "sortModeName": "name",
			}
		}
		return map[string]any{
			"type": "scene", "width": 100, "height": 30, "activeScreen": 0,
			"shell": map[string]any{
				"id": "shell", "kind": "shell", "title": "Panels",
				"activePanel": 0,
				"commandLine": map[string]any{
					"id": "cmd", "kind": "commandLine", "prompt": ">", "text": "",
				},
				"panels": []map[string]any{
					panel("left", 0, `D:\left`, 1), panel("right", 1, `D:\right`, 1),
				},
			},
		}
	}
	clone := func(source map[string]any) map[string]any {
		payload, err := msgpack.Marshal(source)
		if err != nil {
			t.Fatal(err)
		}
		var result map[string]any
		if err := msgpack.Unmarshal(payload, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "both panels",
			mutate: func(scene map[string]any) {
				panels := scene["shell"].(map[string]any)["panels"].([]any)
				panels[0].(map[string]any)["path"] = `D:\left\child`
				panels[0].(map[string]any)["catalogRevision"] = int8(2)
				panels[1].(map[string]any)["cursor"] = int8(1)
			},
		},
		{
			name: "sort mode",
			mutate: func(scene map[string]any) {
				panel := scene["shell"].(map[string]any)["panels"].([]any)[0].(map[string]any)
				panel["path"] = `D:\left\child`
				panel["catalogRevision"] = int8(2)
				panel["sortModeName"] = "size"
			},
		},
		{
			name: "dialog",
			mutate: func(scene map[string]any) {
				panel := scene["shell"].(map[string]any)["panels"].([]any)[0].(map[string]any)
				panel["path"] = `D:\left\child`
				panel["catalogRevision"] = int8(2)
				scene["dialogs"] = []map[string]any{{"id": "unexpected"}}
			},
		},
		{
			name: "command text",
			mutate: func(scene map[string]any) {
				panel := scene["shell"].(map[string]any)["panels"].([]any)[0].(map[string]any)
				panel["path"] = `D:\left\child`
				panel["catalogRevision"] = int8(2)
				scene["shell"].(map[string]any)["commandLine"].(map[string]any)["text"] = "dir"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &buf}}
			base := baseScene()
			renderer.SetSemanticScene(base)
			renderer.Flush()
			if _, err := extUiReadMessage(&buf); err != nil {
				t.Fatal(err)
			}
			changed := clone(base)
			tc.mutate(changed)
			renderer.SetSemanticScene(changed)
			renderer.Flush()
			message, err := extUiReadMessage(&buf)
			if err != nil {
				t.Fatalf("fallback scene was not sent: %v", err)
			}
			if message["type"] != "scene" {
				t.Fatalf("unsafe transition used compact patch: %#v", message)
			}
		})
	}
}

func TestExtUiRenderer_DirectPanelCatalogPrecedesRenderAndReconcilesWithChrome(t *testing.T) {
	panel := func(path string, revision int64) map[string]any {
		return map[string]any{
			"id": "left", "kind": "filePanel", "side": 0, "active": true,
			"path": path, "title": path, "catalogRevision": revision,
			"selectionRevision": revision, "metadataDeferred": true,
			"metadataRevision": revision, "cursorEntryId": path + ":up", "cursor": 0,
			"loading": true, "catalogProvisional": false, "sourceKind": "os",
			"sortModeName": "name", "sortReverse": false,
			"entries": []map[string]any{{
				"index": 0, "entryId": path + ":up", "name": "..", "path": path,
				"isDir": true, "isUp": true, "isImage": false, "selected": false,
			}},
		}
	}
	base := map[string]any{
		"type": "scene", "schema": "app", "version": extui.SceneVersion,
		"activeScreen": 0,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "title": `Panels: D:\Code`,
			"activePanel": 0, "showPanels": true,
			"commandLine": map[string]any{"id": "cmd", "kind": "commandLine", "prompt": `D:\Code>`},
			"panels": []map[string]any{
				panel(`D:\Code`, 1),
				{
					"id": "right", "kind": "filePanel", "side": 1, "active": false,
					"path": `D:\right`, "catalogRevision": int64(1),
					"metadataDeferred": true, "metadataRevision": int64(1),
					"sourceKind": "os", "sortModeName": "name",
					"entries": []map[string]any{},
				},
			},
		},
	}
	changed := semanticShallowMapCopy(base)
	changedShell := semanticShallowMapCopy(base["shell"].(map[string]any))
	changed["shell"] = changedShell
	changedPanels := append([]map[string]any(nil),
		base["shell"].(map[string]any)["panels"].([]map[string]any)...)
	changedPanel := panel(`D:\Code\f4`, 2)
	changedPanels[0] = changedPanel
	changedShell["panels"] = changedPanels
	changedShell["title"] = `Panels: D:\Code\f4`
	changedCommandLine := semanticShallowMapCopy(
		base["shell"].(map[string]any)["commandLine"].(map[string]any))
	changedCommandLine["prompt"] = `D:\Code\f4>`
	changedShell["commandLine"] = changedCommandLine

	var buf bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &buf}}
	renderer.SetSemanticScene(base)
	renderer.Flush()
	if _, err := extUiReadMessage(&buf); err != nil {
		t.Fatal(err)
	}

	renderer.QueuePanelCatalogState(0, changedPanel,
		`Panels: D:\Code\f4`, "trace:direct-catalog")
	immediate, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("direct catalog was not sent before render/Flush: %v", err)
	}
	immediateOperation := extUiScenePatchPanelOperation(immediate, 0)
	if immediate["type"] != "scene_patch" ||
		immediateOperation["op"] != "catalog_replace" ||
		immediateOperation["panel"].(map[string]any)["path"] != `D:\Code\f4` {
		t.Fatalf("unexpected direct catalog: %#v", immediate)
	}

	renderer.SetSemanticScene(changed)
	renderer.Flush()
	chrome, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatalf("post-render chrome reconciliation was not sent: %v", err)
	}
	if chrome["type"] != "command_line" ||
		chrome["commandLine"].(map[string]any)["prompt"] != `D:\Code\f4>` {
		t.Fatalf("unexpected chrome reconciliation: %#v", chrome)
	}
	if _, duplicated := chrome["panel"]; duplicated {
		t.Fatalf("chrome reconciliation retransmitted the catalog: %#v", chrome)
	}
	if !semanticScenesEqual(renderer.lastScene, changed) {
		t.Fatal("direct catalog reconciliation did not adopt the authoritative scene")
	}
	renderer.SetSemanticScene(changed)
	renderer.Flush()
	if buf.Len() != 0 {
		t.Fatalf("reconciled catalog was sent again (%d bytes)", buf.Len())
	}
}

func TestExtUiRenderer_DirectPanelCatalogMismatchForcesAuthoritativeScene(t *testing.T) {
	basePanel := map[string]any{
		"id": "left", "kind": "filePanel", "side": 0, "active": true,
		"path": `D:\old`, "catalogRevision": int64(1), "metadataDeferred": true,
		"metadataRevision": int64(1), "sourceKind": "os", "sortModeName": "name",
		"entries": []map[string]any{},
	}
	base := map[string]any{
		"type": "scene", "schema": "app", "version": extui.SceneVersion,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "title": "Panels", "activePanel": 0,
			"panels": []map[string]any{basePanel, {
				"id": "right", "kind": "filePanel", "side": 1, "active": false,
				"path": `D:\right`, "catalogRevision": int64(1), "metadataDeferred": true,
				"metadataRevision": int64(1), "sourceKind": "os", "sortModeName": "name",
				"entries": []map[string]any{},
			}},
		},
	}
	var buf bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &buf}}
	renderer.nativeSemanticSurfaceEnabled = true
	renderer.SetSemanticScene(base)
	renderer.Flush()
	_, _ = extUiReadMessage(&buf)
	direct := semanticShallowMapCopy(basePanel)
	direct["path"] = `D:\new`
	direct["catalogRevision"] = int64(2)
	renderer.QueuePanelCatalogState(0, direct, "Panels: new", "")
	_, _ = extUiReadMessage(&buf)
	if renderer.ConsumeSemanticRenderPhaseDeferral(0) {
		t.Fatal("direct catalog skipped its immediate authoritative proof")
	}
	// Exact scene/catalog bases allow consecutive completed directory results
	// to chain without waiting for a redundant full-scene proof.
	second := semanticShallowMapCopy(direct)
	second["path"] = `D:\newer`
	second["catalogRevision"] = int64(3)
	renderer.BeginSemanticSceneUpdate()
	renderer.QueuePanelCatalogState(0, second, "Panels: newer", "")
	renderer.EndSemanticSceneUpdate()
	chained, err := extUiReadMessage(&buf)
	if err != nil || chained["type"] != "scene_patch" ||
		extUiScenePatchPanelOperation(chained, 0)["op"] != "catalog_replace" {
		t.Fatalf("exact chained catalog was not emitted: %#v, %v", chained, err)
	}
	if renderer.ConsumeSemanticRenderPhaseDeferral(0) {
		t.Fatal("chained catalog hid a later derived-state render")
	}

	changed := map[string]any{}
	for key, value := range base {
		changed[key] = value
	}
	changed["shell"] = semanticShallowMapCopy(base["shell"].(map[string]any))
	changed["shell"].(map[string]any)["panels"] = []map[string]any{second, base["shell"].(map[string]any)["panels"].([]map[string]any)[1]}
	changed["dialogs"] = []map[string]any{{"id": "unexpected"}}
	renderer.SetSemanticScene(changed)
	renderer.Flush()
	correction, err := extUiReadMessage(&buf)
	if err != nil || correction["type"] != "scene" {
		t.Fatalf("unsafe direct projection was not corrected by a full scene: %#v, %v", correction, err)
	}
}

func TestExtUiRenderer_DirectPanelCatalogRollbackForcesAuthoritativeScene(t *testing.T) {
	base := BuildAppSceneFromLegacy(nil,
		productionPanelCatalogLegacy(`D:\Code\f4`, 10, 20, false, "old"))
	projected := BuildAppSceneFromLegacy(nil,
		productionPanelCatalogLegacy(`D:\Code\f4\plugins`, 11, 21, false, "new"))
	projectedPanels, ok := semanticScenePanelMaps(projected)
	if !ok || len(projectedPanels) != 2 {
		t.Fatalf("projected panel missing: %#v", projected["shell"])
	}

	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}}
	renderer.SetSemanticScene(base)
	renderer.Flush()
	if _, err := extUiReadMessage(&wire); err != nil {
		t.Fatalf("initial scene was not sent: %v", err)
	}
	renderer.QueuePanelCatalogState(0, projectedPanels[0],
		`Panels: D:\Code\f4\plugins`, "")
	if direct, err := extUiReadMessage(&wire); err != nil ||
		direct["type"] != "scene_patch" ||
		extUiScenePatchPanelOperation(direct, 0)["op"] != "catalog_replace" {
		t.Fatalf("direct projection was not sent: %#v, %v", direct, err)
	}

	// The authoritative UI mutation can still reject/roll back after the
	// optimistic catalog has crossed the wire. Re-exporting the old scene must
	// correct the client, not be mistaken for an equal/no-op scene.
	renderer.SetSemanticScene(base)
	renderer.Flush()
	correction, err := extUiReadMessage(&wire)
	if err != nil || correction["type"] != "scene" {
		t.Fatalf("rolled-back direct projection was not corrected: %#v, %v", correction, err)
	}
	if !semanticScenesEqual(renderer.lastScene, base) {
		t.Fatal("rollback correction did not retain the authoritative base scene")
	}
}

func TestExtUiRenderer_FlushSendsSemanticStateBeforeCellFrame(t *testing.T) {
	var buf bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &buf}}
	scene := func(path string, revision int64) map[string]any {
		return map[string]any{
			"type": "scene",
			"shell": map[string]any{
				"id": "shell", "kind": "shell", "activePanel": 0,
				"panels": []map[string]any{
					{
						"id": "left", "kind": "filePanel", "side": 0, "active": true,
						"path": path, "catalogRevision": revision, "metadataDeferred": true,
						"metadataRevision": revision,
						"entries":          []map[string]any{{"entryId": path, "name": "..", "isDir": true}},
					},
					{
						"id": "right", "kind": "filePanel", "side": 1, "active": false,
						"path": `D:\right`, "catalogRevision": int64(1), "metadataDeferred": true,
						"metadataRevision": int64(1),
						"entries":          []map[string]any{{"entryId": "right", "name": "right"}},
					},
				},
			},
		}
	}
	queueFrame := func() {
		renderer.pendingFrame = map[string]any{
			"type": "frame", "width": 1, "height": 1, "full": true,
			"cells": [][3]uint64{{0, 'x', 0}},
		}
	}

	queueFrame()
	renderer.SetSemanticScene(scene(`D:\left`, 1))
	renderer.Flush()
	first, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	second, err := extUiReadMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if first["type"] != "scene" || second["type"] != "frame" {
		t.Fatalf("full scene/frame order = %v, %v", first["type"], second["type"])
	}

	queueFrame()
	renderer.SetSemanticScene(scene(`D:\left\child`, 2))
	renderer.Flush()
	first, err = extUiReadMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	second, err = extUiReadMessage(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if first["type"] != "panel_catalog" || second["type"] != "frame" {
		t.Fatalf("panel patch/frame order = %v, %v", first["type"], second["type"])
	}
}

func TestExtUiRenderer_SuppressesHiddenNativeCellFrameAndRestoresFallback(t *testing.T) {
	var wire bytes.Buffer
	renderer := NewExtUiRenderer(nil, &extUiMessageSender{w: &wire})
	renderer.nativeSemanticSurfaceEnabled = true
	nativeScene := map[string]any{
		"type": "scene", "schema": "app", "presentation": "full",
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "mode": "panels",
			"activePanel": 0, "showPanels": true,
		},
	}
	cells := []vtui.CharInfo{{Char: 'x', Attributes: 7}}

	renderer.SetSemanticScene(nativeScene)
	renderer.SetCursor(0, 0, true, vtui.CursorShapeBlock)
	renderer.Render(cells, nil, 1, 1, true)
	renderer.Flush()
	message, err := extUiReadMessage(&wire)
	if err != nil || message["type"] != "scene" {
		t.Fatalf("native scene message = %#v, %v", message, err)
	}
	if wire.Len() != 0 {
		t.Fatalf("hidden native grid emitted %d extra bytes", wire.Len())
	}
	if renderer.pendingFrame != nil || !renderer.cursorDirty {
		t.Fatalf("suppressed state lost fallback snapshot: frame=%#v cursorDirty=%v",
			renderer.pendingFrame, renderer.cursorDirty)
	}

	textScene := map[string]any{
		"type": "scene", "schema": "app", "presentation": "text",
		"shell": nativeScene["shell"],
	}
	renderer.SetSemanticScene(textScene)
	// Do not reveal the fallback scene until its authoritative cell snapshot is
	// ready. The Qt decoder applies protocol messages independently, so sending
	// the scene here could expose the retained pre-suppression texture.
	renderer.Flush()
	if wire.Len() != 0 {
		t.Fatalf("fallback scene crossed the wire before its frame: %d bytes", wire.Len())
	}
	if !renderer.fallbackRevealPending || renderer.pendingScene == nil {
		t.Fatalf("fallback reveal was not retained: pending=%v scene=%#v",
			renderer.fallbackRevealPending, renderer.pendingScene)
	}
	// Even an unchanged screen buffer must become a full frame when the hidden
	// fallback grid is revealed for the first time.
	renderer.Render(cells, cells, 1, 1, false)
	renderer.Flush()
	for index, want := range []string{"frame", "cursor", "scene"} {
		message, err = extUiReadMessage(&wire)
		if err != nil || message["type"] != want {
			t.Fatalf("fallback message %d = %#v, %v; want %q", index, message, err, want)
		}
		if want == "frame" && message["full"] != true {
			t.Fatalf("restored fallback frame is not full: %#v", message)
		}
	}
	if wire.Len() != 0 {
		t.Fatalf("unexpected fallback messages: %d bytes", wire.Len())
	}
	if renderer.fallbackRevealPending || renderer.pendingScene != nil {
		t.Fatalf("fallback reveal remained pending: pending=%v scene=%#v",
			renderer.fallbackRevealPending, renderer.pendingScene)
	}
}

func TestExtUiRenderer_IncrementalTextPresentationRevealIsAtomic(t *testing.T) {
	var wire bytes.Buffer
	renderer := NewExtUiRenderer(nil, &extUiMessageSender{w: &wire})
	renderer.nativeSemanticSurfaceEnabled = true
	nativeScene := map[string]any{
		"type": "scene", "schema": extui.Schema,
		"version": extui.SceneVersion, "presentation": "gui",
		"width": 1, "height": 1, "activeScreen": 0,
		"shell": map[string]any{
			"id": "shell", "kind": "shell", "mode": "panels",
			"activePanel": 0, "showPanels": true,
		},
	}
	cells := []vtui.CharInfo{{Char: 'x', Attributes: 7}}

	renderer.SetSemanticScene(nativeScene)
	renderer.Render(cells, nil, 1, 1, true)
	renderer.Flush()
	initial, err := extUiReadMessage(&wire)
	if err != nil || initial["type"] != "scene" {
		t.Fatalf("initial native scene = %#v, %v", initial, err)
	}

	patch := extui.ScenePatch{
		BaseRevision: renderer.sceneRevision,
		Revision:     renderer.sceneRevision + 1,
		Root: &extui.MapPatch{Set: map[string]any{
			"presentation": "text",
		}},
	}
	// Reproduce the state established by SetSemanticSceneIncremental after it
	// applies the logical patch and arms the compatibility-grid reveal.
	renderer.mu.Lock()
	renderer.sceneRevision = patch.Revision
	renderer.pendingScenePatch = patch.ToMap()
	renderer.lastScene = semanticSceneStructuralMapCopy(renderer.lastScene)
	applyAppScenePatchToSnapshot(renderer.lastScene, patch)
	renderer.lastCompactScene = compactAppSemanticScene(renderer.lastScene)
	renderer.setNativeCellFrameSuppression(renderer.lastScene)
	renderer.mu.Unlock()

	if !renderer.fallbackRevealPending || renderer.pendingScenePatch == nil {
		t.Fatalf("incremental fallback was not armed: pending=%v patch=%#v",
			renderer.fallbackRevealPending, renderer.pendingScenePatch)
	}
	// The retained native grid may be unchanged, but its first visible frame
	// must still be complete and precede the presentation patch atomically.
	renderer.Render(cells, cells, 1, 1, false)
	renderer.Flush()
	for index, want := range []string{"frame", "cursor", "scene_patch"} {
		message, readErr := extUiReadMessage(&wire)
		if readErr != nil || message["type"] != want {
			t.Fatalf("incremental fallback message %d = %#v, %v; want %q",
				index, message, readErr, want)
		}
		if want == "frame" && message["full"] != true {
			t.Fatalf("incremental fallback frame is not full: %#v", message)
		}
		if want == "scene_patch" {
			root, _ := message["root"].(map[string]any)
			set, _ := root["set"].(map[string]any)
			if set["presentation"] != "text" {
				t.Fatalf("incremental fallback patch = %#v", message)
			}
		}
	}
	if wire.Len() != 0 {
		t.Fatalf("unexpected incremental fallback messages: %d bytes", wire.Len())
	}
	if renderer.fallbackRevealPending || renderer.pendingScenePatch != nil {
		t.Fatalf("incremental fallback remained pending: pending=%v patch=%#v",
			renderer.fallbackRevealPending, renderer.pendingScenePatch)
	}
}

func TestSemanticSceneOwnsNativeSurfaceKeepsFallbackProtocols(t *testing.T) {
	tests := []struct {
		name  string
		scene map[string]any
		want  bool
	}{
		{name: "native panels", want: true, scene: map[string]any{
			"schema": "app", "shell": map[string]any{"kind": "shell"},
		}},
		{name: "native terminal", want: true, scene: map[string]any{
			"schema": "app", "shell": map[string]any{"kind": "shell", "terminalActive": true},
		}},
		{name: "native document", want: true, scene: map[string]any{
			"schema": "app", "surface": map[string]any{"kind": "viewer"},
		}},
		{name: "text presentation", scene: map[string]any{
			"schema": "app", "presentation": "text", "shell": map[string]any{"kind": "shell"},
		}},
		{name: "declared fallback", scene: map[string]any{
			"schema": "app", "shell": map[string]any{"kind": "shell", "fallback": true},
		}},
		{name: "nested fallback widget", scene: map[string]any{
			"schema": "app", "surface": map[string]any{
				"kind": "viewer", "children": []map[string]any{{"kind": "fallbackWidget"}},
			},
		}},
		{name: "legacy producer", scene: map[string]any{
			"type": "scene", "frames": []map[string]any{{"kind": "panels"}},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := semanticSceneOwnsNativeSurface(tc.scene); got != tc.want {
				t.Fatalf("semanticSceneOwnsNativeSurface() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtUiRenderer_CellFramesRemainWithoutNegotiatedNativeCapability(t *testing.T) {
	var wire bytes.Buffer
	renderer := NewExtUiRenderer(nil, &extUiMessageSender{w: &wire})
	renderer.SetSemanticScene(map[string]any{
		"type": "scene", "schema": "app",
		"shell": map[string]any{"kind": "shell", "mode": "panels"},
	})
	renderer.Render([]vtui.CharInfo{{Char: 'x'}}, nil, 1, 1, true)
	renderer.Flush()
	for index, want := range []string{"scene", "frame", "cursor"} {
		message, err := extUiReadMessage(&wire)
		if err != nil || message["type"] != want {
			t.Fatalf("legacy message %d = %#v, %v; want %q", index, message, err, want)
		}
	}
}

func TestExtUiRenderer_DeduplicatesWindowTitles(t *testing.T) {
	var wire bytes.Buffer
	renderer := NewExtUiRenderer(nil, &extUiMessageSender{w: &wire})
	renderer.SetWindowTitle("f4")
	renderer.SetWindowTitle("f4")
	message, err := extUiReadMessage(&wire)
	if err != nil || message["type"] != "title" || message["title"] != "f4" {
		t.Fatalf("title message = %#v, %v", message, err)
	}
	if wire.Len() != 0 {
		t.Fatalf("duplicate title emitted %d extra bytes", wire.Len())
	}
}
