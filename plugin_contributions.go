package main

import (
	"errors"
	"fmt"
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

func (c *coreAPI) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	command.ID = strings.TrimSpace(command.ID)
	if err := validatePluginCommand(command); err != nil {
		return nil, err
	}

	token := &struct{}{}
	pluginCommandRegistry.Lock()
	if _, exists := pluginCommandRegistry.byID[command.ID]; exists {
		pluginCommandRegistry.Unlock()
		return nil, fmt.Errorf("plugin command %q is already registered", command.ID)
	}
	pluginCommandRegistry.byID[command.ID] = registeredPluginCommand{command: command, token: token}
	pluginCommandRegistry.order = append(pluginCommandRegistry.order, command.ID)
	pluginCommandRegistry.Unlock()

	return &unregisterFunc{fn: func() {
		pluginCommandRegistry.Lock()
		if current, ok := pluginCommandRegistry.byID[command.ID]; ok && current.token == token {
			delete(pluginCommandRegistry.byID, command.ID)
			for index, id := range pluginCommandRegistry.order {
				if id == command.ID {
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
		registered[id] = command
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
		commands = append(commands, entry.command)
	}
	return commands
}

func actionPluginConfiguration(pf *PanelsFrame) {
	commands := pluginCommandsSnapshot(vfs.PluginCommandConfig, pf)
	if len(commands) == 0 {
		vtui.ShowMessage(Msg("Plugins.ConfigTitle"), Msg("Plugins.ConfigEmpty"), []string{Msg("vtui.Ok")})
		return
	}

	labels := make([]string, len(commands))
	for i := range commands {
		labels[i] = commands[i].Label
	}
	pf.Menu(Msg("Plugins.ConfigTitle"), labels, func(index int) {
		if index >= 0 && index < len(commands) {
			commands[index].Run(pf)
		}
	})
}
