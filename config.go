package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unxed/vtui"
)

var (
	cachedF4ConfigDir string
	configDirOnce     sync.Once
)

func GetF4ConfigDir() string {
	configDirOnce.Do(func() {
		exe, err := osExecutable()
		if err != nil {
			exe = os.Args[0]
		}
		exeDir := filepath.Dir(exe)

		// Ищем Far.exe.ini (имя_бинарника.ini) или f4.ini в папке программы
		iniPath := exe + ".ini"
		if _, err := os.Stat(iniPath); os.IsNotExist(err) {
			iniPath = filepath.Join(exeDir, "f4.ini")
		}

		useSystemProfiles := true
		if _, err := os.Stat(iniPath); err == nil {
			ini := ParseIni(bytesReader(iniPath))
			if ini.GetString("General", "UseSystemProfiles", "1") == "0" {
				useSystemProfiles = false
			}
		}

		if !useSystemProfiles {
			cachedF4ConfigDir = filepath.Join(exeDir, "Profile")
			_ = os.MkdirAll(cachedF4ConfigDir, 0755)
		} else {
			sysDir, _ := os.UserConfigDir()
			cachedF4ConfigDir = filepath.Join(sysDir, "f4")
		}
	})
	return cachedF4ConfigDir
}

func bytesReader(p string) io.Reader {
	b, _ := os.ReadFile(p)
	return bytes.NewReader(b)
}

func resetConfigDirForTest() {
	configDirOnce = sync.Once{}
	cachedF4ConfigDir = ""
}

type PanelScrollbarMode int

const (
	PanelScrollbarOff PanelScrollbarMode = iota
	PanelScrollbarMinimal
	PanelScrollbarFull
)

func (m PanelScrollbarMode) String() string {
	switch m {
	case PanelScrollbarMinimal:
		return "minimal"
	case PanelScrollbarFull:
		return "full"
	default:
		return "off"
	}
}

func ParsePanelScrollbarMode(value string) PanelScrollbarMode {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "minimal":
		return PanelScrollbarMinimal
	case "full":
		return PanelScrollbarFull
	default:
		return PanelScrollbarOff
	}
}

type F4Config struct {
	ColorStyle               string
	Language                 string
	HelpLanguage             string
	AlwaysShowMenuBar        bool
	ShowHiddenFiles          bool
	HighlightDir             bool
	SeparateFileExtensions   bool
	PanelScrollbarMode       PanelScrollbarMode
	SavePanelPaths           bool
	InfoPanelBytes           bool // Ctrl+L info panel: true = raw bytes, false = human (GiB/MiB…)
	InfoPanelCPUGPU          bool // Ctrl+L info panel: show CPU and GPU sections (off by default)
	EscTogglePanels          bool // ESC toggles panels visibility (Far ships this as a macro; on by default)
	KeepTerminalCursor       bool
	AnnounceKittyTerm        bool // introduce the built-in terminal as kitty, so that image tools use the graphics protocol
	CommandLineAutoComplete  bool
	NavigationMode           PanelNavigationMode
	SearchCommandStayFocused bool
	SyncPanelLoad            bool
	EditorAutoComplete       bool
	EditorAutoCompleteMask   string
	EditorExpandTabs         int
	EditorAutoIndent         bool
	EditorCursorBeyondEOL    bool
	EditorTabSize            int
	EditorUseEditorConfig    bool
	EditorCrosshair          bool
	UseExternalEditor        bool
	ExternalEditorCommand    string
	EditorAutodetectCodePage bool
	EditorHighlighter        string
	EditorColorerScheme      string
	EditorColorerBackground  bool
	EditorColorerSyntax      bool
	EditorColorerCatalog     string
	EditorCrossMode          int
	EditorDefaultCodePage    int
	ViewerAutodetectCodePage bool
	ViewerDefaultCodePage    int
	SlideShowDelay           int
	ImageExternalTimeout     int
	ImageDecoderPriority     string
	RegisteredPlugins        []string
	ConfirmCopy              bool
	ConfirmMove              bool
	ConfirmDelete            bool
	ConfirmExit              bool
	DeleteCancelFocused      bool
	DefaultFileOpMode        int
	FileOpPathDisplay        int
	MacroRecordFormat        int
	GuiFont                  string
	GuiFontSize              int
	GuiCols                  int
	GuiRows                  int
	ConsoleTitleTemplate     string
	UpdateChannel            int    // 0 = Stable, 1 = Nightly
	UpdateInterval           int    // 0 = Never, 1 = Every start, 2 = Daily, 3 = Weekly
	LastUpdateCheck          int64  // Unix timestamp
	LastUpdateVersion        string // Version string or PublishedAt timestamp

	// [Layout] mirrors far2l's config.ini section of the same name so
	// a config shared with far2l keeps working in both. Adjusted by
	// Ctrl+Left/Right (width split) and Ctrl+Up/Down (panel/terminal
	// vertical split, applied symmetrically to both height fields).
	// Ctrl+Clear resets all three to 0.
	WidthDecrement       int
	LeftHeightDecrement  int
	RightHeightDecrement int

	// LayoutExtras is any [Layout] key we don't recognise (e.g. far2l's
	// FullscreenHelp, PanelsDisposition). Read at LoadConfig and written
	// back verbatim on SaveConfig so f4 doesn't strip far2l-only options
	// from a shared config file.
	LayoutExtras map[string]string
}

