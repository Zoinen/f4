package panel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/fusefs"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

func writeFuseListRecord(t *testing.T, dir string) {
	t.Helper()
	record := fusefs.Record{
		ID:         "registry-mount",
		Source:     "archive.zip",
		MountPoint: filepath.Join(string(filepath.Separator), "tmp", "f4-list-mount"),
		PID:        os.Getpid(),
		Started:    time.Now().Add(-2 * time.Minute),
		ReadOnly:   true,
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "registry-mount.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func closeFuseListTop(t *testing.T) {
	t.Helper()
	if top := vtui.FrameManager.GetTopFrame(); top != nil {
		top.Close()
		vtui.FrameManager.RemoveFrame(top)
	}
}

func TestMountRowsReadsRegistryAndHandlesRegistryError(t *testing.T) {
	registryDir := t.TempDir()
	t.Setenv("F4_FUSE_REGISTRY", registryDir)
	writeFuseListRecord(t, registryDir)

	rows := mountRows()
	if len(rows) != 1 || rows[0].point == "" || rows[0].source != "archive.zip" || rows[0].mode != "ro" || rows[0].live != nil {
		t.Fatalf("mountRows() = %#v; want one foreign registry row", rows)
	}
	if rows[0].note == "" || rows[0].age < time.Minute {
		t.Fatalf("registry row metadata = %#v; want pid note and age", rows[0])
	}

	badRegistry := filepath.Join(t.TempDir(), "registry-file")
	if err := os.WriteFile(badRegistry, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("F4_FUSE_REGISTRY", badRegistry)
	if rows := mountRows(); rows != nil {
		t.Fatalf("mountRows() with unreadable registry = %#v; want no rows", rows)
	}
}

func TestShowMountListAndForeignMountAction(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	registryDir := t.TempDir()
	t.Setenv("F4_FUSE_REGISTRY", registryDir)

	showMountList(&PanelsFrame{})
	if vtui.FrameManager.GetTopFrame() == nil {
		t.Fatal("empty mount list did not open its action dialog")
	}
	closeFuseListTop(t)

	writeFuseListRecord(t, registryDir)
	showMountList(&PanelsFrame{})
	if vtui.FrameManager.GetTopFrame() == nil {
		t.Fatal("registry mount list did not open")
	}
	closeFuseListTop(t)

	askMountAction(&PanelsFrame{}, mountRow{point: "/tmp/foreign", source: "shell", mode: "rw"})
	top := vtui.FrameManager.GetTopFrame()
	dlg, ok := top.(*vtui.Window)
	if !ok {
		t.Fatalf("foreign mount action frame = %T, want message window", top)
	}
	dlg.OnResult(0)
	closeFuseListTop(t)
}
