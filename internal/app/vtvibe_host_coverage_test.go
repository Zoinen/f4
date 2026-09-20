package app

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/vtvibe"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestVtvibeHostConfigAndSettings(t *testing.T) {
	setupPortableIni(t, "0")
	for _, name := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(name, "")
	}
	writeVtvibeINI(t, "[general]\nbase_url = http://localhost:9999/v1\nmodel = local-model\nkey = ini-key\n")

	cfg, source := vtvibeConfig()
	if cfg.BaseURL != "http://localhost:9999/v1" || cfg.Model != "local-model" || cfg.APIKey != "ini-key" || source != vtvibeIniName {
		t.Fatalf("INI config = %#v, source %q", cfg, source)
	}

	t.Setenv("GOOGLE_API_KEY", " google-key ")
	cfg, source = vtvibeConfig()
	if cfg.APIKey != "google-key" || source != "GOOGLE_API_KEY" {
		t.Fatalf("Google environment key = %q from %q", cfg.APIKey, source)
	}
	t.Setenv("GEMINI_API_KEY", " gemini-key ")
	cfg, source = vtvibeConfig()
	if cfg.APIKey != "gemini-key" || source != "GEMINI_API_KEY" {
		t.Fatalf("Gemini environment priority = %q from %q", cfg.APIKey, source)
	}

	writeVtvibeINI(t, "[general]\nzeta = last\nalpha = first\n")
	if err := vtvibeSaveSetting("model", "chosen"); err != nil {
		t.Fatalf("vtvibeSaveSetting() error = %v", err)
	}
	data, err := os.ReadFile(vtvibeIniPath())
	if err != nil {
		t.Fatal(err)
	}
	want := "[general]\nalpha = first\nmodel = chosen\nzeta = last\n"
	if string(data) != want {
		t.Fatalf("saved settings = %q, want %q", data, want)
	}
}

func TestVtvibeHostNoKeyAndErrorPaths(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	setupPortableIni(t, "0")
	for _, name := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(name, "")
	}
	writeVtvibeINI(t, "[general]\nbase_url = https://example.invalid/v1\n")

	if aiAskAction() {
		t.Fatal("AI ask without a panels workspace was handled")
	}
	aiCommand(nil, "help")
	aiSetViewMode(nil, "ai://ctx", false)
	if top := vtui.FrameManager.GetTopFrame(); top != nil {
		t.Fatalf("AI command without panels opened a frame: %T", top)
	}

	aiSend(&panel.PanelsFrame{}, "question")
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(time.Second):
		t.Fatal("no-key notification was not posted")
	}
	if top := vtui.FrameManager.GetTopFrame(); top == nil {
		t.Fatal("no-key notification did not open a dialog")
	} else {
		top.Close()
		vtui.FrameManager.RemoveFrame(top)
	}

	aiShowError(vtvibe.ErrNoKey)
	if top := vtui.FrameManager.GetTopFrame(); top == nil {
		t.Fatal("ErrNoKey did not open an error dialog")
	} else {
		top.Close()
		vtui.FrameManager.RemoveFrame(top)
	}
	aiShowError(errors.New(strings.Repeat("x", 700)))
	if top := vtui.FrameManager.GetTopFrame(); top == nil {
		t.Fatal("long error did not open an error dialog")
	} else {
		top.Close()
		vtui.FrameManager.RemoveFrame(top)
	}

	aiSetupDialog(&panel.PanelsFrame{})
	if top := vtui.FrameManager.GetTopFrame(); top == nil {
		t.Fatal("AI setup did not open its key prompt")
	} else {
		top.Close()
		vtui.FrameManager.RemoveFrame(top)
	}
}

func TestAIVFSWrapperRejectsUnrelatedKeysAndApps(t *testing.T) {
	wrapper := &aiVFSWrapper{AIVFS: vtvibe.NewVFS(vtvibe.NewSession())}
	event := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_1,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	if wrapper.ProcessPanelKey(nil, event) {
		t.Fatal("AI wrapper handled a nil app")
	}
	if wrapper.ProcessPanelKey(&panel.PanelsFrame{}, event) {
		t.Fatal("AI wrapper handled an unrelated panel")
	}
	event.KeyDown = false
	if wrapper.ProcessPanelKey(&panel.PanelsFrame{}, event) {
		t.Fatal("AI wrapper handled a key-up event")
	}
}
