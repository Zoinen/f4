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

	vtui.SetupStderrLog()
	vtui.DebugLog("MAIN: Starting with args: %v", os.Args)

	defer func() {
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
		case "-test-plugins":
			vtui.ConfigDiskLogging(true)
			vtui.DebugLog("--- PLUGIN TEST MODE ---")
			pm := NewPluginManager()
			pm.LoadAll()
			pm.CloseAll()
			return
		case "--tty":
			ttyMode = true
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
		fmt.Println(vtui.GetVersionInfo())
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
	if err := tryRunDefaultGui(); err != nil {
		vtui.DebugLog("MAIN: Falling back to console mode: %v", err)
		ManageSessions()
	}
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
	vtui.ConfigDiskLogging(true)
	vtui.DebugLog("=== F4 STARTUP [%s] PID:%d ===", vtui.GetVersionInfo(), os.Getpid())

	SetDefaultF4Palette()
	InitLang()
	LoadConfig()
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

	configDir, _ := os.UserConfigDir()

	os.MkdirAll(filepath.Join(configDir, "f4"), 0755)
	MacroMgr = NewMacroManager(filepath.Join(configDir, "f4", "key_macros.ini"))
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
	}
	vtui.FrameManager.Push(panels)

	vtui.FrameManager.MenuBar = panels.menuBar
	vtui.FrameManager.KeyBar = panels.keyBar

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
}

var getSessionIniPath = func() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "f4", "session.ini")
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

	LastFindFileMask = ini.GetString("FindFile", "Mask", "*")
	LastFindFileText = ini.GetString("FindFile", "Text", "")

	LastLeftPath = ini.GetString("Session", "LeftPath", "")
	LastRightPath = ini.GetString("Session", "RightPath", "")
	LastLeftCursor = ini.GetString("Session", "LeftCursor", "")
	LastRightCursor = ini.GetString("Session", "RightCursor", "")
	activeStr := ini.GetString("Session", "ActivePanel", "1")
	fmt.Sscanf(activeStr, "%d", &LastActivePanel)

	vtui.DebugLog("SESSION: Loaded state from %s", path)
}

func SaveSession() {
	path := getSessionIniPath()
	os.MkdirAll(filepath.Dir(path), 0755)

	if AppConfig.SavePanelPaths && vtui.FrameManager != nil {
		for _, s := range vtui.FrameManager.Screens {
			for _, f := range s.Frames {
				if pf, ok := f.(*PanelsFrame); ok {
					LastLeftPath, LastRightPath = pf.GetPaths()
					LastActivePanel = pf.activeIdx
					if fsp, ok := pf.panels[0].(*FileSystemPanel); ok {
						LastLeftCursor = fsp.GetSelectedName()
					}
					if fsp, ok := pf.panels[1].(*FileSystemPanel); ok {
						LastRightCursor = fsp.GetSelectedName()
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

	sb.WriteString("\n[FindFile]\n")
	sb.WriteString(fmt.Sprintf("Mask = %s\n", LastFindFileMask))
	sb.WriteString(fmt.Sprintf("Text = %s\n", LastFindFileText))

	sb.WriteString("\n[Session]\n")
	sb.WriteString(fmt.Sprintf("LeftPath = %s\n", LastLeftPath))
	sb.WriteString(fmt.Sprintf("RightPath = %s\n", LastRightPath))
	sb.WriteString(fmt.Sprintf("LeftCursor = %s\n", LastLeftCursor))
	sb.WriteString(fmt.Sprintf("RightCursor = %s\n", LastRightCursor))
	sb.WriteString(fmt.Sprintf("ActivePanel = %d\n", LastActivePanel))

	err := os.WriteFile(path, []byte(sb.String()), 0644)
	if err != nil {
		vtui.DebugLog("SESSION: Failed to save state: %v", err)
		return
	}

	vtui.DebugLog("SESSION: Saved state to %s", path)
}
