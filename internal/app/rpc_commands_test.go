package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
	msgpack "github.com/vmihailenco/msgpack/v5"
)

func TestPluginInitResponseAcceptsLegacyAndExtendedWireFormats(t *testing.T) {
	legacy, err := msgpack.Marshal([]string{"Legacy drive"})
	if err != nil {
		t.Fatal(err)
	}
	var legacyResponse plughost.PluginInitResponse
	if err := msgpack.Unmarshal(legacy, &legacyResponse); err != nil {
		t.Fatalf("legacy Plugin.Init response: %v", err)
	}
	if !reflect.DeepEqual(legacyResponse.Drives, []string{"Legacy drive"}) || len(legacyResponse.Commands) != 0 {
		t.Fatalf("legacy response decoded as %#v", legacyResponse)
	}

	extended, err := msgpack.Marshal(map[string]any{
		"Drives": []string{"New drive"},
		"Commands": []plughost.PluginCommandDescriptor{{
			ID:       "sample.command",
			Location: uint8(vfs.PluginCommandPanel),
			Label:    "Sample command",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var extendedResponse plughost.PluginInitResponse
	if err := msgpack.Unmarshal(extended, &extendedResponse); err != nil {
		t.Fatalf("extended Plugin.Init response: %v", err)
	}
	if !reflect.DeepEqual(extendedResponse.Drives, []string{"New drive"}) || len(extendedResponse.Commands) != 1 || extendedResponse.Commands[0].ID != "sample.command" {
		t.Fatalf("extended response decoded as %#v", extendedResponse)
	}
}

type rpcCommandTestTransport struct {
	methods  []string
	requests []plughost.PluginRunCommandRequest
}

func (transport *rpcCommandTestTransport) Call(method string, params any, _ any) error {
	transport.methods = append(transport.methods, method)
	if request, ok := params.(plughost.PluginRunCommandRequest); ok {
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
	oldLanguage := config.App.Language
	config.App.Language = "ru"
	t.Cleanup(func() { config.App.Language = oldLanguage })

	transport := &rpcCommandTestTransport{}
	registrations := &plughost.PluginSessionRegistrations{}
	descriptor := plughost.PluginCommandDescriptor{
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
	if err := plughost.RegisterRPCPluginCommands(&CoreAPI{}, transport, "test-rpc", []plughost.PluginCommandDescriptor{descriptor}, registrations); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registrations.Unregister)

	hiddenApp := &rpcCommandTestApp{active: plughost.NewRPCVFS(transport, "Different Drive")}
	if command, ok := findPluginCommandByID(plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, hiddenApp), commandID); ok {
		t.Fatalf("drive-scoped RPC command leaked into another drive: %#v", command)
	}

	app := &rpcCommandTestApp{active: plughost.NewRPCVFS(transport, "rpc test drive")}
	command, ok := findPluginCommandByID(plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, app), commandID)
	if !ok {
		t.Fatal("RPC command is missing in its active drive")
	}
	if got := plughost.PluginCommandDisplayLabel(command); got != "Показать RPC-приветствие" {
		t.Fatalf("localized label = %q", got)
	}
	if got := plughost.PluginCommandDisplayDescription(command); got != "Показать приветствие внешнего плагина" {
		t.Fatalf("localized description = %q", got)
	}
	if command.MenuPath != "Commands" {
		t.Fatalf("menu path = %q, want Commands", command.MenuPath)
	}
	wantSearch := []string{"hello", "привет", "Показать RPC-приветствие", "Показать приветствие внешнего плагина"}
	if got := plughost.PluginCommandSearchTerms(command); !reflect.DeepEqual(got, wantSearch) {
		t.Fatalf("search terms = %#v, want %#v", got, wantSearch)
	}
	if !plughost.ExecutePluginCommand(vfs.PluginCommandPanel, commandID, app) {
		t.Fatal("live RPC command was not executed")
	}
	if !reflect.DeepEqual(transport.methods, []string{"Plugin.RunCommand"}) ||
		!reflect.DeepEqual(transport.requests, []plughost.PluginRunCommandRequest{{ID: commandID}}) {
		t.Fatalf("transport calls = %#v / %#v", transport.methods, transport.requests)
	}

	registrations.Unregister()
	if _, ok := findPluginCommandByID(plughost.PluginCommandsSnapshot(vfs.PluginCommandPanel, app), commandID); ok {
		t.Fatal("RPC command survived session cleanup")
	}
}

func TestPluginSessionRegistrationsRejectLateContribution(t *testing.T) {
	registrations := &plughost.PluginSessionRegistrations{}
	registrations.Unregister()
	called := 0
	if registrations.Add(plughost.NewUnregisterFunc(func() { called++ })) {
		t.Fatal("closed session accepted a late contribution")
	}
	if called != 1 {
		t.Fatalf("late contribution cleanup calls = %d", called)
	}
}
