package main

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	archivefs "github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestSemanticDropArchiveMatrix(t *testing.T) {
	for _, fromArchive := range []bool{false, true} {
		for _, toArchive := range []bool{false, true} {
			if !fromArchive && !toArchive {
				continue
			}
			for _, move := range []bool{false, true} {
				t.Run(fmt.Sprintf("sourceArchive=%v/targetArchive=%v/move=%v", fromArchive, toArchive, move), func(t *testing.T) {
					makeFS := func(archived, seeded bool) vfs.VFS {
						dir := t.TempDir()
						local := vfs.NewOSVFS(dir)
						if !archived {
							if seeded {
								if err := os.WriteFile(filepath.Join(dir, "item.txt"), []byte("archive matrix"), 0600); err != nil {
									t.Fatal(err)
								}
							}
							return local
						}
						path := filepath.Join(dir, "fixture.zip")
						f, err := os.Create(path)
						if err != nil {
							t.Fatal(err)
						}
						writer := zip.NewWriter(f)
						if seeded {
							entry, err := writer.Create("item.txt")
							if err != nil {
								t.Fatal(err)
							}
							if _, err := entry.Write([]byte("archive matrix")); err != nil {
								t.Fatal(err)
							}
						}
						if err := writer.Close(); err != nil {
							t.Fatal(err)
						}
						if err := f.Close(); err != nil {
							t.Fatal(err)
						}
						fs, err := archivefs.NewArchiveVFS(local, path)
						if err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() { _ = fs.Close() })
						return fs
					}
					vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
					pf := setupMockPanelsFrame(t)
					defer pf.Close()
					pf.showPanels, pf.showLeftPanel, pf.showRightPanel = true, true, true
					src, dst := pf.panels[0].(*FileSystemPanel), pf.panels[1].(*FileSystemPanel)
					src.vfs, dst.vfs = makeFS(fromArchive, true), makeFS(toArchive, false)
					src.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "item.txt"}}}
					dst.entries = nil
					s := src.semanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
					d := dst.semanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 1, false)
					op := "copy"
					if move {
						op = "move"
					}
					a := map[string]any{"side": 1, "panelId": d.ID, "catalogRevision": d.CatalogRevision, "path": d.Path, "operation": op, "source": map[string]any{"side": 0, "panelId": s.ID, "catalogRevision": s.CatalogRevision, "path": s.Path, "entryIds": []string{s.Entries[0].EntryID}}}
					plan, err := pf.planSemanticDrop(a)
					if err != nil {
						t.Fatal(err)
					}
					result := make(chan error, 1)
					executeFileOpAt(nil, plan.source, plan.target.fs, plan.sourceDir, plan.names, plan.target.dir, plan.move, 0, nil, func(err error) { result <- err })
					deadline := time.After(10 * time.Second)
					for {
						select {
						case err := <-result:
							if err != nil {
								t.Fatal(err)
							}
							goto copied
						case task := <-vtui.FrameManager.TaskChan:
							task()
						case <-deadline:
							t.Fatal("archive transfer timed out")
						}
					}
				copied:
					reader, err := dst.vfs.Open(context.Background(), dst.vfs.Join(dst.vfs.GetPath(), "item.txt"))
					if err != nil {
						t.Fatal(err)
					}
					buffer := make([]byte, len("archive matrix"))
					_, err = reader.ReadAt(context.Background(), buffer, 0)
					_ = reader.Close()
					if err != nil || string(buffer) != "archive matrix" {
						t.Fatalf("copied content=%q err=%v", buffer, err)
					}
					_, err = src.vfs.Stat(context.Background(), src.vfs.Join(src.vfs.GetPath(), "item.txt"))
					if move && err == nil {
						t.Fatal("move retained source member")
					}
					if !move && err != nil {
						t.Fatalf("copy removed source: %v", err)
					}
				})
			}
		}
	}
}

