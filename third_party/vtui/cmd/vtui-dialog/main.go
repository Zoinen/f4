package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/term"
)

type localFS struct{ path string }

func (v *localFS) GetPath() string         { return v.path }
func (v *localFS) SetPath(p string) error  { v.path = p; return nil }
func (v *localFS) Join(e ...string) string { return filepath.Join(e...) }
func (v *localFS) Dir(p string) string     { return filepath.Dir(p) }
func (v *localFS) Base(p string) string    { return filepath.Base(p) }
func (v *localFS) ReadDir(ctx context.Context, p string, onChunk func([]vtui.FSItem)) error {
	entries, err := os.ReadDir(p)
	if err != nil {
		return err
	}
	var items []vtui.FSItem
	for _, e := range entries {
		items = append(items, vtui.FSItem{Name: e.Name(), IsDir: e.IsDir()})
	}
	if len(items) > 0 && onChunk != nil {
		onChunk(items)
	}
	return nil
}

type dialogConfig struct {
	title       string
	width       int
	height      int
	okLabel     string
	cancelLabel string
	extraLabel  string
	backend     string
	jsonOut     bool

	msgbox      string
	yesno       string
	inputbox    string
	passwordbox string
	filebox     string
	dirbox      string
	vuiPath     string

	menuTitle string
	menuItems []string

	checkTitle string
	checkItems []string

	radioTitle string
	radioItems []string
}

type dialogResult struct {
	ExitCode int            `json:"exitCode"`
	Value    string         `json:"value,omitempty"`
	Values   map[string]any `json:"values,omitempty"`
}

func main() {
	cfg := parseFlags()

	if len(os.Args) == 1 {
		printUsage()
		os.Exit(1)
	}

	res := runDialog(cfg)

	if cfg.jsonOut {
		data, _ := json.Marshal(res)
		fmt.Println(string(data))
	} else if res.Value != "" {
		fmt.Println(res.Value)
	}

	os.Exit(res.ExitCode)
}

func parseFlags() dialogConfig {
	var cfg dialogConfig

	flag.StringVar(&cfg.title, "title", " Dialog ", "Dialog title")
	flag.IntVar(&cfg.width, "width", 0, "Dialog width in character cells (0 = auto)")
	flag.IntVar(&cfg.height, "height", 0, "Dialog height in character cells (0 = auto)")
	flag.StringVar(&cfg.okLabel, "ok-label", "&Ok", "Custom label for OK button")
	flag.StringVar(&cfg.cancelLabel, "cancel-label", "Cancel", "Custom label for Cancel button")
	flag.StringVar(&cfg.extraLabel, "extra-button", "", "Add an extra action button with label")
	flag.StringVar(&cfg.backend, "backend", "", "Rendering backend: ansi, winapi, gogpu, x11, wayland, ebiten")
	flag.StringVar(&cfg.backend, "tty", "", "TTY rendering mode: ansi, winapi (default: ansi, or winapi under Wine)")
	flag.BoolVar(&cfg.jsonOut, "json", false, "Output result formatted as JSON")

	flag.StringVar(&cfg.msgbox, "msgbox", "", "Display a message box with text")
	flag.StringVar(&cfg.msgbox, "info", "", "Alias for --msgbox")
	flag.StringVar(&cfg.yesno, "yesno", "", "Display a Yes/No confirmation question")
	flag.StringVar(&cfg.inputbox, "inputbox", "", "Display a text input dialog with prompt")
	flag.StringVar(&cfg.passwordbox, "passwordbox", "", "Display a password input dialog with prompt")
	flag.StringVar(&cfg.filebox, "filebox", "", "Display a file selection dialog starting at path")
	flag.StringVar(&cfg.dirbox, "dirbox", "", "Display a directory selection dialog starting at path")
	flag.StringVar(&cfg.vuiPath, "vui", "", "Load and display a declarative .vui document")

	flag.StringVar(&cfg.menuTitle, "menu", "", "Display a menu dialog: --menu <title> <tag1> <item1> ...")
	flag.StringVar(&cfg.checkTitle, "checklist", "", "Display a checklist: --checklist <title> <tag1> <item1> <on|off> ...")
	flag.StringVar(&cfg.radioTitle, "radiolist", "", "Display a radiolist: --radiolist <title> <tag1> <item1> <on|off> ...")

	flag.Parse()
	args := flag.Args()

	if cfg.menuTitle != "" {
		cfg.menuItems = args
	} else if cfg.checkTitle != "" {
		cfg.checkItems = args
	} else if cfg.radioTitle != "" {
		cfg.radioItems = args
	}

	return cfg
}

