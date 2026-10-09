package cloudfox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func yandexDialogControls(t *testing.T, dialog *vtui.Window) (*vtui.Edit, []*vtui.Button) {
	t.Helper()
	var edit *vtui.Edit
	var buttons []*vtui.Button
	for _, child := range dialog.GetChildren() {
		switch control := child.(type) {
		case *vtui.Edit:
			edit = control
		case *vtui.Button:
			buttons = append(buttons, control)
		}
	}
	if edit == nil || len(buttons) != 2 {
		t.Fatalf("Yandex dialog controls: edit=%v buttons=%d", edit != nil, len(buttons))
	}
	return edit, buttons
}
func yandexDialog(t *testing.T, ctx context.Context) (*vtui.Window, chan yandexCodeResult) {
	t.Helper()
	result := make(chan yandexCodeResult, 1)
	showYandexAuthorizationCodeDialog(ctx, result)
	dialog, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want Yandex dialog", vtui.FrameManager.GetTopFrame())
	}
	return dialog, result
}

func TestPromptYandexAuthorizationCodeRequiresActiveUI(t *testing.T) {
	oldManager := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = oldManager })

	if _, err := promptYandexAuthorizationCode(context.Background()); err == nil || err.Error() != "cloudfox: cannot request a Yandex authorization code without an active UI" {
		t.Fatalf("missing-UI error = %v", err)
	}
}

func TestPromptYandexAuthorizationCodeHonorsCanceledContext(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := promptYandexAuthorizationCode(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled prompt error = %v", err)
	}
	select {
	case task := <-fm.TaskChan:
		task()
	case <-time.After(time.Second):
		t.Fatal("canceled prompt did not post its task")
	}
}

func TestPromptYandexAuthorizationCodeRoundTrip(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	out := make(chan yandexCodeResult, 1)
	go func() {
		code, err := promptYandexAuthorizationCode(ctx)
		out <- yandexCodeResult{code: code, err: err}
	}()
	task := <-fm.TaskChan
	task()
	dialog, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want Yandex dialog", fm.GetTopFrame())
	}
	edit, buttons := yandexDialogControls(t, dialog)
	edit.SetText("  authorization-code  ")
	buttons[0].OnClick()
	select {
	case result := <-out:
		if result.err != nil || result.code != "authorization-code" {
			t.Fatalf("prompt result = %#v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("prompt did not return after submit")
	}
	cancel()
}

func TestShowYandexAuthorizationCodeDialogHasControls(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dialog, _ := yandexDialog(t, ctx)
	edit, buttons := yandexDialogControls(t, dialog)
	if edit.GetText() != "" || !buttons[0].IsDefault {
		t.Fatalf("initial Yandex controls: edit=%q default=%v", edit.GetText(), buttons[0].IsDefault)
	}
	if fm.GetTopFrame() != dialog {
		t.Fatal("Yandex dialog was not pushed as the top frame")
	}
}

func TestShowYandexAuthorizationCodeDialogRejectsEmptyCode(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dialog, result := yandexDialog(t, ctx)
	_, buttons := yandexDialogControls(t, dialog)
	buttons[0].OnClick()
	select {
	case got := <-result:
		t.Fatalf("empty code produced result %#v", got)
	default:
	}
	if fm.GetTopFrame() == dialog {
		t.Fatal("empty code closed the authorization dialog")
	}
}

func TestShowYandexAuthorizationCodeDialogTrimsAndClearsCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dialog, result := yandexDialog(t, ctx)
	edit, buttons := yandexDialogControls(t, dialog)
	edit.SetText("\tcode-value\n")
	buttons[0].OnClick()
	got := <-result
	if got.err != nil || got.code != "code-value" {
		t.Fatalf("submitted result = %#v", got)
	}
	if edit.GetText() != "" {
		t.Fatalf("authorization code remained in edit: %q", edit.GetText())
	}
}

func TestShowYandexAuthorizationCodeDialogCancelReturnsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dialog, result := yandexDialog(t, ctx)
	_, buttons := yandexDialogControls(t, dialog)
	buttons[1].OnClick()
	got := <-result
	if !errors.Is(got.err, context.Canceled) || got.code != "" {
		t.Fatalf("cancel result = %#v", got)
	}
}

func TestShowYandexAuthorizationCodeDialogIgnoresNonNegativeResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dialog, result := yandexDialog(t, ctx)
	dialog.OnResult(0)
	select {
	case got := <-result:
		t.Fatalf("non-negative result produced %#v", got)
	default:
	}
}

func TestShowYandexAuthorizationCodeDialogNegativeResultIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	dialog, result := yandexDialog(t, ctx)
	dialog.OnResult(-1)
	got := <-result
	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("negative result = %#v", got)
	}
}

func TestShowYandexAuthorizationCodeDialogClosesOnContextCancel(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	dialog, result := yandexDialog(t, ctx)
	cancel()
	select {
	case task := <-fm.TaskChan:
		task()
	case <-time.After(time.Second):
		t.Fatal("context cancellation did not post a close task")
	}
	if !dialog.IsDone() {
		t.Fatal("authorization dialog remained open after context cancellation")
	}
	if got := <-result; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("context cancellation result = %#v", got)
	}
}