var AppConfig = F4Config{
	ColorStyle:               "Modern",
	Language:                 "en",
	HelpLanguage:             "en",
	AlwaysShowMenuBar:        false,
	ShowHiddenFiles:          true,
	HighlightDir:             true,
	SeparateFileExtensions:   false,
	PanelScrollbarMode:       PanelScrollbarOff,
	SavePanelPaths:           true,
	InfoPanelBytes:           false,
	InfoPanelCPUGPU:          false,
	EscTogglePanels:          true,
	KeepTerminalCursor:       false,
	AnnounceKittyTerm:        true,
	CommandLineAutoComplete:  true,
	NavigationMode:           NavigationClassic,
	SearchCommandStayFocused: false,
	SyncPanelLoad:            false,
	EditorAutoComplete:       true,
	EditorAutoCompleteMask:   "*.go;*.c;*.cpp;*.h;*.hpp;*.py;*.js;*.ts;*.rs;*.java;*.sh;*.txt;*.md;*.html;*.css;*.json",
	EditorExpandTabs:         0,
	EditorAutoIndent:         true,
	EditorCursorBeyondEOL:    false,
	EditorTabSize:            4,
	EditorUseEditorConfig:    true,
	EditorCrosshair:          false,
	UseExternalEditor:        false,
	ExternalEditorCommand:    "",
	EditorAutodetectCodePage: true,
	EditorHighlighter:        "Chroma",
	EditorColorerScheme:      "",
	EditorColorerBackground:  true,
	EditorColorerSyntax:      true,
	EditorColorerCatalog:     "",
	EditorCrossMode:          ColorerCrossBoth,
	EditorDefaultCodePage:    65001,
	ViewerAutodetectCodePage: true,
	ViewerDefaultCodePage:    65001,
	SlideShowDelay:           defaultSlideShowDelay,
	ImageExternalTimeout:     defaultImageExternalTimeout,
	ImageDecoderPriority:     "",
	ConfirmCopy:              true,
	ConfirmMove:              true,
	ConfirmDelete:            true,
	ConfirmExit:              true,
	DeleteCancelFocused:      true,
	DefaultFileOpMode:        0,
	FileOpPathDisplay:        0,
	GuiFont:                  "",
	GuiFontSize:              18,
	GuiCols:                  100,
	GuiRows:                  30,
	ConsoleTitleTemplate:     "f4 %Ver %Platform %Admin - %State",
	UpdateChannel:            0,
	UpdateInterval:           3, // Default to Weekly
	LastUpdateCheck:          0,
	LastUpdateVersion:        "",
}

var getUserConfigIniPath = func() string {
	return filepath.Join(GetF4ConfigDir(), "settings.ini")
}

var getConfigIniPaths = func() []string {
	userPath := getUserConfigIniPath()
	if runtime.GOOS == "windows" {
		progData := os.Getenv("ProgramData")
		if progData != "" {
			return []string{filepath.Join(progData, "f4", "settings.ini"), userPath}
		}
		return []string{userPath}
	}
	// For unix-like systems
	return []string{"/etc/f4/settings.ini", userPath}
}

