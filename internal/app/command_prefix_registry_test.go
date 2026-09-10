package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"testing"

	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestCommandPrefixRegistrationDispatchAndUpdate(t *testing.T) {
	api := &CoreAPI{}
	var argument string
	registration, err := api.RegisterCommandPrefix("test.command-prefix", "Media_Test", func(_ vfs.App, value string) {
		argument = value
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	if !panel.DispatchCommandPrefix(nil, `  MEDIA_TEST: "clip one.mp4"`) {
		t.Fatal("registered prefix was not dispatched")
	}
	if argument != ` "clip one.mp4"` {
		t.Fatalf("argument = %q", argument)
	}
	if panel.DispatchCommandPrefix(nil, "unrelated:value") {
		t.Fatal("unknown prefix was consumed")
	}

	if err := registration.SetPrefix("Changed"); err != nil {
		t.Fatal(err)
	}
	if panel.DispatchCommandPrefix(nil, "Media_Test:value") {
		t.Fatal("old prefix remained active")
	}
	if !panel.DispatchCommandPrefix(nil, "changed:value") {
		t.Fatal("updated prefix was not active")
	}

	if err := registration.SetPrefix(""); err != nil {
		t.Fatal(err)
	}
	if panel.DispatchCommandPrefix(nil, "changed:value") {
		t.Fatal("disabled prefix was dispatched")
	}
}

func TestCommandPrefixRegistrationRejectsInvalidAndDuplicate(t *testing.T) {
	api := &CoreAPI{}
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
	api := &CoreAPI{}
	called := false
	registration, err := api.RegisterCommandPrefix("test.command-prefix-enter", "CorePrefix", func(app vfs.App, argument string) {
		called = app != nil && argument == " selected.mkv"
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pty := pf.Pty.(*paneltest.MockPty)
	pf.CmdLine.Edit.SetText("CorePrefix: selected.mkv")
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if !called {
		t.Fatal("prefix handler was not invoked with the panel app and raw argument")
	}
	if got := pf.CmdLine.Edit.GetText(); got != "" {
		t.Fatalf("command line was not cleared: %q", got)
	}
	if len(pty.Written) != 0 {
		t.Fatalf("prefix leaked to term.PTY: %q", pty.Written)
	}
	if !pf.ShowPanels {
		t.Fatal("prefix command unexpectedly hid panels")
	}
}

func TestCommandPrefixOpensRegisteredDrive(t *testing.T) {
	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()

	t.Cleanup(sysinfo.SnapshotDrives())
	sysinfo.SetDrives([]sysinfo.DriveEntry{{Name: "ExampleDrive", Factory: func() vfs.VFS {
		return vfs.NewNullVFS(0)
	}}})

	if !panel.DispatchCommandPrefix(pf, "EXAMPLEDRIVE:") {
		t.Fatal("drive prefix was not consumed")
	}
	if _, ok := pf.GetActivePanel().Vfs.(*vfs.NullVFS); !ok {
		t.Fatalf("active panel VFS = %T, want *vfs.NullVFS", pf.GetActivePanel().Vfs)
	}
}

func TestCommandPrefixDriveRequiresBarePrefix(t *testing.T) {
	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()

	t.Cleanup(sysinfo.SnapshotDrives())
	sysinfo.SetDrives([]sysinfo.DriveEntry{{Name: "ExampleDrive", Factory: func() vfs.VFS {
		return vfs.NewNullVFS(0)
	}}})

	if panel.DispatchCommandPrefix(pf, "ExampleDrive:/child") {
		t.Fatal("drive prefix with an argument was consumed")
	}
}
