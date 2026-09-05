package archive

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/unxed/archives"
	"github.com/unxed/sevenzip"
	"github.com/unxed/vtui"
	zipperarchive "github.com/unxed/zipper/archive"
)

type archivePasswordResult struct {
	password string
	err      error
}

// archivePasswordPrompt is replaceable in tests so password handling can be
// verified without depending on an interactive terminal.
var archivePasswordPrompt = promptArchivePassword

type archivePasswordValidationError struct {
	message string
}

func (e archivePasswordValidationError) Error() string {
	return "archive password rejected: " + e.message
}

func newArchivePasswordValidationError(format string, args ...any) error {
	return archivePasswordValidationError{message: fmt.Sprintf(format, args...)}
}

// isArchivePasswordRetryError extends zipper's password classification with
// lazy 7z payload errors.  7z archives may leave their headers unencrypted;
// in that case opening and listing the archive succeeds with an empty or
// wrong password, and the decoder reports only a sevenzip.ReadError on the
// first file read.  The ReadError does not always carry the Encrypted flag,
// so zipperarchive.IsPasswordError cannot identify this case by itself.
func isArchivePasswordRetryError(err error) bool {
	if zipperarchive.IsPasswordError(err) {
		return true
	}
	var readErr sevenzip.ReadError
	if errors.As(err, &readErr) {
		return true
	}
	var readErrPtr *sevenzip.ReadError
	return errors.As(err, &readErrPtr) && readErrPtr != nil
}

func promptArchivePassword(ctx context.Context, archiveName string) (string, error) {
	if vtui.FrameManager == nil {
		return "", errors.New("archive: cannot request a password without an active UI")
	}

	result := make(chan archivePasswordResult, 1)
	vtui.FrameManager.PostTask(func() {
		showArchivePasswordDialog(archiveName, result)
	})

	select {
	case value := <-result:
		return value.password, value.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func showArchivePasswordDialog(archiveName string, result chan<- archivePasswordResult) {
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
	finish := func(value archivePasswordResult) {
		if finished {
			return
		}
		finished = true
		result <- value
	}
	ok.OnClick = func() {
		finish(archivePasswordResult{password: password.GetText()})
		password.SetText("")
		dlg.Close()
	}
	cancel.OnClick = func() { dlg.Close() }
	dlg.OnResult = func(code int) {
		if code < 0 {
			finish(archivePasswordResult{err: context.Canceled})
		}
	}

	vtui.FrameManager.Push(dlg)
}

// promptArchivePasswordUntilProvided asks for a password the way FAR does:
// an empty answer simply shows the dialog again, and only closing the
// dialog (Cancel/Esc) gives up.
func promptArchivePasswordUntilProvided(ctx context.Context, displayName string) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		password, err := archivePasswordPrompt(ctx, displayName)
		if err != nil {
			return "", err
		}
		if password != "" {
			return password, nil
		}
	}
}

func openArchiveFSWithPasswordPrompt(ctx context.Context, localPath, displayName string, backing io.Closer) (zipperarchive.FileSystem, string, bool, error) {
	var password string
	for {
		fsys, cleanupTransferred, err := openArchiveFSWithContext(ctx, localPath, displayName, backing, password)
		if err == nil {
			return fsys, password, cleanupTransferred, nil
		}
		if cleanupTransferred || !isArchivePasswordRetryError(err) {
			return nil, "", cleanupTransferred, err
		}

		password, err = promptArchivePasswordUntilProvided(ctx, displayName)
		if err != nil {
			return nil, "", cleanupTransferred, err
		}
	}
}

// openWithPassword is called after an archive operation failed because the
// archive (or a member) needs a password or rejected the current one. Every
// backend used by f4 opens archives lazily and only reports a wrong password
// while listing or reading, so the password that is currently installed has
// already been tried by the failed operation. It is therefore never reused
// silently: the user is asked for a new one, and the caller retries the
// operation with it. A retry that fails again simply lands here again, so
// the prompt keeps coming back until the password is right or the dialog is
// closed, which is exactly FAR's behaviour. Concurrent failures share one
// prompt: whoever arrives after a newer password was installed just retries.
func (v *ArchiveVFS) openWithPassword(ctx context.Context, cause error) error {
	if !isArchivePasswordRetryError(cause) {
		return cause
	}
	if ctx == nil {
		ctx = context.Background()
	}

	v.mu.Lock()
	localPath := v.activePath()
	displayName := v.displayName
	failedGen := v.passwordGen
	v.mu.Unlock()

	v.passwordPromptMu.Lock()
	defer v.passwordPromptMu.Unlock()

	v.mu.Lock()
	if v.isClosed {
		v.mu.Unlock()
		return errors.New("archive VFS is closed")
	}
	if v.passwordGen != failedGen {
		// Another operation has just installed a newer password. Let the
		// caller retry with it before bothering the user again.
		v.mu.Unlock()
		return nil
	}
	v.mu.Unlock()

	for {
		password, err := promptArchivePasswordUntilProvided(ctx, displayName)
		if err != nil {
			return err
		}
		fsys, _, err := openArchiveFSWithContext(ctx, localPath, displayName, nil, password)
		if err != nil {
			if isArchivePasswordRetryError(err) {
				// Eager rejection (encrypted headers): ask again right away.
				continue
			}
			return err
		}
		return v.installPasswordFS(fsys, password)
	}
}

func (v *ArchiveVFS) installPasswordFS(fsys zipperarchive.FileSystem, password string) error {
	v.mu.Lock()
	if v.isClosed {
		v.mu.Unlock()
		_ = fsys.Close()
		return errors.New("archive VFS is closed")
	}
	v.cancelCleanupLocked()
	oldFS := v.fsys
	v.fsys = fsys
	v.password = password
	v.passwordGen++
	v.mu.Unlock()
	if oldFS != nil {
		_ = oldFS.Close()
	}
	return nil
}

func archivePasswordFormat(format archives.Format, password string) (archives.Format, bool) {
	if password == "" {
		return format, false
	}
	switch format := format.(type) {
	case archives.Rar:
		format.Password = password
		return format, true
	case *archives.Rar:
		format.Password = password
		return format, true
	case archives.SevenZip:
		format.Password = password
		return format, true
	case *archives.SevenZip:
		format.Password = password
		return format, true
	default:
		return format, false
	}
}
