package main

import (
	"bytes"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
)

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
