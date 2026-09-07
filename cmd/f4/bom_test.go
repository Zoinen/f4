package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestShowEditor_UTF8BOMIsNotDisplayedOrLostOnSave(t *testing.T) {
	for _, memoryMap := range []bool{false, true} {
		t.Run(map[bool]string{false: "async", true: "mapped"}[memoryMap], func(t *testing.T) {
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			drainPendingTasks()

			dir := t.TempDir()
			path := filepath.Join(dir, "bom.txt")
			text := "first line\nsecond line\n"
			raw := append([]byte{0xEF, 0xBB, 0xBF}, []byte(text)...)
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}

			oldMemoryMap := AppConfig.EditorMemoryMap
			AppConfig.EditorMemoryMap = memoryMap
			defer func() { AppConfig.EditorMemoryMap = oldMemoryMap }()

			filesystem := vfs.NewOSVFS(dir)
			pf := NewPanelsFrame()
			pf.panels[0] = NewFileSystemPanel(0, 0, 40, 20, filesystem)
			pf.panels[1] = NewFileSystemPanel(40, 0, 40, 20, filesystem.Clone())
			pf.ResizeConsole(120, 60)
			vtui.FrameManager.Push(pf)

			f, err := filesystem.Open(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			showEditor(pf, filesystem, path, f)

			ev, _ := findOpenedEditor(filesystem, path)
			if ev == nil {
				t.Fatal("editor was not opened")
			}
			defer ev.Close()
			if !ev.utf8BOM {
				t.Fatal("editor did not remember the UTF-8 BOM")
			}
			if ev.HexMode {
				t.Fatal("BOM-marked UTF-8 text opened in hex mode")
			}
			if memoryMap && ev.mapped == nil {
				t.Fatal("mapped editor fell back to async buffer")
			}
			if !memoryMap && ev.mapped != nil {
				t.Fatal("non-mapped editor unexpectedly created a mapping")
			}

			got, err := ev.pt.GetRange(0, ev.pt.Size())
			if err != nil {
				t.Fatalf("read logical editor text: %v", err)
			}
			if string(got) != text {
				t.Fatalf("editor text = %q, want %q", string(got), text)
			}

			// SaveToFile replaces the buffers that the background indexer reads.
			// Stop and join that worker first so the regression test also remains
			// race-clean when the race shard schedules it during indexing.
			ev.cancelIndexing()
			ev.indexWG.Wait()
			drainPendingTasks()

			ev.modified = true
			ev.SaveToFile(nil)
			waitEditorSave(t, ev)
			drainPendingTasks()
			if ev.modified {
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

	result := loadDefaultQuickView(context.Background(), vfs.NewOSVFS(dir), path)
	if result.err != nil {
		t.Fatalf("quick view load: %v", result.err)
	}
	if result.binary {
		t.Fatal("BOM-marked UTF-8 text opened as binary in Quick View")
	}
	if len(result.lines) < 2 || result.lines[0] != "first" || result.lines[1] != "second" {
		t.Fatalf("quick view lines = %#v, want first/second", result.lines)
	}
}
