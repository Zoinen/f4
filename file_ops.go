package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type globalAwareReporter struct {
	original  TaskReporter
	getGlobal func(action string) (string, int, string)
	tracker   *FileOpTracker
	onBytes   func(int)
}

func (w *globalAwareReporter) StartFile(name string, size int64) {
	if w.tracker != nil {
		w.tracker.StartFile(name, size)
	}
}

func (w *globalAwareReporter) UpdateBytes(n int) {
	if w.tracker != nil {
		w.tracker.UpdateBytes(n)
	}
	if w.onBytes != nil {
		w.onBytes(n)
	}
}

func (w *globalAwareReporter) FileDone() {
	if w.tracker != nil {
		w.tracker.FileDone()
	}
}
func (w *globalAwareReporter) FileSkipped() {
	if w.tracker != nil {
		w.tracker.FileSkipped()
	}
}

func (w *globalAwareReporter) DirDone() {
	if w.tracker != nil {
		w.tracker.DirDone()
	}
}

func (w *globalAwareReporter) UpdateScan(currentPath string, files, dirs int64) {
	w.original.UpdateScan(currentPath, files, dirs)
}

func (w *globalAwareReporter) IsCancelled() bool {
	return w.original.IsCancelled()
}

func (w *globalAwareReporter) UpdateTransfer(action, filename string, currentPct int, totalText string, totalPct int, speedText string) {
	gTotalText, gTotalPct, gTimeSpeedText := w.getGlobal(action)
	displayFileName := filename
	if totalText != "" && !strings.HasPrefix(totalText, "Total:") && !strings.HasPrefix(totalText, "Extracting:") && !strings.HasPrefix(totalText, "Moving:") && !strings.HasPrefix(totalText, "Copying:") {
		displayFileName = filename + " (" + totalText + ")"
	} else if strings.HasPrefix(totalText, "Extracting:") {
		displayFileName = filename + " (" + totalText + ")"
	}
	w.original.UpdateTransfer(action, displayFileName, currentPct, gTotalText, gTotalPct, gTimeSpeedText)
}

type FileOpState struct {
	OverwriteAll bool
	SkipAll      bool
	SkippedCount int
	OnBytes      func(int)
	Tracker      *FileOpTracker
	UpdateUI     func(force bool)
	Anchor       vtui.Frame
	Buffer       []byte
	IsMove       bool
	S2SDir       int // 0: unknown, 1: push, 2: pull, 3: disabled
}

// formatSize formats a byte count into a human-readable string.
func formatSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatIntWithSpaces converts an int64 to string with spaces as thousands separators.
func formatIntWithSpaces(n int64) string {
	s := strconv.FormatInt(n, 10)
	l := len(s)
	var res strings.Builder
	for i, char := range s {
		res.WriteRune(char)
		if (l-i-1)%3 == 0 && i != l-1 {
			res.WriteByte(' ')
		}
	}
	return res.String()
}

func resolveFileOpDestination(srcVfs, dstVfs vfs.VFS, destInput string) (vfs.VFS, string) {
	// The passive panel supplies the initial absolute destination shown in the
	// dialog. Once the user enters a relative path, however, it is relative to
	// the active (source) panel, just like other panel path operations.
	if dstVfs.IsAbs(destInput) {
		return dstVfs, destInput
	}
	if srcVfs.IsAbs(destInput) {
		return srcVfs, destInput
	}
	return srcVfs, srcVfs.Join(srcVfs.GetPath(), destInput)
}

