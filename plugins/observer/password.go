package observer

// This file gives newObserverVFS (vfs.go) and Provider.CanOpen (provider.go)
// f4#1563's own answer to OpenStorage returning SOR_PASSWORD_REQUIRED --
// item 3 of what "часть 5 из N" (status/1563.md in the accounting
// repository) deliberately left for a later, separate part, done here in
// part 7.
//
// The retry loop mirrors plugins/archive/password.go's own
// openArchiveFSWithPasswordPrompt/promptArchivePasswordUntilProvided, but is
// considerably simpler: API v6 (ModuleDef.h) has exactly one password slot
// per OpenStorage call and no separate "wrong password" result code, so
// there is nothing here like plugins/archive's per-member ZipCrypto/RAR
// header reclassification -- just try, and on SOR_PASSWORD_REQUIRED ask and
// retry, until the module accepts a password or the user gives up.
//
// The dialog deliberately reuses plugins/archive's own
// "Archive.PasswordTitle"/"Archive.Password" language keys instead of
// adding Observer-specific ones: the text ("Archive password"/"Password:")
// reads fine for any container format Enter opens, and tools/langfmt's own
// contract ("a translation may omit keys") is exactly what makes reusing an
// already-translated pair preferable to adding a new one that would need
// its own pass through internal/i18n/lang's other language files.

import (
	"context"
	"errors"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type observerPasswordResult struct {
	password string
	err      error
}

// observerPasswordPrompt is replaceable in tests, the same way
// plugins/archive.archivePasswordPrompt is.
var observerPasswordPrompt = promptObserverPassword

func promptObserverPassword(ctx context.Context, containerName string) (string, error) {
	if vtui.FrameManager == nil {
		return "", errors.New("observer: cannot request a password without an active UI")
	}

	result := make(chan observerPasswordResult, 1)
	vtui.FrameManager.PostTask(func() {
		showObserverPasswordDialog(containerName, result)
	})

	select {
	case value := <-result:
		return value.password, value.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// showObserverPasswordDialog mirrors plugins/archive's
// showArchivePasswordDialog layout and behaviour exactly, including leaving
// containerName unused in the dialog itself -- neither does
// plugins/archive's own archiveName in its current dialog; both are kept as
// parameters for a possible future title.
func showObserverPasswordDialog(containerName string, result chan<- observerPasswordResult) {
	dlg := vtui.NewCenteredDialog(52, 7, vtui.Msg("Archive.PasswordTitle"))
	dlg.ShowClose = true

	x := dlg.X1 + 2
	y := dlg.Y1 + 2
	password := vtui.NewPasswordEdit(x+12, y, 34, "")
	dlg.AddItem(vtui.NewLabel(x, y, vtui.Msg("Archive.Password"), password))
	dlg.AddItem(password)

	ok := vtui.NewButton(dlg.X1+15, dlg.Y2-2, vtui.Msg("vtui.Ok"))
	ok.IsDefault = true
	cancel := vtui.NewButton(dlg.X1+28, dlg.Y2-2, vtui.Msg("vtui.Cancel"))
	dlg.AddItem(ok)
	dlg.AddItem(cancel)

	finished := false
	finish := func(value observerPasswordResult) {
		if finished {
			return
		}
		finished = true
		result <- value
	}
	ok.OnClick = func() {
		finish(observerPasswordResult{password: password.GetText()})
		password.SetText("")
		dlg.Close()
	}
	cancel.OnClick = func() { dlg.Close() }
	dlg.OnResult = func(code int) {
		if code < 0 {
			finish(observerPasswordResult{err: context.Canceled})
		}
	}

	vtui.FrameManager.Push(dlg)
}

// promptObserverPasswordUntilProvided asks for a password the way FAR does
// (plugins/archive.promptArchivePasswordUntilProvided): an empty answer just
// brings the dialog back, only Cancel/Esc gives up.
func promptObserverPasswordUntilProvided(ctx context.Context, containerName string) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		password, err := observerPasswordPrompt(ctx, containerName)
		if err != nil {
			return "", err
		}
		if password != "" {
			return password, nil
		}
	}
}

// openStorageWithPasswordPrompt drives OpenStorage, asking for and retrying
// with a password for as long as the module keeps answering
// SOR_PASSWORD_REQUIRED. containerName is only used for the dialog; guestPath
// is OpenStorage's own FilePath (the guest-visible path under the WASI mount,
// e.g. "/"+guestName), unrelated to and never influenced by containerName.
//
// The interactive-prompt hold (vfs.HoldInteractivePrompt) is only taken once
// a password is actually needed, mirroring
// plugins/archive.openArchiveFSWithPasswordPrompt: the common case (no
// password at all) never touches it.
func openStorageWithPasswordPrompt(ctx context.Context, mod *Module, containerName, guestPath string) (OpenResult, error) {
	var password string
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()

	for {
		res, err := mod.OpenStorage(StorageOpenParams{FilePath: guestPath, Password: password})
		if err != nil {
			return OpenResult{}, err
		}
		if res.Code != SORPasswordRequired {
			return res, nil
		}

		if release == nil {
			release = vfs.HoldInteractivePrompt()
		}
		password, err = promptObserverPasswordUntilProvided(ctx, containerName)
		if err != nil {
			return OpenResult{}, err
		}
	}
}
