package multiarc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/unxed/f4/vfs"
)

// addCommandID is the regular build's own ID for Add to archive (see
// plugins/archive), shared so that a key binding or a macro written for
// "archive.add" does the same thing in both builds.
const addCommandID = "archive.add"

// addCommand is Add to archive as the lite build offers it: the same
// label, localization keys, Files-menu place and far2l shortcut (Shift+F1)
// as the regular build's, backed by createArchive instead of the native
// archive libraries.
func addCommand() vfs.PluginCommand {
	return vfs.PluginCommand{
		ID:             addCommandID,
		Location:       vfs.PluginCommandPanel,
		Label:          "Add to archive",
		LabelKey:       "Archive.Command.Add",
		MenuPath:       "Files",
		Shortcut:       "Shift+F1",
		Description:    "Create an archive from the selected files",
		DescriptionKey: "Archive.Command.Add.Desc",
		SearchKeys:     []string{"Attributes.Archive"},
		Run:            actionAddArchive,
	}
}

// selectedForArchive is the panel selection without "..", which is
// navigation, not something to archive (the regular build learned this in
// #983).
func selectedForArchive(app vfs.App) []string {
	var names []string
	for _, name := range app.GetSelectedNames() {
		if name != "" && name != ".." {
			names = append(names, name)
		}
	}
	return names
}

// suggestedArchiveBase is the name Add to archive offers before the
// suffix, the regular build's rule (f4#1504): the item's own name for a
// single item, the panel directory's for several.
func suggestedArchiveBase(names []string, srcDir string) string {
	if len(names) == 1 {
		return names[0]
	}
	if base := filepath.Base(srcDir); base != "." && base != string(filepath.Separator) && base != "" {
		return base
	}
	return "archive"
}

// actionAddArchive runs on the UI goroutine, as every command and hotkey
// handler does, so everything that can block -- probing the tools, which
// runs "tar --version", and every app.Message -- happens on a goroutine of
// its own.
func actionAddArchive(app vfs.App) {
	names := selectedForArchive(app)
	if len(names) == 0 {
		return
	}
	osvfs, ok := app.GetActivePanelVFS().(*vfs.OSVFS)
	if !ok {
		go app.Message(" Add to archive ", "In this build, Add to archive works on files on the local disk only.", []string{"&Ok"})
		return
	}
	srcDir, err := osvfs.Abs(osvfs.GetPath())
	if err != nil {
		go app.Message(" Error ", err.Error(), []string{"&Ok"})
		return
	}
	go func() {
		suggestion := suggestedArchiveBase(names, srcDir) + defaultCreateSuffix(context.Background())
		app.InputBox(" Add to archive ", "Archive name:", suggestion, func(name string) {
			if name == "" {
				return
			}
			go runAddArchive(app, srcDir, names, name)
		})
	}()
}

// runAddArchive creates the archive the user named, relative to the
// panel's directory unless the name is absolute. It checks the format
// first, so that a name no tool can make is reported before the user is
// asked about overwriting anything.
func runAddArchive(app vfs.App, srcDir string, names []string, name string) {
	target := filepath.Clean(name)
	if !filepath.IsAbs(target) {
		target = filepath.Join(srcDir, target)
	}
	if _, err := planCreate(context.Background(), filepath.Base(target)); err != nil {
		app.Message(" Error ", err.Error(), []string{"&Ok"})
		return
	}
	if info, err := os.Stat(target); err == nil {
		if info.IsDir() {
			app.Message(" Error ", fmt.Sprintf("%s is a folder.", target), []string{"&Ok"})
			return
		}
		if app.Message(" Warning ", "The target archive already exists.\nDo you want to overwrite it?", []string{"&Yes", "&No"}) != 0 {
			return
		}
	}
	app.RunProgressTask(" Archiving... ", "Archiving files...", false, func(ctx context.Context, update func(string, int)) error {
		vfs.GlobalArchiveLockManager.Lock(target)
		defer vfs.GlobalArchiveLockManager.Unlock(target)
		update(fmt.Sprintf("Archiving into %s", filepath.Base(target)), -1)
		return createArchive(ctx, srcDir, names, target)
	}, func(err error) {
		// onComplete runs on the UI goroutine, which app.Message waits on.
		if err != nil && !errors.Is(err, context.Canceled) {
			go app.Message(" Error ", fmt.Sprintf("Archiving failed:\n%v", err), []string{"&Ok"})
		}
		if err == nil && filepath.Dir(target) == srcDir {
			app.SetPendingSelection(filepath.Base(target))
		}
		app.RefreshAll()
	})
}
