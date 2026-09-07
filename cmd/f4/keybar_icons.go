package main

import "strings"

// keyBarIconForAction maps stable action IDs, never translated labels, to the
// small Lucide subset exposed by the Qt host. The final token-based fallbacks
// keep user-remapped and plugin-provided actions useful without making the
// semantic key-bar payload depend on presentation text.
func keyBarIconForAction(actionName string) string {
	name := strings.ToLower(strings.TrimSpace(actionName))
	if name == "" || name == "none" {
		return ""
	}

	switch name {
	case "app.help":
		return "circle-question-mark"
	case "app.mainmenu", "panel.usermenu":
		return "menu"
	case "workspace.list":
		return "panels-top-left"
	case "file.view", "terminal.viewlog", "editor.switchtoviewer", "panel.quickview":
		return "eye"
	case "file.edit", "terminal.editlog", "viewer.switchtoeditor":
		return "file-pen-line"
	case "file.new":
		return "file-plus"
	case "file.copy", "file.copyinplace":
		return "copy"
	case "file.move":
		return "folder-input"
	case "file.rename":
		return "pencil"
	case "file.makedir":
		return "folder-plus"
	case "file.delete", "file.deletepermanent":
		return "trash-2"
	case "app.quit":
		return "log-out"
	case "editor.quit", "viewer.quit", "workspace.close":
		return "x"
	case "panel.pluginmenu", "settings.pluginconfiguration", "settings.plugins":
		return "plug"
	case "file.find", "editor.search", "editor.searchnext", "editor.searchforward",
		"editor.searchprevious", "viewer.search", "viewer.searchnext", "viewer.searchprevious",
		"app.commandpalette":
		return "search"
	case "panel.commandhistory", "panel.viewereditorhistory":
		return "clock-3"
	case "panel.foldershistory":
		return "folder-clock"
	case "panel.sortbyname":
		return "arrow-down-a-z"
	case "panel.sortbyext":
		return "file-type"
	case "panel.sortbytime":
		return "clock-3"
	case "panel.sortbysize", "panel.sortmenu":
		return "arrow-down-wide-narrow"
	case "panel.sortunsorted":
		return "list-restart"
	case "panel.leftdrivemenu", "panel.rightdrivemenu":
		return "hard-drive"
	case "panel.toggleleftpanel":
		return "panel-left"
	case "panel.togglerightpanel":
		return "panel-right"
	case "editor.save", "app.savesettings":
		return "save"
	case "editor.wordwrap", "viewer.wrapmode":
		return "text-wrap"
	case "editor.hexmode", "viewer.hexmode":
		return "binary"
	case "editor.showwhitespaces":
		return "space"
	case "editor.codepagenext", "editor.codepagemenu", "editor.convertcodepage",
		"viewer.codepagenext", "viewer.codepagemenu":
		return "languages"
	case "editor.replace":
		return "refresh-cw"
	case "viewer.goto":
		return "locate-fixed"
	case "app.togglewindowsize":
		return "maximize-2"
	case "debug.dummyoperation":
		return "flask-conical"
	}

	switch {
	case strings.Contains(name, "drive"):
		return "hard-drive"
	case strings.Contains(name, "delete"), strings.Contains(name, "remove"):
		return "trash-2"
	case strings.Contains(name, "copy"):
		return "copy"
	case strings.Contains(name, "rename"):
		return "pencil"
	case strings.Contains(name, "move"):
		return "folder-input"
	case strings.Contains(name, "folder"), strings.Contains(name, "directory"):
		return "folder"
	case strings.Contains(name, "search"), strings.Contains(name, "find"):
		return "search"
	case strings.Contains(name, "edit"):
		return "file-pen-line"
	case strings.Contains(name, "view"):
		return "eye"
	case strings.Contains(name, "save"):
		return "save"
	case strings.Contains(name, "sort"):
		return "arrow-down-wide-narrow"
	case strings.Contains(name, "history"):
		return "clock-3"
	case strings.Contains(name, "settings"), strings.Contains(name, "config"):
		return "file-cog"
	case strings.Contains(name, "terminal"), strings.Contains(name, "command"):
		return "square-terminal"
	case strings.Contains(name, "close"), strings.Contains(name, "quit"), strings.Contains(name, "exit"):
		return "x"
	default:
		return "circle-play"
	}
}
