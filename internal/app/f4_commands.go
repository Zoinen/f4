package app

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// f4CommandPrefix is the command-line prefix of f4's own pseudo commands:
// far2l's far: under f4's name (#1177).
const f4CommandPrefix = "f4"

// f4Commands maps each f4: pseudo command to the action it runs. The pseudo
// command and its Commands menu item are one action, the way far2l's
// far:about and Commands > About FAR both reach FarAbout.
var f4Commands = []struct {
	name   string
	action string
}{
	{name: "about", action: "App.About"},
	{name: "config", action: "App.ConfigEditor"},
}

func init() {
	if _, err := (&coreAPI{}).RegisterCommandPrefix(f4CommandPrefix, f4CommandPrefix, runF4Command); err != nil {
		vtui.DebugLog("F4COMMANDS: cannot register the f4: prefix: %v", err)
	}
}

// f4CommandAction resolves what follows "f4:" to the action it names.
func f4CommandAction(argument string) (string, bool) {
	name := strings.TrimSpace(argument)
	for _, command := range f4Commands {
		if strings.EqualFold(name, command.name) {
			return command.action, true
		}
	}
	return "", false
}

// runF4Command is the f4: prefix handler. far2l hands an unknown far:
// command to the shell; a registered prefix has already consumed the line
// by the time its handler runs, so f4 says which commands exist instead.
func runF4Command(_ vfs.App, argument string) {
	if name, ok := f4CommandAction(argument); ok {
		RunAction(name)
		return
	}
	known := make([]string, 0, len(f4Commands))
	for _, command := range f4Commands {
		known = append(known, f4CommandPrefix+":"+command.name)
	}
	vtui.ShowMessage(i18n.Msg("F4Command.Title"),
		fmt.Sprintf(i18n.Msg("F4Command.Unknown"), f4CommandPrefix+":"+strings.TrimSpace(argument), strings.Join(known, ", ")),
		[]string{i18n.Msg("vtui.Ok")})
}

// actionAbout is f4:about and Commands > About f4.
func actionAbout() bool {
	dialog.ShowAbout(aboutFacts())
	return true
}

// actionConfigEditor is f4:config and Commands > Configuration editor. An
// edit refreshes the application the way Settings Center's Apply does.
func actionConfigEditor() bool {
	dialog.ShowConfigEditor(settingsHost{}.ApplyRuntime)
	return true
}

// aboutFacts collects what dialog cannot reach from its layer.
func aboutFacts() dialog.AboutFacts {
	facts := dialog.AboutFacts{
		Version: getLongVersionInfo(),
		Admin:   getAdminString(),
		Shell:   terminal.GetSystemShell(),
		Plugins: plughost.GlobalPluginManager.Names(),
		Copy:    terminal.SetClipboardAsync,
	}
	for _, entry := range panel.CommandPrefixSnapshot() {
		facts.CommandPrefixes = append(facts.CommandPrefixes, dialog.AboutCommandPrefix{Prefix: entry.Prefix, Owner: entry.Id})
	}
	return facts
}