// normalizeHighlighter maps an arbitrary config value to one of the engines
// the editor knows about, falling back to the default one.
func normalizeHighlighter(name string) string {
	for _, known := range []string{"Chroma", "Colorer", "None"} {
		if strings.EqualFold(name, known) {
			return known
		}
	}
	return "Chroma"
}

func LoadConfig() {
	paths := getConfigIniPaths()
	ini := &IniFile{data: make(map[string]map[string]string)}

	for _, path := range paths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			vtui.DebugLog("CONFIG: Loading and merging config from %s", path)
			partialIni := LoadIni(path)
			ini.Merge(partialIni)
		}
	}

	AppConfig.ShowHiddenFiles = ini.GetString("Panel", "ShowHiddenFiles", "1") == "1"
	AppConfig.ColorStyle = ini.GetString("Interface", "ColorStyle", "Modern")
	AppConfig.Language = ini.GetString("Interface", "Language", "en")
	AppConfig.HelpLanguage = ini.GetString("Interface", "HelpLanguage", "en")
	AppConfig.ConsoleTitleTemplate = ini.GetString("Interface", "ConsoleTitleTemplate", "f4 %Ver %Platform %Admin - %State")
	AppConfig.AlwaysShowMenuBar = ini.GetString("Interface", "AlwaysShowMenuBar", "0") == "1"
	if AppConfig.ConsoleTitleTemplate == "f4 - %State" {
		AppConfig.ConsoleTitleTemplate = "f4 %Ver %Platform %Admin - %State"
	}
	AppConfig.HighlightDir = ini.GetString("Panel", "HighlightDir", "1") == "1"
	AppConfig.SeparateFileExtensions = ini.GetString("Panel", "SeparateFileExtensions", "0") == "1"
	if mode := ini.GetString("Panel", "PanelScrollbarMode", ""); mode != "" {
		AppConfig.PanelScrollbarMode = ParsePanelScrollbarMode(mode)
	} else if ini.GetString("Panel", "ShowPanelScrollbars", "0") == "1" {
		// Migration from the short-lived boolean setting.
		AppConfig.PanelScrollbarMode = PanelScrollbarFull
	} else {
		AppConfig.PanelScrollbarMode = PanelScrollbarOff
	}
	AppConfig.SavePanelPaths = ini.GetString("Panel", "SavePanelPaths", "1") == "1"
	AppConfig.InfoPanelBytes = ini.GetString("Panel", "InfoPanelBytes", "0") == "1"
	AppConfig.InfoPanelCPUGPU = ini.GetString("Panel", "InfoPanelCPUGPU", "0") == "1"
	AppConfig.EscTogglePanels = ini.GetString("Panel", "EscTogglePanels", "1") == "1"
	AppConfig.KeepTerminalCursor = ini.GetString("Panel", "KeepTerminalCursor", "0") == "1"
	AppConfig.CommandLineAutoComplete = ini.GetString("Panel", "CommandLineAutoComplete", "1") == "1"
	if mode := ini.GetString("Panel", "NavigationMode", ""); mode != "" {
		AppConfig.NavigationMode = ParsePanelNavigationMode(mode)
	} else if ini.GetString("Panel", "VimHotkeys", "0") == "1" {
		// Migration from settings written before NavigationMode was introduced.
		AppConfig.NavigationMode = NavigationVim
	} else {
		AppConfig.NavigationMode = NavigationClassic
	}
	AppConfig.SearchCommandStayFocused = ini.GetString("Panel", "SearchCommandStayFocused", "0") == "1"
	AppConfig.SyncPanelLoad = ini.GetString("Panel", "SyncPanelLoad", "0") == "1"
	fmt.Sscanf(ini.GetString("Panel", "DefaultFileOpMode", "0"), "%d", &AppConfig.DefaultFileOpMode)
	AppConfig.ConfirmCopy = ini.GetString("System", "ConfirmCopy", "1") == "1"
	AppConfig.ConfirmMove = ini.GetString("System", "ConfirmMove", "1") == "1"
	AppConfig.ConfirmDelete = ini.GetString("System", "ConfirmDelete", "1") == "1"
	AppConfig.ConfirmExit = ini.GetString("System", "ConfirmExit", "1") == "1"
	AppConfig.DeleteCancelFocused = ini.GetString("System", "DeleteCancelFocused", "1") == "1"
	AppConfig.AnnounceKittyTerm = ini.GetString("System", "AnnounceKittyTerm", "1") == "1"
	fmt.Sscanf(ini.GetString("System", "MacroRecordFormat", "0"), "%d", &AppConfig.MacroRecordFormat)
	fmt.Sscanf(ini.GetString("Panel", "FileOpPathDisplay", "0"), "%d", &AppConfig.FileOpPathDisplay)
	AppConfig.GuiFont = ini.GetString("Appearance", "GuiFont", "")
	fmt.Sscanf(ini.GetString("Appearance", "GuiFontSize", "18"), "%d", &AppConfig.GuiFontSize)
	if AppConfig.GuiFontSize <= 0 {
		AppConfig.GuiFontSize = 18
	}
	fmt.Sscanf(ini.GetString("Appearance", "GuiCols", "100"), "%d", &AppConfig.GuiCols)
	if AppConfig.GuiCols <= 0 {
		AppConfig.GuiCols = 100
	}
	fmt.Sscanf(ini.GetString("Appearance", "GuiRows", "30"), "%d", &AppConfig.GuiRows)
	if AppConfig.GuiRows <= 0 {
		AppConfig.GuiRows = 30
	}
	fmt.Sscanf(ini.GetString("Update", "Channel", "0"), "%d", &AppConfig.UpdateChannel)
	fmt.Sscanf(ini.GetString("Update", "Interval", "3"), "%d", &AppConfig.UpdateInterval)
	fmt.Sscanf(ini.GetString("Update", "LastCheck", "0"), "%d", &AppConfig.LastUpdateCheck)
	AppConfig.LastUpdateVersion = ini.GetString("Update", "LastVersion", "")

	AppConfig.EditorAutoComplete = ini.GetString("Editor", "AutoComplete", "1") == "1"
	AppConfig.EditorAutoCompleteMask = ini.GetString("Editor", "AutoCompleteMask", "*.go;*.c;*.cpp;*.h;*.hpp;*.py;*.js;*.ts;*.rs;*.java;*.sh;*.txt;*.md;*.html;*.css;*.json")

	AppConfig.EditorExpandTabs = 0
	fmt.Sscanf(ini.GetString("Editor", "ExpandTabs", "0"), "%d", &AppConfig.EditorExpandTabs)
	AppConfig.EditorAutoIndent = ini.GetString("Editor", "AutoIndent", "1") == "1"
	AppConfig.EditorCursorBeyondEOL = ini.GetString("Editor", "CursorBeyondEOL", "0") == "1"
	AppConfig.EditorUseEditorConfig = ini.GetString("Editor", "UseEditorConfig", "1") == "1"
	AppConfig.EditorCrosshair = ini.GetString("Editor", "Crosshair", "0") == "1"
	AppConfig.EditorAutodetectCodePage = ini.GetString("Editor", "AutodetectCodePage", "1") == "1"
	AppConfig.EditorHighlighter = normalizeHighlighter(ini.GetString("Editor", "Highlighter", "Chroma"))
	AppConfig.EditorColorerScheme = ini.GetString("Editor", "ColorerScheme", "")
	AppConfig.EditorColorerBackground = ini.GetString("Editor", "ColorerBackground", "1") == "1"
	AppConfig.EditorColorerSyntax = ini.GetString("Editor", "ColorerSyntax", "1") == "1"
	AppConfig.EditorColorerCatalog = ini.GetString("Editor", "ColorerCatalog", "")
	AppConfig.EditorCrossMode = ColorerCrossBoth
	fmt.Sscanf(ini.GetString("Editor", "CrossMode", "3"), "%d", &AppConfig.EditorCrossMode)
	if AppConfig.EditorCrossMode < ColorerCrossOff || AppConfig.EditorCrossMode > ColorerCrossBoth {
		AppConfig.EditorCrossMode = ColorerCrossBoth
	}
	fmt.Sscanf(ini.GetString("Editor", "DefaultCodePage", "65001"), "%d", &AppConfig.EditorDefaultCodePage)
	AppConfig.ViewerAutodetectCodePage = ini.GetString("Viewer", "AutodetectCodePage", "1") == "1"
	fmt.Sscanf(ini.GetString("Viewer", "DefaultCodePage", "65001"), "%d", &AppConfig.ViewerDefaultCodePage)
	AppConfig.SlideShowDelay = defaultSlideShowDelay
	fmt.Sscanf(ini.GetString("Images", "SlideShowDelay", "5"), "%d", &AppConfig.SlideShowDelay)
	if AppConfig.SlideShowDelay <= 0 {
		AppConfig.SlideShowDelay = defaultSlideShowDelay
	}
	AppConfig.ImageExternalTimeout = defaultImageExternalTimeout
	fmt.Sscanf(ini.GetString("Images", "ExternalTimeout", "20"), "%d", &AppConfig.ImageExternalTimeout)
	if AppConfig.ImageExternalTimeout <= 0 {
		AppConfig.ImageExternalTimeout = defaultImageExternalTimeout
	}
	AppConfig.ImageDecoderPriority = ini.GetString("Images", "DecoderPriority", "")
	SetImageDecoderPriorities(ParseImageDecoderPriorities(AppConfig.ImageDecoderPriority))
	AppConfig.UseExternalEditor = ini.GetString("Editor", "UseExternalEditor", "0") == "1"
	AppConfig.ExternalEditorCommand = ini.GetString("Editor", "ExternalEditorCommand", "")
	plugStr := ini.GetString("Plugins", "List", "")
	if plugStr != "" {
		AppConfig.RegisteredPlugins = strings.Split(plugStr, "|")
	}
	AppConfig.EditorTabSize = 4
	fmt.Sscanf(ini.GetString("Editor", "TabSize", "4"), "%d", &AppConfig.EditorTabSize)

	// [Layout] — three known keys plus round-trip storage for anything else.
	fmt.Sscanf(ini.GetString("Layout", "WidthDecrement", "0"), "%d", &AppConfig.WidthDecrement)
	fmt.Sscanf(ini.GetString("Layout", "LeftHeightDecrement", "0"), "%d", &AppConfig.LeftHeightDecrement)
	fmt.Sscanf(ini.GetString("Layout", "RightHeightDecrement", "0"), "%d", &AppConfig.RightHeightDecrement)
	AppConfig.LayoutExtras = nil
	if layout, ok := ini.data["Layout"]; ok {
		for k, v := range layout {
			switch k {
			case "WidthDecrement", "LeftHeightDecrement", "RightHeightDecrement":
				continue
			}
			if AppConfig.LayoutExtras == nil {
				AppConfig.LayoutExtras = make(map[string]string)
			}
			AppConfig.LayoutExtras[k] = v
		}
	}
}

