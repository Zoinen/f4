package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/unxed/f4/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

var (
	LastFindFileMask = "*"
	LastFindFileText = ""
	LastLeftPath     = ""
	LastRightPath    = ""
	LastLeftCursor   = ""
	LastRightCursor  = ""
	LastActivePanel  = 1

	LastLeftViewMode  = 0
	LastRightViewMode = 0
	LastLeftSortMode  = 0
	LastRightSortMode = 0
	LastLeftSortRev   = false
	LastRightSortRev  = false

	LastShowPanels = true
	LastShowLeft   = true
	LastShowRight  = true
)

func actionFoldersHistory(pf *PanelsFrame) {
	if vtui.GlobalHistoryProvider == nil {
		return
	}
	h := vtui.GlobalHistoryProvider.LoadHistory("folders")
	if len(h) == 0 {
		vtui.ShowMessage(Msg("History.Title"), Msg("History.EmptyFolders"), []string{Msg("vtui.Ok")})
		return
	}

	menu := vtui.NewVMenu(Msg("History.FoldersTitle"))
	for _, p := range h {
		menu.AddItem(vtui.MenuItem{Text: p})
	}

	// Setup shortcuts
	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0
		ctrl := (e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed)) != 0

		idx := menu.SelectPos
		if idx < 0 || idx >= len(menu.Items) {
			return false
		}
		path := menu.Items[idx].Text

		if e.VirtualKeyCode == vtinput.VK_RETURN {
			if ctrl {
				// Insert into command line
				pf.cmdLine.InsertString(path)
				menu.Close()
				return true
			}
			menu.Close()
			targetPanel := pf.getActivePanel()
			if shift {
				targetPanel = pf.getInactivePanel()
			}
			if targetPanel != nil {
				pf.NavigateToPath(targetPanel, path)
			}
			// Update MRU order
			AddFolderHistory(path)
			return true
		}

		if (e.VirtualKeyCode == vtinput.VK_DELETE || e.VirtualKeyCode == vtinput.VK_BACK) && shift {
			// Delete item
			h = append(h[:idx], h[idx+1:]...)
			vtui.GlobalHistoryProvider.SaveHistory("folders", h)
			menu.Items = append(menu.Items[:idx], menu.Items[idx+1:]...)
			menu.ItemCount = len(menu.Items)
			if menu.ItemCount == 0 {
				menu.Close()
			} else {
				if menu.SelectPos >= menu.ItemCount {
					menu.SetSelectPos(menu.ItemCount - 1)
				}
				vtui.FrameManager.Redraw()
			}
			return true
		}
		return false
	}

	// Sizing and positioning
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	width := scrW - 6
	if width > 120 {
		width = 120
	}
	height := len(h) + 2
	maxH := scrH - 4
	if maxH < 6 {
		maxH = 6
	}
	if height > maxH {
		height = maxH
	}

	menu.SetPosition((scrW-width)/2, (scrH-height)/2, (scrW-width)/2+width-1, (scrH-height)/2+height-1)
	vtui.FrameManager.Push(menu)
}

func actionCommandHistory(pf *PanelsFrame) {
	h := pf.cmdLine.Edit.History
	if len(h) == 0 {
		vtui.ShowMessage(Msg("History.Title"), Msg("History.EmptyCommands"), []string{Msg("vtui.Ok")})
		return
	}

	menu := vtui.NewVMenu(Msg("History.CommandsTitle"))
	for _, p := range h {
		menu.AddItem(vtui.MenuItem{Text: p})
	}

	// Setup shortcuts
	menu.OnKeyDown = func(e *vtinput.InputEvent) bool {
		shift := (e.ControlKeyState & vtinput.ShiftPressed) != 0

		idx := menu.SelectPos
		if idx < 0 || idx >= len(menu.Items) {
			return false
		}
		cmd := menu.Items[idx].Text

		if e.VirtualKeyCode == vtinput.VK_RETURN {
			menu.Close()
			pf.cmdLine.Edit.SetText(cmd)
			pf.cmdLine.Edit.HistoryPos = -1
			return true
		}

		if (e.VirtualKeyCode == vtinput.VK_DELETE || e.VirtualKeyCode == vtinput.VK_BACK) && shift {
			// Delete item
			h = append(h[:idx], h[idx+1:]...)
			pf.cmdLine.Edit.History = h
			if vtui.GlobalHistoryProvider != nil {
				vtui.GlobalHistoryProvider.SaveHistory("cmdline", h)
			}
			menu.Items = append(menu.Items[:idx], menu.Items[idx+1:]...)
			menu.ItemCount = len(menu.Items)
			if menu.ItemCount == 0 {
				menu.Close()
			} else {
				if menu.SelectPos >= menu.ItemCount {
					menu.SetSelectPos(menu.ItemCount - 1)
				}
				vtui.FrameManager.Redraw()
			}
			return true
		}
		return false
	}

	// Sizing and positioning
	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	width := scrW - 6
	if width > 120 {
		width = 120
	}
	height := len(h) + 2
	maxH := scrH - 4
	if maxH < 6 {
		maxH = 6
	}
	if height > maxH {
		height = maxH
	}

	menu.SetPosition((scrW-width)/2, (scrH-height)/2, (scrW-width)/2+width-1, (scrH-height)/2+height-1)
	vtui.FrameManager.Push(menu)
}

func actionEditFileExternal(pf *PanelsFrame, v vfs.VFS, path string, size int64) {
	cmdStr := AppConfig.ExternalEditorCommand
	if cmdStr == "" {
		cmdStr = os.Getenv("EDITOR")
		if cmdStr == "" {
			cmdStr = "nano" // Fallback
		}
	}

	// 1. If it's a local OSVFS file, we can just run the editor directly.
	if osvfs, ok := v.(*vfs.OSVFS); ok {
		absPath, _ := osvfs.Abs(path)
		runExternalEditor(pf, cmdStr, absPath)
		return
	}

	// 2. If it's a remote file, we need to download it to a temp file, edit, and upload back if changed.
	ext := filepath.Ext(path)
	if ext == "" {
		ext = ".txt"
	}
	tmpFile, err := os.CreateTemp("", "f4-extedit-*"+ext)
	if err != nil {
		vtui.ShowMessage(Msg("Error.Title"), fmt.Sprintf(Msg("ExtEdit.TempError"), err), []string{Msg("vtui.Ok")})
		return
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close() // Will be reopened by VFS/editor

	pf.RunProgressTask(" Downloading... ", "Preparing to download...", false, func(ctx context.Context, update func(msg string, percent int)) error {
		src, err := v.Open(ctx, path)
		if err != nil {
			// If file does not exist, it's a new file. Just create an empty temp file.
			if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "file does not exist") {
				return nil
			}
			return err
		}
		defer src.Close()

		dst, err := os.Create(tmpPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		buf := make([]byte, 128*1024)
		var downloaded int64
		for {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			n, err := src.Read(ctx, buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return werr
				}
				downloaded += int64(n)
				pct := 0
				if size > 0 {
					pct = int((downloaded * 100) / size)
				}
				update("Downloading...", pct)
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				return err
			}
		}
		return nil
	}, func(err error) {
		if err != nil && err != context.Canceled {
			vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to download file:\n%v", err), []string{"&Ok"})
			os.Remove(tmpPath)
			return
		}
		if err == context.Canceled {
			os.Remove(tmpPath)
			return
		}

		stBefore, err := os.Stat(tmpPath)
		if err != nil {
			os.Remove(tmpPath)
			return
		}
		modTimeBefore := stBefore.ModTime()

		runExternalEditor(pf, cmdStr, tmpPath)

		stAfter, err := os.Stat(tmpPath)
		if err == nil && stAfter.ModTime().After(modTimeBefore) {
			pf.RunProgressTask(" Uploading... ", "Preparing to upload...", false, func(ctx context.Context, update func(msg string, percent int)) error {
				src, err := os.Open(tmpPath)
				if err != nil {
					return err
				}
				defer src.Close()

				dst, err := v.Create(ctx, path)
				if err != nil {
					return err
				}
				defer dst.Close()

				buf := make([]byte, 128*1024)
				var uploaded int64
				for {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					n, err := src.Read(buf)
					if n > 0 {
						if _, werr := dst.Write(buf[:n]); werr != nil {
							return werr
						}
						uploaded += int64(n)
						pct := 0
						if stAfter.Size() > 0 {
							pct = int((uploaded * 100) / stAfter.Size())
						}
						update("Uploading...", pct)
					}
					if err != nil {
						if err == io.EOF {
							break
						}
						return err
					}
				}
				return nil
			}, func(err error) {
				os.Remove(tmpPath)
				if err != nil && err != context.Canceled {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to upload file:\n%v", err), []string{"&Ok"})
				}
				pf.RefreshAll()
			})
		} else {
			os.Remove(tmpPath)
			pf.RefreshAll()
		}
	})
}

