package main

import (
	"context"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/unxed/vtui"
)

//go:embed help/en.hlf
var defaultHelpData string

//go:embed README.md
var readmeData string

type memoryHelpVFS struct {
	files map[string]string
}

func (m *memoryHelpVFS) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	content, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

var mdLinkRegex = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

func convertMarkdownLinks(line string) string {
	return mdLinkRegex.ReplaceAllString(line, "~$1~$2@")
}

func parseMarkdownToHelpTopic(name string, mdContent string) *vtui.HelpTopic {
	topic := &vtui.HelpTopic{
		Name: name,
	}
	lines := strings.Split(mdContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			headerText := strings.TrimSpace(trimmed[level:])
			line = "#" + headerText + "#"
			if level == 1 && topic.StickyRows == 0 {
				topic.StickyRows = 1
				topic.Lines = append([]string{headerText}, topic.Lines...)
				continue
			}
			line = convertMarkdownLinks(line)
			topic.Lines = append(topic.Lines, line)
			continue
		}

		wrapped := vtui.WrapText(line, 70)
		for _, wLine := range wrapped {
			wLine = convertMarkdownLinks(wLine)
			topic.Lines = append(topic.Lines, wLine)
		}
	}
	return topic
}

// helpActionStrings holds the .lng strings of the configured help
// language. Generated key topics use it so their language matches the
// static .hlf content even when it differs from the UI language.
var helpActionStrings map[string]string

// helpMsg resolves an i18n key preferring the help language strings,
// falling back to the UI language.
func helpMsg(key string) string {
	if helpActionStrings != nil {
		if s, ok := helpActionStrings[key]; ok {
			return s
		}
	}
	return Msg(key)
}

// loadHelpLangStrings loads the .lng map for a help language code.
// Returns nil for English (embedded strings already cover it via Msg)
// or when no language file is found.
func loadHelpLangStrings(code string) map[string]string {
	if code == "" || code == "en" || code == "eng" {
		return nil
	}
	exeDir := filepath.Dir(os.Args[0])
	userDir := filepath.Join(GetF4ConfigDir(), "lang")
	candidates := []string{
		filepath.Join(userDir, code+".lng"),
		filepath.Join(exeDir, "lang", code+".lng"),
		filepath.Join("lang", code+".lng"),
	}
	for _, cand := range candidates {
		if _, err := os.Stat(cand); err == nil {
			return loadLangMapFromINI(LoadIni(cand))
		}
	}
	return nil
}

func InitHelpSystem() {
	files := map[string]string{}

	versionedEnglishHelp := strings.ReplaceAll(defaultHelpData, "%Ver", getLongVersionInfo())
	files["help.hlf"] = versionedEnglishHelp

	lang := AppConfig.HelpLanguage
	if lang == "" {
		lang = "en"
	}

	hasLocalHelp := false
	if lang != "en" && lang != "eng" {
		exeDir := filepath.Dir(os.Args[0])
		userDir := filepath.Join(GetF4ConfigDir(), "help")

		candidates := []string{
			filepath.Join(userDir, lang+".hlf"),
			filepath.Join(exeDir, "help", lang+".hlf"),
			filepath.Join("help", lang+".hlf"), // Fallback for "go run ." development
		}

		var helpContent string
		for _, cand := range candidates {
			if data, err := os.ReadFile(cand); err == nil {
				helpContent = string(data)
				vtui.DebugLog("HELP: Loaded language help file from disk: %s", cand)
				break
			}
		}

		if helpContent != "" {
			versionedHelp := strings.ReplaceAll(helpContent, "%Ver", getLongVersionInfo())
			files["help_local.hlf"] = versionedHelp
			hasLocalHelp = true
		}
	}

	v := &memoryHelpVFS{
		files: files,
	}
	vtui.GlobalHelpEngine = vtui.NewHelpEngine(v)

	_ = vtui.GlobalHelpEngine.LoadFile("help.hlf")

	if hasLocalHelp {
		_ = vtui.GlobalHelpEngine.LoadFile("help_local.hlf")
	}
	flattenVisRenHelp(vtui.GlobalHelpEngine)

	readmeTopic := parseMarkdownToHelpTopic("README", readmeData)
	vtui.GlobalHelpEngine.AddTopic(readmeTopic)

	// Key binding topics are generated from the action registry (the
	// single source of truth), overriding the static stubs in .hlf
	// files and reflecting the user's hotkeys.ini overrides.
	helpActionStrings = loadHelpLangStrings(lang)
	vtui.GlobalHelpEngine.AddTopic(generateKeysHelpTopic("ViewerEditor",
		helpMsg("Help.ViewerEditor"), []string{"Editor", "Viewer", "Common"}, "ViewerNav"))
	vtui.GlobalHelpEngine.AddTopic(generateKeysHelpTopic("PanelNav",
		helpMsg("Help.PanelNav"), []string{"Shell", "Terminal", "Common"}, "ShellNav"))
}