func TestSemanticDropPlan(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pf.showPanels, pf.showLeftPanel, pf.showRightPanel = true, true, true
	src := pf.panels[0].(*FileSystemPanel)
	dst := pf.panels[1].(*FileSystemPanel)
	src.vfs = vfs.NewOSVFS(t.TempDir())
	dst.vfs = vfs.NewOSVFS(t.TempDir())
	src.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "a.txt"}}, {VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	dst.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true}}, {VFSItem: vfs.VFSItem{Name: "file.txt"}}, {VFSItem: vfs.VFSItem{Name: "..", IsDir: true}}}
	endpoint := func(fp *FileSystemPanel, side int) map[string]any {
		fp.updateSemanticRevisions()
		return map[string]any{"side": side, "panelId": vtui.SemanticID(fp), "path": fp.vfs.GetPath(), "catalogRevision": fp.catalogRevision}
	}
	id := func(fp *FileSystemPanel, idx int) string {
		kind, _ := fp.semanticSourceInfo()
		result, _ := fp.semanticEntryMetadata(fp.entries[idx], kind)
		return result
	}
	makeAction := func() map[string]any {
		a := endpoint(dst, 1)
		s := endpoint(src, 0)
		s["entryIds"] = []string{id(src, 0)}
		a["source"] = s
		a["operation"] = "move"
		return a
	}
	t.Run("same directory silently skipped", func(t *testing.T) {
		for _, operation := range []string{"copy", "move"} {
			a := endpoint(src, 0)
			source := endpoint(src, 0)
			source["entryIds"] = []string{id(src, 0)}
			a["source"], a["operation"] = source, operation
			plan, err := pf.planSemanticDrop(a)
			if err != nil || !plan.skip {
				t.Fatalf("same-directory %s must be a silent no-op: %+v, %v", operation, plan, err)
			}
			// The handler must return synchronously without opening a dialog
			// or starting the transfer pipeline.
			if !pf.handleSemanticDrop(a) {
				t.Fatal("drop not handled")
			}
		}
	})
	t.Run("same panel parent directory", func(t *testing.T) {
		for _, operation := range []string{"copy", "move"} {
			a := endpoint(src, 0)
			source := endpoint(src, 0)
			source["entryIds"] = []string{id(src, 0)}
			a["source"], a["operation"], a["entryId"] = source, operation, id(src, 1)
			plan, err := pf.planSemanticDrop(a)
			if err != nil || plan.skip || plan.target.dir != src.vfs.Dir(src.vfs.GetPath()) || plan.target.entryIdx != 1 || plan.move != (operation == "move") {
				t.Fatalf("parent %s: %+v, %v", operation, plan, err)
			}
		}
	})
	t.Run("directory and source snapshot", func(t *testing.T) {
		a := makeAction()
		a["entryId"] = id(dst, 0)
		p, err := pf.planSemanticDrop(a)
		if err != nil {
			t.Fatal(err)
		}
		if p.target.dir != filepath.Join(dst.vfs.GetPath(), "folder") || !p.move || len(p.names) != 1 || p.names[0] != "a.txt" || p.sourceDir != src.vfs.GetPath() {
			t.Fatalf("bad plan: %+v", p)
		}
	})
	for _, idx := range []int{1, 2} {
		t.Run(dst.entries[idx].Name, func(t *testing.T) {
			a := makeAction()
			a["entryId"] = id(dst, idx)
			p, err := pf.planSemanticDrop(a)
			expected := dst.vfs.GetPath()
			if idx == 2 {
				expected = dst.vfs.Dir(expected)
			}
			if err != nil || p.target.dir != expected {
				t.Fatalf("file/parent drop: %+v %v", p, err)
			}
		})
	}
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"stale target", func(a map[string]any) { a["catalogRevision"] = int64(-1) }},
		{"replaced target", func(a map[string]any) { a["panelId"] = "another-panel" }},
		{"navigated target", func(a map[string]any) { a["path"] = "another-path" }},
		{"stale source", func(a map[string]any) { a["source"].(map[string]any)["catalogRevision"] = int64(-1) }},
		{"unknown source entry", func(a map[string]any) { a["source"].(map[string]any)["entryIds"] = []string{"missing"} }},
		{"parent source", func(a map[string]any) { a["source"].(map[string]any)["entryIds"] = []string{id(src, 1)} }},
		{"external move", func(a map[string]any) {
			delete(a, "source")
			a["paths"] = []string{filepath.Join(src.vfs.GetPath(), "a.txt")}
		}},
		{"external relative path", func(a map[string]any) { delete(a, "source"); a["operation"] = "copy"; a["paths"] = []string{"a.txt"} }},
		{"unsupported link", func(a map[string]any) { a["operation"] = "link" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			a := makeAction()
			test.mutate(a)
			if _, err := pf.planSemanticDrop(a); err == nil {
				t.Fatal("invalid drop accepted")
			}
		})
	}
	t.Run("external copy", func(t *testing.T) {
		a := makeAction()
		delete(a, "source")
		a["operation"] = "copy"
		a["paths"] = []string{filepath.Join(src.vfs.GetPath(), "a.txt")}
		if _, err := pf.planSemanticDrop(a); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("prepare complete selection and reject stale response", func(t *testing.T) {
		a := endpoint(src, 0)
		a["requestId"] = "drag-request"
		a["entryIds"] = []string{id(src, 0)}
		semanticLivePanels.Store(vtui.SemanticID(src), src)
		defer semanticLivePanels.Delete(vtui.SemanticID(src))
		response := prepareSemanticDrag(a)
		if response["ok"] != true || response["local"] != true || response["requestId"] != "drag-request" {
			t.Fatalf("unexpected drag preparation: %#v", response)
		}
		paths := semanticStringSlice(response["paths"])
		if len(paths) != 1 || paths[0] != filepath.Join(src.vfs.GetPath(), "a.txt") {
			t.Fatalf("wrong paths: %#v", paths)
		}
		a["catalogRevision"] = int64(-1)
		if prepareSemanticDrag(a)["ok"] != false {
			t.Fatal("stale preparation accepted")
		}
	})
}

func TestSemanticDropCapabilitySurvivesFullSceneRebuild(t *testing.T) {
	// Dialogs and workspace changes rebuild the typed scene from the legacy
	// projection. Writable panels must remain drop targets after that rebuild.
	for _, allowed := range []bool{true, false} {
		legacy := productionPanelCatalogLegacy(`D:\source`, 1, 1, false, "item.txt")
		frames := legacy["frames"].([]map[string]any)
		for _, panel := range frames[0]["panels"].([]map[string]any) {
			panel["dropAllowed"] = allowed
		}
		scene := BuildAppSceneFromLegacy(nil, legacy)
		panels, ok := semanticScenePanelMaps(scene)
		if !ok || len(panels) != 2 {
			t.Fatal("rebuilt scene lost its panels")
		}
		for _, panel := range panels {
			if panel["dropAllowed"] != allowed {
				t.Fatalf("scene rebuild changed drop capability: want %v, got %v", allowed, panel["dropAllowed"])
			}
		}
	}
}

func TestQueuedDropCancelKeepsOriginatingWorkspace(t *testing.T) {
	srcDir, dstDir := t.TempDir(), t.TempDir()
	for _, dir := range []string{srcDir, dstDir} {
		if err := os.WriteFile(filepath.Join(dir, "conflict.txt"), []byte(dir), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rig := newDialogLayoutRig(t, srcDir)
	defer rig.panels.Close()
	for attempt := 0; attempt < 3; attempt++ {
		result := make(chan error, 1)
		ExecuteFileOpWithResult(rig.panels, vfs.NewOSVFS(srcDir), vfs.NewOSVFS(dstDir), []string{"conflict.txt"}, dstDir, false, 0, func(err error) { result <- err })
		dlg := waitForDialog(t, Msg("Warning.Title")).(*vtui.Window)
		if vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx] != rig.baseScreen {
			t.Fatal("overwrite dialog switched away from originating panels")
		}
		clickDialogButton(t, dlg, "Cancel")
		vtui.FrameManager.Pop()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel result: %v", err)
				}
				goto completed
			case task := <-vtui.FrameManager.TaskChan:
				task()
			default:
				time.Sleep(time.Millisecond)
			}
		}
		t.Fatal("cancelled copy did not finish")
	completed:
		if vtui.FrameManager.Screens[vtui.FrameManager.ActiveIdx] != rig.baseScreen {
			t.Fatal("cancel switched away from panels")
		}
	}
}