func runExternalEditor(pf *PanelsFrame, cmdStr, path string) {
	parts := strings.Fields(cmdStr)
	if len(parts) == 0 {
		return
	}

	args := append(parts[1:], path)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if fsp := pf.getActivePanel(); fsp != nil {
		if _, isLocal := fsp.vfs.(*vfs.OSVFS); isLocal {
			cmd.Dir = fsp.vfs.GetPath()
		}
	}

	vtui.Suspend()
	err := cmd.Run()
	vtui.Resume()

	if err != nil {
		vtui.FrameManager.PostTask(func() {
			vtui.ShowMessage(" Error ", fmt.Sprintf("Editor exited with error:\n%v", err), []string{"&Ok"})
		})
	}
	vtui.FrameManager.PostTask(func() {
		pf.RefreshAll()
	})
}

func showEditor(pf *PanelsFrame, v vfs.VFS, path string, f vfs.ReadAtCloser) {
	var pt *piecetable.PieceTable
	var buf *AsyncBuffer
	cpID := AppConfig.EditorDefaultCodePage

	if f != nil {
		size := f.Size()
		detectLen := 16 * 1024
		if int64(detectLen) > size {
			detectLen = int(size)
		}
		header := make([]byte, detectLen)
		_, _ = f.ReadAt(context.Background(), header, 0)

		cpID = vfs.DetectEncoding(header, AppConfig.EditorAutodetectCodePage, AppConfig.EditorDefaultCodePage)

		if cpID == 65001 {
			buf = NewAsyncBuffer(context.Background(), f)
			buf.prewarm()
			pt = piecetable.NewWithBuffer(buf)
		} else {
			fullData := make([]byte, size)
			_, _ = f.ReadAt(context.Background(), fullData, 0)
			decoded, err := vfs.DecodeBytes(fullData, cpID)
			if err != nil {
				decoded = fullData
				cpID = 65001
			}
			pt = piecetable.New(decoded)
		}
	} else {
		pt = piecetable.New(nil)
	}

	editor := NewEditorView(pt, v, path)
	editor.Codepage = cpID
	if GlobalFileState != nil && path != "" {
		if state := GlobalFileState.GetState(path); state != nil {
			editor.WordWrap = state.EditorWrap
			editor.targetLine = state.EditorLine
			editor.targetPos = state.EditorPos
			editor.targetTopRow = state.EditorTopRow
			editor.targetLeft = state.EditorLeft
		}
	}
	editor.file = f
	editor.asyncBuf = buf
	editor.ResizeConsole(pf.lastW, pf.lastH)
	editor.StartIndexing()

	vtui.FrameManager.AddScreen(editor)
}

func findOpenedEditor(v vfs.VFS, path string) (*EditorView, int) {
	var absPath string
	isLocal := false
	if osvfs, ok := v.(*vfs.OSVFS); ok {
		isLocal = true
		absPath, _ = osvfs.Abs(path)
	}

	if vtui.FrameManager == nil {
		return nil, -1
	}

	for i, s := range vtui.FrameManager.Screens {
		for _, f := range s.Frames {
			if ev, ok := f.(*EditorView); ok && !ev.IsDone() {
				if isLocal && ev.vfs != nil {
					if evOSVFS, evOk := ev.vfs.(*vfs.OSVFS); evOk {
						evAbsPath, _ := evOSVFS.Abs(ev.filePath)
						if evAbsPath == absPath {
							return ev, i
						}
					}
				} else {
					if ev.filePath == path {
						return ev, i
					}
				}
			}
		}
	}
	return nil, -1
}

func actionOpenEditor(pf *PanelsFrame, v vfs.VFS, path string) {
	existingEditor, screenIdx := findOpenedEditor(v, path)
	if existingEditor != nil {
		var buttons []string
		if existingEditor.modified {
			buttons = []string{Msg("FileOp.BtnCurrent"), Msg("FileOp.BtnNewInstance"), Msg("vtui.Cancel")}
		} else {
			buttons = []string{Msg("FileOp.BtnCurrent"), Msg("FileOp.BtnReload"), Msg("FileOp.BtnNewInstance"), Msg("vtui.Cancel")}
		}

		vtui.FrameManager.PostTask(func() {
			dlg := vtui.ShowMessage(Msg("Warning.Title"), fmt.Sprintf(Msg("FileOp.AlreadyOpened"), vtui.TruncateMiddle(v.Base(path), 40)), buttons)
			dlg.OnResult = func(res int) {
				if res == 0 {
					vtui.FrameManager.SwitchScreen(screenIdx)
				} else if res == 1 && len(buttons) == 4 { // Reload
					existingEditor.Close()
					openEditorInternal(pf, v, path)
				} else if (res == 1 && len(buttons) == 3) || (res == 2 && len(buttons) == 4) { // New instance
					openEditorInternal(pf, v, path)
				}
			}
		})
		return
	}
	openEditorInternal(pf, v, path)
}

func openEditorInternal(pf *PanelsFrame, v vfs.VFS, path string) {
	if _, isLocal := v.(*vfs.OSVFS); isLocal {
		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			var f vfs.ReadAtCloser
			if v != nil {
				if stat, errStat := v.Stat(ctx.Context, path); errStat == nil && stat.IsDir {
					ctx.RunOnUI(func() {
						vtui.ShowMessage(" Error ", "Cannot edit a directory.", []string{"&Ok"})
					})
					return
				}
				var err error
				f, err = v.Open(ctx.Context, path)
				if err != nil {
					if os.IsNotExist(err) {
						f = nil
					} else {
						ctx.RunOnUI(func() {
							if err == os.ErrInvalid {
								vtui.ShowMessage(" Error ", "Cannot open special files (Named Pipes, Sockets, Devices).", []string{"&Ok"})
							} else {
								vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open file:\n%v", err), []string{"&Ok"})
							}
						})
						return
					}
				}
			}
			ctx.RunOnUI(func() {
				showEditor(pf, v, path, f)
			})
		})
		return
	}

	var f vfs.ReadAtCloser
	pf.RunProgressTask(" Opening... ", "Preparing to edit file...", false, func(ctx context.Context, update func(msg string, percent int)) error {
		update("Opening file...", -1)
		var err error
		if v != nil {
			if stat, errStat := v.Stat(ctx, path); errStat == nil && stat.IsDir {
				return fmt.Errorf("cannot edit a directory")
			}
			ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(update))
			f, err = v.Open(ctx, path)
			if err != nil {
				if os.IsNotExist(err) || strings.Contains(err.Error(), "no such file") || strings.Contains(err.Error(), "not found") {
					f = nil
					return nil
				}
				return err
			}
		}
		return nil
	}, func(err error) {
		if err != nil {
			if err != context.Canceled {
				if err == os.ErrInvalid {
					vtui.ShowMessage(" Error ", "Cannot open special files (Named Pipes, Sockets, Devices).", []string{"&Ok"})
				} else {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open file:\n%v", err), []string{"&Ok"})
				}
			}
			return
		}
		showEditor(pf, v, path, f)
	})
}

func findOpenedViewer(v vfs.VFS, path string) (*ViewerView, int) {
	var absPath string
	isLocal := false
	if osvfs, ok := v.(*vfs.OSVFS); ok {
		isLocal = true
		absPath, _ = osvfs.Abs(path)
	}

	if vtui.FrameManager == nil {
		return nil, -1
	}

	for i, s := range vtui.FrameManager.Screens {
		for _, f := range s.Frames {
			if vv, ok := f.(*ViewerView); ok && !vv.IsDone() {
				if isLocal && vv.vfs != nil {
					if vvOSVFS, evOk := vv.vfs.(*vfs.OSVFS); evOk {
						vvAbsPath, _ := vvOSVFS.Abs(vv.path)
						if vvAbsPath == absPath {
							return vv, i
						}
					}
				} else {
					if vv.path == path {
						return vv, i
					}
				}
			}
		}
	}
	return nil, -1
}

func showViewer(pf *PanelsFrame, viewer *ViewerView, path string) {
	if GlobalFileState != nil && path != "" {
		if state := GlobalFileState.GetState(path); state != nil {
			viewer.TopOffset = state.ViewerOffset
			if viewer.TopOffset > viewer.backend.Size() {
				viewer.TopOffset = viewer.backend.Size() - 1
			}
			if viewer.TopOffset < 0 {
				viewer.TopOffset = 0
			}
			viewer.WrapMode = state.ViewerWrap
			viewer.HexMode = state.ViewerHex
		}
	}
	viewer.ResizeConsole(pf.lastW, pf.lastH)
	vtui.FrameManager.AddScreen(viewer)
}

