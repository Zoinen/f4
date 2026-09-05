package main

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestCommandPrefixRegistrationDispatchAndUpdate(t *testing.T) {
	api := &coreAPI{}
	var argument string
	registration, err := api.RegisterCommandPrefix("test.command-prefix", "Media_Test", func(_ vfs.App, value string) {
		argument = value
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	if !dispatchCommandPrefix(nil, `  MEDIA_TEST: "clip one.mp4"`) {
		t.Fatal("registered prefix was not dispatched")
	}
	if argument != ` "clip one.mp4"` {
		t.Fatalf("argument = %q", argument)
	}
	if dispatchCommandPrefix(nil, "unrelated:value") {
		t.Fatal("unknown prefix was consumed")
	}

	if err := registration.SetPrefix("Changed"); err != nil {
		t.Fatal(err)
	}
	if dispatchCommandPrefix(nil, "Media_Test:value") {
		t.Fatal("old prefix remained active")
	}
	if !dispatchCommandPrefix(nil, "changed:value") {
		t.Fatal("updated prefix was not active")
	}

	if err := registration.SetPrefix(""); err != nil {
		t.Fatal(err)
	}
	if dispatchCommandPrefix(nil, "changed:value") {
		t.Fatal("disabled prefix was dispatched")
	}
}

func TestCommandPrefixRegistrationRejectsInvalidAndDuplicate(t *testing.T) {
	api := &coreAPI{}
	registration, err := api.RegisterCommandPrefix("test.command-prefix-owner", "UniquePrefix", func(vfs.App, string) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	if _, err := api.RegisterCommandPrefix("test.command-prefix-duplicate", "uniqueprefix", func(vfs.App, string) {}); err == nil {
		t.Fatal("case-insensitive duplicate prefix was accepted")
	}
	if _, err := api.RegisterCommandPrefix("test.command-prefix-invalid", "bad prefix", func(vfs.App, string) {}); err == nil {
		t.Fatal("invalid prefix was accepted")
	}

	registration.Unregister()
	if err := registration.SetPrefix("another"); err == nil {
		t.Fatal("unregistered prefix was updated")
	}
}

func TestPanelsFrameCommandPrefixIsConsumedBeforePTY(t *testing.T) {
	api := &coreAPI{}
	called := false
	registration, err := api.RegisterCommandPrefix("test.command-prefix-enter", "CorePrefix", func(app vfs.App, argument string) {
		called = app != nil && argument == " selected.mkv"
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pty := pf.pty.(*mockPty)
	pf.cmdLine.Edit.SetText("CorePrefix: selected.mkv")
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if !called {
		t.Fatal("prefix handler was not invoked with the panel app and raw argument")
	}
	if got := pf.cmdLine.Edit.GetText(); got != "" {
		t.Fatalf("command line was not cleared: %q", got)
	}
	if len(pty.written) != 0 {
		t.Fatalf("prefix leaked to PTY: %q", pty.written)
	}
	if !pf.showPanels {
		t.Fatal("prefix command unexpectedly hid panels")
	}
}