func TestExternalDropCancelStopsRemainingGroups(t *testing.T) {
	first, second, target := t.TempDir(), t.TempDir(), t.TempDir()
	for _, path := range []string{filepath.Join(first, "conflict.txt"), filepath.Join(target, "conflict.txt"), filepath.Join(second, "later.txt")} {
		if err := os.WriteFile(path, []byte(path), 0600); err != nil {
			t.Fatal(err)
		}
	}
	rig := newDialogLayoutRig(t, first)
	defer rig.panels.Close()
	oldMode := AppConfig.DefaultFileOpMode
	AppConfig.DefaultFileOpMode = 0
	defer func() { AppConfig.DefaultFileOpMode = oldMode }()
	rig.panels.dropExternalFiles(dropTargetInfo{fs: vfs.NewOSVFS(target), dir: target}, []string{filepath.Join(first, "conflict.txt"), filepath.Join(second, "later.txt")}, false)
	dlg := waitForDialog(t, Msg("Warning.Title")).(*vtui.Window)
	clickDialogButton(t, dlg, "Cancel")
	vtui.FrameManager.Pop()
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			if _, err := os.Stat(filepath.Join(target, "later.txt")); !os.IsNotExist(err) {
				t.Fatalf("cancel continued with the next source directory: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(target, "conflict.txt"))
			if err != nil || string(data) != filepath.Join(target, "conflict.txt") {
				t.Fatalf("cancel changed destination: %q %v", data, err)
			}
			return
		}
	}
}