func actionOpenViewer(pf *PanelsFrame, v vfs.VFS, path string) {
	existingViewer, screenIdx := findOpenedViewer(v, path)
	if existingViewer != nil {
		vtui.FrameManager.PostTask(func() {
			dlg := vtui.ShowMessage(Msg("Warning.Title"), fmt.Sprintf(Msg("FileOp.AlreadyViewed"), vtui.TruncateMiddle(v.Base(path), 40)), []string{Msg("FileOp.BtnCurrent"), Msg("FileOp.BtnReload"), Msg("FileOp.BtnNewInstance"), Msg("vtui.Cancel")})
			dlg.OnResult = func(res int) {
				if res == 0 {
					vtui.FrameManager.SwitchScreen(screenIdx)
				} else if res == 1 { // Reload
					existingViewer.Close()
					openViewerInternal(pf, v, path)
				} else if res == 2 { // New instance
					openViewerInternal(pf, v, path)
				}
			}
		})
		return
	}
	openViewerInternal(pf, v, path)
}

func openViewerInternal(pf *PanelsFrame, v vfs.VFS, path string) {
	if _, isLocal := v.(*vfs.OSVFS); isLocal {
		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			if v != nil {
				if stat, err := v.Stat(ctx.Context, path); err == nil && stat.IsDir {
					ctx.RunOnUI(func() {
						vtui.ShowMessage(" Error ", "Cannot view a directory.", []string{"&Ok"})
					})
					return
				}
			}

			viewer, err := NewViewerView(ctx.Context, v, path)
			ctx.RunOnUI(func() {
				if err == nil {
					showViewer(pf, viewer, path)
				} else {
					vtui.DebugLog("PANELS: Failed to open viewer for %s: %v", path, err)
					if err == os.ErrInvalid {
						vtui.ShowMessage(" Error ", "Cannot open special files (Named Pipes, Sockets, Devices).", []string{"&Ok"})
					} else {
						vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open file:\n%v", err), []string{"&Ok"})
					}
				}
			})
		})
		return
	}

	var viewer *ViewerView
	pf.RunProgressTask(" Opening... ", "Preparing to open file...", false, func(ctx context.Context, update func(msg string, percent int)) error {
		update("Opening file...", -1)
		ctx = context.WithValue(ctx, vfs.ProgressKey, vfs.ProgressCallback(update))
		var err error
		viewer, err = NewViewerView(ctx, v, path)
		return err
	}, func(err error) {
		if err != nil {
			if err != context.Canceled {
				if err == os.ErrInvalid {
					vtui.ShowMessage(" Error ", "Cannot open special files (Named Pipes, Sockets, Devices).", []string{"&Ok"})
				} else {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open file:\n%v", err), []string{"&Ok"})
				}
			}
			return
		}
		showViewer(pf, viewer, path)
	})
}

func actionViewerSearch(vv *ViewerView) {
	vtui.InputBox(Msg("Viewer.SearchTitle"), "Search for:", "", func(pattern string) {
		if pattern == "" {
			return
		}
		title := " Searching... "
		msg := fmt.Sprintf("Looking for: %s", pattern)

		vtui.FrameManager.PostTask(func() {
			dlg := vtui.NewCenteredDialog(50, 8, title)
			lbl := vtui.NewLabel(0, 0, msg, nil)
			dlg.AddItem(lbl)
			btnCancel := vtui.NewButton(0, 0, "&Cancel")
			dlg.AddItem(btnCancel)

			vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 50-4, 8-4)
			vbox.Add(lbl, vtui.Margins{}, vtui.AlignCenter)
			vbox.Add(btnCancel, vtui.Margins{Top: 1}, vtui.AlignCenter)
			vbox.Apply()

			vtui.FrameManager.AddScreenHeadless(dlg)

			_ = vtui.RunAsync(func(ctx *vtui.TaskContext) {
				btnCancel.OnClick = func() { ctx.Cancel(); dlg.Close() }
				foundOffset := int64(-1)
				currOff := vv.TopOffset + 1
				fileSize := vv.backend.Size()
				patternLower := strings.ToLower(pattern)

				for currOff < fileSize {
					if ctx.Err() != nil {
						return
					}
					percent := int((currOff * 100) / fileSize)
					ctx.RunOnUI(func() { dlg.SetProgress(percent) })

					data, err := vv.backend.ReadAt(currOff, 256*1024)
					if err == piecetable.ErrLoading {
						time.Sleep(20 * time.Millisecond)
						continue
					}
					if err != nil || len(data) == 0 {
						break
					}

					idx := strings.Index(strings.ToLower(string(data)), patternLower)
					if idx != -1 {
						foundOffset = currOff + int64(idx)
						break
					}
					currOff += int64(len(data)) - int64(len(patternLower))
					if currOff < 0 {
						currOff = 0
					}
				}

				ctx.RunOnUI(func() {
					dlg.Close()
					if foundOffset != -1 {
						vv.TopOffset = vv.backend.FindLineStart(foundOffset)
						vtui.FrameManager.Redraw()
					} else if ctx.Err() == nil {
						vtui.ShowMessage(" Search ", "Pattern not found.", []string{"&Ok"})
					}
				})
			})
		})
	})
}

func actionExecute(pf *PanelsFrame, v vfs.VFS, dir, name, path string) {
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		if _, isLocal := v.(*vfs.OSVFS); isLocal {
			if fi, err := os.Stat(path); err == nil {
				if fi.Mode()&(os.ModeNamedPipe|os.ModeSocket|os.ModeDevice|os.ModeCharDevice) != 0 {
					ctx.RunOnUI(func() {
						vtui.ShowMessage(" Error ", "Cannot open special files (Named Pipes, Sockets, Devices).", []string{"&Ok"})
					})
					return
				}
			}
		}
		runnable := vfs.IsTerminalRunnable(ctx.Context, v, path)
		if runnable {
			ctx.RunOnUI(func() {
				// Add to command history since it's a shell-executable file.
				// This centralized logic ensures consistent history across manual and Enter launches.
				historyCmd := name
				if strings.Contains(historyCmd, " ") && !strings.HasPrefix(historyCmd, "\"") && !strings.HasPrefix(historyCmd, "'") {
					historyCmd = "\"" + historyCmd + "\""
				}
				_, isOS := v.(*vfs.OSVFS)
				_, isPty := v.(vfs.PtyProvider)
				isWindowsShell := runtime.GOOS == "windows" && isOS

				if !isWindowsShell {
					historyCmd = "./" + historyCmd
				}
				pf.cmdLine.Edit.AddHistory(historyCmd)
				pf.cmdLine.Edit.HistoryPos = -1

				activePty := pf.getActivePTY()
				if activePty != nil {
					cmd := name
					var cmdToWire string

					useDir := isOS || isPty

					actualDir := ""
					if useDir {
						actualDir = dir
					}

					if isWindowsShell {
						// Combine directory sync with the command to allow excision
						if actualDir != "" {
							cmdToWire = fmt.Sprintf("cd /d \"%s\" & %s\r", actualDir, historyCmd)
						} else {
							cmdToWire = fmt.Sprintf("%s\r", historyCmd)
						}
					} else {
						// On Unix, use single quotes for paths to prevent Bash history expansion
						sqCmd := strings.ReplaceAll(cmd, "'", "'\\''")
						// Используем OSC 133 для уведомления терминала о начале и конце выполнения.
						if actualDir != "" {
							sqDir := strings.ReplaceAll(actualDir, "'", "'\\''")
							cmdToWire = fmt.Sprintf(" set +H; cd '%s' && { printf \"\\033]133;C\\007\"; ./'%s' ; printf \"\\033]133;D\\007\"; }\r", sqDir, sqCmd)
						} else {
							cmdToWire = fmt.Sprintf(" set +H; { printf \"\\033]133;C\\007\"; ./'%s' ; printf \"\\033]133;D\\007\"; }\r", sqCmd)
						}
					}
					vtui.DebugLog("ACTIONS: Sending to PTY: %q", cmdToWire)

					cleanCmd := "./" + cmd
					if isWindowsShell {
						cleanCmd = cmd
					}
					if !isWindowsShell {
						pf.termView.PrintCleanCommand(cleanCmd)
					}

					pf.executing = true
					pf.returnToPanels = true

					if !isWindowsShell {
						pf.termView.SetMuted(true)
					}
					activePty.Write([]byte(cmdToWire))
					pf.showPanels = false
				}
			})
		} else {
			if _, isLocal := v.(*vfs.OSVFS); !isLocal {
				ctx.RunOnUI(func() {
					vtui.ShowMessage(" Error ", "Cannot execute non-runnable files on a remote file system.", []string{"&Ok"})
				})
				return
			}
			var cmd *exec.Cmd
			switch runtime.GOOS {
			case "linux":
				cmd = exec.Command("xdg-open", path)
			case "windows":
				cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
			case "darwin":
				cmd = exec.Command("open", path)
			}
			if cmd != nil {
				if _, isLocal := v.(*vfs.OSVFS); isLocal {
					cmd.Dir = dir
				}
				vtui.DebugLog("ACTIONS: Executing external command: %s", cmd.String())
				err := cmd.Run()
				if err != nil {
					vtui.DebugLog("ACTIONS: External command failed: %v", err)
					ctx.RunOnUI(func() {
						vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to open file:\n%v", err), []string{"&Ok"})
					})
				}
			}
		}
	})
}

