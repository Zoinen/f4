package app

import (
	"errors"
	"fmt"
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

func TestVtvibeHostProviderPreset(t *testing.T) {
	setupPortableIni(t, "0")
	for _, name := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY", "XAI_API_KEY"} {
		t.Setenv(name, "")
	}
	t.Setenv("OPENAI_API_KEY", "openai-key")
	t.Setenv("XAI_API_KEY", "xai-key")
	writeVtvibeINI(t, "[general]\nprovider = xai\nbase_url = http://ignored/v1\nmodel = "+vtvibe.DefaultModel+"\n")

	cfg, source := vtvibeConfig()
	if cfg.BaseURL != "https://api.x.ai/v1" || cfg.APIKey != "xai-key" || source != "XAI_API_KEY" {
		t.Fatalf("xAI preset = %#v from %q", cfg, source)
	}
	if cfg.Model != vtvibe.ProviderByID("xai").Model {
		t.Fatalf("leftover Gemini model kept for xAI: %q", cfg.Model)
	}
}

// f4#1842, stage H6: a dialog's own mode wins over the setting for new ones.
func TestVtvibeDialogModeOverridesTheSetting(t *testing.T) {
	setupPortableIni(t, "0")
	session := vtvibe.NewSession()
	writeVtvibeINI(t, "[general]\nnonstop = true\n")
	if !aiNonstop(session) {
		t.Fatal("a dialog without its own mode does not follow the setting")
	}
	session.SetMode(vtvibe.ModeQA)
	if aiNonstop(session) {
		t.Fatal("the dialog's own mode lost to the setting")
	}
	writeVtvibeINI(t, "[general]\n")
	session.SetMode(vtvibe.ModeDefault)
	if aiNonstop(session) {
		t.Fatal("the default is not question and answer")
	}
}

// f4#1842, stage H6: the dialog's own GitHub token wins over the setting.
func TestVtvibeGitHubTokenOfTheDialogOverridesTheSetting(t *testing.T) {
	setupPortableIni(t, "0")
	session := vtvibe.NewSession()
	writeVtvibeINI(t, "[general]\ngithub_token = global-token\n")
	if env := aiAgentConfig(session)().ToolEnv; len(env) != 2 || env[0] != "GH_TOKEN=global-token" {
		t.Fatalf("the setting's token is not given to the commands: %v", env)
	}
	session.SetGitHubToken("dialog-token")
	if token, _ := aiGitHubToken(session); token != "dialog-token" {
		t.Fatalf("token %q", token)
	}
	session.SetGitHubToken("")
	writeVtvibeINI(t, "[general]\n")
	if env := aiAgentConfig(session)().ToolEnv; env != nil {
		t.Fatalf("a token appeared from nowhere: %v", env)
	}
}

// f4#1842, stage H5: a worker the manager started for no particular order
// reports without a "#0".
func TestVtvibeWorkerReportNamesTheOrderOnlyWhenThereIsOne(t *testing.T) {
	ok := vtvibe.WorkerResult{ID: 2, Report: "all green"}
	if text := aiTaskResultText(ok, 0); strings.Contains(text, "#0") || !strings.Contains(text, "#2") || !strings.Contains(text, "all green") {
		t.Fatalf("report without an order: %q", text)
	}
	if text := aiTaskResultText(ok, 5); !strings.Contains(text, "#5") {
		t.Fatalf("report for order 5: %q", text)
	}
	failed := vtvibe.WorkerResult{ID: 3, Err: errors.New("boom")}
	if text := aiTaskResultText(failed, 0); strings.Contains(text, "#0") || !strings.Contains(text, "boom") {
		t.Fatalf("failure without an order: %q", text)
	}
}

// f4#1842, stage H8: a worker's report tells what the gate did.
func TestVtvibeWorkerReportTellsTheGate(t *testing.T) {
	text := aiTaskResultText(vtvibe.WorkerResult{ID: 1, Err: errors.New("did not pass"), GateReturns: 2, Gate: "- rule 1 broken"}, 3)
	if !strings.Contains(text, "2") || !strings.Contains(text, "- rule 1 broken") {
		t.Fatalf("report %q", text)
	}
	setupPortableIni(t, "0")
	if aiGateRules() != "" {
		t.Fatal("rules appeared without a file")
	}
}

// f4#1842, stage H8: the manager's rules are kept, one per line, the oldest
// going first past the limit.
func TestVtvibeLearnedRulesAreKeptAndBounded(t *testing.T) {
	setupPortableIni(t, "0")
	for i := 0; i < maxLearnedRules+2; i++ {
		aiLearnRule(fmt.Sprintf("rule %d\nsecond line", i))
	}
	lines := strings.Split(strings.TrimSpace(aiLearnedRules()), "\n")
	if len(lines) != maxLearnedRules || lines[0] != "- rule 2 second line" || lines[len(lines)-1] != fmt.Sprintf("- rule %d second line", maxLearnedRules+1) {
		t.Fatalf("%d rules, first %q, last %q", len(lines), lines[0], lines[len(lines)-1])
	}
}

// f4#1842, stage H9: ai:cost prices what it can and still shows the tokens
// of what it cannot.
func TestVtvibeCostText(t *testing.T) {
	text := aiCostText([]vtvibe.ModelCost{
		{Model: "paid", Usage: vtvibe.Usage{In: 12345, Out: 678}, Cost: 0.25, Priced: true},
		{Model: "local", Usage: vtvibe.Usage{In: 10, Out: 2}},
	})
	if !strings.Contains(text, "paid") || !strings.Contains(text, "12.3k") || !strings.Contains(text, "$0.2500") ||
		!strings.Contains(text, "local") || !strings.Contains(text, "12.4k") {
		t.Fatalf("cost text %q", text)
	}
}

// f4#1842, stage H9: the model menu lists free models first, in the order
// its rows are printed, so a row's index names its model.
func TestVtvibeModelsMenuOrder(t *testing.T) {
	ordered := aiModelsInOrder([]vtvibe.ModelInfo{{ID: "a"}, {ID: "b:free", Free: true}, {ID: "c"}, {ID: "d:free", Free: true}})
	lines := aiModelLines(ordered, len(ordered))
	if len(ordered) != 4 || ordered[0].ID != "b:free" || ordered[1].ID != "d:free" || ordered[2].ID != "a" || ordered[3].ID != "c" {
		t.Fatalf("order %v", ordered)
	}
	for i, m := range ordered {
		if !strings.Contains(lines[i], m.ID) {
			t.Fatalf("row %d %q is not model %q", i, lines[i], m.ID)
		}
	}
}