func TestSemanticDragPreparationAfterPublishedCatalog(t *testing.T) {
	for _, paged := range []bool{false, true} {
		t.Run(fmt.Sprint("paged=", paged), func(t *testing.T) {
			oldRows := setExtUiPanelCatalogRowsEnabled(paged)
			oldMetadata := setExtUiPanelCatalogMetadataEnabled(false)
			defer setExtUiPanelCatalogRowsEnabled(oldRows)
			defer setExtUiPanelCatalogMetadataEnabled(oldMetadata)
			fp := &FileSystemPanel{vfs: vfs.NewOSVFS(t.TempDir()), table: vtui.NewTable(0, 0, 80, 40, nil), selectedItems: make(map[string]bool), entries: []*fileEntry{{VFSItem: vfs.VFSItem{Name: "a.txt"}}}}
			defer fp.unpublishSemanticMetadataSnapshot()
			for attempt := 0; attempt < 3; attempt++ {
				model := fp.semanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
				a := map[string]any{"panelId": model.ID, "catalogRevision": model.CatalogRevision, "path": model.Path, "entryIds": []string{model.Entries[0].EntryID}, "requestId": "repeat"}
				if response := prepareSemanticDrag(a); response["ok"] != true {
					t.Fatalf("attempt %d: published panel cannot prepare drag: %#v", attempt, response)
				}
			}
		})
	}
}

func TestTemporaryPanelDragExportsRealLocalPaths(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "one.txt"), []byte("reference"), 0600); err != nil {
		t.Fatal(err)
	}
	tmp := newTempPanelVFS(nil, &tempPanelStore{}, 0)
	if err := tmp.AddReferences(context.Background(), vfs.NewOSVFS(dir), []string{"one.txt"}); err != nil {
		t.Fatal(err)
	}
	items := readTempPanelItems(t, tmp)
	paths, ok := localDragPaths(&FileSystemPanel{vfs: tmp}, []string{items[0].Name})
	if !ok || len(paths) != 1 || paths[0] != filepath.Join(dir, "one.txt") {
		t.Fatalf("temporary panel export: %v, %v", paths, ok)
	}
}