func actionNewFile(pf *PanelsFrame) {
	if fsp := pf.getActivePanel(); fsp != nil {
		dir := fsp.vfs.GetPath()
		activeVfs := fsp.vfs
		vtui.InputBox(Msg("Edit.NewFileTitle"), Msg("Edit.NewFilePrompt"), "", func(name string) {
			if name == "" {
				name = "newfile.txt"
			}
			path := activeVfs.Join(dir, name)
			if AppConfig.UseExternalEditor {
				actionEditFileExternal(pf, activeVfs, path, 0)
				return
			}
			actionOpenEditor(pf, activeVfs, path)
		})
	}
}

func actionViewTerminalLog(pf *PanelsFrame) {
	v := &TerminalLogVFS{tv: pf.termView}
	actionOpenViewer(pf, v, "Terminal Log")
}

func actionEditTerminalLog(pf *PanelsFrame) {
	v := &TerminalLogVFS{tv: pf.termView}
	actionOpenEditor(pf, v, "Terminal Log")
}

func actionViewFile(pf *PanelsFrame) {
	if fsp := pf.getActivePanel(); fsp != nil {
		idx := fsp.GetCursorIndex()
		if idx < 0 || idx >= len(fsp.entries) {
			return
		}
		if fsp.entries[idx].IsDir {
			actionCalcDirSize(pf, fsp, idx)
			return
		}
		name := fsp.GetSelectedName()
		path := fsp.vfs.Join(fsp.vfs.GetPath(), name)
		actionOpenViewer(pf, fsp.vfs, path)
	}
}

func actionCalcDirSize(pf *PanelsFrame, fsp *FileSystemPanel, idx int) {
	entry := fsp.entries[idx]
	name := entry.Name
	basePath := fsp.vfs.GetPath()

	var targetPath string
	if name == ".." {
		targetPath = fsp.vfs.Dir(basePath)
	} else {
		targetPath = fsp.vfs.Join(basePath, name)
	}

	opDlg := NewFileOpProgressDialog(" Calculating Size... ")
	var taskCtx *vtui.TaskContext
	opDlg.btnCancel.OnClick = func() {
		if taskCtx != nil {
			taskCtx.Cancel()
		}
		opDlg.Close()
	}

	vtui.FrameManager.PostTask(func() {
		vtui.FrameManager.AddScreenHeadless(opDlg)
	})

	taskCtx = vtui.RunAsync(func(ctx *vtui.TaskContext) {
		var totalStats vfs.OpStats
		lastScanUpdate := time.Now()
		totalStats, scanErr := vfs.CalculateStats(ctx.Context, fsp.vfs, targetPath, []string{""}, func(currentPath string, stats vfs.OpStats) {
			now := time.Now()
			if now.Sub(lastScanUpdate) > 50*time.Millisecond {
				lastScanUpdate = now
				ctx.RunOnUI(func() {
					opDlg.UpdateScan(currentPath, stats.Files, stats.Dirs)
					vtui.FrameManager.Redraw()
				})
			}
		})

		ctx.RunOnUI(func() {
			opDlg.Close()
			if scanErr != nil && scanErr != context.Canceled {
				vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to calculate size:\n%v", scanErr), []string{"&Ok"})
				return
			}
			if ctx.Err() == nil {
				entry.Size = totalStats.Bytes
				entry.SizeCalculated = true
				if fsp.sortMode == SortSize {
					fsp.sortEntries()
					// Keep cursor on the same item after re-sorting
					for i, e := range fsp.entries {
						if e == entry {
							fsp.SetCursorIndex(i)
							break
						}
					}
				}
				fsp.Refresh()
			}
		})
	})
}

func actionEditFile(pf *PanelsFrame) {
	if fsp := pf.getActivePanel(); fsp != nil {
		idx := fsp.GetCursorIndex()
		if idx < 0 || idx >= len(fsp.entries) {
			return
		}
		if fsp.entries[idx].IsDir {
			vtui.ShowMessage(" Error ", "Cannot edit a directory.", []string{"&Ok"})
			return
		}
		name := fsp.GetSelectedName()
		path := fsp.vfs.Join(fsp.vfs.GetPath(), name)

		if AppConfig.UseExternalEditor {
			actionEditFileExternal(pf, fsp.vfs, path, fsp.entries[idx].Size)
			return
		}

		actionOpenEditor(pf, fsp.vfs, path)
	}
}

