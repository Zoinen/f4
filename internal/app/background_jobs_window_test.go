package app

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

func TestShowBackgroundJobs_RendersAndControlsJobs(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	oldRegistry := terminal.GlobalBackgroundJobs
	t.Cleanup(func() { terminal.GlobalBackgroundJobs = oldRegistry })
	registry := terminal.NewBackgroundJobRegistry()
	terminal.GlobalBackgroundJobs = registry

	cancelled := false
	running := registry.Start("Long task", func() { cancelled = true })
	running.SetStatus("halfway")
	finished := registry.Start("Done task", nil)
	opened := false
	finished.FinishWith("answer", func() { opened = true })

	ShowBackgroundJobs(nil)
	top := vtui.FrameManager.GetTopFrame()
	container, ok := top.(interface{ GetChildren() []vtui.UIElement })
	if !ok {
		t.Fatal("background jobs dialog does not expose its controls")
	}

	var list *vtui.ListBox
	var buttons []*vtui.Button
	for _, child := range container.GetChildren() {
		switch item := child.(type) {
		case *vtui.ListBox:
			list = item
		case *vtui.Button:
			buttons = append(buttons, item)
		}
	}
	if list == nil || len(buttons) != 3 {
		t.Fatalf("dialog controls = list %v, buttons %d; want one list and three buttons", list != nil, len(buttons))
	}
	if len(list.Items) != 2 || !strings.Contains(list.Items[0], "running") || !strings.Contains(list.Items[0], "halfway") || !strings.Contains(list.Items[1], "finished") || !strings.Contains(list.Items[1], "answer") {
		t.Fatalf("job rows = %#v, want running/finished rows with statuses", list.Items)
	}

	list.SelectPos = 0
	buttons[1].OnClick()
	if !cancelled || registry.List()[0].Status != "cancelling" {
		t.Fatalf("cancel result = cancelled %v, jobs %#v", cancelled, registry.List())
	}

	// A running job cannot be opened; the dialog refreshes the rows and stays up.
	buttons[0].OnClick()
	if top.IsDone() || len(registry.List()) != 2 {
		t.Fatalf("opening running job changed dialog/list: done=%v jobs=%#v", top.IsDone(), registry.List())
	}

	list.SelectPos = 1
	buttons[0].OnClick()
	if !opened || !top.IsDone() {
		t.Fatalf("opening finished job = opened %v, dialog done %v", opened, top.IsDone())
	}
	if len(registry.List()) != 1 || registry.List()[0].ID != running.ID() {
		t.Fatalf("remaining jobs = %#v, want only running job", registry.List())
	}
}

func TestShowBackgroundJobs_EmptyListAndClose(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	oldRegistry := terminal.GlobalBackgroundJobs
	t.Cleanup(func() { terminal.GlobalBackgroundJobs = oldRegistry })
	terminal.GlobalBackgroundJobs = terminal.NewBackgroundJobRegistry()

	ShowBackgroundJobs(nil)
	top := vtui.FrameManager.GetTopFrame()
	container := top.(interface{ GetChildren() []vtui.UIElement })
	var list *vtui.ListBox
	var buttons []*vtui.Button
	for _, child := range container.GetChildren() {
		switch item := child.(type) {
		case *vtui.ListBox:
			list = item
		case *vtui.Button:
			buttons = append(buttons, item)
		}
	}
	if list == nil || len(list.Items) != 1 || list.Items[0] != i18n.Msg("Jobs.Empty") {
		t.Fatalf("empty rows = %#v, want the empty marker", list)
	}

	list.SelectPos = 99
	buttons[0].OnClick()
	buttons[1].OnClick()
	if top.IsDone() {
		t.Fatal("invalid selection closed the empty dialog")
	}
	buttons[2].OnClick()
	if !top.IsDone() {
		t.Fatal("close button did not close the dialog")
	}
}
