package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	archivefs "github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type dropDeniedCreateVFS struct{ vfs.VFS }

func (d dropDeniedCreateVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, os.ErrPermission
}

func TestSemanticDropWriteFailurePreservesSource(t *testing.T) {
	for _, kind := range []string{"local", "temporary", "zip"} {
		for _, move := range []bool{false, true} {
			for _, choice := range []string{"Abort", "Skip", "Retry then Abort"} {
				t.Run(fmt.Sprintf("%s/move=%v/%s", kind, move, choice), func(t *testing.T) {
					source, name := dropConflictFixture(t, kind, "source must survive", false)
					destination := dropDeniedCreateVFS{vfs.NewOSVFS(t.TempDir())}
					rig := newDialogLayoutRig(t, t.TempDir())
					defer rig.panels.Close()
					result := make(chan error, 1)
					ExecuteFileOpWithResult(rig.panels, source, destination, []string{name}, destination.GetPath(), move, 0, func(err error) { result <- err })
					dlg := waitForDialog(t, " Error ").(*vtui.Window)
					if vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx] != rig.baseScreen {
						t.Fatal("error dialog changed workspace")
					}
					if choice == "Retry then Abort" {
						clickDialogButton(t, dlg, "Retry")
						vtui.FrameManager.Pop()
						dlg = waitForDialog(t, " Error ").(*vtui.Window)
					}
					button := choice
					if choice == "Retry then Abort" {
						button = "Abort"
					}
					clickDialogButton(t, dlg, button)
					vtui.FrameManager.Pop()
					deadline := time.After(5 * time.Second)
					for {
						select {
						case err := <-result:
							if (choice == "Skip" && err != nil) || (choice != "Skip" && !errors.Is(err, context.Canceled)) {
								t.Fatalf("write failure result: %v", err)
							}
							goto complete
						case task := <-vtui.FrameManager.TaskChan:
							task()
						case <-deadline:
							t.Fatal("write failure did not finish")
						}
					}
				complete:
					reader, err := source.Open(context.Background(), source.Join(source.GetPath(), name))
					if err != nil {
						t.Fatal(err)
					}
					buffer := make([]byte, len("source must survive"))
					_, err = reader.ReadAt(context.Background(), buffer, 0)
					_ = reader.Close()
					if err != nil || string(buffer) != "source must survive" {
						t.Fatalf("failed operation changed source: %q %v", buffer, err)
					}
					if _, err := destination.Stat(context.Background(), destination.Join(destination.GetPath(), "item.txt")); !os.IsNotExist(err) {
						t.Fatalf("failed write created destination: %v", err)
					}
					if vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx] != rig.baseScreen {
						t.Fatal("write failure changed workspace")
					}
				})
			}
		}
	}
}

func dropConflictFixture(t *testing.T, kind, payload string, directory bool) (vfs.VFS, string) {
	t.Helper()
	dir := t.TempDir()
	local := vfs.NewOSVFS(dir)
	name, payloadName := "item.txt", "item.txt"
	if directory {
		name, payloadName = "folder", "folder/nested/item.txt"
	}
	if kind == "zip" {
		archivePath := filepath.Join(dir, "fixture.zip")
		f, err := os.Create(archivePath)
		if err != nil {
			t.Fatal(err)
		}
		writer := zip.NewWriter(f)
		entry, err := writer.Create(payloadName)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write([]byte(payload)); err != nil {
			t.Fatal(err)
		}
		if err = writer.Close(); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		fs, err := archivefs.NewArchiveVFS(local, archivePath)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = fs.Close() })
		return fs, name
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, payloadName)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, payloadName), []byte(payload), 0600); err != nil {
		t.Fatal(err)
	}
	if kind == "temporary" {
		tmp := newTempPanelVFS(nil, &tempPanelStore{}, 0)
		if err := tmp.AddReferences(context.Background(), local, []string{name}); err != nil {
			t.Fatal(err)
		}
		return tmp, readTempPanelItems(t, tmp)[0].Name
	}
	return local, name
}

func TestSemanticDropConflictMatrix(t *testing.T) {
	for _, directory := range []bool{false, true} {
		for _, sourceKind := range []string{"local", "temporary", "zip"} {
			for _, targetKind := range []string{"local", "zip"} {
				for _, move := range []bool{false, true} {
					for _, choice := range []string{"Cancel", "Skip", "Overwrite"} {
						t.Run(fmt.Sprintf("directory=%v/%s-to-%s/move=%v/%s", directory, sourceKind, targetKind, move, choice), func(t *testing.T) {
							source, name := dropConflictFixture(t, sourceKind, "new source content", directory)
							target, _ := dropConflictFixture(t, targetKind, "old destination content", directory)
							rig := newDialogLayoutRig(t, t.TempDir())
							defer rig.panels.Close()
							src, dst := rig.panels.panels[0].(*FileSystemPanel), rig.panels.panels[1].(*FileSystemPanel)
							src.vfs, dst.vfs = source, target
							src.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: name, IsDir: directory}}}
							dst.entries = nil
							s := src.semanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
							d := dst.semanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 1, false)
							operation := "copy"
							if move {
								operation = "move"
							}
							a := map[string]any{"side": 1, "panelId": d.ID, "catalogRevision": d.CatalogRevision, "path": d.Path, "operation": operation,
								"source": map[string]any{"side": 0, "panelId": s.ID, "catalogRevision": s.CatalogRevision, "path": s.Path, "entryIds": []string{s.Entries[0].EntryID}}}
							plan, err := rig.panels.planSemanticDrop(a)
							if err != nil {
								t.Fatal(err)
							}
							result := make(chan error, 1)
							executeFileOpAt(rig.panels, plan.source, plan.target.fs, plan.sourceDir, plan.names, plan.target.dir, plan.move, 0, nil, func(err error) { result <- err })
							dlg := waitForDialog(t, Msg("Warning.Title")).(*vtui.Window)
							clickDialogButton(t, dlg, choice)
							vtui.FrameManager.Pop()
							deadline := time.After(5 * time.Second)
							for {
								select {
								case err := <-result:
									if choice == "Cancel" {
										if !errors.Is(err, context.Canceled) {
											t.Fatalf("cancel result: %v", err)
										}
									} else if err != nil {
										t.Fatal(err)
									}
									goto complete
								case task := <-vtui.FrameManager.TaskChan:
									task()
								case <-deadline:
									t.Fatal("conflict operation did not finish")
								}
							}
						complete:
							if vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx] != rig.baseScreen {
								t.Fatal("conflict changed workspace")
							}
							want := "old destination content"
							if choice == "Overwrite" {
								want = "new source content"
							}
							payloadPath := target.Join(target.GetPath(), "item.txt")
							if directory {
								payloadPath = target.Join(target.GetPath(), "folder", "nested", "item.txt")
							}
							reader, err := target.Open(context.Background(), payloadPath)
							if err != nil {
								t.Fatal(err)
							}
							buffer := make([]byte, len(want))
							_, err = reader.ReadAt(context.Background(), buffer, 0)
							_ = reader.Close()
							if err != nil || string(buffer) != want {
								t.Fatalf("destination content=%q err=%v", buffer, err)
							}
							_, err = source.Stat(context.Background(), source.Join(source.GetPath(), name))
							if move && choice == "Overwrite" {
								if err == nil {
									t.Fatal("successful move retained source")
								}
							} else if err != nil {
								t.Fatalf("cancel/skip/copy removed source: %v", err)
							}
						})
					}
				}
			}
		}
	}
}