func ExecuteFileOp(pf *PanelsFrame, srcVfs, dstVfs vfs.VFS, names []string, destInput string, isMove bool, mode int, onComplete func()) {
	dstVfs, destPath := resolveFileOpDestination(srcVfs, dstVfs, destInput)

	isTargetDir := len(names) > 1
	if !isTargetDir {
		if strings.HasSuffix(destInput, "/") || strings.HasSuffix(destInput, "\\") {
			isTargetDir = true
		} else if stat, err := dstVfs.Stat(context.Background(), destPath); err == nil && stat.IsDir {
			isTargetDir = true
		} else if destInput == "." || destInput == ".." {
			isTargetDir = true
		}
	}

	if isMove && pf != nil {
		if fspSrc := pf.getActivePanel(); fspSrc != nil {
			fspSrc.pendingSelection = fspSrc.GetSuccessorName()
		}
	}

	var preconds []OpPrecondition
	for _, name := range names {
		if st, err := srcVfs.Stat(context.Background(), srcVfs.Join(srcVfs.GetPath(), name)); err == nil {
			preconds = append(preconds, OpPrecondition{
				Vfs: srcVfs, Path: srcVfs.Join(srcVfs.GetPath(), name), MTime: st.MTime, Size: st.Size, IsDir: st.IsDir,
			})
		}
	}

	actionDesc := "Copy"
	actionTitle := "Copying"
	dialogTitle := " Copying... "
	if isMove {
		actionDesc = "Move"
		actionTitle = "Moving"
		dialogTitle = " Moving... "
	} else if srcVfs.ParentVFS() != nil && dstVfs.ParentVFS() == nil {
		actionDesc = "Extract"
		actionTitle = "Extracting"
		dialogTitle = " Extracting... "
	} else if srcVfs.ParentVFS() == nil && dstVfs.ParentVFS() != nil {
		actionDesc = "Archive"
		actionTitle = "Archiving"
		dialogTitle = " Archiving... "
	}
	desc := fmt.Sprintf("%d item(s) -> %s", len(names), vtui.TruncateMiddle(destInput, 15))

	runFunc := func(ctx context.Context, reporter TaskReporter, anchor vtui.Frame) error {
		startTime := time.Now()
		dirToEnsure := destPath
		if !isTargetDir {
			dirToEnsure = dstVfs.Dir(destPath)
		}

		if dirToEnsure != "" && dirToEnsure != "." {
			st, err := dstVfs.Stat(ctx, dirToEnsure)
			if err != nil {
				if mkErr := dstVfs.MkDir(ctx, dirToEnsure); mkErr != nil {
					return fmt.Errorf("failed to create target dir: %w", mkErr)
				}
			} else if !st.IsDir {
				return fmt.Errorf("target path component is not a directory: %s", dirToEnsure)
			}
		}

		var totalStats vfs.OpStats
		scanErr := error(nil)
		lastScanUpdate := startTime
		totalStats, scanErr = vfs.CalculateStats(ctx, srcVfs, srcVfs.GetPath(), names, func(currentPath string, stats vfs.OpStats) {
			now := time.Now()
			if now.Sub(lastScanUpdate) > 50*time.Millisecond {
				lastScanUpdate = now
				reporter.UpdateScan(currentPath, stats.Files, stats.Dirs)
			}
		})
		reporter.UpdateScan("", totalStats.Files, totalStats.Dirs)

		if scanErr != nil {
			return scanErr
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		tracker := NewFileOpTracker(totalStats)
		lastUpdate := startTime
		lastSpeedUpdate := startTime
		bytesSinceLastSpeedUpdate := int64(0)
		currentSpeed := float64(0)

		lastLoggedTime := startTime
		lastLoggedPct := -1

		getGlobalStats := func(action string) (string, int, string) {
			now := time.Now()
			_, totalPct, _ := tracker.GetProgress()
			processed, total := tracker.GetStats()

			var totalText string
			if total.Bytes > 0 {
				totalText = fmt.Sprintf("Total: %s / %s", formatSize(processed.Bytes), formatSize(total.Bytes))
			} else {
				totalText = fmt.Sprintf("Total: %d / %d items", processed.Files+processed.Dirs, total.Files+total.Dirs)
			}

			elapsed := now.Sub(startTime)
			elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)

			const ItemOverhead = 32 * 1024
			vProcessed := float64(processed.Bytes + (processed.Files+processed.Dirs)*ItemOverhead)
			vTotal := float64(total.Bytes + (total.Files+total.Dirs)*ItemOverhead)

			etaStr := "Remaining: ??:??:??"
			if vTotal > 0 && vProcessed > 0 && elapsed.Seconds() > 0.5 {
				if action == "Locating" || action == "Waiting" || action == "Scanning" || action == "Archiving" {
					etaStr = "Remaining: ??:??:??"
				} else {
					ratio := vProcessed / vTotal
					etaSecs := (elapsed.Seconds() / ratio) - elapsed.Seconds()
					if etaSecs < 0 {
						etaSecs = 0
					}
					if etaSecs > 359999 {
						etaStr = "Remaining: >99 hours"
					} else {
						etaDur := time.Duration(etaSecs * float64(time.Second))
						etaStr = fmt.Sprintf("Remaining: %02d:%02d:%02d", int(etaDur.Hours()), int(etaDur.Minutes())%60, int(etaDur.Seconds())%60)
					}
				}
			}

			speedStr := ""
			if currentSpeed > 0 {
				speedStr = formatSize(int64(currentSpeed)) + "/s"
			}

			timeSpeedText := fmt.Sprintf("%-16s %-21s %15s", elapsedStr, etaStr, speedStr)
			return totalText, totalPct, timeSpeedText
		}

		var updateUI func(force bool)
		wrapRep := &globalAwareReporter{
			original:  reporter,
			getGlobal: getGlobalStats,
			tracker:   tracker,
			onBytes: func(n int) {
				bytesSinceLastSpeedUpdate += int64(n)
				if updateUI != nil {
					updateUI(false)
				}
			},
		}
		ctx = context.WithValue(ctx, vfs.ReporterKey, wrapRep)

		updateUI = func(force bool) {
			now := time.Now()
			if force || now.Sub(lastUpdate) >= 100*time.Millisecond {
				speedDur := now.Sub(lastSpeedUpdate).Seconds()
				if speedDur >= 1.0 {
					currentSpeed = float64(bytesSinceLastSpeedUpdate) / speedDur
					lastSpeedUpdate = now
					bytesSinceLastSpeedUpdate = 0
				}
				lastUpdate = now

				filePct, _, currName := tracker.GetProgress()
				processed, total := tracker.GetStats()

				action := actionTitle
				gTotalText, gTotalPct, gTimeSpeedText := getGlobalStats(action)

				if gTotalPct >= lastLoggedPct+5 || now.Sub(lastLoggedTime) >= 5*time.Second {
					parts := strings.Fields(gTimeSpeedText)
					elapsedStr, etaStr, speedStr := "", "", ""
					if len(parts) >= 2 {
						elapsedStr = parts[1]
					}
					if len(parts) >= 4 {
						etaStr = parts[3]
					}
					if len(parts) >= 5 {
						speedStr = parts[4]
					}

					vtui.DebugLog("FILEOP: %d%% | Items: %d/%d | Proc: %d/%d B | %s | %s | %s",
						gTotalPct,
						processed.Files+processed.Dirs, total.Files+total.Dirs,
						processed.Bytes, total.Bytes,
						elapsedStr, etaStr, speedStr)
					lastLoggedPct = gTotalPct
					lastLoggedTime = now
				}

				reporter.UpdateTransfer(action, currName, filePct, gTotalText, gTotalPct, gTimeSpeedText)
			}
		}

		state := &FileOpState{
			Tracker:  tracker,
			UpdateUI: updateUI,
			OnBytes: func(n int) {
				tracker.UpdateBytes(n)
				bytesSinceLastSpeedUpdate += int64(n)
				updateUI(false)
			},
			Anchor: anchor,
			Buffer: make([]byte, 128*1024),
			IsMove: isMove,
		}

		updateUI(true)
		// OPTIMIZATION: Check if the source VFS supports bulk copying (e.g. for sequential archives)
		if !isMove && srcVfs != dstVfs {
			if bulkCopier, ok := srcVfs.(vfs.BulkCopier); ok {
				err := bulkCopier.CopyBulk(ctx, names, dstVfs, destPath, wrapRep)
				if err == nil {
					updateUI(true)
					return nil
				}
				vtui.DebugLog("FILEOP: Bulk copy failed, falling back to sequential: %v", err)
			}
		}

		for _, name := range names {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			srcPath := srcVfs.Join(srcVfs.GetPath(), name)
			targetItemPath := destPath
			if isTargetDir {
				targetItemPath = dstVfs.Join(destPath, name)
			}

			if isMove && vfs.SameSession(srcVfs, dstVfs) {
				if _, err := dstVfs.Stat(ctx, targetItemPath); err != nil {
					if err := srcVfs.Rename(ctx, srcPath, targetItemPath); err == nil {
						vtui.DebugLog("FILEOP: Optimized server-side rename: %s -> %s", srcPath, targetItemPath)
						handleArchiveIndexOp(srcVfs, srcPath, dstVfs, targetItemPath, true)

						itemStat, _ := dstVfs.Stat(ctx, targetItemPath)
						if itemStat.IsDir {
							tracker.DirDone()
						} else {
							displayString := name
							if AppConfig.FileOpPathDisplay == 1 {
								displayString = srcPath
							} else if AppConfig.FileOpPathDisplay == 2 {
								displayString = srcPath + " -> " + targetItemPath
							}
							tracker.StartFile(displayString, itemStat.Size)
							tracker.UpdateBytes(int(itemStat.Size))
							tracker.FileDone()
						}
						updateUI(true)
						continue
					}
				}
			}

			err := recursiveCopy(ctx, srcVfs, srcPath, dstVfs, targetItemPath, state, 0)
			if err != nil {
				return err
			}

			if isMove && state.SkippedCount == 0 {
				srcVfs.Remove(ctx, srcPath)
			}
			updateUI(true)
		}
		return nil
	}

	if mode == 0 { // Queue
		rk1 := getResourceKey(srcVfs)
		rk2 := getResourceKey(dstVfs)
		var keys []string
		if rk1 != "" {
			keys = append(keys, rk1)
		}
		if rk2 != "" && rk2 != rk1 {
			keys = append(keys, rk2)
		}
		task := &QueueTask{
			Type:          actionDesc,
			Desc:          desc,
			Preconditions: preconds,
			ResKeys:       keys,
			Run:           runFunc,
			OnComplete:    onComplete,
		}
		GlobalQueueManager.Enqueue(task)
	} else { // Foreground or Background
		dlg := NewFileOpProgressDialog(dialogTitle)
		var taskCtx *vtui.TaskContext
		dlg.btnCancel.OnClick = func() { dlg.SetExitCode(1) }
		dlg.OnResult = func(code int) {
			if taskCtx != nil {
				taskCtx.Cancel()
			}
		}

		reporter := &DialogReporter{dlg: dlg}

		vtui.FrameManager.PostTask(func() {
			if mode == 1 && pf != nil {
				clone := pf.Clone()
				vtui.FrameManager.AddScreen(clone)
				vtui.FrameManager.Push(dlg)
			} else {
				vtui.FrameManager.AddScreenHeadless(dlg)
			}
		})

		taskCtx = vtui.RunAsync(func(ctx *vtui.TaskContext) {
			err := runFunc(ctx.Context, reporter, dlg)
			ctx.RunOnUI(func() {
				dlg.Close()
				if pf != nil {
					pf.RefreshAll()
				}
				if onComplete != nil {
					onComplete()
				}
				if err != nil && err != context.Canceled {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Operation failed:\n%v", err), []string{"&Ok"})
				}
			})
		})
	}
}

