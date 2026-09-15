package filemenu

import (
	"os/exec"
	"runtime"
	"sort"
	"strings"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// AppKit must be entered on the process's original OS thread, including when
// this executable is launched as a helper from a terminal-mode F4.
func init() { runtime.LockOSThread() }

type cocoaPoint struct{ X, Y float64 }

const Platform = "darwin"

func nsString(value string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), value)
}

func cocoaApplications(paths []string, workspace objc.ID) []Entry {
	common := map[string]Entry{}
	for index, path := range paths {
		url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("fileURLWithPath:"), nsString(path))
		apps := workspace.Send(objc.RegisterName("URLsForApplicationsToOpenURL:"), url)
		current := map[string]Entry{}
		for i := uintptr(0); i < uintptr(apps.Send(objc.RegisterName("count"))); i++ {
			app := apps.Send(objc.RegisterName("objectAtIndex:"), i)
			appPath := app.Send(objc.RegisterName("path"))
			path := objc.Send[string](appPath, objc.RegisterName("UTF8String"))
			label := appPath.Send(objc.RegisterName("lastPathComponent")).Send(objc.RegisterName("stringByDeletingPathExtension"))
			current[path] = Entry{ID: "app:" + path, Label: objc.Send[string](label, objc.RegisterName("UTF8String"))}
		}
		if index == 0 {
			common = current
		} else {
			for id := range common {
				if _, ok := current[id]; !ok {
					delete(common, id)
				}
			}
		}
	}
	var entries []Entry
	for _, entry := range common {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Label < entries[j].Label })
	return entries
}

func showNative(r Request) Result {
	if _, err := purego.Dlopen("/System/Library/Frameworks/AppKit.framework/AppKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	workspace := objc.ID(objc.GetClass("NSWorkspace")).Send(objc.RegisterName("sharedWorkspace"))
	apps := cocoaApplications(r.Paths, workspace)
	if r.Operation == "applications" {
		return Result{Outcome: Selected, Entries: apps}
	}
	if r.Operation != "" {
		return cocoaInvoke(r, r.Operation, apps)
	}
	application := objc.ID(objc.GetClass("NSApplication")).Send(objc.RegisterName("sharedApplication"))
	application.Send(objc.RegisterName("setActivationPolicy:"), int64(1)) // accessory: no Dock icon
	application.Send(objc.RegisterName("finishLaunching"))
	previous := workspace.Send(objc.RegisterName("frontmostApplication"))
	selected := ""
	actionSelector := objc.RegisterName("chooseFileAction:")
	var ids []string
	class, err := objc.RegisterClass("F4FileMenuTarget", objc.GetClass("NSObject"), nil, nil, []objc.MethodDef{{Cmd: actionSelector, Fn: func(_ objc.ID, _ objc.SEL, sender objc.ID) {
		index := int(sender.Send(objc.RegisterName("tag")))
		if index >= 0 && index < len(ids) {
			selected = ids[index]
		}
	}}})
	if err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	target := objc.ID(class).Send(objc.RegisterName("new"))
	defer target.Send(objc.RegisterName("release"))
	var build func([]Entry) objc.ID
	build = func(entries []Entry) objc.ID {
		menu := objc.ID(objc.GetClass("NSMenu")).Send(objc.RegisterName("new"))
		menu.Send(objc.RegisterName("setAutoenablesItems:"), false)
		for _, entry := range entries {
			children := entry.Children
			if entry.ID == "open-with" {
				children = apps
				if len(children) == 0 {
					continue
				}
			}
			item := objc.ID(objc.GetClass("NSMenuItem")).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithTitle:action:keyEquivalent:"), nsString(entry.Label), actionSelector, nsString(""))
			item.Send(objc.RegisterName("setTarget:"), target)
			item.Send(objc.RegisterName("setEnabled:"), !entry.Disabled)
			item.Send(objc.RegisterName("setTag:"), int64(len(ids)))
			ids = append(ids, entry.ID)
			if len(children) > 0 {
				sub := build(children)
				item.Send(objc.RegisterName("setSubmenu:"), sub)
				sub.Send(objc.RegisterName("release"))
			}
			menu.Send(objc.RegisterName("addItem:"), item)
			item.Send(objc.RegisterName("release"))
		}
		return menu
	}
	menu := build(r.Entries)
	defer menu.Send(objc.RegisterName("release"))
	point := objc.Send[cocoaPoint](objc.ID(objc.GetClass("NSEvent")), objc.RegisterName("mouseLocation"))
	if r.Position.Valid {
		point = cocoaPoint{float64(r.Position.X), float64(r.Position.Y)}
	}
	application.Send(objc.RegisterName("activateIgnoringOtherApps:"), true)
	// NSMenu runs its own tracking event loop and fits the popup to the screen.
	menu.Send(objc.RegisterName("popUpMenuPositioningItem:atLocation:inView:"), objc.ID(0), point, objc.ID(0))
	if selected == "" {
		previous.Send(objc.RegisterName("activateWithOptions:"), uint64(0))
		return Result{Outcome: Cancelled}
	}
	if selected == "open" || selected == "quick-look" || selected == "copy-files" || strings.HasPrefix(selected, "app:") {
		if strings.HasPrefix(selected, "app:") {
			r.Application = selected
			selected = "open-with"
		}
		return cocoaInvoke(r, selected, apps)
	}
	previous.Send(objc.RegisterName("activateWithOptions:"), uint64(0))
	return Result{Outcome: Selected, Action: selected}
}

func cocoaInvoke(r Request, operation string, apps []Entry) Result {
	var command string
	var args []string
	switch operation {
	case "open":
		command = "/usr/bin/open"
		args = []string{"--"}
	case "open-with":
		found := false
		for _, app := range apps {
			if app.ID == r.Application {
				found = true
			}
		}
		if !found {
			return Result{Outcome: Failed, Error: "Application no longer supports this selection"}
		}
		command = "/usr/bin/open"
		args = []string{"-a", strings.TrimPrefix(r.Application, "app:"), "--"}
	case "quick-look":
		command = "/usr/bin/qlmanage"
		args = []string{"-p"}
	case "copy-files":
		urls := objc.ID(objc.GetClass("NSMutableArray")).Send(objc.RegisterName("array"))
		for _, path := range r.Paths {
			url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("fileURLWithPath:"), nsString(path))
			urls.Send(objc.RegisterName("addObject:"), url)
		}
		board := objc.ID(objc.GetClass("NSPasteboard")).Send(objc.RegisterName("generalPasteboard"))
		board.Send(objc.RegisterName("clearContents"))
		if board.Send(objc.RegisterName("writeObjects:"), urls) == 0 {
			return Result{Outcome: Failed, Error: "Cannot copy files to clipboard"}
		}
		return Result{Outcome: Invoked}
	default:
		return Result{Outcome: Unavailable}
	}
	args = append(args, r.Paths...)
	if err := exec.Command(command, args...).Run(); err != nil {
		return Result{Outcome: Failed, Error: err.Error()}
	}
	return Result{Outcome: Invoked}
}
