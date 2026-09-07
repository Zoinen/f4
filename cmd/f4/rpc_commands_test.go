package main

import (
	"context"
	"reflect"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/vmihailenco/msgpack/v5"
)

func TestPluginInitResponseAcceptsLegacyAndExtendedWireFormats(t *testing.T) {
	legacy, err := msgpack.Marshal([]string{"Legacy drive"})
	if err != nil {
		t.Fatal(err)
	}
	var legacyResponse PluginInitResponse
	if err := msgpack.Unmarshal(legacy, &legacyResponse); err != nil {
		t.Fatalf("legacy Plugin.Init response: %v", err)
	}
	if !reflect.DeepEqual(legacyResponse.Drives, []string{"Legacy drive"}) || len(legacyResponse.Commands) != 0 {
		t.Fatalf("legacy response decoded as %#v", legacyResponse)
	}

	extended, err := msgpack.Marshal(map[string]any{
		"Drives": []string{"New drive"},
		"Commands": []PluginCommandDescriptor{{
			ID:       "sample.command",
			Location: uint8(vfs.PluginCommandPanel),
			Label:    "Sample command",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var extendedResponse PluginInitResponse
	if err := msgpack.Unmarshal(extended, &extendedResponse); err != nil {
		t.Fatalf("extended Plugin.Init response: %v", err)
	}
	if !reflect.DeepEqual(extendedResponse.Drives, []string{"New drive"}) || len(extendedResponse.Commands) != 1 || extendedResponse.Commands[0].ID != "sample.command" {
		t.Fatalf("extended response decoded as %#v", extendedResponse)
	}
}

type rpcCommandTestTransport struct {
	methods  []string
	requests []PluginRunCommandRequest
}

func (transport *rpcCommandTestTransport) Call(method string, params any, _ any) error {
	transport.methods = append(transport.methods, method)
	if request, ok := params.(PluginRunCommandRequest); ok {
		transport.requests = append(transport.requests, request)
	}
	return nil
}

type rpcCommandTestApp struct{ active vfs.VFS }

func (app *rpcCommandTestApp) GetActivePanelVFS() vfs.VFS                { return app.active }
func (*rpcCommandTestApp) GetPassivePanelVFS() vfs.VFS                   { return nil }
func (*rpcCommandTestApp) GetSelectedNames() []string                    { return nil }
func (*rpcCommandTestApp) GetSelectedName() string                       { return "" }
func (*rpcCommandTestApp) RefreshAll()                                   {}
func (*rpcCommandTestApp) SetPendingSelection(string)                    {}
func (*rpcCommandTestApp) Message(string, string, []string) int          { return -1 }
func (*rpcCommandTestApp) InputBox(string, string, string, func(string)) {}
func (*rpcCommandTestApp) Menu(string, []string, func(int))              {}
func (*rpcCommandTestApp) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (*rpcCommandTestApp) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}

func findPluginCommandByID(commands []vfs.PluginCommand, id string) (vfs.PluginCommand, bool) {
	for _, command := range commands {
		if command.ID == id {
			return command, true
		}
	}
	return vfs.PluginCommand{}, false
}

func TestRPCPluginCommandsRegisterLocalizeExecuteAndUnregister(t *testing.T) {
	const commandID = "test.rpc-command.greeting"
	oldLanguage := AppConfig.Language
	AppConfig.Language = "ru"
	t.Cleanup(func() { AppConfig.Language = oldLanguage })

	transport := &rpcCommandTestTransport{}
	registrations := &pluginSessionRegistrations{}
	descriptor := PluginCommandDescriptor{
		ID:          commandID,
		Location:    uint8(vfs.PluginCommandPanel),
		Label:       "Show greeting",
		Description: "Show the RPC greeting",
		Shortcut:    "F1",
		MenuPath:    "Commands",
		LocalizedLabels: map[string]string{
			"ru": "Показать RPC-приветствие",
		},
		LocalizedDescriptions: map[string]string{
			"ru": "Показать приветствие внешнего плагина",
		},
		SearchTerms:  []string{"hello", "привет"},
		ActiveDrives: []string{"RPC Test Drive"},
	}
	if err := registerRPCPluginCommands(&coreAPI{}, transport, "test-rpc", []PluginCommandDescriptor{descriptor}, registrations); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registrations.Unregister)

	hiddenApp := &rpcCommandTestApp{active: NewRPCVFS(transport, "Different Drive")}
	if command, ok := findPluginCommandByID(pluginCommandsSnapshot(vfs.PluginCommandPanel, hiddenApp), commandID); ok {
		t.Fatalf("drive-scoped RPC command leaked into another drive: %#v", command)
	}

	app := &rpcCommandTestApp{active: NewRPCVFS(transport, "rpc test drive")}
	command, ok := findPluginCommandByID(pluginCommandsSnapshot(vfs.PluginCommandPanel, app), commandID)
	if !ok {
		t.Fatal("RPC command is missing in its active drive")
	}
	if got := pluginCommandDisplayLabel(command); got != "Показать RPC-приветствие" {
		t.Fatalf("localized label = %q", got)
	}
	if got := pluginCommandDisplayDescription(command); got != "Показать приветствие внешнего плагина" {
		t.Fatalf("localized description = %q", got)
	}
	if command.MenuPath != "Commands" {
		t.Fatalf("menu path = %q, want Commands", command.MenuPath)
	}
	wantSearch := []string{"hello", "привет", "Показать RPC-приветствие", "Показать приветствие внешнего плагина"}
	if got := pluginCommandSearchTerms(command); !reflect.DeepEqual(got, wantSearch) {
		t.Fatalf("search terms = %#v, want %#v", got, wantSearch)
	}
	if !executeRegisteredPluginCommand(vfs.PluginCommandPanel, commandID, app) {
		t.Fatal("live RPC command was not executed")
	}
	if !reflect.DeepEqual(transport.methods, []string{"Plugin.RunCommand"}) ||
		!reflect.DeepEqual(transport.requests, []PluginRunCommandRequest{{ID: commandID}}) {
		t.Fatalf("transport calls = %#v / %#v", transport.methods, transport.requests)
	}

	registrations.Unregister()
	if _, ok := findPluginCommandByID(pluginCommandsSnapshot(vfs.PluginCommandPanel, app), commandID); ok {
		t.Fatal("RPC command survived session cleanup")
	}
}

func TestPluginSessionRegistrationsRejectLateContribution(t *testing.T) {
	registrations := &pluginSessionRegistrations{}
	registrations.Unregister()
	called := 0
	if registrations.Add(&unregisterFunc{fn: func() { called++ }}) {
		t.Fatal("closed session accepted a late contribution")
	}
	if called != 1 {
		t.Fatalf("late contribution cleanup calls = %d", called)
	}
}
