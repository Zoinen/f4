//go:generate go -C tools/icons run .

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func main() {
	vtui.AppName = "f4"
	var sudoDispatcher string

	// Initialize SudoClient immediately for all process types
	execPath, err := os.Executable()
	if err != nil {
		execPath = os.Args[0]
	}
	absExecPath, _ := filepath.Abs(execPath)
	vfs.InitSudoClient(absExecPath, "")

	if os.Getenv("F4_ASKPASS_PARENT") != "" {
		vfs.RunSudoAskpass()
		return
	}

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--sudo-dispatcher" {
			if i+1 < len(os.Args) {
				sudoDispatcher = os.Args[i+1]
			}
			break
		} else if strings.HasPrefix(arg, "--sudo-dispatcher=") {
			sudoDispatcher = arg[len("--sudo-dispatcher="):]
			break
		}
	}
	if sudoDispatcher != "" {
		vfs.RunSudoDispatcher(sudoDispatcher)
		return
	}

	// Setup crash/stderr location before any logging starts; in portable mode
	// this keeps crash reports inside <configDir>\crashes (Profile\crashes).
	vtui.CrashDirFull = filepath.Join(GetF4ConfigDir(), "crashes")

	vtui.SetupStderrLog()
	vtui.DebugLog("MAIN: Starting with args: %v", os.Args)
	LoadConfig() // Load config early to apply GUI font settings

	defer func() {
		SaveSession() // Гарантирует сохранение размеров и путей при любом выходе
		if r := recover(); r != nil {
			vtui.DebugLog("FATAL PANIC IN MAIN: %v", r)
			crashPath := vtui.RecordCrash(r, nil)
			vtui.Suspend()
			// We print to os.Stdout here because os.Stderr is redirected to the log file!
			fmt.Fprintf(os.Stdout, "\n[f4] FATAL PANIC IN MAIN: %v\n", r)
			if crashPath != "" {
				fmt.Fprintf(os.Stdout, "[f4] Crash report saved to: %s\n", crashPath)
			}
			vtui.CleanupStderrLog()
			os.Exit(2)
		}
		vtui.CleanupStderrLog()
	}()
	// Defer disk logging to prevent launcher processes from polluting rotation queue.
	// Logging will be enabled in InitCore() for workers and standalone sessions.
	vtui.ConfigDiskLogging(false)
	var serverPath, clientPath string
	var cpuprofile string
	var guiMode bool
	var guiBackend string
	var ttyMode bool
	var version bool
	var attachedMode bool

	exeName := filepath.Base(absExecPath)
	if strings.Contains(strings.ToLower(exeName), "gui") {
		guiMode = true
	}

	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]

		// Handle --flag=value format
		flagName := arg
		flagVal := ""
		if eqIdx := strings.IndexByte(arg, '='); eqIdx != -1 {
			flagName = arg[:eqIdx]
			flagVal = arg[eqIdx+1:]
		}

		switch flagName {
		case "-v", "--version":
			version = true
		case "--debug":
			os.Setenv("VTUI_DEBUG", "1")
		case "--gui":
			guiMode = true
			if flagVal != "" {
				guiBackend = flagVal
			} else if i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				guiBackend = os.Args[i+1]
				i++
			}
		case "--log":
			if flagVal != "" {
				os.Setenv("VTUI_DEBUG", flagVal)
			} else if i+1 < len(os.Args) {
				os.Setenv("VTUI_DEBUG", os.Args[i+1])
				i++
			}
		case "--server":
			if flagVal != "" {
				serverPath = flagVal
			} else if i+1 < len(os.Args) {
				serverPath = os.Args[i+1]
				i++
			}
		case "--client":
			if flagVal != "" {
				clientPath = flagVal
			} else if i+1 < len(os.Args) {
				clientPath = os.Args[i+1]
				i++
			}
		case "--input":
			if flagVal != "" {
				vtinput.InputMode = flagVal
			} else if i+1 < len(os.Args) {
				vtinput.InputMode = os.Args[i+1]
				i++
			}
		case "--cpuprofile":
			if flagVal != "" {
				cpuprofile = flagVal
			} else if i+1 < len(os.Args) {
				cpuprofile = os.Args[i+1]
				i++
			}
		case "--new-plugin":
			pluginName := flagVal
			if pluginName == "" && i+1 < len(os.Args) && !strings.HasPrefix(os.Args[i+1], "-") {
				pluginName = os.Args[i+1]
				i++
			}
			os.Exit(RunNewPlugin(pluginName, os.Stdout, os.Stderr))
		case "-test-plugins":
			vtui.ConfigDiskLogging(true)
			vtui.DebugLog("--- PLUGIN TEST MODE ---")
			pm := NewPluginManager()
			pm.LoadAll()
			pm.CloseAll()
			return
		case "--tty":
			ttyMode = true
		case "--attached":
			attachedMode = true
		case "--sudo-dispatcher":
			if flagVal != "" {
				sudoDispatcher = flagVal
			} else if i+1 < len(os.Args) {
				sudoDispatcher = os.Args[i+1]
				i++
			}
		}
	}

	if version {
		fmt.Println(getFormattedVersionInfo())
		return
	}

	for _, arg := range os.Args {
		if arg == "--askpass" {
			vfs.RunSudoAskpass()
			return
		}
	}

	if serverPath != "" {
		runServer(serverPath)
		return
	}
	if clientPath != "" {
		runClient(clientPath)
		return
	}
	if cpuprofile != "" {
		f, err := os.Create(cpuprofile)
		if err != nil {
			panic(err)
		}
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}

	if ttyMode {
		ManageSessions()
		return
	}

	if guiMode {
		checkAndDetach(attachedMode)
		if guiBackend != "" {
			if err := RunGui(guiBackend); err != nil {
				fmt.Fprintf(os.Stderr, "\n[f4] FATAL GUI ERROR: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := tryRunDefaultGui(); err != nil {
				fmt.Fprintf(os.Stderr, "\n[f4] FATAL GUI ERROR: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	// Default auto-detect mode (neither --gui nor --tty specified)
	if shouldTryGui() {
		checkAndDetach(attachedMode)
		if err := tryRunDefaultGui(); err != nil {
			vtui.DebugLog("MAIN: GUI auto-detect failed after detach: %v", err)
			os.Exit(1)
		}
		return
	}

	vtui.DebugLog("MAIN: Falling back to console mode")
	ManageSessions()
}

func shouldTryGui() bool {
	if runtime.GOOS == "windows" {
		// On Windows, we compile separate binaries for console (f4.exe) and GUI (f4-gui.exe).
		// We do not auto-detect GUI mode; it must be requested via filename or --gui flag.
		return false
	}
	if runtime.GOOS == "darwin" {
		return true
	}
	return os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") != ""
}

func tryRunDefaultGui() error {
	var errs []string
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		vtui.DebugLog("GUI_AUTO: Trying gogpu...")
		if err := RunGui("gogpu"); err == nil {
			return nil
		} else {
			errs = append(errs, fmt.Sprintf("gogpu: %v", err))
		}
		if os.Getenv("DISPLAY") != "" {
			vtui.DebugLog("GUI_AUTO: Trying x11...")
			if err := RunGui("x11"); err == nil {
				return nil
			} else {
				errs = append(errs, fmt.Sprintf("x11: %v", err))
			}
		}
	} else {
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			vtui.DebugLog("GUI_AUTO: Trying wayland...")
			if err := RunGui("wayland"); err == nil {
				return nil
			} else {
				errs = append(errs, fmt.Sprintf("wayland: %v", err))
			}
		}
		if os.Getenv("DISPLAY") != "" {
			vtui.DebugLog("GUI_AUTO: Trying x11...")
			if err := RunGui("x11"); err == nil {
				return nil
			} else {
				errs = append(errs, fmt.Sprintf("x11: %v", err))
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("all GUI backends failed: %s", strings.Join(errs, "; "))
	}
	return fmt.Errorf("no suitable GUI environment detected")
}

func InitCore() *vtui.ScreenBuf {
	// Environment Diagnostics
	vtui.DebugLog("ENV: OS=%s ARCH=%s", runtime.GOOS, runtime.GOARCH)
	if wt := os.Getenv("WT_SESSION"); wt != "" {
		vtui.DebugLog("ENV: Running inside Windows Terminal (WT_SESSION set)")
	}
	if term := os.Getenv("TERM"); term != "" {
		vtui.DebugLog("ENV: TERM=%s", term)
	}
	width, height, err := vtui.GetTerminalSize()
	if err != nil {
		vtui.DebugLog("CORE: term.GetSize(0) failed: %v", err)
	}
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}

	scr := vtui.NewScreenBuf()
	scr.AllocBuf(width, height)

	vtui.FrameManager.Init(scr)

	SetupUI()

	vtui.DebugLog("CORE: Initialization complete")
	return scr
}

func SetupUI() {
	vtui.ConfigDiskLogging(os.Getenv("VTUI_DEBUG") != "")
	vtui.DebugLog("=== F4 STARTUP [%s] PID:%d ===", getFormattedVersionInfo(), os.Getpid())

	SetDefaultF4Palette()
	LoadConfig()
	InitLang()
	InitHelpSystem()
	if err := ApplyColorStyle(AppConfig.ColorStyle); err != nil {
		vtui.DebugLog("COLORS: %v; falling back to Modern", err)
		AppConfig.ColorStyle = "Modern"
		_ = ApplyColorStyle(AppConfig.ColorStyle)
	}
	vtui.GlobalHistoryProvider = NewF4HistoryProvider()
	GlobalFileState = NewF4FileStateProvider()
	vtinput.Logger = vtui.DebugLog // Pipe vtinput logs to vtui's debug logger
	vtui.GlobalClipboardAccessManager = NewF4ClipboardAuth()
	RegisterDrive("Null VFS", func() vfs.VFS { return vfs.NewNullVFS(50 * 1024 * 1024) }) // 50 MB/s

	configDir := GetF4ConfigDir()

	// Initialize File Highlighting
	highlightPath := filepath.Join(configDir, "highlight.ini")
	if _, err := os.Stat(highlightPath); os.IsNotExist(err) {
		createDefaultHighlightIni(highlightPath)
	}
	if _, err := os.Stat(highlightPath); err == nil {
		highlightIni := LoadIni(highlightPath)
		GlobalFileHighlighter.LoadFromIni(highlightIni)
	}

	// CrashDirFull задаётся рано (см. main()); здесь только повторная
	// синхронизация для vfs, чтобы конфиг портативного режима был единым.
	vfs.CustomConfigDir = configDir

	// Load legacy color overrides if they exist
	legacyColorsPath := filepath.Join(configDir, "farcolors.ini")
	if _, err := os.Stat(legacyColorsPath); err == nil {
		legacyIni := LoadIni(legacyColorsPath)
		InitColors(legacyIni)
	}

	os.MkdirAll(configDir, 0755)
	GlobalHotkeysMgr = NewHotkeyManager(filepath.Join(configDir, "hotkeys.ini"))
	MacroMgr = NewMacroManager(filepath.Join(configDir, "key_macros.ini"))
	MacroMgr.LoadLuaMacros(filepath.Join(configDir, "Macros", "scripts"))
	vtui.FrameManager.EventFilter = MacroMgr.Filter
	LoadSession()
	vtui.ManageCursorStyle = !AppConfig.KeepTerminalCursor
	vtui.FrameManager.Push(vtui.NewDesktop())

	width := vtui.FrameManager.GetScreenSize()
	height := vtui.FrameManager.GetScreenHeight()

	panels := NewPanelsFrame()
	panels.ResizeConsole(width, height)
	if AppConfig.SavePanelPaths {
		lp := panels.panels[0].(*FileSystemPanel)
		rp := panels.panels[1].(*FileSystemPanel)

		// Восстанавливаем режимы отображения и типы сортировки панелей
		leftMode := ViewMode(LastLeftViewMode)
		if leftMode != ViewModeMedium && leftMode != ViewModeDetailed && leftMode != ViewModeBrief {
			leftMode = ViewModeMedium
		}
		lp.SetViewMode(leftMode)
		lp.sortMode = SortMode(LastLeftSortMode)
		lp.sortReverse = LastLeftSortRev

		rightMode := ViewMode(LastRightViewMode)
		if rightMode != ViewModeMedium && rightMode != ViewModeDetailed && rightMode != ViewModeBrief {
			rightMode = ViewModeMedium
		}
		rp.SetViewMode(rightMode)
		rp.sortMode = SortMode(LastRightSortMode)
		rp.sortReverse = LastRightSortRev

		if LastLeftPath != "" && panels.NavigateToPath(lp, LastLeftPath) {
			// Navigated successfully
		} else {
			if LastLeftPath != "" {
				lp.vfs.SetPath(LastLeftPath)
			}
			lp.ReadDirectory()
		}
		if LastRightPath != "" && panels.NavigateToPath(rp, LastRightPath) {
			// Navigated successfully
		} else {
			if LastRightPath != "" {
				rp.vfs.SetPath(LastRightPath)
			}
			rp.ReadDirectory()
		}
		lp.pendingSelection = LastLeftCursor
		rp.pendingSelection = LastRightCursor
		panels.activeIdx = LastActivePanel

		panels.showPanels = LastShowPanels
		panels.showLeftPanel = LastShowLeft
		panels.showRightPanel = LastShowRight
		if LastWidePanel == 0 || LastWidePanel == 1 {
			panels.wide = true
			panels.widePanel = LastWidePanel
			panels.activeIdx = LastWidePanel
			panels.showPanels = true
		}
		panels.ResizeConsole(width, height)
	}
	vtui.FrameManager.Push(panels)

	vtui.FrameManager.MenuBar = panels.menuBar
	vtui.FrameManager.KeyBar = panels.keyBar
	vtui.FrameManager.OnRender = UpdateWindowTitle

	noPlugins := false
	for _, arg := range os.Args {
		if arg == "--no-plugins" {
			noPlugins = true
			break
		}
	}

	if !noPlugins {
		GlobalPluginManager = NewPluginManager()
		go GlobalPluginManager.LoadAll()
	} else {
		vtui.DebugLog("CORE: Plugins disabled by --no-plugins flag")
	}

	// Background update check
	if AppConfig.UpdateInterval > 0 {
		go CheckForUpdates(panels, false)
		go CheckForPluginUpdates()
	}
}

var getSessionIniPath = func() string {
	return filepath.Join(GetF4ConfigDir(), "session.ini")
}

func LoadSession() {
	path := getSessionIniPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}
	ini := LoadIni(path)

	LastEditorSearch = ini.GetString("EditorSearch", "Pattern", "")
	LastEditorSearchCase = ini.GetString("EditorSearch", "CaseSensitive", "0") == "1"
	LastEditorSearchReverse = ini.GetString("EditorSearch", "Reverse", "0") == "1"
	LastEditorSearchRegexp = ini.GetString("EditorSearch", "Regexp", "0") == "1"
	LastEditorSearchWholeWord = ini.GetString("EditorSearch", "WholeWord", "0") == "1"

	LastFindFileMask = ini.GetString("FindFile", "Mask", "*")
	LastFindFileText = ini.GetString("FindFile", "Text", "")

	// Восстанавливаем состояние левой панели
	LastLeftPath = ini.GetString("Panel/Left", "Folder", "")
	LastLeftCursor = ini.GetString("Panel/Left", "CurFile", "")
	fmt.Sscanf(ini.GetString("Panel/Left", "ViewMode", "0"), "%d", &LastLeftViewMode)
	fmt.Sscanf(ini.GetString("Panel/Left", "SortMode", "0"), "%d", &LastLeftSortMode)
	LastLeftSortRev = ini.GetString("Panel/Left", "SortReverse", "0") == "1"

	// Восстанавливаем состояние правой панели
	LastRightPath = ini.GetString("Panel/Right", "Folder", "")
	LastRightCursor = ini.GetString("Panel/Right", "CurFile", "")
	fmt.Sscanf(ini.GetString("Panel/Right", "ViewMode", "0"), "%d", &LastRightViewMode)
	fmt.Sscanf(ini.GetString("Panel/Right", "SortMode", "0"), "%d", &LastRightSortMode)
	LastRightSortRev = ini.GetString("Panel/Right", "SortReverse", "0") == "1"

	// Восстанавливаем глобальное состояние сессии
	activeStr := ini.GetString("Session", "ActivePanel", "1")
	fmt.Sscanf(activeStr, "%d", &LastActivePanel)
	LastWidePanel = -1
	fmt.Sscanf(ini.GetString("Session", "WidePanel", "-1"), "%d", &LastWidePanel)
	if LastWidePanel < -1 || LastWidePanel > 1 {
		LastWidePanel = -1
	}
	LastShowPanels = ini.GetString("Session", "ShowPanels", "1") == "1"
	LastShowLeft = ini.GetString("Session", "ShowLeft", "1") == "1"
	LastShowRight = ini.GetString("Session", "ShowRight", "1") == "1"

	vtui.DebugLog("SESSION: Loaded state from %s", path)
}

func SaveSession() {
	path := getSessionIniPath()
	os.MkdirAll(filepath.Dir(path), 0755)

	if vtui.FrameManager != nil {
		w := vtui.FrameManager.GetScreenSize()
		h := vtui.FrameManager.GetScreenHeight()
		if w > 0 && h > 0 {
			if AppConfig.GuiCols != w || AppConfig.GuiRows != h {
				AppConfig.GuiCols = w
				AppConfig.GuiRows = h
				SaveConfig()
			}
		}
	}

	if AppConfig.SavePanelPaths && vtui.FrameManager != nil {
		for _, s := range vtui.FrameManager.Screens {
			for _, f := range s.Frames {
				if pf, ok := f.(*PanelsFrame); ok {
					LastLeftPath, LastRightPath = pf.GetPaths()
					LastActivePanel = pf.activeIdx
					LastWidePanel = -1
					if pf.wide {
						LastWidePanel = pf.widePanel
					}
					LastShowPanels = pf.showPanels
					LastShowLeft = pf.showLeftPanel
					LastShowRight = pf.showRightPanel
					if fsp, ok := pf.panels[0].(*FileSystemPanel); ok {
						LastLeftCursor = fsp.GetSelectedName()
						LastLeftViewMode = int(fsp.viewMode)
						LastLeftSortMode = int(fsp.sortMode)
						LastLeftSortRev = fsp.sortReverse
					}
					if fsp, ok := pf.panels[1].(*FileSystemPanel); ok {
						LastRightCursor = fsp.GetSelectedName()
						LastRightViewMode = int(fsp.viewMode)
						LastRightSortMode = int(fsp.sortMode)
						LastRightSortRev = fsp.sortReverse
					}
					goto found
				}
			}
		}
	found:
	}

	var sb strings.Builder
	sb.WriteString("[EditorSearch]\n")
	sb.WriteString(fmt.Sprintf("Pattern = %s\n", LastEditorSearch))
	sb.WriteString(fmt.Sprintf("CaseSensitive = %d\n", map[bool]int{true: 1, false: 0}[LastEditorSearchCase]))
	sb.WriteString(fmt.Sprintf("Reverse = %d\n", map[bool]int{true: 1, false: 0}[LastEditorSearchReverse]))
	sb.WriteString(fmt.Sprintf("Regexp = %d\n", map[bool]int{true: 1, false: 0}[LastEditorSearchRegexp]))
	sb.WriteString(fmt.Sprintf("WholeWord = %d\n", map[bool]int{true: 1, false: 0}[LastEditorSearchWholeWord]))

	sb.WriteString("\n[FindFile]\n")
	sb.WriteString(fmt.Sprintf("Mask = %s\n", LastFindFileMask))
	sb.WriteString(fmt.Sprintf("Text = %s\n", LastFindFileText))

	sb.WriteString("\n[Session]\n")
	sb.WriteString(fmt.Sprintf("ActivePanel = %d\n", LastActivePanel))
	sb.WriteString(fmt.Sprintf("WidePanel = %d\n", LastWidePanel))
	sb.WriteString(fmt.Sprintf("ShowPanels = %d\n", map[bool]int{true: 1, false: 0}[LastShowPanels]))
	sb.WriteString(fmt.Sprintf("ShowLeft = %d\n", map[bool]int{true: 1, false: 0}[LastShowLeft]))
	sb.WriteString(fmt.Sprintf("ShowRight = %d\n", map[bool]int{true: 1, false: 0}[LastShowRight]))

	sb.WriteString("\n[Panel/Left]\n")
	sb.WriteString(fmt.Sprintf("Folder = %s\n", LastLeftPath))
	sb.WriteString(fmt.Sprintf("CurFile = %s\n", LastLeftCursor))
	sb.WriteString(fmt.Sprintf("ViewMode = %d\n", LastLeftViewMode))
	sb.WriteString(fmt.Sprintf("SortMode = %d\n", LastLeftSortMode))
	sb.WriteString(fmt.Sprintf("SortReverse = %d\n", map[bool]int{true: 1, false: 0}[LastLeftSortRev]))

	sb.WriteString("\n[Panel/Right]\n")
	sb.WriteString(fmt.Sprintf("Folder = %s\n", LastRightPath))
	sb.WriteString(fmt.Sprintf("CurFile = %s\n", LastRightCursor))
	sb.WriteString(fmt.Sprintf("ViewMode = %d\n", LastRightViewMode))
	sb.WriteString(fmt.Sprintf("SortMode = %d\n", LastRightSortMode))
	sb.WriteString(fmt.Sprintf("SortReverse = %d\n", map[bool]int{true: 1, false: 0}[LastRightSortRev]))

	err := os.WriteFile(path, []byte(sb.String()), 0644)
	if err != nil {
		vtui.DebugLog("SESSION: Failed to save state: %v", err)
		return
	}

	vtui.DebugLog("SESSION: Saved state to %s", path)
}

func getFormattedVersionInfo() string {
	return getLongVersionInfo()
}

func formatVersionSHA(v string) string {
	runes := []rune(v)
	var res []rune
	i := 0
	for i < len(runes) {
		if i+8 <= len(runes) && isHexSequence(runes[i:i+8]) {
			isStandalone := true
			if i > 0 && isHexChar(runes[i-1]) {
				isStandalone = false
			}
			if i+8 < len(runes) && isHexChar(runes[i+8]) {
				isStandalone = false
			}
			if isStandalone {
				res = append(res, runes[i:i+7]...)
				i += 8
				continue
			}
		}
		res = append(res, runes[i])
		i++
	}
	return string(res)
}

func isHexSequence(s []rune) bool {
	for _, r := range s {
		if !isHexChar(r) {
			return false
		}
	}
	return true
}

func isHexChar(r rune) bool {
	return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
}
func createDefaultHighlightIni(path string) {
	content := `[Highlight_0]
Name = Executables
Mask = *.exe, *.bat, *.cmd, *.sh, *.bash
IncludeAttributes = Executable
NormalColor = foreground:#8AE234
SelectedColor = foreground:#8AE234 | background:#0000A0
CursorColor = foreground:#8AE234 | background:#00AAAA

[Highlight_1]
Name = Archives
Mask = *.zip, *.rar, *.tar, *.gz, *.7z, *.tgz, *.bz2, *.xz, *.zst
NormalColor = foreground:#AD7FA8
SelectedColor = foreground:#AD7FA8 | background:#0000A0
CursorColor = foreground:#AD7FA8 | background:#00AAAA

[Highlight_2]
Name = Hidden Files
IncludeAttributes = Hidden
NormalColor = foreground:#729FCF
SelectedColor = foreground:#729FCF | background:#0000A0
CursorColor = foreground:#729FCF | background:#00AAAA

[Highlight_3]
Name = Directories
IncludeAttributes = Directory
NormalColor = foreground:#FFFFFF
SelectedColor = foreground:#FFFFFF | background:#0000A0
CursorColor = foreground:#FFFFFF | background:#00AAAA
`
	_ = os.WriteFile(path, []byte(content), 0644)
}