// generateKeysHelpTopic builds a help topic listing the active key
// bindings of the given areas, straight from the action registry.
// navTarget, when non-empty, appends a link to the static topic holding
// widget-level navigation keys (arrows and the like are not actions).
func generateKeysHelpTopic(name, title string, areas []string, navTarget string) *vtui.HelpTopic {
	topic := &vtui.HelpTopic{Name: name, StickyRows: 1, Lines: []string{title}}

	hm := GlobalHotkeysMgr
	if hm == nil {
		hm = NewHotkeyManager("")
	}
	active := hm.GetActiveBindings()

	keysFor := func(area string, action Action) string {
		var keys []string
		for key, binding := range active[area] {
			parts := strings.SplitN(binding, ":", 2)
			if strings.EqualFold(parts[0], action.Name) {
				keys = append(keys, FormatKeyForUI(key))
			}
		}
		keys = mergeCommandPaletteShortcuts(keys, NativeShortcutsForAction(area, action))
		return strings.Join(keys, " / ")
	}

	for _, area := range areas {
		header := helpMsg("Help.Area." + area)
		if strings.HasPrefix(header, "{") {
			header = area
		}
		topic.Lines = append(topic.Lines, header+":")
		for _, a := range GetOrderedActions() {
			if a.Area != area {
				continue
			}
			keys := keysFor(area, a)
			if keys == "" {
				continue
			}
			desc := a.Description
			if a.DescKey != "" {
				if s := helpMsg(a.DescKey); !strings.HasPrefix(s, "{") {
					desc = s
				}
			}
			topic.Lines = append(topic.Lines, fmt.Sprintf("  %-14s - %s", keys, desc))
		}
		topic.Lines = append(topic.Lines, "")
	}

	if navTarget != "" {
		text := helpMsg("Help.NavigationKeys")
		line := "~" + text + "~" + navTarget + "@"
		topic.Lines = append(topic.Lines, line)
		// AddTopic bypasses LoadFile, so register the link manually.
		topic.Links = append(topic.Links, vtui.HelpLink{
			Text:   text,
			Target: navTarget,
			Line:   len(topic.Lines) - 1,
			X1:     0,
			X2:     len(line) - 1,
		})
	}
	return topic
}

var visRenHelpSections = []string{
	"VisRenQuickStart", "VisRenMasks", "VisRenTransforms", "VisRenMetadata",
	"VisRenSearch", "VisRenPreview", "VisRenEditor", "VisRenRename",
	"VisRenSafety", "VisRenExamples",
}

// flattenVisRenHelp keeps the linked detail topics for quick navigation while
// also appending their complete contents to the main topic. This lets readers
// browse all of VisRen's help continuously with PgDn.
func flattenVisRenHelp(engine *vtui.HelpEngine) {
	if engine == nil {
		return
	}
	index := engine.GetTopic("VisRen")
	if index == nil {
		return
	}
	for _, name := range visRenHelpSections {
		section := engine.GetTopic(name)
		if section == nil || len(section.Lines) == 0 {
			continue
		}
		title := strings.TrimSpace(section.Lines[0])
		index.Lines = append(index.Lines, "", "#"+title+"#")
		start := section.StickyRows
		if start < 0 || start > len(section.Lines) {
			start = 0
		}
		index.Lines = append(index.Lines, section.Lines[start:]...)
	}
}
