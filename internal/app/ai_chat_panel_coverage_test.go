package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/internal/numeric"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestAIChatPanelRichMarkdownRenderingAndBusyState(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	session := vtvibe.NewSession()
	ctxVFS := vtvibe.NewVFS(session)
	file, err := ctxVFS.Create(context.Background(), "/ctx/readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("context")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	reply := "" +
		"See [context](ai://ctx/readme.txt), ai://out/result.go and a wide 界 line.\n" +
		"```go:ai://out/generated.go\npackage main\n```\n" +
		"f0cacc1a AP 3.1\n\n" +
		"f0cacc1a FILE\nresult.go\n\n" +
		"f0cacc1a DELETE\n"
	var requests atomic.Int32
	busyStarted := make(chan struct{})
	releaseBusy := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n := requests.Add(1); n == 2 {
			close(busyStarted)
			<-releaseBusy
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": reply}}},
			"usage":   map[string]int{"prompt_tokens": 3, "completion_tokens": 5},
		})
	}))
	defer server.Close()
	cfg := vtvibe.Config{BaseURL: server.URL, Model: "coverage-model", APIKey: "test"}
	if err := session.Ask(context.Background(), cfg, "show the context"); err != nil {
		t.Fatal(err)
	}
	if session.LastPatch() == nil {
		t.Fatal("the AP response should expose a patch")
	}

	fp := panel.NewFileSystemPanel(0, 0, 34, 24, &aiVFSWrapper{AIVFS: ctxVFS})
	paneltest.WaitForLoad(t, fp)
	cp := NewAIChatPanel(fp)
	cp.SetPosition(0, 0, 33, 23)
	cp.SetFocus(true)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	cp.Show(scr)

	visible := cp.VisibleLinks()
	if len(visible) == 0 {
		t.Fatal("Show did not collect visible markdown links")
	}
	var targets []string
	for _, l := range visible {
		targets = append(targets, l.Target)
	}
	joinedTargets := strings.Join(targets, "\n")
	for _, want := range []string{"ai://ctx/readme.txt", "ai://out/result.go", "ai://out/generated.go"} {
		if !strings.Contains(joinedTargets, want) {
			t.Fatalf("rendered links do not contain %q: %q", want, joinedTargets)
		}
	}
	if cp.barKind() != aiBarPatch {
		t.Fatalf("barKind = %d, want patch bar", cp.barKind())
	}

	// Reach the readme.txt link the same way the user would: Up from the
	// input's first row goes to the status bar (a patch is attached), Up
	// again from there lands on the last visible link.
	cp.Input.SetCursorPos(0, 0)
	if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) || !cp.StatusBarFocused() {
		t.Fatal("Up from input row 0 should focus the patch status bar")
	}
	if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_UP}) || !cp.LinkFocused() {
		t.Fatal("Up from the status bar should focus a response link")
	}

	// Exercise link focus, copy-key handling, paging, horizontal selection,
	// and the mouse path without requiring a real panels frame to navigate.
	if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_F5}) {
		t.Fatal("F5 on a response link was not handled")
	}
	if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_RIGHT}) {
		t.Fatal("right-arrow link navigation was not handled")
	}
	if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}) {
		t.Fatal("Escape did not return focus to input")
	}
	if cp.LinkFocused() || cp.StatusBarFocused() {
		t.Fatal("Escape should leave focus on the input box")
	}
	if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_NEXT}) {
		t.Fatal("PageDown was not handled")
	}
	if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_PRIOR}) {
		t.Fatal("PageUp was not handled")
	}
	if !cp.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.ShiftPressed}) {
		t.Fatal("Shift+Left was not handled")
	}

	// Click exactly on the rendered readme.txt link and confirm it takes
	// focus, using the real coordinates Show just laid out.
	var readmeLink vtui.ChatLink
	found := false
	for _, l := range visible {
		if l.Target == "ai://ctx/readme.txt" {
			readmeLink = l
			found = true
			break
		}
	}
	if !found {
		t.Fatal("readme.txt link missing from the visible set")
	}
	mx, ok := numeric.BoundedInt16(cp.X1 + 1 + readmeLink.Col)
	if !ok {
		t.Fatal("link column out of int16 range")
	}
	my, ok := numeric.BoundedInt16(cp.Y1 + 1 + readmeLink.Row)
	if !ok {
		t.Fatal("link row out of int16 range")
	}
	if !cp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: mx, MouseY: my, ButtonState: vtinput.FromLeft1stButtonPressed}) {
		t.Fatal("click on the readme.txt link was not handled")
	}
	if link, ok := cp.FocusedLink(); !ok || link.Target != "ai://ctx/readme.txt" {
		t.Fatalf("mouse click did not focus the readme.txt link, got %+v ok=%v", link, ok)
	}
	if !cp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1}) {
		t.Fatal("mouse wheel down was not handled")
	}
	if !cp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1}) {
		t.Fatal("mouse wheel up was not handled")
	}
	if cp.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType}) {
		t.Fatal("empty mouse event was unexpectedly handled")
	}

	errCh := make(chan error, 1)
	go func() { errCh <- session.Ask(context.Background(), cfg, "second question") }()
	select {
	case <-busyStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("second request did not reach the test server")
	}
	cp.Show(scr)
	if !cp.Busy {
		t.Fatal("Show should have picked up the session's busy state")
	}
	close(releaseBusy)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
}