func ExecuteDeleteOp(pf *PanelsFrame, activeVfs vfs.VFS, names []string, mode int, onComplete func()) {
	var preconds []OpPrecondition
	for _, name := range names {
		if st, err := activeVfs.Stat(context.Background(), activeVfs.Join(activeVfs.GetPath(), name)); err == nil {
			preconds = append(preconds, OpPrecondition{
				Vfs: activeVfs, Path: activeVfs.Join(activeVfs.GetPath(), name), MTime: st.MTime, Size: st.Size, IsDir: st.IsDir,
			})
		}
	}
	desc := fmt.Sprintf("Delete %d item(s)", len(names))

	runFunc := func(ctx context.Context, reporter TaskReporter, anchor vtui.Frame) error {
		ctx = context.WithValue(ctx, vfs.ReporterKey, reporter)
		var totalStats vfs.OpStats
		scanErr := error(nil)
		totalStats, scanErr = vfs.CalculateStats(ctx, activeVfs, activeVfs.GetPath(), names, func(currentPath string, stats vfs.OpStats) {
			reporter.UpdateScan(currentPath, stats.Files, stats.Dirs)
		})

		if scanErr != nil && scanErr != context.Canceled {
			return fmt.Errorf("failed to scan files: %w", scanErr)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		tracker := NewFileOpTracker(totalStats)
		lastUpdate := time.Now()

		updateUI := func(force bool) {
			now := time.Now()
			if force || now.Sub(lastUpdate) >= 100*time.Millisecond {
				lastUpdate = now
				filePct, totalPct, currName := tracker.GetProgress()
				processed, total := tracker.GetStats()

				var totalText string
				if total.Bytes > 0 {
					totalText = fmt.Sprintf("Total: %d / %d items", processed.Files+processed.Dirs, total.Files+total.Dirs)
				} else {
					totalText = fmt.Sprintf("Total: %d / %d items", processed.Files+processed.Dirs, total.Files+total.Dirs)
				}

				reporter.UpdateTransfer("Deleting", currName, filePct, totalText, totalPct, "")
			}
		}

		updateUI(true)

		var allErrors []string
		var skipAll bool
		for _, name := range names {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fullPath := activeVfs.Join(activeVfs.GetPath(), name)

			displayString := name
			if AppConfig.FileOpPathDisplay > 0 {
				displayString = fullPath
			}
			tracker.StartFile(displayString, 0)
			updateUI(true)
			handleArchiveIndexDelete(ctx, activeVfs, fullPath)

			for {
				err := activeVfs.Remove(ctx, fullPath)
				if err == nil {
					break
				}
				if err == context.Canceled {
					return err
				}

				if skipAll {
					allErrors = append(allErrors, fmt.Sprintf("Skipped '%s':\n%v", name, err))
					break
				}

				choice := askDeleteError(ctx, fmt.Sprintf("Cannot delete '%s'", name), err, anchor)
				if choice == 0 { // Retry
					continue
				} else if choice == 1 { // Skip
					allErrors = append(allErrors, fmt.Sprintf("Skipped '%s':\n%v", name, err))
					break
				} else if choice == 2 { // Skip All
					skipAll = true
					allErrors = append(allErrors, fmt.Sprintf("Skipped '%s':\n%v", name, err))
					break
				} else { // Abort
					return context.Canceled
				}
			}

			tracker.FileDone()
			updateUI(true)
		}
		if len(allErrors) > 0 {
			vtui.FrameManager.PostTask(func() {
				dlgW, dlgH := 60, 15
				scrH := vtui.FrameManager.GetScreenHeight()
				if dlgH > scrH-2 {
					dlgH = scrH - 2
				}
				if dlgH < 8 {
					dlgH = 8
				}

				dlg := vtui.NewCenteredDialog(dlgW, dlgH, " Deletion Errors ")
				dlg.ShowClose = true

				var listItems []string
				for _, errStr := range allErrors {
					lines := vtui.WrapText(errStr, dlgW-6)
					listItems = append(listItems, lines...)
					listItems = append(listItems, strings.Repeat("-", dlgW-6))
				}
				if len(listItems) > 0 {
					listItems = listItems[:len(listItems)-1]
				}

				lb := vtui.NewListBox(0, 0, dlgW-4, dlgH-6, listItems)
				btnOk := vtui.NewButton(0, 0, "&Ok")
				btnOk.IsDefault = true
				btnOk.OnClick = func() { dlg.Close() }

				dlg.AddItem(lb)
				dlg.AddItem(btnOk)

				vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, dlgW-4, dlgH-4)
				vbox.Add(lb, vtui.Margins{Bottom: 1}, vtui.AlignFill)

				hbox := vtui.NewHBoxLayout(0, 0, dlgW-4, 1)
				hbox.HorizontalAlign = vtui.AlignCenter
				hbox.Add(btnOk, vtui.Margins{}, vtui.AlignTop)

				vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
				vbox.Apply()

				if anchor != nil {
					vtui.FrameManager.PushToFrameScreen(anchor, dlg)
				} else {
					vtui.FrameManager.Push(dlg)
				}
			})
		}
		return nil
	}

	if mode == 0 { // Queue
		rk := getResourceKey(activeVfs)
		var keys []string
		if rk != "" {
			keys = append(keys, rk)
		}
		task := &QueueTask{
			Type:          "Delete",
			Desc:          desc,
			Preconditions: preconds,
			ResKeys:       keys,
			Run:           runFunc,
			OnComplete:    onComplete,
		}
		GlobalQueueManager.Enqueue(task)
	} else {
		dlg := NewFileOpProgressDialog(" Deleting... ")
		var taskCtx *vtui.TaskContext
		dlg.btnCancel.OnClick = func() { dlg.SetExitCode(1) }
		dlg.OnResult = func(code int) {
			if taskCtx != nil {
				taskCtx.Cancel()
			}
		}

		reporter := &DialogReporter{dlg: dlg}

		vtui.FrameManager.PostTask(func() {
			if mode == 1 && pf != nil {
				clone := pf.Clone()
				vtui.FrameManager.AddScreen(clone)
				vtui.FrameManager.Push(dlg)
			} else {
				vtui.FrameManager.AddScreenHeadless(dlg)
			}
		})

		taskCtx = vtui.RunAsync(func(ctx *vtui.TaskContext) {
			err := runFunc(ctx.Context, reporter, dlg)
			ctx.RunOnUI(func() {
				dlg.Close()
				if pf != nil {
					pf.RefreshAll()
				}
				if onComplete != nil {
					onComplete()
				}
				if err != nil && err != context.Canceled {
					vtui.ShowMessage(" Error ", fmt.Sprintf("Deletion failed:\n%v", err), []string{"&Ok"})
				}
			})
		})
	}
}

