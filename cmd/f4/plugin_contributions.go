package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var _ vfs.ContributionHost = (*coreAPI)(nil)

type unregisterFunc struct {
	once sync.Once
	fn   func()
}

func (r *unregisterFunc) Unregister() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		if r.fn != nil {
			r.fn()
		}
	})
}

type registeredPluginCommand struct {
	command vfs.PluginCommand
	token   *struct{}
}

var pluginCommandRegistry = struct {
	sync.RWMutex
	byID  map[string]registeredPluginCommand
	order []string
}{byID: make(map[string]registeredPluginCommand)}

func validatePluginCommand(command vfs.PluginCommand) error {
	command.ID = strings.TrimSpace(command.ID)
	if command.ID == "" {
		return errors.New("plugin command ID is empty")
	}
	if strings.TrimSpace(command.Label) == "" {
		return fmt.Errorf("plugin command %q has an empty label", command.ID)
	}
	if command.Run == nil {
		return fmt.Errorf("plugin command %q has no handler", command.ID)
	}
	if command.Location != vfs.PluginCommandPanel && command.Location != vfs.PluginCommandConfig {
		return fmt.Errorf("plugin command %q has an invalid location", command.ID)
	}
	return nil
}

func clonePluginCommand(command vfs.PluginCommand) vfs.PluginCommand {
	command.SearchKeys = append([]string(nil), command.SearchKeys...)
	command.SearchTerms = append([]string(nil), command.SearchTerms...)
	command.LocalizedLabels = clonePluginCommandLocalizedText(command.LocalizedLabels)
	command.LocalizedDescriptions = clonePluginCommandLocalizedText(command.LocalizedDescriptions)
	return command
}

func clonePluginCommandLocalizedText(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for language, value := range values {
		cloned[language] = value
	}
	return cloned
}

func normalizePluginCommandLanguage(language string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
}

func pluginCommandLanguageCandidates() []string {
	seen := make(map[string]bool)
	var candidates []string
	appendLanguage := func(language string) {
		language = normalizePluginCommandLanguage(language)
		if language == "" {
			return
		}
		if !seen[language] {
			seen[language] = true
			candidates = append(candidates, language)
		}
		if separator := strings.IndexByte(language, '-'); separator > 0 {
			base := language[:separator]
			if !seen[base] {
				seen[base] = true
				candidates = append(candidates, base)
			}
		}
	}
	appendLanguage(AppConfig.Language)
	appendLanguage(AppConfig.FallbackLanguage)
	appendLanguage("en")
	return candidates
}