func TestAIChatPanelFormattingAndSessionSelectionContracts(t *testing.T) {
	if formatApplyPatchLabel(nil, 100) != "" {
		t.Fatal("nil patch produced a button")
	}
	patch := &vtvibe.Patch{Files: []string{"main.go"}}
	if got := formatApplyPatchLabel(patch, 100); !strings.Contains(got, "main.go") {
		t.Fatalf("patch button = %q", got)
	}
	if got := formatApplyPatchLabel(patch, 8); got == "" {
		t.Fatal("narrow patch button lost its fallback marker")
	}
	if formatAttachedFilesLabel(nil, 100) != "" {
		t.Fatal("empty attachment list produced a label")
	}
	for _, tc := range []struct {
		prefix string
		files  []string
		width  int
		want   string
	}{
		{"x", nil, 6, "x"},
		{"prefix", []string{"file"}, 5, ""},
		{"x", []string{"long-name"}, 3, ""},
		{"x ", []string{"a", "b"}, 8, "x a, b "},
	} {
		if got := formatBarLabel(tc.prefix, tc.files, tc.width); got != tc.want {
			t.Errorf("formatBarLabel(%q, %#v, %d) = %q, want %q", tc.prefix, tc.files, tc.width, got, tc.want)
		}
	}
	for _, tc := range []struct {
		s     string
		width int
		want  int
	}{
		{"", 3, 0}, {"abc", 0, 3}, {"界x", 1, 0}, {"界x", 2, len("界")}, {"abc", 9, 3},
	} {
		if got := cellCutChat(tc.s, tc.width); got != tc.want {
			t.Errorf("cellCutChat(%q, %d) = %d, want %d", tc.s, tc.width, got, tc.want)
		}
	}

	s := vtvibe.NewSession()
	fp := &panel.FileSystemPanel{Vfs: &aiVFSWrapper{AIVFS: vtvibe.NewVFS(s)}}
	cp := &AIChatPanel{src: fp}
	if cp.getSession() != s {
		t.Fatal("wrapper session was not selected")
	}
	fp.Vfs = vtvibe.NewVFS(s)
	if cp.getSession() != s {
		t.Fatal("bare AIVFS session was not selected")
	}
	fp.Vfs = vfs.NewNullVFS(0)
	if cp.getSession() == nil {
		t.Fatal("fallback AI session is nil")
	}
	if (&AIChatPanel{}).GetSelectedName() != "" {
		t.Fatal("nil chat source returned a selection")
	}

	unfocused := &AIChatPanel{ChatWindow: vtui.NewChatWindow(0, 0, 10, 10, "")}
	if unfocused.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN}) {
		t.Fatal("unfocused chat panel handled a key")
	}
}

// extraChatOutputLink is the AI-specific ExtraLink hook wired into
// NewAIChatPanel; test it directly since vtui's own ChatWindow tests only
// cover the generic ExtraLink mechanism, not this fenced-code convention.
func TestExtraChatOutputLink(t *testing.T) {
	for _, tc := range []struct {
		line       string
		wantTarget string
		wantOK     bool
	}{
		{"```go:generated.go", "ai://out/generated.go", true},
		{"```go:ai://out/generated.go", "ai://out/generated.go", true},
		{"```go:ai://generated.go", "ai://out/generated.go", true},
		{"```go:/out/generated.go", "ai://out/generated.go", true},
		{"```go:out/generated.go", "ai://out/generated.go", true},
		{"```go", "", false},
		{"not a fence", "", false},
		{"```go:", "", false},
	} {
		label, target, ok := extraChatOutputLink(tc.line)
		if ok != tc.wantOK || target != tc.wantTarget || (ok && label != target) {
			t.Errorf("extraChatOutputLink(%q) = (%q, %q, %v), want (_, %q, %v)", tc.line, label, target, ok, tc.wantTarget, tc.wantOK)
		}
	}
}