func TestTemporaryPanelCannotOverwriteItsReferencedSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "original.txt")
	if err := os.WriteFile(path, []byte("must survive"), 0600); err != nil {
		t.Fatal(err)
	}
	local := vfs.NewOSVFS(dir)
	tmp := newTempPanelVFS(nil, &tempPanelStore{}, 0)
	if err := tmp.AddReferences(context.Background(), local, []string{"original.txt"}); err != nil {
		t.Fatal(err)
	}
	name := readTempPanelItems(t, tmp)[0].Name
	err := recursiveCopy(context.Background(), tmp, tmp.Join(tmp.GetPath(), name), local, path, &FileOpState{OverwriteAll: true}, 0)
	data, readErr := os.ReadFile(path)
	if err == nil || readErr != nil || string(data) != "must survive" {
		t.Fatalf("self-copy err=%v data=%q readErr=%v", err, data, readErr)
	}
}

func TestSemanticDropRealTemporaryMatrix(t *testing.T) {
	for _, isDir := range []bool{false, true} {
		for _, srcTemp := range []bool{false, true} {
			for _, dstTemp := range []bool{false, true} {
				for _, move := range []bool{false, true} {
					t.Run(fmt.Sprintf("directory=%v/sourceTemporary=%v/targetTemporary=%v/move=%v", isDir, srcTemp, dstTemp, move), func(t *testing.T) {
						srcDir, dstDir := t.TempDir(), t.TempDir()
						itemName, payloadName := "item.txt", "item.txt"
						if isDir {
							itemName = "folder"
							payloadName = filepath.Join(itemName, "nested", "item.txt")
							if err := os.MkdirAll(filepath.Dir(filepath.Join(srcDir, payloadName)), 0700); err != nil {
								t.Fatal(err)
							}
						}
						if err := os.WriteFile(filepath.Join(srcDir, payloadName), []byte("matrix payload"), 0600); err != nil {
							t.Fatal(err)
						}
						vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
						pf := setupMockPanelsFrame(t)
						defer pf.Close()
						pf.showPanels, pf.showLeftPanel, pf.showRightPanel = true, true, true
						src, dst := pf.panels[0].(*FileSystemPanel), pf.panels[1].(*FileSystemPanel)
						src.vfs, dst.vfs = vfs.NewOSVFS(srcDir), vfs.NewOSVFS(dstDir)
						store := &tempPanelStore{}
						name := itemName
						if srcTemp {
							tmp := newTempPanelVFS(nil, store, 0)
							if err := tmp.AddReferences(context.Background(), src.vfs, []string{name}); err != nil {
								t.Fatal(err)
							}
							name = readTempPanelItems(t, tmp)[0].Name
							src.vfs = tmp
						}
						if dstTemp {
							dst.vfs = newTempPanelVFS(nil, store, 1)
						}
						src.entries = []*fileEntry{{VFSItem: vfs.VFSItem{Name: name, IsDir: isDir}}}
						dst.entries = nil
						sourceModel := src.semanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 0, true)
						targetModel := dst.semanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 1, false)
						op := "copy"
						if move {
							op = "move"
						}
						a := map[string]any{"side": 1, "panelId": targetModel.ID, "catalogRevision": targetModel.CatalogRevision, "path": targetModel.Path, "operation": op,
							"source": map[string]any{"side": 0, "panelId": sourceModel.ID, "catalogRevision": sourceModel.CatalogRevision, "path": sourceModel.Path, "entryIds": []string{sourceModel.Entries[0].EntryID}}}
						plan, err := pf.planSemanticDrop(a)
						if err != nil {
							t.Fatal(err)
						}
						if plan.references != nil {
							pf.handleSemanticDrop(a)
							if got := readTempPanelItems(t, plan.references); len(got) != 1 {
								t.Fatalf("reference destination: %v", got)
							}
						} else {
							result := make(chan error, 1)
							executeFileOpAt(nil, plan.source, plan.target.fs, plan.sourceDir, plan.names, plan.target.dir, plan.move, 0, nil, func(err error) { result <- err })
							deadline := time.After(3 * time.Second)
							for {
								select {
								case err := <-result:
									if err != nil {
										t.Fatal(err)
									}
									goto transferred
								case task := <-vtui.FrameManager.TaskChan:
									task()
								case <-deadline:
									t.Fatal("transfer did not finish")
								}
							}
						transferred:
							data, err := os.ReadFile(filepath.Join(dstDir, payloadName))
							if err != nil || string(data) != "matrix payload" {
								t.Fatalf("destination data=%q err=%v", data, err)
							}
						}
						_, err = os.Stat(filepath.Join(srcDir, itemName))
						if move && !dstTemp {
							if !os.IsNotExist(err) {
								t.Fatalf("move retained source: %v", err)
							}
						} else if err != nil {
							t.Fatalf("copy/reference removed source: %v", err)
						}
					})
				}
			}
		}
	}
}