func printUsage() {
	fmt.Fprintf(os.Stderr, "vtui-dialog: Desktop-class TUI dialogs for shell scripting\n\n")
	fmt.Fprintf(os.Stderr, "Usage examples:\n")
	fmt.Fprintf(os.Stderr, "  vtui-dialog --msgbox \"Operation completed!\"\n")
	fmt.Fprintf(os.Stderr, "  vtui-dialog --yesno \"Delete all files?\"\n")
	fmt.Fprintf(os.Stderr, "  NAME=$(vtui-dialog --inputbox \"Enter your name:\" \"Alice\")\n")
	fmt.Fprintf(os.Stderr, "  PASS=$(vtui-dialog --passwordbox \"Enter secret token:\")\n")
	fmt.Fprintf(os.Stderr, "  FILE=$(vtui-dialog --filebox .)\n")
	fmt.Fprintf(os.Stderr, "  CHOICE=$(vtui-dialog --menu \"Options\" 1 \"First\" 2 \"Second\")\n")
	fmt.Fprintf(os.Stderr, "  RES=$(vtui-dialog --vui=form.vui --json)\n\n")
	flag.PrintDefaults()
}

func runDialog(cfg dialogConfig) dialogResult {
	var result dialogResult
	result.ExitCode = 1 // Default to cancel/escape

	if cfg.backend == "" {
		cfg.backend = vtui.DefaultConsoleBackend()
	}

	runUI := func() {
		width, height, _ := term.GetSize(int(os.Stdin.Fd()))
		if width <= 0 || height <= 0 {
			width, height = 80, 25
		}

		scr := vtui.NewScreenBuf()
		if cfg.backend == "winapi" || cfg.backend == "win32" {
			scr.Renderer = vtui.NewWin32ConsoleRenderer(scr)
		}
		scr.AllocBuf(width, height)
		vtui.FrameManager.Init(scr)
		vtui.FrameManager.Push(vtui.NewDesktop())

		dlg := buildDialog(cfg, width, height, &result)
		if dlg == nil {
			vtui.FrameManager.Shutdown()
			return
		}

		dlg.OnResult = func(code int) {
			result.ExitCode = code
		}
		vtui.FrameManager.Push(dlg)
	}

	if cfg.backend != "ansi" && cfg.backend != "winapi" && cfg.backend != "win32" && cfg.backend != "" {
		_ = vtui.RunInGUIWindow(80, 25, cfg.backend, "", 18.0, runUI)
	} else {
		restore, err := vtui.PrepareTerminal()
		if err == nil && restore != nil {
			defer restore()
		}
		runUI()
		reader := vtinput.NewReader(os.Stdin, false)
		vtui.FrameManager.Run(reader)
	}

	return result
}

