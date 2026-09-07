package mediainfo

import (
	"context"
	"sync"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type pluginTestRegistration struct {
	mu           sync.Mutex
	unregistered int
}

func (registration *pluginTestRegistration) Unregister() {
	registration.mu.Lock()
	registration.unregistered++
	registration.mu.Unlock()
}

type pluginTestPrefixRegistration struct {
	pluginTestRegistration
	prefix string
}

func (registration *pluginTestPrefixRegistration) SetPrefix(prefix string) error {
	registration.prefix = prefix
	return nil
}

type pluginTestHost struct {
	commands []vfs.PluginCommand
	quick    vfs.QuickViewProvider
	prefix   *pluginTestPrefixRegistration
	macro    vfs.MacroCallProvider
	regs     []*pluginTestRegistration
	logs     []string
}

func (host *pluginTestHost) registration() *pluginTestRegistration {
	registration := &pluginTestRegistration{}
	host.regs = append(host.regs, registration)
	return registration
}

func (*pluginTestHost) GetVersion() string                           { return "test" }
func (host *pluginTestHost) Log(message string)                      { host.logs = append(host.logs, message) }
func (*pluginTestHost) Message(string)                               {}
func (*pluginTestHost) RegisterHighlighter(vtui.HighlighterProvider) {}
func (*pluginTestHost) RegisterVFSProvider(vfs.VFSProvider)          {}
func (*pluginTestHost) RegisterURIProvider(vfs.URIProvider) error    { return nil }
func (*pluginTestHost) RegisterDrive(string, func() vfs.VFS)         {}
func (*pluginTestHost) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {
}
func (*pluginTestHost) RegisterPluginMenuItem(string, func(vfs.App)) {}
func (*pluginTestHost) RunAction(string) bool                        { return false }

func (host *pluginTestHost) RegisterQuickViewProvider(provider vfs.QuickViewProvider) (vfs.Registration, error) {
	host.quick = provider
	return host.registration(), nil
}

func (host *pluginTestHost) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.commands = append(host.commands, command)
	return host.registration(), nil
}

func (host *pluginTestHost) RegisterCommandPrefix(_ string, prefix string, _ func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	host.prefix = &pluginTestPrefixRegistration{prefix: prefix}
	host.regs = append(host.regs, &host.prefix.pluginTestRegistration)
	return host.prefix, nil
}

func (host *pluginTestHost) RegisterMacroCallProvider(provider vfs.MacroCallProvider) (vfs.Registration, error) {
	host.macro = provider
	return host.registration(), nil
}

func TestPluginRegistersAndUnregistersEveryContribution(t *testing.T) {
	host := &pluginTestHost{}
	plugin := NewPlugin(t.TempDir())
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if len(host.commands) != 2 || host.commands[0].ID != panelCommandID || host.commands[1].ID != configCommandID {
		t.Fatalf("commands = %#v", host.commands)
	}
	if host.quick == nil || host.quick.Name() != quickViewID {
		t.Fatalf("quick provider = %#v", host.quick)
	}
	if host.prefix == nil || host.prefix.prefix != "MediaInfo" {
		t.Fatalf("prefix = %#v", host.prefix)
	}
	if len(host.macro.IDs) != 2 || host.macro.IDs[0] != macroID || host.macro.IDs[1] != legacyMacroGUID {
		t.Fatalf("macro IDs = %#v", host.macro.IDs)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	for index, registration := range host.regs {
		registration.mu.Lock()
		count := registration.unregistered
		registration.mu.Unlock()
		if count != 1 {
			t.Errorf("registration %d unregistered %d times", index, count)
		}
	}
}

func TestAnalyzePathRejectsDirectoryBeforeOpening(t *testing.T) {
	plugin := NewPlugin(t.TempDir())
	_, err := plugin.analyzePath(context.Background(), vfs.NewOSVFS(t.TempDir()), "folder", vfs.VFSItem{Name: "folder", IsDir: true}, ModeFast)
	if err == nil {
		t.Fatal("directory was accepted")
	}
}
