package cloudfox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func newMasterPasswordPromptFrameManager(t *testing.T) *vtui.FrameManagerType {
	t.Helper()
	oldManager := vtui.FrameManager
	fm := vtui.NewFrameManager()
	fm.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager = fm
	t.Cleanup(func() {
		fm.Shutdown()
		vtui.FrameManager = oldManager
	})
	return fm
}

func masterPasswordDialogControls(t *testing.T, dialog *vtui.Window) ([]*vtui.Edit, []*vtui.Button) {
	t.Helper()
	var edits []*vtui.Edit
	var buttons []*vtui.Button
	for _, child := range dialog.GetChildren() {
		switch control := child.(type) {
		case *vtui.Edit:
			edits = append(edits, control)
		case *vtui.Button:
			buttons = append(buttons, control)
		}
	}
	return edits, buttons
}

func masterPasswordResultNow(t *testing.T, result <-chan masterPasswordResult) masterPasswordResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	default:
		t.Fatal("master password dialog did not produce a result")
		return masterPasswordResult{}
	}
}

func assertNoMasterPasswordResult(t *testing.T, result <-chan masterPasswordResult) {
	t.Helper()
	select {
	case value := <-result:
		t.Fatalf("master password dialog produced an unexpected result: %#v", value)
	default:
	}
}

func TestVTUIMasterPasswordPrompterRequiresActiveUI(t *testing.T) {
	oldManager := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = oldManager })

	_, err := (vtuiMasterPasswordPrompter{}).PromptMasterPassword(context.Background(), false)
	if err == nil || err.Error() != "cloudfox: cannot unlock portable vault without an active UI" {
		t.Fatalf("PromptMasterPassword error = %v, want missing-UI error", err)
	}
}

func TestVTUIMasterPasswordPrompterHonorsCanceledContext(t *testing.T) {
	oldManager := vtui.FrameManager
	vtui.FrameManager = vtui.NewFrameManager()
	t.Cleanup(func() { vtui.FrameManager = oldManager })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (vtuiMasterPasswordPrompter{}).PromptMasterPassword(ctx, false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PromptMasterPassword error = %v, want context.Canceled", err)
	}
}

func TestVTUIMasterPasswordPrompterRoundTrip(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	type outcome struct {
		password string
		err      error
	}
	out := make(chan outcome, 1)
	go func() {
		password, err := (vtuiMasterPasswordPrompter{}).PromptMasterPassword(context.Background(), false)
		out <- outcome{password: password, err: err}
	}()

	task := <-fm.TaskChan
	task()
	dialog, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want password dialog", fm.GetTopFrame())
	}
	edits, buttons := masterPasswordDialogControls(t, dialog)
	if len(buttons) != 2 {
		t.Fatalf("unlock dialog has %d buttons, want 2", len(buttons))
	}
	if len(edits) != 1 {
		t.Fatalf("unlock dialog has %d edit controls, want 1", len(edits))
	}
	edits[0].SetText("vault-secret")
	buttons[0].OnClick()

	select {
	case got := <-out:
		if got.err != nil || got.password != "vault-secret" {
			t.Fatalf("PromptMasterPassword result = %#v, want vault-secret without error", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("PromptMasterPassword did not receive the accepted password")
	}
	if edits[0].GetText() != "" {
		t.Fatalf("password edit retained secret after accepting: %q", edits[0].GetText())
	}
}

func TestShowMasterPasswordDialogCancelAndFinishGuard(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	result := make(chan masterPasswordResult, 1)
	showMasterPasswordDialog(false, result)
	dialog, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want password dialog", fm.GetTopFrame())
	}
	_, buttons := masterPasswordDialogControls(t, dialog)
	if len(buttons) != 2 {
		t.Fatalf("cancel dialog has %d buttons, want 2", len(buttons))
	}

	// A non-negative result is ignored; cancel then exercises the negative path.
	dialog.OnResult(0)
	assertNoMasterPasswordResult(t, result)
	buttons[1].OnClick()
	got := masterPasswordResultNow(t, result)
	if !errors.Is(got.err, context.Canceled) || got.password != "" {
		t.Fatalf("cancel result = %#v, want context.Canceled", got)
	}

	// Closing again must not send a second result through the buffered channel.
	dialog.OnResult(-1)
	assertNoMasterPasswordResult(t, result)
}

func TestShowMasterPasswordDialogRejectsMismatchedConfirmation(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	result := make(chan masterPasswordResult, 1)
	showMasterPasswordDialog(true, result)
	dialog, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want password dialog", fm.GetTopFrame())
	}
	edits, buttons := masterPasswordDialogControls(t, dialog)
	if len(buttons) != 2 {
		t.Fatalf("creation dialog has %d buttons, want 2", len(buttons))
	}
	if len(edits) != 2 {
		t.Fatalf("creation dialog has %d edit controls, want 2", len(edits))
	}
	edits[0].SetText("first")
	edits[1].SetText("second")
	buttons[0].OnClick()
	assertNoMasterPasswordResult(t, result)

	message, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok || message == dialog {
		t.Fatalf("top frame after mismatch = %T, want mismatch message dialog", fm.GetTopFrame())
	}
	_, messageButtons := masterPasswordDialogControls(t, message)
	if len(messageButtons) != 1 {
		t.Fatalf("mismatch message has %d buttons, want 1", len(messageButtons))
	}
	messageButtons[0].OnClick()

	edits[1].SetText("first")
	buttons[0].OnClick()
	got := masterPasswordResultNow(t, result)
	if got.err != nil || got.password != "first" {
		t.Fatalf("accepted result = %#v, want first without error", got)
	}
	if edits[0].GetText() != "" || edits[1].GetText() != "" {
		t.Fatal("password fields were not cleared after accepting a created vault")
	}
}

func TestShowMasterPasswordDialogWarnsBeforeEmptyPassword(t *testing.T) {
	fm := newMasterPasswordPromptFrameManager(t)
	result := make(chan masterPasswordResult, 1)
	showMasterPasswordDialog(true, result)
	dialog, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want password dialog", fm.GetTopFrame())
	}
	_, buttons := masterPasswordDialogControls(t, dialog)
	if len(buttons) != 2 {
		t.Fatalf("empty-password dialog has %d buttons, want 2", len(buttons))
	}

	buttons[0].OnClick()
	assertNoMasterPasswordResult(t, result)
	warning, ok := fm.GetTopFrame().(*vtui.Window)
	if !ok || warning == dialog {
		t.Fatalf("top frame after empty password = %T, want warning dialog", fm.GetTopFrame())
	}
	_, warningButtons := masterPasswordDialogControls(t, warning)
	if len(warningButtons) != 2 {
		t.Fatalf("empty-password warning has %d buttons, want 2", len(warningButtons))
	}
	warningButtons[1].OnClick()
	assertNoMasterPasswordResult(t, result)

	// Reopen the warning and choose the explicit empty-password confirmation.
	buttons[0].OnClick()
	warning, ok = fm.GetTopFrame().(*vtui.Window)
	if !ok || warning == dialog {
		t.Fatalf("top frame after reopening warning = %T, want warning dialog", fm.GetTopFrame())
	}
	_, warningButtons = masterPasswordDialogControls(t, warning)
	warningButtons[0].OnClick()
	got := masterPasswordResultNow(t, result)
	if got.err != nil || got.password != "" {
		t.Fatalf("empty-password result = %#v, want empty password without error", got)
	}
}