func actionCopyMove(pf *PanelsFrame, isMove bool) {
	fspSrc := pf.getActivePanel()
	fspDst := pf.getInactivePanel()
	if fspSrc == nil || fspDst == nil {
		return
	}

	names := fspSrc.GetSelectedNames()
	if len(names) == 0 {
		return
	}

	title := Msg("Copy.Title")
	prompt := Msg("Copy.Prompt")
	if isMove {
		title = Msg("Move.Title")
		prompt = Msg("Move.Prompt")
	}

	srcVfs, dstVfs := fspSrc.vfs, fspDst.vfs

	initialDest := dstVfs.GetPath()
	if initialDest != "" && !strings.HasSuffix(initialDest, "/") && !strings.HasSuffix(initialDest, "\\") {
		sep := "/"
		if _, isOS := dstVfs.(*vfs.OSVFS); isOS && runtime.GOOS == "windows" {
			sep = "\\"
		}
		initialDest += sep
	}

	onCompleteWithClear := func() {
		if pf != nil {
			if fsp := pf.getActivePanel(); fsp != nil {
				fsp.selectedItems = make(map[string]bool)
				for _, e := range fsp.entries {
					e.Selected = false
				}
			}
			pf.RefreshAll()
		}
	}

	if isMove && !AppConfig.ConfirmMove {
		go ExecuteFileOp(pf, srcVfs, dstVfs, names, initialDest, isMove, AppConfig.DefaultFileOpMode, onCompleteWithClear)
		return
	}

	if !isMove && !AppConfig.ConfirmCopy {
		go ExecuteFileOp(pf, srcVfs, dstVfs, names, initialDest, isMove, AppConfig.DefaultFileOpMode, onCompleteWithClear)
		return
	}

	dlg := vtui.NewCenteredDialog(50, 11, title)
	dlg.ShowClose = true

	promptLbl := vtui.NewLabel(0, 0, fmt.Sprintf(prompt, len(names)), nil)
	dlg.AddItem(promptLbl)

	editDest := vtui.NewEdit(0, 0, 10, initialDest)
	dlg.AddItem(editDest)

	modes := []string{Msg("Op.Queue"), Msg("Op.Background"), Msg("Op.Foreground")}
	comboMode := vtui.NewComboBox(0, 0, 32, modes)
	comboMode.DropdownOnly = true
	defMode := AppConfig.DefaultFileOpMode
	if defMode < 0 || defMode >= len(modes) {
		defMode = 0
	}
	comboMode.Menu.SetSelectPos(defMode)
	comboMode.Edit.SetText(modes[defMode])
	dlg.AddItem(comboMode)

	btnOk := vtui.NewButton(0, 0, Msg("Copy.Btn"))
	if isMove {
		btnOk = vtui.NewButton(0, 0, Msg("Move.Btn"))
	}
	btnOk.IsDefault = true

	btnOk.OnClick = func() {
		dest := editDest.GetText()
		mode := comboMode.Menu.SelectPos
		dlg.Close()
		if dest != "" {
			go ExecuteFileOp(pf, srcVfs, dstVfs, names, dest, isMove, mode, onCompleteWithClear)
		}
	}
	dlg.AddItem(btnOk)

	btnCancel := vtui.NewButton(0, 0, "Cancel")
	btnCancel.OnClick = func() { dlg.Close() }
	dlg.AddItem(btnCancel)

	// Layout Engine
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 50-4, 11-4)
	vbox.Add(promptLbl, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editDest, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(comboMode, vtui.Margins{Top: 1}, vtui.AlignCenter)

	hbox := vtui.NewHBoxLayout(0, 0, 50-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)

	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()
	dlg.SetFocusedItem(editDest)

	vtui.FrameManager.Push(dlg)
}
func actionRename(pf *PanelsFrame) {
	fsp := pf.getActivePanel()
	if fsp == nil {
		return
	}

	name := fsp.getRawSelectedName()
	if name == "" || name == ".." {
		return
	}

	vtui.InputBox(Msg("Dialog.RenameTitle"), fmt.Sprintf(Msg("Dialog.RenamePrompt"), name), name, func(newName string) {
		if newName == "" || newName == name {
			return
		}
		oldPath := fsp.vfs.Join(fsp.vfs.GetPath(), name)
		newPath := fsp.vfs.Join(fsp.vfs.GetPath(), newName)

		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			err := fsp.vfs.Rename(ctx.Context, oldPath, newPath)
			ctx.RunOnUI(func() {
				if err != nil {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to rename:\n%v", err), []string{"&Ok"})
					fsp.pendingSelection = name
				} else {
					// Clear cache to ensure the new name is visible immediately
					delete(fsp.dirCache, fsp.vfs.GetPath())
					fsp.pendingSelection = newName
				}
				pf.RefreshAll()
			})
		})
	})
}
func actionCopyInPlace(pf *PanelsFrame) {
	fsp := pf.getActivePanel()
	if fsp == nil {
		return
	}

	name := fsp.getRawSelectedName()
	if name == "" || name == ".." {
		return
	}

	vtui.InputBox(" Copy ", "Copy '"+name+"' to:", name, func(newName string) {
		if newName == "" || newName == name {
			return
		}
		newPath := fsp.vfs.Join(fsp.vfs.GetPath(), newName)

		onCompleteWithClear := func() {
			if pf != nil {
				if fsp := pf.getActivePanel(); fsp != nil {
					fsp.selectedItems = make(map[string]bool)
					for _, e := range fsp.entries {
						e.Selected = false
					}
				}
				pf.RefreshAll()
			}
		}

		go ExecuteFileOp(pf, fsp.vfs, fsp.vfs, []string{name}, newPath, false, AppConfig.DefaultFileOpMode, onCompleteWithClear)
	})
}
func actionEditorSettings(pf *PanelsFrame) {
	width, height := 78, 23
	dlg := vtui.NewCenteredDialog(width, height, Msg("EditorSettings.Title"))
	dlg.ShowClose = true

	// 1. Initialize Widgets
	comboExpand := vtui.NewComboBox(0, 0, 40, []string{
		"Do not expand tabs",
		"Expand newly entered tabs to spaces",
		"Expand all tabs to spaces",
	})
	comboExpand.DropdownOnly = true
	if AppConfig.EditorExpandTabs >= 0 && AppConfig.EditorExpandTabs <= 2 {
		comboExpand.Menu.SetSelectPos(AppConfig.EditorExpandTabs)
		comboExpand.Edit.SetText(comboExpand.Menu.Items[AppConfig.EditorExpandTabs].Text)
	}
	lblExpand := vtui.NewLabel(0, 0, "Expand t&abs:", comboExpand)

	editTabSize := vtui.NewEdit(0, 0, 4, fmt.Sprintf("%d", AppConfig.EditorTabSize))
	editTabSize.ClearSelection()
	lblTabSize := vtui.NewLabel(0, 0, "Tab si&ze:", editTabSize)

	chkAutoIndent := vtui.NewCheckbox(0, 0, "Auto i&ndent", false)
	if AppConfig.EditorAutoIndent {
		chkAutoIndent.State = 1
	}

	chkCursorEOL := vtui.NewCheckbox(0, 0, "Cursor beyond end of &line", false)
	if AppConfig.EditorCursorBeyondEOL {
		chkCursorEOL.State = 1
	}

	chkEditorConfig := vtui.NewCheckbox(0, 0, "Use .&editorconfig settings files", false)
	if AppConfig.EditorUseEditorConfig {
		chkEditorConfig.State = 1
	}

	chkAuto := vtui.NewCheckbox(0, 0, Msg("EditorSettings.AutoComplete"), false)
	if AppConfig.EditorAutoComplete {
		chkAuto.State = 1
	}

	chkCrosshair := vtui.NewCheckbox(0, 0, "Show cross&hair", false)
	if AppConfig.EditorCrosshair {
		chkCrosshair.State = 1
	}

	editMask := vtui.NewEdit(0, 0, 56, AppConfig.EditorAutoCompleteMask)
	lblMask := vtui.NewLabel(0, 0, Msg("EditorSettings.Mask"), editMask)

	chkExtEdit := vtui.NewCheckbox(0, 0, "Use e&xternal editor", false)
	if AppConfig.UseExternalEditor {
		chkExtEdit.State = 1
	}

	editExtCmd := vtui.NewEdit(0, 0, 20, AppConfig.ExternalEditorCommand)
	lblExtCmd := vtui.NewLabel(0, 0, "E&xternal command:", editExtCmd)

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	// 2. Add to Dialog in desired focus order
	dlg.AddItem(lblExpand)
	dlg.AddItem(comboExpand)
	dlg.AddItem(lblTabSize)
	dlg.AddItem(editTabSize)
	dlg.AddItem(chkAutoIndent)
	dlg.AddItem(chkCursorEOL)
	dlg.AddItem(chkEditorConfig)
	dlg.AddItem(chkAuto)
	dlg.AddItem(chkCrosshair)
	dlg.AddItem(lblMask)
	dlg.AddItem(editMask)
	dlg.AddItem(chkExtEdit)
	dlg.AddItem(lblExtCmd)
	dlg.AddItem(editExtCmd)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)
	dlg.AddLink(chkExtEdit, editExtCmd, vtui.LinkEnableIfChecked)

	// 3. Layout Configuration
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)

	rowTabs := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowTabs.Add(lblExpand, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowTabs.Add(comboExpand, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowTabs, vtui.Margins{}, vtui.AlignFill)

	rowTabSize := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowTabSize.Add(lblTabSize, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowTabSize.Add(editTabSize, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowTabSize, vtui.Margins{Top: 1}, vtui.AlignFill)

	col1 := vtui.NewVBoxLayout(0, 0, (width-4)/2, 5)
	col1.Add(chkAutoIndent, vtui.Margins{}, vtui.AlignLeft)
	col1.Add(chkEditorConfig, vtui.Margins{Top: 1}, vtui.AlignLeft)

	col2 := vtui.NewVBoxLayout(0, 0, (width-4)/2, 5)
	col2.Add(chkCursorEOL, vtui.Margins{}, vtui.AlignLeft)
	col2.Add(chkAuto, vtui.Margins{Top: 1}, vtui.AlignLeft)
	col2.Add(chkCrosshair, vtui.Margins{Top: 1}, vtui.AlignLeft)

	rowChecks := vtui.NewHBoxLayout(0, 0, width-4, 5)
	rowChecks.Add(col1, vtui.Margins{}, vtui.AlignFill)
	rowChecks.Add(col2, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowChecks, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Add(lblMask, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(editMask, vtui.Margins{}, vtui.AlignFill)

	rowExt := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowExt.Add(chkExtEdit, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowExt.Add(lblExtCmd, vtui.Margins{Right: 1, Left: 2}, vtui.AlignLeft)
	rowExt.Add(editExtCmd, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowExt, vtui.Margins{Top: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Apply()

	// 4. Logic
	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		AppConfig.EditorExpandTabs = comboExpand.Menu.SelectPos
		fmt.Sscanf(editTabSize.GetText(), "%d", &AppConfig.EditorTabSize)
		if AppConfig.EditorTabSize <= 0 {
			AppConfig.EditorTabSize = 8
		}

		AppConfig.EditorAutoIndent = chkAutoIndent.State == 1
		AppConfig.EditorCursorBeyondEOL = chkCursorEOL.State == 1
		AppConfig.EditorUseEditorConfig = chkEditorConfig.State == 1
		AppConfig.EditorAutoComplete = chkAuto.State == 1
		AppConfig.EditorCrosshair = chkCrosshair.State == 1
		AppConfig.EditorAutoCompleteMask = editMask.GetText()
		AppConfig.UseExternalEditor = chkExtEdit.State == 1
		AppConfig.ExternalEditorCommand = editExtCmd.GetText()
		SaveConfig()
		dlg.Close()
	}

	vtui.FrameManager.Push(dlg)
}

func actionDelete(pf *PanelsFrame) {
	fsp := pf.getActivePanel()
	if fsp == nil {
		return
	}

	activeVfs := fsp.vfs
	names := fsp.GetSelectedNames()
	if len(names) == 0 {
		return
	}

	if AppConfig.ConfirmDelete == false {
		go ExecuteDeleteOp(pf, activeVfs, names, AppConfig.DefaultFileOpMode, pf.RefreshAll)
		return
	}

	msgName := names[0]
	if len(names) > 1 {
		msgName = fmt.Sprintf("%d items", len(names))
	}

	title := Msg("Delete.Title")
	msg := fmt.Sprintf(Msg("Delete.Confirm"), msgName)
	lines := vtui.WrapText(msg, 46)

	dlg := vtui.NewCenteredDialog(50, 8+len(lines), title)
	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 50-4, (8+len(lines))-4)

	for _, l := range lines {
		t := vtui.NewText(0, 0, l, vtui.Palette[vtui.ColDialogText])
		dlg.AddItem(t)
		vbox.Add(t, vtui.Margins{}, vtui.AlignCenter)
	}

	modes := []string{Msg("Op.Queue"), Msg("Op.Background"), Msg("Op.Foreground")}
	comboMode := vtui.NewComboBox(0, 0, 32, modes)
	comboMode.DropdownOnly = true
	defMode := AppConfig.DefaultFileOpMode
	if defMode < 0 || defMode >= len(modes) {
		defMode = 0
	}
	comboMode.Menu.SetSelectPos(defMode)
	comboMode.Edit.SetText(modes[defMode])
	dlg.AddItem(comboMode)
	vbox.Add(comboMode, vtui.Margins{Top: 1}, vtui.AlignCenter)

	btnDel := vtui.NewButton(0, 0, Msg("Delete.Btn"))
	btnCancel := vtui.NewButton(0, 0, "Cancel")

	if AppConfig.DeleteCancelFocused {
		btnCancel.IsDefault = true
	} else {
		btnDel.IsDefault = true
	}

	dlg.AddItem(btnDel)
	dlg.AddItem(btnCancel)

	hbox := vtui.NewHBoxLayout(0, 0, 50-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnDel, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnDel.OnClick = func() {
		mode := comboMode.Menu.SelectPos
		fsp.pendingSelection = fsp.GetSuccessorName()
		dlg.Close()
		go ExecuteDeleteOp(pf, activeVfs, names, mode, pf.RefreshAll)
	}

	if AppConfig.DeleteCancelFocused {
		dlg.SetFocusedItem(btnCancel)
	} else {
		dlg.SetFocusedItem(btnDel)
	}

	vtui.FrameManager.Push(dlg)
}

func actionMkDir(pf *PanelsFrame) {
	panel := pf.getActivePanel()
	if panel == nil {
		return
	}

	activeVfs := panel.vfs

	dlg := vtui.NewCenteredDialog(40, 11, Msg("MakeFolder.Title"))
	dlg.ShowClose = true

	editName := vtui.NewEdit(0, 0, 10, "")
	lblPrompt := vtui.NewLabel(0, 0, Msg("MakeFolder.Prompt"), editName)
	dlg.AddItem(lblPrompt)
	dlg.AddItem(editName)

	modes := []string{Msg("Op.Queue"), Msg("Op.Background"), Msg("Op.Foreground")}
	comboMode := vtui.NewComboBox(0, 0, 30, modes)
	comboMode.DropdownOnly = true
	defMode := AppConfig.DefaultFileOpMode
	if defMode < 0 || defMode >= len(modes) {
		defMode = 0
	}
	comboMode.Menu.SetSelectPos(defMode)
	comboMode.Edit.SetText(modes[defMode])
	dlg.AddItem(comboMode)

	btnOk := vtui.NewButton(0, 0, "&Ok")
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, "Cancel")
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 40-4, 11-4)
	vbox.Add(lblPrompt, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editName, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(comboMode, vtui.Margins{Top: 1}, vtui.AlignCenter)

	hbox := vtui.NewHBoxLayout(0, 0, 40-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	dlg.SetFocusedItem(editName)

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		name := editName.GetText()
		mode := comboMode.Menu.SelectPos
		dlg.Close()
		if name == "" {
			return
		}
		fullPath := activeVfs.Join(activeVfs.GetPath(), name)

		desc := fmt.Sprintf("Create folder %s", name)
		runFunc := func(ctx context.Context, reporter TaskReporter, anchor vtui.Frame) error {
			reporter.UpdateTransfer("Creating", name, 100, "Folder", 100, "")
			err := activeVfs.MkDir(ctx, fullPath)
			return err
		}

		if mode == 0 { // Queue
			rk := getResourceKey(activeVfs)
			var keys []string
			if rk != "" {
				keys = append(keys, rk)
			}
			task := &QueueTask{
				Type:    "MkDir",
				Desc:    desc,
				ResKeys: keys,
				Run:     runFunc,
				OnComplete: func() {
					panel.pendingSelection = name
					pf.RefreshAll()
				},
			}
			GlobalQueueManager.Enqueue(task)
		} else { // Background / Foreground
			taskCtx := vtui.RunAsync(func(ctx *vtui.TaskContext) {
				err := runFunc(ctx.Context, &DummyReporter{}, nil)
				ctx.RunOnUI(func() {
					if err != nil {
						vtui.ShowMessage(" Error ", fmt.Sprintf(Msg("Operation.Error"), err.Error()), []string{"&Ok"})
					}
					panel.pendingSelection = name
					pf.RefreshAll()
				})
			})
			_ = taskCtx
		}
	}

	vtui.FrameManager.Push(dlg)
}

func actionFindFile(pf *PanelsFrame) {
	activePanel := pf.getActivePanel()
	if activePanel == nil {
		return
	}

	dlg := vtui.NewCenteredDialog(54, 13, Msg("FindFile.Title"))
	dlg.ShowClose = true

	lblMask := vtui.NewLabel(0, 0, Msg("FindFile.MaskPrompt"), nil)
	editMask := vtui.NewEdit(0, 0, 20, LastFindFileMask)
	lblMask.FocusLink = editMask
	dlg.SetFocusedItem(editMask)

	lblText := vtui.NewLabel(0, 0, Msg("FindFile.TextPrompt"), nil)
	editText := vtui.NewEdit(0, 0, 20, LastFindFileText)
	lblText.FocusLink = editText

	btnFind := vtui.NewButton(0, 0, Msg("FindFile.BtnFind"))
	btnFind.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(lblMask)
	dlg.AddItem(editMask)
	dlg.AddItem(lblText)
	dlg.AddItem(editText)
	dlg.AddItem(btnFind)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 54-4, 13-4)
	vbox.Add(lblMask, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editMask, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Add(lblText, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(editText, vtui.Margins{Top: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, 54-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter

	hbox.Spacing = 2
	hbox.Add(btnFind, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)

	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnFind.OnClick = func() {
		LastFindFileMask = editMask.GetText()
		LastFindFileText = editText.GetText()
		SaveSession()
		dlg.Close()
		if LastFindFileMask != "" {
			ExecuteFindFile(pf, activePanel.vfs, activePanel.vfs.GetPath(), LastFindFileMask, LastFindFileText)
		}
	}

	vtui.FrameManager.Push(dlg)
}
func actionPanelSettings(pf *PanelsFrame) {
	dlg := vtui.NewCenteredDialog(60, 24, Msg("PanelSettings.Title"))
	dlg.ShowClose = true

	chkHidden := vtui.NewCheckbox(0, 0, Msg("PanelSettings.ShowHidden"), false)
	chkHidden.State = 0
	if AppConfig.ShowHiddenFiles {
		chkHidden.State = 1
	}

	chkHighlight := vtui.NewCheckbox(0, 0, Msg("PanelSettings.HighlightDir"), false)
	chkHighlight.State = 0
	if AppConfig.HighlightDir {
		chkHighlight.State = 1
	}

	chkPaths := vtui.NewCheckbox(0, 0, Msg("PanelSettings.SavePaths"), false)
	chkPaths.State = 0
	if AppConfig.SavePanelPaths {
		chkPaths.State = 1
	}

	chkCmdAc := vtui.NewCheckbox(0, 0, "Enable command line &auto-completion", false)
	chkCmdAc.State = 0
	if AppConfig.CommandLineAutoComplete {
		chkCmdAc.State = 1
	}
	chkVim := vtui.NewCheckbox(0, 0, Msg("PanelSettings.VimHotkeys"), false)
	chkVim.State = 0
	if AppConfig.VimHotkeys {
		chkVim.State = 1
	}

	chkSync := vtui.NewCheckbox(0, 0, Msg("PanelSettings.SyncPanelLoad"), false)
	chkSync.State = 0
	if AppConfig.SyncPanelLoad {
		chkSync.State = 1
	}

	chkAlwaysMenu := vtui.NewCheckbox(0, 0, "Always show &menu bar", false)
	chkAlwaysMenu.State = 0
	if AppConfig.AlwaysShowMenuBar {
		chkAlwaysMenu.State = 1
	}

	modes := []string{Msg("Op.Queue"), Msg("Op.Background"), Msg("Op.Foreground")}
	comboMode := vtui.NewComboBox(0, 0, 30, modes)
	comboMode.DropdownOnly = true
	comboMode.Menu.SetSelectPos(AppConfig.DefaultFileOpMode)
	comboMode.Edit.SetText(modes[AppConfig.DefaultFileOpMode])
	lblMode := vtui.NewLabel(0, 0, Msg("PanelSettings.DefaultMode"), comboMode)

	pathModes := []string{Msg("Op.PathNameOnly"), Msg("Op.PathFullPath"), Msg("Op.PathSrcDst")}
	comboPath := vtui.NewComboBox(0, 0, 30, pathModes)
	comboPath.DropdownOnly = true
	comboPath.Menu.SetSelectPos(AppConfig.FileOpPathDisplay)
	comboPath.Edit.SetText(pathModes[AppConfig.FileOpPathDisplay])
	lblPath := vtui.NewLabel(0, 0, Msg("PanelSettings.PathDisplay"), comboPath)

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(chkHidden)
	dlg.AddItem(chkHighlight)
	dlg.AddItem(chkPaths)
	dlg.AddItem(chkCmdAc)
	dlg.AddItem(chkVim)
	dlg.AddItem(chkSync)
	dlg.AddItem(chkAlwaysMenu)
	dlg.AddItem(lblMode)
	dlg.AddItem(comboMode)
	dlg.AddItem(lblPath)
	dlg.AddItem(comboPath)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 54-4, 24-4)
	vbox.Add(chkHidden, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(chkHighlight, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(chkPaths, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(chkCmdAc, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(chkVim, vtui.Margins{Top: 1}, vtui.AlignLeft)

	vbox.Add(chkSync, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(chkAlwaysMenu, vtui.Margins{Top: 1}, vtui.AlignLeft)

	rowMode := vtui.NewHBoxLayout(0, 0, 54-4, 1)
	rowMode.Add(lblMode, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowMode.Add(comboMode, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowMode, vtui.Margins{Top: 1}, vtui.AlignFill)

	rowPath := vtui.NewHBoxLayout(0, 0, 54-4, 1)
	rowPath.Add(lblPath, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowPath.Add(comboPath, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowPath, vtui.Margins{Top: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, 54-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)

	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		AppConfig.ShowHiddenFiles = chkHidden.State == 1
		AppConfig.HighlightDir = chkHighlight.State == 1
		AppConfig.SavePanelPaths = chkPaths.State == 1
		AppConfig.CommandLineAutoComplete = chkCmdAc.State == 1
		AppConfig.VimHotkeys = chkVim.State == 1
		AppConfig.SyncPanelLoad = chkSync.State == 1
		AppConfig.AlwaysShowMenuBar = chkAlwaysMenu.State == 1
		AppConfig.DefaultFileOpMode = comboMode.Menu.SelectPos
		AppConfig.FileOpPathDisplay = comboPath.Menu.SelectPos
		SaveConfig()
		dlg.Close()
		pf.ResizeConsole(pf.lastW, pf.lastH)
		pf.RefreshAll()
	}

	vtui.FrameManager.Push(dlg)
}
func actionConfirmationsSettings(pf *PanelsFrame) {
	dlg := vtui.NewCenteredDialog(44, 13, Msg("ConfirmationsSettings.Title"))
	dlg.ShowClose = true

	chkCopy := vtui.NewCheckbox(0, 0, Msg("ConfirmationsSettings.Copy"), false)
	chkCopy.State = 0
	if AppConfig.ConfirmCopy {
		chkCopy.State = 1
	}

	chkMove := vtui.NewCheckbox(0, 0, Msg("ConfirmationsSettings.Move"), false)
	chkMove.State = 0
	if AppConfig.ConfirmMove {
		chkMove.State = 1
	}

	chkDelete := vtui.NewCheckbox(0, 0, Msg("ConfirmationsSettings.Delete"), false)
	chkDelete.State = 0
	if AppConfig.ConfirmDelete {
		chkDelete.State = 1
	}

	chkExit := vtui.NewCheckbox(0, 0, Msg("ConfirmationsSettings.Exit"), false)
	chkExit.State = 0
	if AppConfig.ConfirmExit {
		chkExit.State = 1
	}

	chkDelFocus := vtui.NewCheckbox(0, 0, "Focus 'Cancel' on delete", false)
	chkDelFocus.State = 0
	if AppConfig.DeleteCancelFocused {
		chkDelFocus.State = 1
	}

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(chkCopy)
	dlg.AddItem(chkMove)
	dlg.AddItem(chkDelete)
	dlg.AddItem(chkExit)
	dlg.AddItem(chkDelFocus)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 44-4, 13-4)
	vbox.Add(chkCopy, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(chkMove, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(chkDelete, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(chkExit, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(chkDelFocus, vtui.Margins{}, vtui.AlignLeft)

	hbox := vtui.NewHBoxLayout(0, 0, 44-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)

	vbox.Add(hbox, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		AppConfig.ConfirmCopy = chkCopy.State == 1
		AppConfig.ConfirmMove = chkMove.State == 1
		AppConfig.ConfirmDelete = chkDelete.State == 1
		AppConfig.ConfirmExit = chkExit.State == 1
		AppConfig.DeleteCancelFocused = chkDelFocus.State == 1
		SaveConfig()
		dlg.Close()
		pf.RefreshAll()
	}

	vtui.FrameManager.Push(dlg)
}
func actionUpdateSettings(pf *PanelsFrame) {
	width, height := 54, 11
	dlg := vtui.NewCenteredDialog(width, height, Msg("UpdateSettings.Title"))
	dlg.ShowClose = true

	channels := []string{"Stable releases", "Nightly builds"}
	comboChannel := vtui.NewComboBox(0, 0, 24, channels)
	comboChannel.DropdownOnly = true
	if AppConfig.UpdateChannel >= 0 && AppConfig.UpdateChannel < len(channels) {
		comboChannel.Menu.SetSelectPos(AppConfig.UpdateChannel)
		comboChannel.Edit.SetText(channels[AppConfig.UpdateChannel])
	}
	lblChannel := vtui.NewLabel(0, 0, Msg("UpdateSettings.Channel"), comboChannel)

	intervals := []string{"Never", "Every start", "Daily", "Weekly"}
	comboInterval := vtui.NewComboBox(0, 0, 24, intervals)
	comboInterval.DropdownOnly = true
	if AppConfig.UpdateInterval >= 0 && AppConfig.UpdateInterval < len(intervals) {
		comboInterval.Menu.SetSelectPos(AppConfig.UpdateInterval)
		comboInterval.Edit.SetText(intervals[AppConfig.UpdateInterval])
	}
	lblInterval := vtui.NewLabel(0, 0, Msg("UpdateSettings.Interval"), comboInterval)

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCheck := vtui.NewButton(0, 0, Msg("UpdateSettings.BtnCheck"))
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(lblChannel)
	dlg.AddItem(comboChannel)
	dlg.AddItem(lblInterval)
	dlg.AddItem(comboInterval)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCheck)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)

	rowChannel := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowChannel.Add(lblChannel, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowChannel.Add(comboChannel, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowChannel, vtui.Margins{}, vtui.AlignFill)

	rowInterval := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowInterval.Add(lblInterval, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowInterval.Add(comboInterval, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowInterval, vtui.Margins{Top: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCheck, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)

	vbox.Add(hbox, vtui.Margins{Top: 2}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		AppConfig.UpdateChannel = comboChannel.Menu.SelectPos
		AppConfig.UpdateInterval = comboInterval.Menu.SelectPos
		SaveConfig()
		dlg.Close()
	}
	btnCheck.OnClick = func() {
		AppConfig.UpdateChannel = comboChannel.Menu.SelectPos
		AppConfig.UpdateInterval = comboInterval.Menu.SelectPos
		SaveConfig()
		dlg.Close()
		CheckForUpdates(pf, true)
	}

	vtui.FrameManager.Push(dlg)
}

func actionAppearanceSettings(pf *PanelsFrame) {
	const width, height = 60, 19
	dlg := vtui.NewCenteredDialog(width, height, Msg("AppearanceSettings.Title"))
	dlg.ShowClose = true
	originalStyle := AppConfig.ColorStyle

	styles := AvailableColorStyles()
	names := make([]string, len(styles))
	selected := 0
	for i, style := range styles {
		names[i] = style.Name
		if strings.EqualFold(style.Name, AppConfig.ColorStyle) {
			selected = i
		}
	}

	comboStyle := vtui.NewComboBox(0, 0, 24, names)
	comboStyle.DropdownOnly = true
	if len(names) > 0 {
		comboStyle.Menu.SetSelectPos(selected)
		comboStyle.Edit.SetText(names[selected])
	}
	lblStyle := vtui.NewText(0, 0, Msg("AppearanceSettings.Style"), 0)
	lblStyle.FocusLink = comboStyle
	defaultMenuAction := comboStyle.Menu.OnAction
	comboStyle.Menu.OnAction = func(idx int) {
		defaultMenuAction(idx)
		if idx >= 0 && idx < len(names) {
			if err := ApplyColorStyle(names[idx]); err == nil {
				vtui.FrameManager.Redraw()
			}
		}
	}
	editFont := vtui.NewEdit(0, 0, 30, AppConfig.GuiFont)
	lblFont := vtui.NewLabel(0, 0, Msg("AppearanceSettings.Font"), editFont)

	editSize := vtui.NewEdit(0, 0, 6, fmt.Sprintf("%d", AppConfig.GuiFontSize))
	editSize.Validator = &vtui.IntRangeValidator{Min: 6, Max: 72}
	lblSize := vtui.NewLabel(0, 0, Msg("AppearanceSettings.FontSize"), editSize)

	editTitle := vtui.NewEdit(0, 0, 30, AppConfig.ConsoleTitleTemplate)
	lblTitle := vtui.NewLabel(0, 0, Msg("AppearanceSettings.TitleTemplate"), editTitle)

	chkCursor := vtui.NewCheckbox(0, 0, Msg("PanelSettings.KeepCursor"), false)
	chkCursor.State = 0
	if AppConfig.KeepTerminalCursor {
		chkCursor.State = 1
	}

	btnOk := vtui.NewButton(0, 0, Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnExport := vtui.NewButton(0, 0, Msg("AppearanceSettings.ExportBtn"))
	btnCancel := vtui.NewButton(0, 0, Msg("vtui.Cancel"))

	dlg.AddItem(lblStyle)
	dlg.AddItem(comboStyle)
	dlg.AddItem(lblFont)
	dlg.AddItem(editFont)
	dlg.AddItem(lblSize)
	dlg.AddItem(editSize)
	dlg.AddItem(lblTitle)
	dlg.AddItem(editTitle)
	dlg.AddItem(chkCursor)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnExport)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	rowStyle := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowStyle.Add(lblStyle, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowStyle.Add(comboStyle, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowStyle, vtui.Margins{}, vtui.AlignFill)

	rowFont := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowFont.Add(lblFont, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowFont.Add(editFont, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowFont, vtui.Margins{Top: 1}, vtui.AlignFill)

	rowSize := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowSize.Add(lblSize, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowSize.Add(editSize, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(rowSize, vtui.Margins{Top: 1}, vtui.AlignFill)

	rowTitle := vtui.NewHBoxLayout(0, 0, width-4, 1)
	rowTitle.Add(lblTitle, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowTitle.Add(editTitle, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowTitle, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Add(chkCursor, vtui.Margins{Top: 1}, vtui.AlignLeft)

	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnExport, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() {
		dlg.SetExitCode(-1)
	}
	btnExport.OnClick = func() {
		colorsPath := filepath.Join(GetF4ConfigDir(), "farcolors.ini")
		err := ExportColors(colorsPath)
		if err != nil {
			vtui.ShowMessageOn(dlg, " Error ", fmt.Sprintf("Failed to export colors:\n%v", err), []string{"&Ok"})
		} else {
			vtui.ShowMessageOn(dlg, " Info ", "Current colors successfully exported to:\n"+colorsPath+"\n\nYou can edit this file to customize your palette.", []string{"&Ok"})
		}
	}
	btnOk.OnClick = func() {
		if len(names) > 0 {
			AppConfig.ColorStyle = names[comboStyle.Menu.SelectPos]
		}
		fontChanged := AppConfig.GuiFont != editFont.GetText() || fmt.Sprintf("%d", AppConfig.GuiFontSize) != editSize.GetText()

		AppConfig.ConsoleTitleTemplate = editTitle.GetText()
		AppConfig.GuiFont = editFont.GetText()
		fmt.Sscanf(editSize.GetText(), "%d", &AppConfig.GuiFontSize)
		if AppConfig.GuiFontSize <= 0 {
			AppConfig.GuiFontSize = 18
		}
		AppConfig.KeepTerminalCursor = chkCursor.State == 1
		vtui.ManageCursorStyle = !AppConfig.KeepTerminalCursor
		SaveConfig()

		dlg.SetExitCode(1)

		if fontChanged {
			vtui.FrameManager.PostTask(func() {
				vtui.ShowMessage(" Appearance ", "Font changes in GUI mode will take effect\nafter application restart.", []string{"&Ok"})
			})
		}
	}

	dlg.OnResult = func(code int) {
		if code < 0 {
			_ = ApplyColorStyle(originalStyle)
		}
		vtui.FrameManager.Redraw()
	}

	vtui.FrameManager.Push(dlg)
}

type dialogVFSAdapter struct {
	v vfs.VFS
}

func (a *dialogVFSAdapter) GetPath() string         { return a.v.GetPath() }
func (a *dialogVFSAdapter) SetPath(p string) error  { return a.v.SetPath(p) }
func (a *dialogVFSAdapter) Join(e ...string) string { return a.v.Join(e...) }
func (a *dialogVFSAdapter) Dir(p string) string     { return a.v.Dir(p) }
func (a *dialogVFSAdapter) Base(p string) string    { return a.v.Base(p) }
func (a *dialogVFSAdapter) ReadDir(ctx context.Context, p string, onChunk func([]vtui.FSItem)) error {
	return a.v.ReadDir(ctx, p, func(chunk []vfs.VFSItem) {
		var items []vtui.FSItem
		for _, c := range chunk {
			items = append(items, vtui.FSItem{Name: c.Name, IsDir: c.IsDir})
		}
		onChunk(items)
	})
}
func actionManagePlugins(pf *PanelsFrame) {
	width, height := 60, 16
	dlg := vtui.NewCenteredDialog(width, height, Msg("Plugins.Title"))
	dlg.ShowClose = true

	lb := vtui.NewListBox(0, 0, 56, 10, AppConfig.RegisteredPlugins)

	btnAdd := vtui.NewButton(0, 0, Msg("Plugins.BtnAdd"))
	btnDel := vtui.NewButton(0, 0, Msg("Plugins.BtnRemove"))
	btnClose := vtui.NewButton(0, 0, Msg("Plugins.BtnClose"))

	dlg.AddItem(lb)
	dlg.AddItem(btnAdd)
	dlg.AddItem(btnDel)
	dlg.AddItem(btnClose)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(lb, vtui.Margins{Bottom: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnAdd, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnDel, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnClose, vtui.Margins{}, vtui.AlignTop)

	vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	btnAdd.OnClick = func() {
		startPath := "."
		if fsp := pf.getActivePanel(); fsp != nil {
			if _, ok := fsp.vfs.(*vfs.OSVFS); ok {
				startPath = fsp.vfs.GetPath()
			}
		}
		pluginVfs := &dialogVFSAdapter{v: vfs.NewOSVFS(startPath)}
		vtui.SelectFileDialog(" Add Plugin ", startPath, pluginVfs, func(path string) {
			if path != "" {
				AppConfig.RegisteredPlugins = append(AppConfig.RegisteredPlugins, path)
				SaveConfig()
				lb.Items = AppConfig.RegisteredPlugins
				lb.UpdateRows()
				vtui.FrameManager.Redraw()
				if GlobalPluginManager != nil {
					GlobalPluginManager.LoadExternalPlugin(path)
				}
			}
		})
	}

	btnDel.OnClick = func() {
		idx := lb.SelectPos
		if idx >= 0 && idx < len(AppConfig.RegisteredPlugins) {
			AppConfig.RegisteredPlugins = append(AppConfig.RegisteredPlugins[:idx], AppConfig.RegisteredPlugins[idx+1:]...)
			SaveConfig()
			lb.Items = AppConfig.RegisteredPlugins
			lb.UpdateRows()
			vtui.ShowMessageOn(dlg, " Info ", "Plugin removed from config.\nRestart f4 to fully unload the process.", []string{"&Ok"})
		}
	}

	btnClose.OnClick = func() { dlg.Close() }

	vtui.FrameManager.Push(dlg)
}

func actionFileAttributes(pf *PanelsFrame) {
	fsp := pf.getActivePanel()
	if fsp == nil {
		return
	}

	name := fsp.getRawSelectedName()
	if name == "" || name == ".." {
		return
	}

	fullPath := fsp.vfs.Join(fsp.vfs.GetPath(), name)

	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		item, err := fsp.vfs.Stat(ctx.Context, fullPath)
		ctx.RunOnUI(func() {
			if err != nil {
				vtui.ShowMessage(" Error ", err.Error(), []string{"&Ok"})
				return
			}
			ShowAttributesDialog(pf, fsp.vfs, fullPath, item)
		})
	})
}

func actionLanguage(pf *PanelsFrame) {
	type langInfo struct {
		code string
		name string
	}
	langs := []langInfo{{"en", "English"}}

	exeDir := filepath.Dir(os.Args[0])
	userDir := filepath.Join(GetF4ConfigDir(), "lang")

	// We add "lang" to support running f4 via "go run ." from the project root
	dirs := []string{filepath.Join(exeDir, "lang"), userDir, "lang"}
	seen := map[string]bool{"en": true}

	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".lng") {
				code := strings.TrimSuffix(e.Name(), ".lng")
				if !seen[code] {
					ini := LoadIni(filepath.Join(d, e.Name()))
					name := ini.GetString("Language", "Name", code)
					langs = append(langs, langInfo{code, name})
					seen[code] = true
				}
			}
		}
	}

	menu := vtui.NewVMenu(Msg("Language.Title"))
	currIdx := 0
	for i, l := range langs {
		menu.AddItem(vtui.MenuItem{Text: l.name})
		if l.code == AppConfig.Language {
			currIdx = i
		}
	}

	scrW := vtui.FrameManager.GetScreenSize()
	scrH := vtui.FrameManager.GetScreenHeight()
	w, h := 30, len(langs)+2
	if h > 15 {
		h = 15
	}
	x := (scrW - w) / 2
	y := (scrH - h) / 2
	menu.SetPosition(x, y, x+w-1, y+h-1)
	menu.SetSelectPos(currIdx)

	menu.OnAction = func(idx int) {
		AppConfig.Language = langs[idx].code
		SaveConfig()
		InitLang()
		vtui.FrameManager.PostTask(func() {
			vtui.ShowMessage(Msg("Info.Title"), Msg("Language.Changed"), []string{Msg("vtui.Ok")})
			vtui.FrameManager.Redraw()
		})
	}

	vtui.FrameManager.Push(menu)
}
