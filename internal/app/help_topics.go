package app

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/settings"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// The half of the help system that composes rather than renders: it reads the
// action registry, the user's hotkeys and the running version, none of which a
// layer-3 dialog package may reach.

func InitHelpSystem() {
	files := map[string]string{}

	versionedEnglishHelp := strings.ReplaceAll(dialog.DefaultHelpData, "%Ver", GetLongVersionInfo())
	files["help.hlf"] = versionedEnglishHelp

	lang := config.App.HelpLanguage
	if lang == "" {
		lang = "en"
	}

	hasLocalHelp := false
	// The help language is a path component below. LoadHelpLangStrings checks
	// it for separators and ".." before doing the same thing, and this side did
	// not — a HelpLanguage of "../../etc/passwd" in the ini was read as a help
	// file.
	if !i18n.SafeLanguageCode(lang) {
		lang = "en"
	}

	// An explicitly enabled profile overlay also applies to English. The
	// embedded English help remains the baseline, while help/en.hlf can provide
	// user-specific topics and corrections.
	if config.App.UseLocalLanguageFiles || (lang != "en" && lang != "eng") {
		exeDir := filepath.Dir(os.Args[0])
		candidates := []string{
			filepath.Join(exeDir, "help", lang+".hlf"),
			filepath.Join("help", lang+".hlf"), // Fallback for "go run ." development
		}
		if config.App.UseLocalLanguageFiles {
			userDir := filepath.Join(config.GetF4ConfigDir(), "help")
			candidates = append([]string{filepath.Join(userDir, lang+".hlf")}, candidates...)
		}

		var helpContent string
		for _, cand := range candidates {
			// #nosec G703 -- i18n.SafeLanguageCode above rejects separators and ".." before lang is used as a path component.
			if data, err := os.ReadFile(cand); err == nil {
				helpContent = string(data)
				vtui.DebugLog("HELP: Loaded language help file from disk: %s", cand)
				break
			}
		}

		if helpContent == "" {
			helpContent, _ = dialog.EmbeddedHelp(lang)
		}
		if helpContent != "" {
			versionedHelp := strings.ReplaceAll(helpContent, "%Ver", GetLongVersionInfo())
			files["help_local.hlf"] = versionedHelp
			hasLocalHelp = true
		}
	}

	v := dialog.NewMemoryHelpVFS(files)
	vtui.GlobalHelpEngine = vtui.NewHelpEngine(v)

	_ = vtui.GlobalHelpEngine.LoadFile("help.hlf")

	if hasLocalHelp {
		_ = vtui.GlobalHelpEngine.LoadFile("help_local.hlf")
	}
	dialog.FlattenVisRenHelp(vtui.GlobalHelpEngine)

	readmeTopic := dialog.ParseMarkdownToHelpTopic("README", dialog.ReadmeData)
	vtui.GlobalHelpEngine.AddTopic(readmeTopic)

	// Key binding topics are generated from the action registry (the
	// single source of truth), overriding the static stubs in .hlf
	// files and reflecting the user's hotkeys.ini overrides.
	dialog.HelpActionStrings = dialog.LoadHelpLangStrings(lang)
	settings.InstallHelp(lang)
	vtui.GlobalHelpEngine.AddTopic(GenerateKeysHelpTopic("ViewerEditor",
		dialog.HelpMsg("Help.ViewerEditor"), []string{"Editor", "Viewer", "Common"}, "ViewerNav"))
	vtui.GlobalHelpEngine.AddTopic(GenerateKeysHelpTopic("PanelNav",
		dialog.HelpMsg("Help.PanelNav"), []string{"Shell", "Terminal", "Common"}, "ShellNav"))
}

// generateKeysHelpTopic builds a help topic listing the active key
// bindings of the given areas, straight from the action registry.
// navTarget, when non-empty, appends a link to the static topic holding
// widget-level navigation keys (arrows and the like are not actions).
func GenerateKeysHelpTopic(name, title string, areas []string, navTarget string) *vtui.HelpTopic {
	topic := &vtui.HelpTopic{Name: name, StickyRows: 1, Lines: []string{title}}

	hm := keymap.GlobalHotkeysMgr
	if hm == nil {
		hm = keymap.NewHotkeyManager("")
	}
	active := hm.GetActiveBindings()

	keysFor := func(area string, action action.Action) string {
		var keys []string
		for key, binding := range active[area] {
			parts := strings.SplitN(binding, ":", 2)
			if strings.EqualFold(parts[0], action.Name) {
				keys = append(keys, keymap.FormatKeyForUI(key))
			}
		}
		keys = keymap.MergeShortcuts(keys, keymap.NativeShortcutsForAction(area, action))
		return strings.Join(keys, " / ")
	}

	for _, area := range areas {
		header := dialog.HelpMsg("Help.Area." + area)
		if strings.HasPrefix(header, "{") {
			header = area
		}
		topic.Lines = append(topic.Lines, header+":")
		for _, a := range action.All() {
			if a.Area != area {
				continue
			}
			keys := keysFor(area, a)
			if keys == "" {
				continue
			}
			desc := a.Description
			if a.DescKey != "" {
				if s := dialog.HelpMsg(a.DescKey); !strings.HasPrefix(s, "{") {
					desc = s
				}
			}
			dialog.AppendGeneratedHelpAction(topic, keys, desc)
		}
		topic.Lines = append(topic.Lines, "")
	}

	if navTarget != "" {
		text := dialog.HelpMsg("Help.NavigationKeys")
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