func pluginCommandLocalizedText(values map[string]string) string {
	for _, candidate := range pluginCommandLanguageCandidates() {
		for language, value := range values {
			if normalizePluginCommandLanguage(language) == candidate && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

func pluginCommandDisplayText(key string, localized map[string]string, fallback string) string {
	key = strings.TrimSpace(key)
	if key != "" {
		if value := Msg(key); !strings.HasPrefix(value, "{") {
			return value
		}
	}
	if value := pluginCommandLocalizedText(localized); value != "" {
		return value
	}
	return fallback
}

func pluginCommandDisplayLabel(command vfs.PluginCommand) string {
	return pluginCommandDisplayText(command.LabelKey, command.LocalizedLabels, command.Label)
}

func pluginCommandDisplayDescription(command vfs.PluginCommand) string {
	return pluginCommandDisplayText(command.DescriptionKey, command.LocalizedDescriptions, command.Description)
}

func pluginCommandSearchTerms(command vfs.PluginCommand) []string {
	terms := append([]string(nil), command.SearchTerms...)
	appendLocalized := func(values map[string]string) {
		languages := make([]string, 0, len(values))
		for language := range values {
			languages = append(languages, language)
		}
		sort.Strings(languages)
		for _, language := range languages {
			terms = append(terms, values[language])
		}
	}
	appendLocalized(command.LocalizedLabels)
	appendLocalized(command.LocalizedDescriptions)
	return terms
}

func pluginCommandTranslationKeys(command vfs.PluginCommand) []string {
	keys := make([]string, 0, 2+len(command.SearchKeys))
	seen := make(map[string]bool, 2+len(command.SearchKeys))
	for _, key := range append([]string{command.LabelKey, command.DescriptionKey}, command.SearchKeys...) {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, key)
	}
	return keys
}

func (c *coreAPI) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	command.ID = strings.TrimSpace(command.ID)
	if err := validatePluginCommand(command); err != nil {
		return nil, err
	}
	command = clonePluginCommand(command)

	registryID := strings.ToLower(command.ID)
	token := &struct{}{}
	pluginCommandRegistry.Lock()
	if _, exists := pluginCommandRegistry.byID[registryID]; exists {
		pluginCommandRegistry.Unlock()
		return nil, fmt.Errorf("plugin command %q is already registered", command.ID)
	}
	pluginCommandRegistry.byID[registryID] = registeredPluginCommand{command: command, token: token}
	pluginCommandRegistry.order = append(pluginCommandRegistry.order, registryID)
	pluginCommandRegistry.Unlock()

	return &unregisterFunc{fn: func() {
		pluginCommandRegistry.Lock()
		if current, ok := pluginCommandRegistry.byID[registryID]; ok && current.token == token {
			delete(pluginCommandRegistry.byID, registryID)
			for index, id := range pluginCommandRegistry.order {
				if id == registryID {
					pluginCommandRegistry.order = append(pluginCommandRegistry.order[:index], pluginCommandRegistry.order[index+1:]...)
					break
				}
			}
		}
		pluginCommandRegistry.Unlock()
	}}, nil
}

func pluginCommandsSnapshot(location vfs.PluginCommandLocation, app vfs.App) []vfs.PluginCommand {
	pluginCommandRegistry.RLock()
	ordered := append([]string(nil), pluginCommandRegistry.order...)
	registered := make(map[string]registeredPluginCommand, len(pluginCommandRegistry.byID))
	for id, command := range pluginCommandRegistry.byID {
		registered[id] = registeredPluginCommand{command: clonePluginCommand(command.command), token: command.token}
	}
	pluginCommandRegistry.RUnlock()

	commands := make([]vfs.PluginCommand, 0, len(registered))
	for _, id := range ordered {
		entry, ok := registered[id]
		if !ok || entry.command.Location != location {
			continue
		}
		if entry.command.Visible != nil && !entry.command.Visible(app) {
			continue
		}
		commands = append(commands, clonePluginCommand(entry.command))
	}
	return commands
}

// executeRegisteredPluginCommand resolves a menu/palette selection against
// the live registry. Menus keep a label snapshot while open, but a plugin can
// disconnect and unregister in the meantime; retaining its old Run closure
// would call unloaded plugin code.
func executeRegisteredPluginCommand(location vfs.PluginCommandLocation, id string, app vfs.App) bool {
	if panels, ok := app.(*PanelsFrame); ok {
		if panels == nil || panels.closed || vtui.FrameManager != nil && findPanelsFrameAnyScreen() != panels {
			return false
		}
	}
	registryID := strings.ToLower(strings.TrimSpace(id))
	pluginCommandRegistry.RLock()
	registered, ok := pluginCommandRegistry.byID[registryID]
	if ok {
		registered.command = clonePluginCommand(registered.command)
	}
	pluginCommandRegistry.RUnlock()
	if !ok || registered.command.Location != location {
		return false
	}
	command := registered.command
	if command.Visible != nil && !command.Visible(app) {
		return false
	}
	command.Run(app)
	return true
}

func actionPluginConfiguration(pf *PanelsFrame) {
	commands := pluginCommandsSnapshot(vfs.PluginCommandConfig, pf)
	if len(commands) == 0 {
		vtui.ShowMessage(Msg("Plugins.ConfigTitle"), Msg("Plugins.ConfigEmpty"), []string{Msg("vtui.Ok")})
		return
	}

	labels := make([]string, len(commands))
	for i := range commands {
		labels[i] = pluginCommandDisplayLabel(commands[i])
	}
	pf.Menu(Msg("Plugins.ConfigTitle"), labels, func(index int) {
		if index >= 0 && index < len(commands) {
			executeRegisteredPluginCommand(vfs.PluginCommandConfig, commands[index].ID, pf)
		}
	})
}
