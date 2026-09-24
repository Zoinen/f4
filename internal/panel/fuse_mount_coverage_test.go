package panel

import (
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/internal/fusefs"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type fuseMountCoverageVFS struct {
	*vfs.NullVFS
	clone vfs.VFS
	title string
}

func (v *fuseMountCoverageVFS) Clone() vfs.VFS { return v.clone }

func (v *fuseMountCoverageVFS) PanelTitle(string) string { return v.title }

func mountMessage(t *testing.T) *vtui.Window {
	t.Helper()
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want mount message", vtui.FrameManager.GetTopFrame())
	}
	return dlg
}

func TestMountPlanCoversLocalCloneAndRefusedVFS(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	dir := t.TempDir()
	local := &FileSystemPanel{Vfs: vfs.NewOSVFS(dir)}
	label, run := mountPlan(local, true)
	if label != dir || run == nil {
		t.Fatalf("local mount plan = label %q, run %v; want %q and a runner", label, run != nil, dir)
	}

	clone := &fuseMountCoverageVFS{NullVFS: vfs.NewNullVFS(0), title: "Remote title"}
	if err := clone.SetPath("/clone-root"); err != nil {
		t.Fatal(err)
	}
	remote := &fuseMountCoverageVFS{NullVFS: vfs.NewNullVFS(0), clone: clone, title: "Remote title"}
	if err := remote.SetPath("/panel-root"); err != nil {
		t.Fatal(err)
	}
	label, run = mountPlan(&FileSystemPanel{Vfs: remote}, false)
	if label != "Remote title" || run == nil {
		t.Fatalf("clone mount plan = label %q, run %v; want titled clone", label, run != nil)
	}

	noClone := &fuseMountCoverageVFS{NullVFS: vfs.NewNullVFS(0)}
	if label, run = mountPlan(&FileSystemPanel{Vfs: noClone}, true); label != "" || run != nil {
		t.Fatalf("nil clone plan = label %q, run %v; want refusal", label, run != nil)
	}
	vtui.FrameManager.RemoveFrame(mountMessage(t))

	selfClone := &fuseMountCoverageVFS{NullVFS: vfs.NewNullVFS(0), title: "shared"}
	selfClone.clone = selfClone
	if label, run = mountPlan(&FileSystemPanel{Vfs: selfClone}, true); label != "" || run != nil {
		t.Fatalf("shared clone plan = label %q, run %v; want refusal", label, run != nil)
	}
	vtui.FrameManager.RemoveFrame(mountMessage(t))
}

func TestMountActivePanelUsesPlanningRunner(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	remote := &fuseMountCoverageVFS{NullVFS: vfs.NewNullVFS(0), title: "runner source"}
	clone := &fuseMountCoverageVFS{NullVFS: vfs.NewNullVFS(0)}
	remote.clone = clone
	fsp := &FileSystemPanel{Vfs: remote}
	pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0}
	var calls int
	var gotLabel string
	var gotReadOnly bool
	var gotRun func(context.Context) (*fusefs.Mount, error)
	oldRunner := RunPanelMountTask
	t.Cleanup(func() { RunPanelMountTask = oldRunner })
	RunPanelMountTask = func(_ *PanelsFrame, label string, readOnly bool, run func(context.Context) (*fusefs.Mount, error)) {
		calls++
		gotLabel = label
		gotReadOnly = readOnly
		gotRun = run
	}

	mountActivePanel(pf, true)
	if calls != 1 || gotLabel != "runner source" || !gotReadOnly || gotRun == nil {
		t.Fatalf("mount runner call = %d, label %q, readOnly %v, run %v", calls, gotLabel, gotReadOnly, gotRun != nil)
	}

	fsp.Vfs = nil
	mountActivePanel(pf, false)
	if calls != 1 {
		t.Fatalf("nil VFS invoked mount runner, calls = %d", calls)
	}
}

func TestReportMountCoversErrorsFallbackAndSuccess(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	remote := &fuseMountCoverageVFS{NullVFS: vfs.NewNullVFS(0), clone: vfs.NewNullVFS(0)}
	fsp := &FileSystemPanel{Vfs: remote}
	pf := &PanelsFrame{Panels: [2]Panel{fsp, nil}, ActiveIdx: 0}
	var fallbackCalls int
	var fallbackReadOnly bool
	oldRunner := RunPanelMountTask
	t.Cleanup(func() { RunPanelMountTask = oldRunner })
	RunPanelMountTask = func(_ *PanelsFrame, _ string, readOnly bool, _ func(context.Context) (*fusefs.Mount, error)) {
		fallbackCalls++
		fallbackReadOnly = readOnly
	}

	ReportMount(pf, "remote", nil, errors.New("read-write unavailable"), false)
	dlg := mountMessage(t)
	if dlg.OnResult == nil {
		t.Fatal("read-write error dialog has no fallback handler")
	}
	dlg.OnResult(1)
	vtui.FrameManager.RemoveFrame(dlg)

	ReportMount(pf, "remote", nil, errors.New("read-only unavailable"), true)
	dlg = mountMessage(t)
	if dlg.OnResult != nil {
		t.Fatal("read-only error dialog unexpectedly has a fallback handler")
	}
	vtui.FrameManager.RemoveFrame(dlg)

	ReportMount(pf, "remote", nil, errors.New("retry as read-only"), false)
	dlg = mountMessage(t)
	dlg.OnResult(0)
	vtui.FrameManager.RemoveFrame(dlg)
	if fallbackCalls != 1 || !fallbackReadOnly {
		t.Fatalf("fallback calls = %d, readOnly %v; want one read-only retry", fallbackCalls, fallbackReadOnly)
	}

	ReportMount(pf, "remote", &fusefs.Mount{MountPoint: "/tmp/remote-ro", ReadOnly: true}, nil, true)
	dlg = mountMessage(t)
	dlg.OnResult(1)
	vtui.FrameManager.RemoveFrame(dlg)

	ReportMount(&PanelsFrame{}, "remote", &fusefs.Mount{MountPoint: "/tmp/remote-rw"}, nil, false)
	dlg = mountMessage(t)
	dlg.OnResult(0)
	vtui.FrameManager.RemoveFrame(dlg)
}