func TestSemanticDropExternalFilesAndDirectories(t *testing.T) {
	for _, destination := range []string{"local", "temporary-root", "temporary-directory"} {
		t.Run(destination, func(t *testing.T) {
			sourceDir, targetDir := t.TempDir(), t.TempDir()
			names := []string{"one.txt", filepath.Join("folder", "nested", "two.txt")}
			for _, name := range names {
				path := filepath.Join(sourceDir, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			pf := setupMockPanelsFrame(t)
			defer pf.Close()
			pf.showPanels, pf.showLeftPanel, pf.showRightPanel = true, true, true
			dst := pf.panels[1].(*FileSystemPanel)
			dst.vfs = vfs.NewOSVFS(targetDir)
			dst.entries = nil
			if destination != "local" {
				tmp := newTempPanelVFS(nil, &tempPanelStore{}, 0)
				if destination == "temporary-directory" {
					if err := tmp.AddReferences(context.Background(), vfs.NewOSVFS(filepath.Dir(targetDir)), []string{filepath.Base(targetDir)}); err != nil {
						t.Fatal(err)
					}
					item := readTempPanelItems(t, tmp)[0]
					dst.entries = []*fileEntry{{VFSItem: item}}
				}
				dst.vfs = tmp
			}
			model := dst.semanticPanelModel(&vtui.SemanticContext{Width: 160, Height: 50}, 1, false)
			a := map[string]any{"side": 1, "panelId": model.ID, "catalogRevision": model.CatalogRevision, "path": model.Path, "operation": "copy", "paths": []string{filepath.Join(sourceDir, "one.txt"), filepath.Join(sourceDir, "folder")}}
			if destination == "temporary-directory" {
				a["entryId"] = model.Entries[0].EntryID
			}
			if _, err := pf.planSemanticDrop(a); err != nil {
				t.Fatal(err)
			}
			oldMode := AppConfig.DefaultFileOpMode
			AppConfig.DefaultFileOpMode = 0
			defer func() { AppConfig.DefaultFileOpMode = oldMode }()
			pf.handleSemanticDrop(a)
			deadline := time.Now().Add(5 * time.Second)
			for {
				complete := true
				if destination == "temporary-root" {
					complete = len(readTempPanelItems(t, dst.vfs.(*TempPanelVFS))) == 2
				} else {
					for _, name := range names {
						data, err := os.ReadFile(filepath.Join(targetDir, name))
						complete = complete && err == nil && string(data) == name
					}
				}
				if complete {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("external drop did not produce expected files/references")
				}
				select {
				case task := <-vtui.FrameManager.TaskChan:
					task()
				default:
					time.Sleep(time.Millisecond)
				}
			}
			for _, name := range names {
				data, err := os.ReadFile(filepath.Join(sourceDir, name))
				if err != nil || string(data) != name {
					t.Fatalf("external copy changed source %s: %q %v", name, data, err)
				}
			}
		})
	}
}