func buildDialog(cfg dialogConfig, scrW, scrH int, res *dialogResult) *vtui.Window {
	if cfg.vuiPath != "" {
		win, err := vtui.LoadDialogFile(cfg.vuiPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "vtui-dialog: error loading %s: %v\n", cfg.vuiPath, err)
			return nil
		}
		win.OnResult = func(code int) {
			res.ExitCode = code
			res.Values = make(map[string]any)
			for _, child := range win.GetChildren() {
				collectValues(child, res.Values)
			}
		}
		return win
	}

	if cfg.msgbox != "" {
		buttons := []string{cfg.okLabel}
		if cfg.extraLabel != "" {
			buttons = append(buttons, cfg.extraLabel)
		}
		dlg := vtui.ShowMessage(cfg.title, cfg.msgbox, buttons)
		dlg.OnResult = func(code int) {
			if code == 0 {
				res.ExitCode = 0
			} else if code == 1 && cfg.extraLabel != "" {
				res.ExitCode = 2
			} else {
				res.ExitCode = 1
			}
		}
		return dlg
	}

	if cfg.yesno != "" {
		buttons := []string{"&Yes", "&No"}
		if cfg.extraLabel != "" {
			buttons = append(buttons, cfg.extraLabel)
		}
		dlg := vtui.ShowMessage(cfg.title, cfg.yesno, buttons)
		dlg.OnResult = func(code int) {
			if code == 0 {
				res.ExitCode = 0
			} else if code == 1 {
				res.ExitCode = 1
			} else {
				res.ExitCode = 2
			}
		}
		return dlg
	}

	if cfg.inputbox != "" || cfg.passwordbox != "" {
		isPass := cfg.passwordbox != ""
		prompt := cfg.inputbox
		if isPass {
			prompt = cfg.passwordbox
		}

		defaultVal := ""
		if flag.NArg() > 0 {
			defaultVal = flag.Arg(0)
		}

		w := cfg.width
		if w <= 0 {
			w = 44
		}
		h := cfg.height
		if h <= 0 {
			h = 9
		}

		dlg := vtui.NewCenteredDialog(w, h, cfg.title)
		dlg.ShowClose = true

		var edit *vtui.Edit
		if isPass {
			edit = vtui.NewPasswordEdit(0, 0, 10, defaultVal)
		} else {
			edit = vtui.NewEdit(0, 0, 10, defaultVal)
		}
		lbl := vtui.NewLabel(0, 0, prompt, edit)
		btnOk := vtui.NewButton(0, 0, cfg.okLabel)
		btnCancel := vtui.NewButton(0, 0, cfg.cancelLabel)

		btnOk.OnClick = func() {
			res.ExitCode = 0
			res.Value = edit.GetText()
			dlg.Close()
		}
		btnCancel.OnClick = func() {
			res.ExitCode = 1
			dlg.Close()
		}

		dlg.AddItem(lbl)
		dlg.AddItem(edit)
		dlg.AddItem(btnOk)
		dlg.AddItem(btnCancel)

		layout := vtui.NewAutoLayout(dlg.X1+2, dlg.Y1+2, w-4, h-4)
		layout.
			PinTop(lbl, 0).PinLeft(lbl, 0).
			StackVertical(1, lbl, edit).FillWidth(edit, 0, 0).
			PinBottom(btnOk, 0).PinBottom(btnCancel, 0).
			StackHorizontal(2, btnOk, btnCancel).
			CenterHorizontalGroup(btnOk, btnCancel)
		layout.Apply()

		return dlg
	}

	if cfg.menuTitle != "" {
		w := cfg.width
		if w <= 0 {
			w = 40
		}
		h := cfg.height
		if h <= 0 {
			h = 12
		}

		var tags []string
		var items []string
		for i := 0; i+1 < len(cfg.menuItems); i += 2 {
			tags = append(tags, cfg.menuItems[i])
			items = append(items, cfg.menuItems[i+1])
		}

		dlg := vtui.NewCenteredDialog(w, h, cfg.title)
		dlg.ShowClose = true

		lb := vtui.NewListBox(0, 0, 10, 5, items)
		btnOk := vtui.NewButton(0, 0, cfg.okLabel)
		btnCancel := vtui.NewButton(0, 0, cfg.cancelLabel)

		btnOk.OnClick = func() {
			res.ExitCode = 0
			if lb.SelectPos >= 0 && lb.SelectPos < len(tags) {
				res.Value = tags[lb.SelectPos]
			}
			dlg.Close()
		}
		btnCancel.OnClick = func() {
			res.ExitCode = 1
			dlg.Close()
		}

		lb.OnAction = func(idx int) {
			btnOk.OnClick()
		}

		dlg.AddItem(lb)
		dlg.AddItem(btnOk)
		dlg.AddItem(btnCancel)

		layout := vtui.NewAutoLayout(dlg.X1+2, dlg.Y1+2, w-4, h-4)
		layout.
			PinTop(lb, 0).FillWidth(lb, 0, 0).
			PinBottom(btnOk, 0).PinBottom(btnCancel, 0).
			StackHorizontal(2, btnOk, btnCancel).
			CenterHorizontalGroup(btnOk, btnCancel).
			StackVertical(1, lb, btnOk)
		layout.Apply()

		return dlg
	}

	if cfg.filebox != "" {
		initial := cfg.filebox
		if initial == "" {
			initial = "."
		}
		abs, err := filepath.Abs(initial)
		if err == nil {
			initial = abs
		}
		vfs := &localFS{path: initial}
		dlg := vtui.SelectFileDialog(cfg.title, initial, vfs, func(chosen string) {
			res.ExitCode = 0
			res.Value = chosen
		})
		return dlg
	}

	if cfg.dirbox != "" {
		initial := cfg.dirbox
		if initial == "" {
			initial = "."
		}
		abs, err := filepath.Abs(initial)
		if err == nil {
			initial = abs
		}
		vfs := &localFS{path: initial}
		dlg := vtui.SelectDirDialog(cfg.title, initial, vfs)
		dlg.OnResult = func(code int) {
			if code == 1 {
				res.ExitCode = 0
				res.Value = vfs.GetPath()
			} else {
				res.ExitCode = 1
			}
		}
		return dlg
	}

	return nil
}

func collectValues(el vtui.UIElement, out map[string]any) {
	if el == nil {
		return
	}
	id := el.GetId()
	if id != "" && !strings.HasPrefix(id, "auto:") {
		if dc, ok := el.(vtui.DataControl); ok {
			out[id] = dc.GetData()
		}
	}
	if c, ok := el.(vtui.Container); ok {
		for _, child := range c.GetChildren() {
			collectValues(child, out)
		}
	}
}
