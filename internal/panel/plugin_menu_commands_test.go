package panel

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestCommandsForPluginMenuDropsTheBasicShellCommands(t *testing.T) {
	in := []vfs.PluginCommand{
		{ID: "archive.add", NotInPluginMenu: true},
		{ID: "proclist.open"},
		{ID: "archive.extract", NotInPluginMenu: true},
	}
	got := commandsForPluginMenu(in)
	if len(got) != 1 || got[0].ID != "proclist.open" {
		t.Fatalf("commands = %#v, want only the ordinary plugin command", got)
	}
	if len(commandsForPluginMenu(nil)) != 0 {
		t.Fatal("nothing in, nothing out")
	}
}
