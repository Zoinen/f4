package cloudfox

import (
	"context"
	"errors"
	"strings"

	"github.com/unxed/vtui"
)

type yandexCodeResult struct {
	code string
	err  error
}

// promptYandexAuthorizationCode asks for the short-lived code displayed on
// Yandex's fixed verification_code redirect page. The code stays in memory
// and is never written to profile metadata or command history.
func promptYandexAuthorizationCode(ctx context.Context) (string, error) {
	if vtui.FrameManager == nil {
		return "", errors.New("cloudfox: cannot request a Yandex authorization code without an active UI")
	}
	result := make(chan yandexCodeResult, 1)
	vtui.FrameManager.PostTask(func() {
		if err := ctx.Err(); err != nil {
			result <- yandexCodeResult{err: err}
			return
		}
		showYandexAuthorizationCodeDialog(ctx, result)
	})
	select {
	case value := <-result:
		return value.code, value.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func showYandexAuthorizationCodeDialog(ctx context.Context, result chan<- yandexCodeResult) {
	dlg := vtui.NewCenteredDialog(68, 10, vtui.Msg("CloudFox.YandexAuthorizationTitle"))
	dlg.ShowClose = true
	x := dlg.X1 + 2
	y := dlg.Y1 + 2

	instructions := vtui.NewText(x, y, "Authorize in the browser, then paste the displayed code here:", vtui.Palette[vtui.ColDialogText])
	instructions.SetPosition(x, y, dlg.X2-2, y)
	dlg.AddItem(instructions)

	code := vtui.NewEdit(x+20, y+2, 42, "")
	dlg.AddItem(vtui.NewLabel(x, y+2, vtui.Msg("CloudFox.YandexAuthorizationCode"), code))
	dlg.AddItem(code)

	submit := vtui.NewButton(dlg.X1+20, dlg.Y2-2, vtui.Msg("CloudFox.Continue"))
	submit.IsDefault = true
	cancel := vtui.NewButton(dlg.X1+36, dlg.Y2-2, vtui.Msg("vtui.Cancel"))
	dlg.AddItem(submit)
	dlg.AddItem(cancel)

	finished := false
	finish := func(value yandexCodeResult) {
		if finished {
			return
		}
		finished = true
		result <- value
	}
	submit.OnClick = func() {
		value := strings.TrimSpace(code.GetText())
		if value == "" {
			vtui.ShowMessageOn(dlg, " Yandex authorization ", "Paste the authorization code shown in the browser.", []string{"&OK"})
			return
		}
		finish(yandexCodeResult{code: value})
		code.SetText("")
		dlg.Close()
	}
	cancel.OnClick = dlg.Close
	dlg.OnResult = func(resultCode int) {
		if resultCode < 0 {
			finish(yandexCodeResult{err: context.Canceled})
		}
	}
	dlg.SetFocusedItem(code)
	vtui.FrameManager.Push(dlg)

	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	go func() {
		<-ctx.Done()
		if frames != nil {
			frames.PostTask(func() {
				if !dlg.IsDone() {
					dlg.Close()
				}
			})
		}
	}()
}
