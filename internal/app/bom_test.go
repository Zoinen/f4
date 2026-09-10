package app

import (
	"context"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"testing"
)

func TestShowEditor_UTF8BOMIsNotDisplayedOrLostOnSave(t *testing.T) {
	for _, memoryMap := range []bool{false, true} {
		t.Run(map[bool]string{false: "async", true: "mapped"}[memoryMap], func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			testutil.DrainPendingTasks()

			dir := t.TempDir()
			path := filepath.Join(dir, "bom.txt")
			text := "first line\nsecond line\n"
			raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte(text)...)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}

			oldMemoryMap := config.App.EditorMemoryMap
			config.App.EditorMemoryMap = memoryMap
			defer func() { config.App.EditorMemoryMap = oldMemoryMap }()

			filesystem := vfs.NewOSVFS(dir)
			pf := panel.NewPanelsFrame()
			pf.Panels[0] = panel.NewFileSystemPanel(0, 0, 40, 20, filesystem)
			pf.Panels[1] = panel.NewFileSystemPanel(40, 0, 40, 20, filesystem.Clone())
			pf.ResizeConsole(120, 60)
			vtui.FrameManager.Push(pf)

			f, err := filesystem.Open(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			ShowEditor(pf, filesystem, path, f)

			ev, _ := FindOpenedEditor(filesystem, path)
			if ev == nil {
				t.Fatal("editor was not opened")
			}
			defer ev.Close()
			if !ev.Utf8BOM {
				t.Fatal("editor did not remember the UTF-8 BOM")
			}
			if ev.HexMode {
				t.Fatal("BOM-marked UTF-8 text opened in hex mode")
			}
			if memoryMap && ev.Mapped == nil {
				t.Fatal("mapped editor fell back to async buffer")
			}
			if !memoryMap && ev.Mapped != nil {
				t.Fatal("non-mapped editor unexpectedly created a mapping")
			}

			got, err := ev.Pt.GetRange(0, ev.Pt.Size())
			if err != nil {
				t.Fatalf("read logical editor text: %v", err)
			}
			if string(got) != text {
				t.Fatalf("editor text = %q, want %q", string(got), text)
			}

			// SaveToFile replaces the buffers that the background indexer reads.
			// Stop and join that worker first so the regression test also remains
			// race-clean when the race shard schedules it during indexing.
			ev.CancelIndexing()
			ev.WaitForIndexing()
			testutil.DrainPendingTasks()

			ev.Modified = true
			ev.SaveToFile(nil)
			waitEditorSave(t, ev)
			testutil.DrainPendingTasks()
			if ev.Modified {
				t.Fatal("editor remained modified after saving BOM-marked text")
			}
			got, err = os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(raw) {
				t.Fatalf("saved bytes = %x, want %x", got, raw)
			}
		})
	}
}

func TestQuickView_UTF8BOMIsNotDisplayed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bom.txt")
	if err := os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, []byte("first\nsecond\n")...), 0600); err != nil {
		t.Fatal(err)
	}

	result := panel.LoadDefaultQuickView(context.Background(), vfs.NewOSVFS(dir), path)
	if result.Err != nil {
		t.Fatalf("quick view load: %v", result.Err)
	}
	if result.Binary {
		t.Fatal("BOM-marked UTF-8 text opened as binary in Quick View")
	}
	if len(result.Lines) < 2 || result.Lines[0] != "first" || result.Lines[1] != "second" {
		t.Fatalf("quick view lines = %#v, want first/second", result.Lines)
	}
}