func SaveConfig() {
	path := getUserConfigIniPath()
	os.MkdirAll(filepath.Dir(path), 0755)

	var sb strings.Builder
	sb.WriteString("[Interface]\n")
	sb.WriteString(fmt.Sprintf("ColorStyle = %s\n", AppConfig.ColorStyle))
	sb.WriteString(fmt.Sprintf("Language = %s\n", AppConfig.Language))
	sb.WriteString(fmt.Sprintf("HelpLanguage = %s\n", AppConfig.HelpLanguage))
	sb.WriteString(fmt.Sprintf("ConsoleTitleTemplate = %s\n", AppConfig.ConsoleTitleTemplate))
	sb.WriteString(fmt.Sprintf("AlwaysShowMenuBar = %d\n\n", map[bool]int{true: 1, false: 0}[AppConfig.AlwaysShowMenuBar]))
	sb.WriteString("[Panel]\n")
	sb.WriteString(fmt.Sprintf("ShowHiddenFiles = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ShowHiddenFiles]))
	sb.WriteString(fmt.Sprintf("HighlightDir = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.HighlightDir]))
	sb.WriteString(fmt.Sprintf("SeparateFileExtensions = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.SeparateFileExtensions]))
	sb.WriteString(fmt.Sprintf("PanelScrollbarMode = %s\n", AppConfig.PanelScrollbarMode.String()))
	sb.WriteString(fmt.Sprintf("SavePanelPaths = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.SavePanelPaths]))
	sb.WriteString(fmt.Sprintf("InfoPanelBytes = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.InfoPanelBytes]))
	sb.WriteString(fmt.Sprintf("InfoPanelCPUGPU = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.InfoPanelCPUGPU]))
	sb.WriteString(fmt.Sprintf("EscTogglePanels = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EscTogglePanels]))
	sb.WriteString(fmt.Sprintf("KeepTerminalCursor = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.KeepTerminalCursor]))
	sb.WriteString(fmt.Sprintf("CommandLineAutoComplete = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.CommandLineAutoComplete]))
	sb.WriteString(fmt.Sprintf("NavigationMode = %s\n", AppConfig.NavigationMode.String()))
	sb.WriteString(fmt.Sprintf("SearchCommandStayFocused = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.SearchCommandStayFocused]))
	// Keep the legacy key synchronized for older f4 versions and shared configs.
	sb.WriteString(fmt.Sprintf("VimHotkeys = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.NavigationMode == NavigationVim]))
	sb.WriteString(fmt.Sprintf("SyncPanelLoad = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.SyncPanelLoad]))
	sb.WriteString(fmt.Sprintf("DefaultFileOpMode = %d\n", AppConfig.DefaultFileOpMode))
	sb.WriteString(fmt.Sprintf("FileOpPathDisplay = %d\n", AppConfig.FileOpPathDisplay))

	sb.WriteString("\n[System]\n")
	sb.WriteString(fmt.Sprintf("ConfirmCopy = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ConfirmCopy]))
	sb.WriteString(fmt.Sprintf("ConfirmMove = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ConfirmMove]))
	sb.WriteString(fmt.Sprintf("ConfirmDelete = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ConfirmDelete]))
	sb.WriteString(fmt.Sprintf("ConfirmExit = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ConfirmExit]))
	sb.WriteString(fmt.Sprintf("DeleteCancelFocused = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.DeleteCancelFocused]))
	sb.WriteString(fmt.Sprintf("AnnounceKittyTerm = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.AnnounceKittyTerm]))
	sb.WriteString(fmt.Sprintf("MacroRecordFormat = %d\n", AppConfig.MacroRecordFormat))

	sb.WriteString("\n[Appearance]\n")
	sb.WriteString(fmt.Sprintf("GuiFont = %s\n", AppConfig.GuiFont))
	sb.WriteString(fmt.Sprintf("GuiFontSize = %d\n", AppConfig.GuiFontSize))
	sb.WriteString(fmt.Sprintf("GuiCols = %d\n", AppConfig.GuiCols))
	sb.WriteString(fmt.Sprintf("GuiRows = %d\n", AppConfig.GuiRows))

	sb.WriteString("\n[Update]\n")
	sb.WriteString(fmt.Sprintf("Channel = %d\n", AppConfig.UpdateChannel))
	sb.WriteString(fmt.Sprintf("Interval = %d\n", AppConfig.UpdateInterval))
	sb.WriteString(fmt.Sprintf("LastCheck = %d\n", AppConfig.LastUpdateCheck))
	sb.WriteString(fmt.Sprintf("LastVersion = %s\n", AppConfig.LastUpdateVersion))
	sb.WriteString("\n[Editor]\n")
	sb.WriteString(fmt.Sprintf("AutoComplete = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EditorAutoComplete]))
	sb.WriteString(fmt.Sprintf("AutoCompleteMask = %s\n", AppConfig.EditorAutoCompleteMask))

	sb.WriteString(fmt.Sprintf("ExpandTabs = %d\n", AppConfig.EditorExpandTabs))
	sb.WriteString(fmt.Sprintf("AutoIndent = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EditorAutoIndent]))
	sb.WriteString(fmt.Sprintf("CursorBeyondEOL = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EditorCursorBeyondEOL]))
	sb.WriteString(fmt.Sprintf("UseEditorConfig = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EditorUseEditorConfig]))
	sb.WriteString(fmt.Sprintf("Crosshair = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EditorCrosshair]))
	sb.WriteString(fmt.Sprintf("TabSize = %d\n", AppConfig.EditorTabSize))
	sb.WriteString(fmt.Sprintf("UseExternalEditor = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.UseExternalEditor]))
	sb.WriteString(fmt.Sprintf("ExternalEditorCommand = %s\n", AppConfig.ExternalEditorCommand))
	sb.WriteString(fmt.Sprintf("AutodetectCodePage = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EditorAutodetectCodePage]))
	sb.WriteString(fmt.Sprintf("Highlighter = %s\n", AppConfig.EditorHighlighter))
	sb.WriteString(fmt.Sprintf("ColorerScheme = %s\n", AppConfig.EditorColorerScheme))
	sb.WriteString(fmt.Sprintf("ColorerBackground = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EditorColorerBackground]))
	sb.WriteString(fmt.Sprintf("ColorerSyntax = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.EditorColorerSyntax]))
	sb.WriteString(fmt.Sprintf("ColorerCatalog = %s\n", AppConfig.EditorColorerCatalog))
	sb.WriteString(fmt.Sprintf("CrossMode = %d\n", AppConfig.EditorCrossMode))
	sb.WriteString(fmt.Sprintf("DefaultCodePage = %d\n", AppConfig.EditorDefaultCodePage))

	sb.WriteString("\n[Viewer]\n")
	sb.WriteString(fmt.Sprintf("AutodetectCodePage = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ViewerAutodetectCodePage]))
	sb.WriteString(fmt.Sprintf("DefaultCodePage = %d\n", AppConfig.ViewerDefaultCodePage))
	sb.WriteString("\n[Images]\n")
	sb.WriteString(fmt.Sprintf("SlideShowDelay = %d\n", AppConfig.SlideShowDelay))
	sb.WriteString(fmt.Sprintf("ExternalTimeout = %d\n", AppConfig.ImageExternalTimeout))
	sb.WriteString(fmt.Sprintf("DecoderPriority = %s\n", AppConfig.ImageDecoderPriority))
	sb.WriteString("\n[Plugins]\n")
	sb.WriteString(fmt.Sprintf("List = %s\n", strings.Join(AppConfig.RegisteredPlugins, "|")))

	// [Layout]: emit our three keys plus any unrecognised keys we loaded
	// (round-trip). Keys are written alphabetically to match far2l's
	// on-disk order, so a diff against far2l's config.ini stays minimal.
	layoutKeys := map[string]string{
		"WidthDecrement":       fmt.Sprintf("%d", AppConfig.WidthDecrement),
		"LeftHeightDecrement":  fmt.Sprintf("%d", AppConfig.LeftHeightDecrement),
		"RightHeightDecrement": fmt.Sprintf("%d", AppConfig.RightHeightDecrement),
	}
	for k, v := range AppConfig.LayoutExtras {
		if _, taken := layoutKeys[k]; taken {
			continue
		}
		layoutKeys[k] = v
	}
	names := make([]string, 0, len(layoutKeys))
	for k := range layoutKeys {
		names = append(names, k)
	}
	sort.Strings(names)
	sb.WriteString("\n[Layout]\n")
	for _, k := range names {
		sb.WriteString(fmt.Sprintf("%s=%s\n", k, layoutKeys[k]))
	}

	err := os.WriteFile(path, []byte(sb.String()), 0644)
	if err != nil {
		vtui.DebugLog("CONFIG: Failed to save application settings: %v", err)
		return
	}

	vtui.DebugLog("CONFIG: Saved application settings to %s", path)
}

// RequestSaveConfig schedules a debounced SaveConfig call. Multiple calls
// within the debounce window collapse into a single write. Used by the
// panel-resize hotkeys, where holding Ctrl+Arrow can fire many times per
// second and we don't want to fsync on every keystroke. The final value
// still lands on disk because the shutdown path calls SaveConfig directly.
func RequestSaveConfig() {
	saveConfigTimerMu.Lock()
	defer saveConfigTimerMu.Unlock()
	if saveConfigTimer != nil {
		saveConfigTimer.Stop()
	}
	saveConfigTimer = time.AfterFunc(saveConfigDebounce, SaveConfig)
}

var (
	saveConfigTimerMu sync.Mutex
	saveConfigTimer   *time.Timer
)

const saveConfigDebounce = 500 * time.Millisecond
