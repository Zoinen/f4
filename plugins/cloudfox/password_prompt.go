package cloudfox

import (
	"context"
	"errors"

	"github.com/unxed/vtui"
)

const emptyMasterPasswordWarning = "Using an empty master password leaves the CloudFox vault effectively unprotected. " +
	"OAuth tokens, access keys, and passwords stored on this drive must be treated as plaintext: " +
	"anyone who can read the vault file can recover and use them. Continue without a master password?"

// vtuiMasterPasswordPrompter keeps the portable vault password entirely in a
// masked, transient control. The prompt is marshalled to the UI thread because
// vault access normally happens from a file-operation worker.
type vtuiMasterPasswordPrompter struct{}

type masterPasswordResult struct {
	password string
	err      error
}

func (vtuiMasterPasswordPrompter) PromptMasterPassword(ctx context.Context, creating bool) (string, error) {
	if vtui.FrameManager == nil {
		return "", errors.New("cloudfox: cannot unlock portable vault without an active UI")
	}
	result := make(chan masterPasswordResult, 1)
	vtui.FrameManager.PostTask(func() {
		showMasterPasswordDialog(creating, result)
	})
	select {
	case value := <-result:
		return value.password, value.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func showMasterPasswordDialog(creating bool, result chan<- masterPasswordResult) {
	title := " CloudFox vault "
	height := 9
	if creating {
		height = 11
	}
	dlg := vtui.NewCenteredDialog(58, height, title)
	dlg.ShowClose = true

	x := dlg.X1 + 2
	y := dlg.Y1 + 2
	width := 34
	password := vtui.NewPasswordEdit(x+18, y, width, "")
	dlg.AddItem(vtui.NewLabel(x, y, vtui.Msg("CloudFox.VaultMasterPassword"), password))
	dlg.AddItem(password)

	var confirmation *vtui.Edit
	if creating {
		confirmation = vtui.NewPasswordEdit(x+18, y+2, width, "")
		dlg.AddItem(vtui.NewLabel(x, y+2, vtui.Msg("CloudFox.VaultRepeatPassword"), confirmation))
		dlg.AddItem(confirmation)
	}

	buttonY := dlg.Y2 - 2
	save := vtui.NewButton(dlg.X1+18, buttonY, vtui.Msg("CloudFox.VaultUnlock"))
	if creating {
		save.SetText("&Create")
	}
	save.IsDefault = true
	cancel := vtui.NewButton(dlg.X1+31, buttonY, vtui.Msg("vtui.Cancel"))
	dlg.AddItem(save)
	dlg.AddItem(cancel)

	finished := false
	finish := func(value masterPasswordResult) {
		if finished {
			return
		}
		finished = true
		result <- value
	}
	accept := func(value string) {
		finish(masterPasswordResult{password: value})
		password.SetText("")
		if confirmation != nil {
			confirmation.SetText("")
		}
		dlg.Close()
	}
	dlg.OnResult = func(code int) {
		if code < 0 {
			finish(masterPasswordResult{err: context.Canceled})
		}
	}
	save.OnClick = func() {
		value := password.GetText()
		if confirmation != nil && value != confirmation.GetText() {
			vtui.ShowMessageOn(dlg, " CloudFox ", "Passwords do not match.", []string{"&OK"})
			return
		}
		if creating && value == "" {
			warning := vtui.ShowMessageOn(
				dlg,
				" Warning ",
				emptyMasterPasswordWarning,
				[]string{"&Use empty password", "&Cancel"},
			)
			warning.OnResult = func(code int) {
				if code == 0 {
					accept("")
				}
			}
			return
		}
		accept(value)
	}
	cancel.OnClick = func() { dlg.Close() }
	vtui.FrameManager.Push(dlg)
}

var _ MasterPasswordPrompter = vtuiMasterPasswordPrompter{}