// closeOnce wraps a Close so that it can be called explicitly where its
// error matters and still be left in a defer as a safety net. A writer that
// buffers — every network file system does, FISH+ among them — only sends
// its last chunk from Close, so a copy is not finished until Close has
// succeeded, and a dropped error there leaves a truncated file behind while
// the panel reports success.
func closeOnce(c io.Closer) func() error {
	closed := false
	return func() error {
		if closed {
			return nil
		}
		closed = true
		return c.Close()
	}
}

// canonicalOSPath resolves symlinks in the longest existing prefix and then
// reattaches any not-yet-created path components. filepath.EvalSymlinks on the
// complete destination is insufficient for copy operations because that path
// commonly does not exist yet (and on macOS /var aliases /private/var).
func canonicalOSPath(path string) string {
	current := filepath.Clean(path)
	missing := make([]string, 0, 4)
	for {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return resolved
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(path)
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func recursiveCopy(ctx context.Context, srcVfs vfs.VFS, srcPath string, dstVfs vfs.VFS, destPath string, state *FileOpState, depth int) error {
	if depth > 1000 {
		return fmt.Errorf("maximum recursion depth exceeded (circular structure?)")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	stat, err := srcVfs.Stat(ctx, srcPath)
	if err != nil {
		return err
	}

	absSrc, _ := srcVfs.Abs(srcPath)
	absDst, _ := dstVfs.Abs(destPath)

	realSrc := absSrc
	realDst := absDst

	if _, ok := srcVfs.(*vfs.OSVFS); ok {
		realSrc = canonicalOSPath(absSrc)
	}
	if _, ok := dstVfs.(*vfs.OSVFS); ok {
		realDst = canonicalOSPath(absDst)
	}

	cleanSrc := filepath.ToSlash(filepath.Clean(realSrc))
	cleanDst := filepath.ToSlash(filepath.Clean(realDst))

	if runtime.GOOS == "windows" {
		cleanSrc = strings.ToLower(cleanSrc)
		cleanDst = strings.ToLower(cleanDst)
	}

	if cleanSrc == cleanDst {
		if stat.IsDir {
			return fmt.Errorf("cannot copy folder into itself (source equals destination)")
		}
		return fmt.Errorf("cannot copy file onto itself (source equals destination)")
	}

	prefixSrc := cleanSrc
	if !strings.HasSuffix(prefixSrc, "/") {
		prefixSrc += "/"
	}

	if strings.HasPrefix(cleanDst, prefixSrc) {
		if stat.IsDir {
			return fmt.Errorf("cannot copy folder into itself (destination is a subfolder)")
		}
		return fmt.Errorf("cannot copy file into its own subfolder")
	}

	dstStat, err := dstVfs.Stat(ctx, destPath)
	exists := err == nil

	if stat.IsDir {
		if !exists {
			if err := dstVfs.MkDir(ctx, destPath); err != nil {
				return err
			}
		} else if !dstStat.IsDir {
			return fmt.Errorf("cannot overwrite file with folder: %s", dstVfs.Base(destPath))
		}

		var items []vfs.VFSItem
		err := srcVfs.ReadDir(ctx, srcPath, func(chunk []vfs.VFSItem) {
			items = append(items, chunk...)
		})
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Name == ".." {
				continue
			}
			if err := recursiveCopy(ctx, srcVfs, srcVfs.Join(srcPath, item.Name), dstVfs, dstVfs.Join(destPath, item.Name), state, depth+1); err != nil {
				return err
			}
		}
		itemToSet := stat
		itemToSet.Uid = -1
		itemToSet.Gid = -1
		if itemToSet.UnixMode == 0 {
			itemToSet.UnixMode = 0755
		}
		_ = dstVfs.SetAttributes(ctx, destPath, itemToSet)

		if state.Tracker != nil {
			state.Tracker.DirDone()
			if state.UpdateUI != nil {
				state.UpdateUI(false)
			}
		}
		return nil
	}

	itemName := dstVfs.Base(destPath)
	if state.Tracker != nil {
		displayString := itemName
		if AppConfig.FileOpPathDisplay == 1 {
			displayString = srcPath
		} else if AppConfig.FileOpPathDisplay == 2 {
			displayString = srcPath + " -> " + destPath
		}
		state.Tracker.StartFile(displayString, stat.Size)
		if state.UpdateUI != nil {
			state.UpdateUI(false)
		}
	}

	skipFile := func() {
		state.SkippedCount++
		if state.Tracker != nil {
			state.Tracker.FileSkipped()
			if state.UpdateUI != nil {
				state.UpdateUI(true)
			}
		}
	}

	destPathForFile := destPath

	for {
		dstStat, err := dstVfs.Stat(ctx, destPathForFile)
		exists := err == nil

		if !exists {
			break
		}
		if dstStat.IsDir {
			return fmt.Errorf("cannot overwrite folder with file: %s", dstVfs.Base(destPathForFile))
		}
		if state.SkipAll {
			skipFile()
			return nil
		}
		if state.OverwriteAll {
			break
		}

		choice, remember := AskOverwrite(ctx, destPathForFile, stat, dstStat, state.Anchor)
		if choice == 1 { // Overwrite
			if remember {
				state.OverwriteAll = true
				vtui.DebugLog("FILEOP: User chose OVERWRITE ALL")
			}
			break
		} else if choice == 2 { // Skip
			if remember {
				state.SkipAll = true
				vtui.DebugLog("FILEOP: User chose SKIP ALL")
			}
			skipFile()
			return nil
		} else if choice == 3 { // Rename
			newName := AskRename(ctx, dstVfs.Base(destPathForFile), state.Anchor)
			if newName == "" {
				return context.Canceled
			}
			destPathForFile = dstVfs.Join(dstVfs.Dir(destPathForFile), newName)
			continue
		} else if choice == 4 || choice == 5 { // Append, Resume
			resultChan := make(chan int, 1)
			vtui.FrameManager.PostTask(func() {
				errDlg := vtui.ShowMessage(" Unsupported ", "Append/Resume not supported by current VFS implementation.", []string{"&Ok"})
				errDlg.OnResult = func(c int) { resultChan <- c }
			})
			<-resultChan
			continue
		} else { // Cancel
			return context.Canceled
		}
	}

	// Optimize using server-side copy if both VFS share the same session/connection
	if ssc, ok := dstVfs.(vfs.ServerSideCopier); ok && vfs.SameSession(srcVfs, dstVfs) {
		err := ssc.Copy(ctx, srcPath, destPathForFile)
		if err == nil {
			if state.Tracker != nil {
				state.Tracker.UpdateBytes(int(stat.Size))
				handleArchiveIndexOp(srcVfs, srcPath, dstVfs, destPathForFile, state.IsMove)
				state.Tracker.FileDone()
				if state.UpdateUI != nil {
					state.UpdateUI(false)
				}
			}
			return nil
		}
		vtui.DebugLog("FILEOP: Server-side copy failed, falling back to streaming: %v", err)
	}

	// Optimize using server-to-server direct copy if they are on different hosts,
	// but we can run commands on one of them and have connection info for the other.
	if state.S2SDir != 3 { // Not disabled
		pushed, pulled := false, false

		tryPush := state.S2SDir == 0 || state.S2SDir == 1
		tryPull := state.S2SDir == 0 || state.S2SDir == 2

		if tryPush {
			if rner, ok1 := srcVfs.(vfs.CommandRunner); ok1 {
				if cip, ok2 := dstVfs.(vfs.ConnectionInfoProvider); ok2 {
					if host, port, user, ok := cip.ConnectionInfo(); ok {
						var scpDst string
						if user != "" {
							scpDst = fmt.Sprintf("%s@%s:%q", user, host, destPathForFile)
						} else {
							scpDst = fmt.Sprintf("%s:%q", host, destPathForFile)
						}

						scpCmd := fmt.Sprintf("scp -o ConnectTimeout=10 -P %s -o StrictHostKeyChecking=no -p %q %s",
							port, srcPath, scpDst)
						vtui.DebugLog("FILEOP: Attempting server-to-server push: %s", scpCmd)
						codePush, errPush := rner.RunCommand(ctx, srcVfs.Dir(srcPath), scpCmd, nil)
						if errPush == nil && codePush == 0 {
							pushed = true
							state.S2SDir = 1
						} else {
							vtui.DebugLog("FILEOP: Server-to-server push failed (code: %d): %v", codePush, errPush)
						}
					}
				}
			}
		}

		if !pushed && tryPull {
			if rner, ok1 := dstVfs.(vfs.CommandRunner); ok1 {
				if cip, ok2 := srcVfs.(vfs.ConnectionInfoProvider); ok2 {
					if host, port, user, ok := cip.ConnectionInfo(); ok {
						var scpSrc string
						if user != "" {
							scpSrc = fmt.Sprintf("%s@%s:%q", user, host, srcPath)
						} else {
							scpSrc = fmt.Sprintf("%s:%q", host, srcPath)
						}

						scpCmd := fmt.Sprintf("scp -o ConnectTimeout=10 -P %s -o StrictHostKeyChecking=no -p %s %q",
							port, scpSrc, destPathForFile)
						vtui.DebugLog("FILEOP: Attempting server-to-server pull: %s", scpCmd)
						codePull, errPull := rner.RunCommand(ctx, dstVfs.Dir(destPathForFile), scpCmd, nil)
						if errPull == nil && codePull == 0 {
							pulled = true
							state.S2SDir = 2
						} else {
							vtui.DebugLog("FILEOP: Server-to-server pull failed (code: %d): %v", codePull, errPull)
						}
					}
				}
			}
		}

		if pushed || pulled {
			if state.Tracker != nil {
				state.Tracker.UpdateBytes(int(stat.Size))
				handleArchiveIndexOp(srcVfs, srcPath, dstVfs, destPathForFile, state.IsMove)
				state.Tracker.FileDone()
				if state.UpdateUI != nil {
					state.UpdateUI(false)
				}
			}
			return nil
		} else if state.S2SDir == 0 {
			// If both probed and failed (or couldn't even probe), disable S2S for this operation
			state.S2SDir = 3
			vtui.DebugLog("FILEOP: Server-to-server copy disabled after probing failed or unavailable")
		}
	}

	var srcFile vfs.ReadAtCloser
	for {
		srcFile, err = srcVfs.Open(ctx, srcPath)
		if err == nil {
			break
		}
		choice := AskError(ctx, "Cannot open source file", err, state.Anchor)
		if choice == 1 {
			skipFile()
			return nil
		}
		if choice == 2 {
			return context.Canceled
		}
	}
	defer srcFile.Close()

	var dstFile io.WriteCloser
	for {
		dstFile, err = dstVfs.Create(ctx, destPathForFile)
		if err == nil {
			break
		}
		choice := AskError(ctx, "Cannot create destination file", err, state.Anchor)
		if choice == 1 {
			skipFile()
			return nil
		}
		if choice == 2 {
			return context.Canceled
		}
	}

	closeDst := closeOnce(dstFile)
	copySuccess := false
	defer func() {
		closeDst()
		if !copySuccess {
			dstVfs.Remove(context.Background(), destPathForFile)
		}
	}()

	buf := state.Buffer
	if buf == nil {
		buf = make([]byte, 128*1024)
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, rerr := srcFile.Read(ctx, buf)
		if n > 0 {
			if _, werr := dstFile.Write(buf[:n]); werr != nil {
				return werr
			}
			if state.OnBytes != nil {
				state.OnBytes(n)
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			return rerr
		}
	}

	if cerr := closeDst(); cerr != nil {
		return cerr
	}
	copySuccess = true

	if copySuccess {
		itemToSet := stat
		itemToSet.Uid = -1
		itemToSet.Gid = -1
		if itemToSet.UnixMode == 0 {
			itemToSet.UnixMode = 0644
		}
		_ = dstVfs.SetAttributes(ctx, destPathForFile, itemToSet)
	}

	if state.Tracker != nil {
		if copySuccess {
			handleArchiveIndexOp(srcVfs, srcPath, dstVfs, destPathForFile, state.IsMove)
		}
		state.Tracker.FileDone()
		if state.UpdateUI != nil {
			state.UpdateUI(false)
		}
	}
	return nil
}

// AskOverwrite shows a rich modal dialog for file conflicts.
func AskOverwrite(ctx context.Context, destPath string, srcStat, dstStat vfs.VFSItem, anchor vtui.Frame) (int, bool) {
	resultChan := make(chan int, 1)
	rememberChan := make(chan bool, 1)
	var dlg *vtui.Window

	vtui.FrameManager.PostTask(func() {
		if ctx.Err() != nil {
			return
		}

		width := 76
		height := 13
		dlg = vtui.NewCenteredDialog(width, height, " Warning ")

		lbl1 := vtui.NewLabel(0, 0, "File already exists", nil)
		truncPath := vtui.TruncateMiddle(destPath, width-6)
		lbl2 := vtui.NewLabel(0, 0, truncPath, nil)

		sep1 := vtui.NewSeparator(0, 0, width, true, true)

		formatInfo := func(label string, stat vfs.VFSItem) string {
			dateStr := stat.MTime.Format("02.01.2006 15:04:05")
			return fmt.Sprintf("%-10s %15d  %s", label, stat.Size, dateStr)
		}

		lblNew := vtui.NewLabel(0, 0, formatInfo("New", srcStat), nil)
		lblExist := vtui.NewLabel(0, 0, formatInfo("Existing", dstStat), nil)

		sep2 := vtui.NewSeparator(0, 0, width, true, true)

		chkRem := vtui.NewCheckbox(0, 0, "Reme&mber choice", false)

		sep3 := vtui.NewSeparator(0, 0, width, true, true)

		btnOver := vtui.NewButton(0, 0, "&Overwrite")
		btnOver.IsDefault = true
		btnSkip := vtui.NewButton(0, 0, "&Skip")
		btnRen := vtui.NewButton(0, 0, "&Rename")
		btnApp := vtui.NewButton(0, 0, "&Append")
		btnRes := vtui.NewButton(0, 0, "Res&ume")
		btnCan := vtui.NewButton(0, 0, "&Cancel")

		dlg.AddItem(lbl1)
		dlg.AddItem(lbl2)
		dlg.AddItem(sep1)
		dlg.AddItem(lblNew)
		dlg.AddItem(lblExist)
		dlg.AddItem(sep2)
		dlg.AddItem(chkRem)
		dlg.AddItem(sep3)
		dlg.AddItem(btnOver)
		dlg.AddItem(btnSkip)
		dlg.AddItem(btnRen)
		dlg.AddItem(btnApp)
		dlg.AddItem(btnRes)
		dlg.AddItem(btnCan)

		vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
		vbox.Add(lbl1, vtui.Margins{}, vtui.AlignCenter)
		vbox.Add(lbl2, vtui.Margins{}, vtui.AlignCenter)
		vbox.Add(sep1, vtui.Margins{Left: -2, Right: -2}, vtui.AlignFill)
		vbox.Add(lblNew, vtui.Margins{}, vtui.AlignLeft)
		vbox.Add(lblExist, vtui.Margins{}, vtui.AlignLeft)
		vbox.Add(sep2, vtui.Margins{Left: -2, Right: -2}, vtui.AlignFill)
		vbox.Add(chkRem, vtui.Margins{}, vtui.AlignLeft)
		vbox.Add(sep3, vtui.Margins{Left: -2, Right: -2}, vtui.AlignFill)

		hbox := vtui.NewHBoxLayout(0, 0, width-4, 1)
		hbox.HorizontalAlign = vtui.AlignCenter
		hbox.Spacing = 1
		hbox.Add(btnOver, vtui.Margins{}, vtui.AlignTop)
		hbox.Add(btnSkip, vtui.Margins{}, vtui.AlignTop)
		hbox.Add(btnRen, vtui.Margins{}, vtui.AlignTop)
		hbox.Add(btnApp, vtui.Margins{}, vtui.AlignTop)
		hbox.Add(btnRes, vtui.Margins{}, vtui.AlignTop)
		hbox.Add(btnCan, vtui.Margins{}, vtui.AlignTop)

		vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
		vbox.Apply()

		btnOver.OnClick = func() { resultChan <- 1; rememberChan <- (chkRem.State == 1); dlg.Close() }
		btnSkip.OnClick = func() { resultChan <- 2; rememberChan <- (chkRem.State == 1); dlg.Close() }
		btnRen.OnClick = func() { resultChan <- 3; rememberChan <- (chkRem.State == 1); dlg.Close() }
		btnApp.OnClick = func() { resultChan <- 4; rememberChan <- (chkRem.State == 1); dlg.Close() }
		btnRes.OnClick = func() { resultChan <- 5; rememberChan <- (chkRem.State == 1); dlg.Close() }
		btnCan.OnClick = func() { resultChan <- 6; rememberChan <- false; dlg.Close() }

		dlg.OnResult = func(code int) {
			if code < 0 {
				select {
				case resultChan <- 6:
					rememberChan <- false
				default:
				}
			}
		}
		if anchor != nil {
			vtui.FrameManager.PushToFrameScreen(anchor, dlg)
		} else {
			vtui.FrameManager.Push(dlg)
		}
	})

	select {
	case res := <-resultChan:
		rem := <-rememberChan
		return res, rem
	case <-ctx.Done():
		vtui.FrameManager.PostTask(func() {
			if dlg != nil && !dlg.IsDone() {
				dlg.Close()
			}
		})
		return 6, false
	}
}

// askDeleteError handles delete errors with Retry/Skip/Skip All/Abort options.
func askDeleteError(ctx context.Context, op string, err error, anchor vtui.Frame) int {
	resultChan := make(chan int, 1)
	var dlg *vtui.Window

	vtui.FrameManager.PostTask(func() {
		if ctx.Err() != nil {
			return
		}
		msg := fmt.Sprintf("%s:\n%s\n\n%s", op, err.Error(), "What to do?")
		if anchor != nil {
			dlg = vtui.ShowMessageOn(anchor, " Error ", msg,
				[]string{Msg("Btn.Retry"), "&Skip", Msg("Btn.SkipAll"), "&Abort"})
		} else {
			dlg = vtui.ShowMessage(" Error ", msg,
				[]string{Msg("Btn.Retry"), "&Skip", Msg("Btn.SkipAll"), "&Abort"})
		}
		dlg.OnResult = func(code int) {
			if code < 0 {
				code = 3
			}
			select {
			case resultChan <- code:
			default:
			}
		}
	})

	select {
	case res := <-resultChan:
		return res
	case <-ctx.Done():
		vtui.FrameManager.PostTask(func() {
			if dlg != nil && !dlg.IsDone() {
				dlg.Close()
			}
		})
		return 3
	}
}

func AskRename(ctx context.Context, oldName string, anchor vtui.Frame) string {
	resultChan := make(chan string, 1)
	var dlg *vtui.Window
	vtui.FrameManager.PostTask(func() {
		if ctx.Err() != nil {
			return
		}
		dlg = vtui.InputBoxOn(anchor, " Rename ", "New name:", oldName, func(s string) {
			select {
			case resultChan <- s:
			default:
			}
		})
		dlg.OnResult = func(code int) {
			if code < 0 {
				select {
				case resultChan <- "":
				default:
				}
			}
		}
	})
	select {
	case res := <-resultChan:
		return res
	case <-ctx.Done():
		vtui.FrameManager.PostTask(func() {
			if dlg != nil && !dlg.IsDone() {
				dlg.Close()
			}
		})
		return ""
	}
}

// AskError handles I/O errors by asking user for Retry/Skip/Abort
func AskError(ctx context.Context, op string, err error, anchor vtui.Frame) int {
	resultChan := make(chan int, 1)
	var dlg *vtui.Window

	vtui.FrameManager.PostTask(func() {
		if ctx.Err() != nil {
			return
		}
		msg := fmt.Sprintf("%s:\n%s\n\n%s", op, err.Error(), "What to do?")
		if anchor != nil {
			dlg = vtui.ShowMessageOn(anchor, " Error ", msg, []string{Msg("Btn.Retry"), "&Skip", "&Abort"})
		} else {
			dlg = vtui.ShowMessage(" Error ", msg, []string{Msg("Btn.Retry"), "&Skip", "&Abort"})
		}
		dlg.OnResult = func(code int) {
			if code < 0 {
				code = 2
			}
			select {
			case resultChan <- code:
			default:
			}
		}
	})

	select {
	case res := <-resultChan:
		return res
	case <-ctx.Done():
		vtui.FrameManager.PostTask(func() {
			if dlg != nil && !dlg.IsDone() {
				dlg.Close()
			}
		})
		return 2 // 2 matches Abort button index
	}
}
