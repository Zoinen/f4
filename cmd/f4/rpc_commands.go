package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

// PluginCommandDescriptor is the transport-safe form of vfs.PluginCommand.
// It is deliberately declarative: the host never calls a remote Visible
// callback on the UI goroutine. ActiveDrives is an optional cached context
// constraint; an empty list means that the command is globally visible.
type PluginCommandDescriptor struct {
	ID                    string
	Location              uint8
	Label                 string
	Description           string
	Shortcut              string
	MenuPath              string
	LocalizedLabels       map[string]string
	LocalizedDescriptions map[string]string
	SearchTerms           []string
	ActiveDrives          []string
}

type PluginInitResponse struct {
	Drives   []string
	Commands []PluginCommandDescriptor
	Panels   []PluginPanelDescriptor
}

// UnmarshalMsgpack keeps the command extension backwards compatible with
// plugins that still return the original []string drive list from
// Plugin.Init. Newer plugins return the structured response above.
func (r *PluginInitResponse) UnmarshalMsgpack(data []byte) error {
	if r == nil {
		return errors.New("nil plugin init response")
	}
	var legacy []string
	if err := msgpack.Unmarshal(data, &legacy); err == nil {
		r.Drives = append(r.Drives[:0], legacy...)
		r.Commands = nil
		r.Panels = nil
		return nil
	}
	type responseAlias PluginInitResponse
	var response responseAlias
	if err := msgpack.Unmarshal(data, &response); err != nil {
		return fmt.Errorf("decode plugin init response: %w", err)
	}
	*r = PluginInitResponse(response)
	return nil
}

type PluginRunCommandRequest struct {
	ID string
}

// pluginSessionRegistrations owns contributions created by one transport.
// Add remains safe when Serve has already returned: a late registration is
// immediately removed instead of escaping a disconnected session.
type pluginSessionRegistrations struct {
	mu     sync.Mutex
	closed bool
	items  []vfs.Registration
}

func (r *pluginSessionRegistrations) Add(registration vfs.Registration) bool {
	if registration == nil {
		return false
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		registration.Unregister()
		return false
	}
	r.items = append(r.items, registration)
	r.mu.Unlock()
	return true
}

func (r *pluginSessionRegistrations) Unregister() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	items := append([]vfs.Registration(nil), r.items...)
	r.items = nil
	r.mu.Unlock()
	for index := len(items) - 1; index >= 0; index-- {
		items[index].Unregister()
	}
}

func descriptorFallbackLabel(descriptor PluginCommandDescriptor) string {
	if label := strings.TrimSpace(descriptor.Label); label != "" {
		return label
	}
	if label := strings.TrimSpace(descriptor.LocalizedLabels["en"]); label != "" {
		return label
	}
	keys := make([]string, 0, len(descriptor.LocalizedLabels))
	for language, label := range descriptor.LocalizedLabels {
		if strings.TrimSpace(label) != "" {
			keys = append(keys, language)
		}
	}
	sort.Strings(keys)
	if len(keys) != 0 {
		return strings.TrimSpace(descriptor.LocalizedLabels[keys[0]])
	}
	return ""
}

func rpcPluginCommandVisible(activeDrives []string, app vfs.App) bool {
	if len(activeDrives) == 0 {
		return true
	}
	if app == nil {
		return false
	}
	active, ok := app.GetActivePanelVFS().(*RPCVFS)
	if !ok || active == nil {
		return false
	}
	for _, drive := range activeDrives {
		if strings.EqualFold(strings.TrimSpace(drive), active.driveName) {
			return true
		}
	}
	return false
}

func registerRPCPluginCommands(
	api vfs.HostAPI,
	back PluginTransport,
	pluginName string,
	descriptors []PluginCommandDescriptor,
	registrations *pluginSessionRegistrations,
) error {
	if len(descriptors) == 0 {
		return nil
	}
	contributions, ok := api.(vfs.ContributionHost)
	if !ok {
		return fmt.Errorf("plugin %q declares commands, but the host has no command contribution API", pluginName)
	}
	if back == nil {
		return errors.New("plugin command transport is nil")
	}
	for _, raw := range descriptors {
		descriptor := raw
		descriptor.ID = strings.TrimSpace(descriptor.ID)
		label := descriptorFallbackLabel(descriptor)
		if descriptor.ID == "" {
			return fmt.Errorf("plugin %q declares a command with an empty ID", pluginName)
		}
		if label == "" {
			return fmt.Errorf("plugin %q command %q has no label", pluginName, descriptor.ID)
		}
		location := vfs.PluginCommandLocation(descriptor.Location)
		if location != vfs.PluginCommandPanel && location != vfs.PluginCommandConfig {
			return fmt.Errorf("plugin %q command %q has invalid location %d", pluginName, descriptor.ID, descriptor.Location)
		}

		activeDrives := append([]string(nil), descriptor.ActiveDrives...)
		commandID := descriptor.ID
		registration, err := contributions.RegisterPluginCommand(vfs.PluginCommand{
			ID:                    commandID,
			Location:              location,
			Label:                 label,
			Description:           descriptor.Description,
			Shortcut:              descriptor.Shortcut,
			MenuPath:              descriptor.MenuPath,
			LocalizedLabels:       descriptor.LocalizedLabels,
			LocalizedDescriptions: descriptor.LocalizedDescriptions,
			SearchTerms:           descriptor.SearchTerms,
			Visible: func(app vfs.App) bool {
				return rpcPluginCommandVisible(activeDrives, app)
			},
			Run: func(vfs.App) {
				if err := back.Call("Plugin.RunCommand", PluginRunCommandRequest{ID: commandID}, nil); err != nil {
					vtui.DebugLog("PLUGIN COMMAND [%s/%s]: %v", pluginName, commandID, err)
				}
			},
		})
		if err != nil {
			return fmt.Errorf("register plugin %q command %q: %w", pluginName, commandID, err)
		}
		if !registrations.Add(registration) {
			return fmt.Errorf("plugin %q disconnected while registering command %q", pluginName, commandID)
		}
	}
	return nil
}
