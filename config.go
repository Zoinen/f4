package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/unxed/vtui"
)

type F4Config struct {
	ColorStyle              string
	ShowHiddenFiles         bool
	HighlightDir            bool
	SavePanelPaths          bool
	KeepTerminalCursor      bool
	CommandLineAutoComplete bool
	VimHotkeys              bool
	SyncPanelLoad           bool
	EditorAutoComplete      bool
	EditorAutoCompleteMask  string
	EditorExpandTabs        int
	EditorAutoIndent        bool
	EditorCursorBeyondEOL   bool
	EditorTabSize           int
	EditorUseEditorConfig   bool
	EditorCrosshair         bool
	UseExternalEditor       bool
	ExternalEditorCommand   string
	RegisteredPlugins       []string
	ConfirmCopy             bool
	ConfirmMove             bool
	ConfirmDelete           bool
	ConfirmExit             bool
	DefaultFileOpMode       int
}

var AppConfig = F4Config{
	ColorStyle:              "Modern",
	ShowHiddenFiles:         true,
	HighlightDir:            true,
	SavePanelPaths:          true,
	KeepTerminalCursor:      false,
	CommandLineAutoComplete: true,
	VimHotkeys:              false,
	SyncPanelLoad:           false,
	EditorAutoComplete:      true,
	EditorAutoCompleteMask:  "*.go;*.c;*.cpp;*.h;*.hpp;*.py;*.js;*.ts;*.rs;*.java;*.sh;*.txt;*.md;*.html;*.css;*.json",
	EditorExpandTabs:        0,
	EditorAutoIndent:        true,
	EditorCursorBeyondEOL:   false,
	EditorTabSize:           4,
	EditorUseEditorConfig:   true,
	EditorCrosshair:         false,
	UseExternalEditor:       false,
	ExternalEditorCommand:   "",
	ConfirmCopy:             true,
	ConfirmMove:             true,
	ConfirmDelete:           true,
	ConfirmExit:             true,
	DefaultFileOpMode:       0,
}

var getUserConfigIniPath = func() string {
	configDir, _ := os.UserConfigDir()
	return filepath.Join(configDir, "f4", "settings.ini")
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
	AppConfig.HighlightDir = ini.GetString("Panel", "HighlightDir", "1") == "1"
	AppConfig.SavePanelPaths = ini.GetString("Panel", "SavePanelPaths", "1") == "1"
	AppConfig.KeepTerminalCursor = ini.GetString("Panel", "KeepTerminalCursor", "0") == "1"
	AppConfig.CommandLineAutoComplete = ini.GetString("Panel", "CommandLineAutoComplete", "1") == "1"
	AppConfig.VimHotkeys = ini.GetString("Panel", "VimHotkeys", "0") == "1"
	AppConfig.SyncPanelLoad = ini.GetString("Panel", "SyncPanelLoad", "0") == "1"
	fmt.Sscanf(ini.GetString("Panel", "DefaultFileOpMode", "0"), "%d", &AppConfig.DefaultFileOpMode)
	AppConfig.ConfirmCopy = ini.GetString("System", "ConfirmCopy", "1") == "1"
	AppConfig.ConfirmMove = ini.GetString("System", "ConfirmMove", "1") == "1"
	AppConfig.ConfirmDelete = ini.GetString("System", "ConfirmDelete", "1") == "1"
	AppConfig.ConfirmExit = ini.GetString("System", "ConfirmExit", "1") == "1"

	AppConfig.EditorAutoComplete = ini.GetString("Editor", "AutoComplete", "1") == "1"
	AppConfig.EditorAutoCompleteMask = ini.GetString("Editor", "AutoCompleteMask", "*.go;*.c;*.cpp;*.h;*.hpp;*.py;*.js;*.ts;*.rs;*.java;*.sh;*.txt;*.md;*.html;*.css;*.json")

	AppConfig.EditorExpandTabs = 0
	fmt.Sscanf(ini.GetString("Editor", "ExpandTabs", "0"), "%d", &AppConfig.EditorExpandTabs)
	AppConfig.EditorAutoIndent = ini.GetString("Editor", "AutoIndent", "1") == "1"
	AppConfig.EditorCursorBeyondEOL = ini.GetString("Editor", "CursorBeyondEOL", "0") == "1"
	AppConfig.EditorUseEditorConfig = ini.GetString("Editor", "UseEditorConfig", "1") == "1"
	AppConfig.EditorCrosshair = ini.GetString("Editor", "Crosshair", "0") == "1"
	AppConfig.UseExternalEditor = ini.GetString("Editor", "UseExternalEditor", "0") == "1"
	AppConfig.ExternalEditorCommand = ini.GetString("Editor", "ExternalEditorCommand", "")
	plugStr := ini.GetString("Plugins", "List", "")
	if plugStr != "" {
		AppConfig.RegisteredPlugins = strings.Split(plugStr, "|")
	}
	AppConfig.EditorTabSize = 4
	fmt.Sscanf(ini.GetString("Editor", "TabSize", "4"), "%d", &AppConfig.EditorTabSize)

}

func SaveConfig() {
	path := getUserConfigIniPath()
	os.MkdirAll(filepath.Dir(path), 0755)

	var sb strings.Builder
	sb.WriteString("[Interface]\n")
	sb.WriteString(fmt.Sprintf("ColorStyle = %s\n\n", AppConfig.ColorStyle))
	sb.WriteString("[Panel]\n")
	sb.WriteString(fmt.Sprintf("ShowHiddenFiles = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ShowHiddenFiles]))
	sb.WriteString(fmt.Sprintf("HighlightDir = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.HighlightDir]))
	sb.WriteString(fmt.Sprintf("SavePanelPaths = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.SavePanelPaths]))
	sb.WriteString(fmt.Sprintf("KeepTerminalCursor = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.KeepTerminalCursor]))
	sb.WriteString(fmt.Sprintf("CommandLineAutoComplete = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.CommandLineAutoComplete]))
	sb.WriteString(fmt.Sprintf("VimHotkeys = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.VimHotkeys]))
	sb.WriteString(fmt.Sprintf("SyncPanelLoad = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.SyncPanelLoad]))
	sb.WriteString(fmt.Sprintf("DefaultFileOpMode = %d\n", AppConfig.DefaultFileOpMode))

	sb.WriteString("\n[System]\n")
	sb.WriteString(fmt.Sprintf("ConfirmCopy = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ConfirmCopy]))
	sb.WriteString(fmt.Sprintf("ConfirmMove = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ConfirmMove]))
	sb.WriteString(fmt.Sprintf("ConfirmDelete = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ConfirmDelete]))
	sb.WriteString(fmt.Sprintf("ConfirmExit = %d\n", map[bool]int{true: 1, false: 0}[AppConfig.ConfirmExit]))
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
	sb.WriteString("\n[Plugins]\n")
	sb.WriteString(fmt.Sprintf("List = %s\n", strings.Join(AppConfig.RegisteredPlugins, "|")))

	err := os.WriteFile(path, []byte(sb.String()), 0644)
	if err != nil {
		vtui.DebugLog("CONFIG: Failed to save application settings: %v", err)
		return
	}

	vtui.DebugLog("CONFIG: Saved application settings to %s", path)
}
