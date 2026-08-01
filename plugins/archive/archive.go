package archive

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/zipper/archive"
)

var activeOps sync.Map

type ArchivePlugin struct{}

func (p *ArchivePlugin) Init(api vfs.HostAPI) error {
	api.RegisterVFSProvider(&ArchiveProvider{})

	api.RegisterGlobalHotkey(vtinput.VK_F1, vtinput.ShiftPressed, func(app vfs.App) {
		actionArchiveCommands(app)
	})

	return nil
}

type ioCtxReader struct {
	r   io.Reader
	ctx context.Context
}

func (cr *ioCtxReader) Read(p []byte) (int, error) {
	if cr.ctx.Err() != nil {
		return 0, cr.ctx.Err()
	}
	return cr.r.Read(p)
}
func actionArchiveCommands(app vfs.App) {
	app.Menu(" Archive Commands ", []string{"&1. Add to archive", "&2. Extract files"}, func(idx int) {
		switch idx {
		case 0:
			actionAddArchive(app)
		case 1:
			actionExtractArchive(app)
		}
	})
}

func actionExtractArchive(app vfs.App) {
	srcVfs := app.GetActivePanelVFS()
	dstVfs := app.GetPassivePanelVFS()
	if srcVfs == nil || dstVfs == nil {
		return
	}

	name := app.GetSelectedName()
	if name == "" || name == ".." {
		return
	}

	srcPath := srcVfs.Join(srcVfs.GetPath(), name)
	destDir := dstVfs.GetPath()

	if osvfs, ok := srcVfs.(*vfs.OSVFS); ok {
		srcPath, _ = osvfs.Abs(srcPath)
	} else {
		app.Message(" Error ", "Extraction supported only from local filesystem", []string{"&Ok"})
		return
	}

	isBusy := false
	if _, active := activeOps.Load(srcPath); active {
		isBusy = true
	} else if !vfs.GlobalArchiveLockManager.TryLock(srcPath) {
		isBusy = true
	} else {
		// TryLock succeeded, meaning it was NOT busy. We must unlock it here
		// so that the background worker can safely Lock() it later.
		vfs.GlobalArchiveLockManager.Unlock(srcPath)
	}

	waitLock := true
	if isBusy {
		res := app.Message(" Archive Busy ", "This archive is currently being processed.\nRunning multiple operations simultaneously may severely degrade performance.", []string{"&Queue", "&Parallel", "&Cancel"})
		if res == 2 || res < 0 {
			return
		}
		waitLock = (res == 0)
	}

	app.RunAdvancedProgressTask(" Extracting... ", false, func(ctx context.Context, reporter vfs.TaskReporter) error {
		if waitLock {
			reporter.UpdateTransfer("Waiting", "in queue...", -1, "", -1, "")
			vfs.GlobalArchiveLockManager.Lock(srcPath)
			defer vfs.GlobalArchiveLockManager.Unlock(srcPath)
		}
		reporter.UpdateTransfer("Extracting", "files...", -1, "", -1, "")

		ex, err := archive.NewExtractor(srcPath, destDir, archive.Options{Xattrs: false, SafeWrites: true})
		if err != nil {
			return err
		}
		defer ex.Close()

		done := make(chan struct{})
		defer close(done)
		startTime := time.Now()

		showProgress := func() {
			bytes, entries := ex.Written()
			elapsed := time.Since(startTime)
			speed := float64(0)
			if elapsed.Seconds() > 0 {
				speed = float64(bytes) / elapsed.Seconds()
			}
			speedStr := formatSize(int64(speed)) + "/s"

			elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)

			// Нам также нужно поправить и второе вхождение в actionAddArchive:
			timeSpeedText := fmt.Sprintf("%-16s %-21s %15s", elapsedStr, "", speedStr)

			totalText := fmt.Sprintf("Total: %s", formatSize(bytes))

			currFile := fmt.Sprintf("%d files", entries)
			if fp, ok := ex.(interface{ CurrentFile() string }); ok {
				if name := fp.CurrentFile(); name != "" {
					currFile = name
				}
			}

			reporter.UpdateTransfer("Extracting", currFile, -1, totalText, -1, timeSpeedText)
		}
		showProgress()

		go func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case <-ticker.C:
					showProgress()
				}
			}
		}()

		return ex.Extract(ctx)

	}, func(err error) {
		if err != nil && err != context.Canceled {
			go app.Message(" Error ", fmt.Sprintf("Extraction failed:\n%v", err), []string{"&Ok"})
		}
		app.RefreshAll()
	})
}

func actionAddArchive(app vfs.App) {
	activeVfs := app.GetActivePanelVFS()
	if activeVfs == nil {
		return
	}

	names := app.GetSelectedNames()
	if len(names) == 0 {
		return
	}

	arcName := activeVfs.Base(activeVfs.GetPath())
	if arcName == "." || arcName == "" {
		arcName = "archive"
	}
	arcName += ".zip"

	app.InputBox(" Add to archive ", "Archive name:", arcName, func(name string) {
		if name == "" {
			return
		}
		fullArcPath := activeVfs.Join(activeVfs.GetPath(), name)

		go func() {
			var absArcPath string
			if osvfs, ok := activeVfs.(*vfs.OSVFS); ok {
				absArcPath, _ = osvfs.Abs(fullArcPath)
			} else {
				absArcPath = fullArcPath
			}

			isBusy := false
			if _, active := activeOps.Load(absArcPath); active {
				isBusy = true
			} else if !vfs.GlobalArchiveLockManager.TryLock(absArcPath) {
				isBusy = true
			} else {
				vfs.GlobalArchiveLockManager.Unlock(absArcPath)
			}

			waitLock := true
			if isBusy {
				res := app.Message(" Archive Busy ", "This archive is currently being processed.\nRunning multiple operations simultaneously may severely degrade performance.", []string{"&Queue", "&Parallel", "&Cancel"})
				if res == 2 || res < 0 {
					return
				}
				waitLock = (res == 0)
			}

			if _, err := activeVfs.Stat(context.Background(), fullArcPath); err == nil {
				msg := "The target archive already exists.\nDo you want to overwrite it?"
				if app.Message(" Warning ", msg, []string{"&Yes", "&No"}) != 0 {
					return
				}
			}

			app.RunAdvancedProgressTask(" Archiving... ", false, func(ctx context.Context, reporter vfs.TaskReporter) error {
				if waitLock {
					reporter.UpdateTransfer("Waiting", "in queue...", -1, "", -1, "")
					vfs.GlobalArchiveLockManager.Lock(absArcPath)
					defer vfs.GlobalArchiveLockManager.Unlock(absArcPath)
				}
				reporter.UpdateTransfer("Archiving", "files...", -1, "", -1, "")

				fileMap := make(map[string]os.FileInfo)
				var totalBytes int64
				for _, n := range names {
					if ctx.Err() != nil {
						return ctx.Err()
					}
					reporter.UpdateScan(n, int64(len(fileMap)), 0)

					fullPath := activeVfs.Join(activeVfs.GetPath(), n)
					if osvfs, ok := activeVfs.(*vfs.OSVFS); ok {
						absPath, _ := osvfs.Abs(fullPath)
						filepath.Walk(absPath, func(p string, fi os.FileInfo, e error) error {
							if e == nil {
								fileMap[p] = fi
								if !fi.IsDir() {
									totalBytes += fi.Size()
								}
							}
							return nil
						})
					}
				}

				a, err := archive.NewArchiver(fullArcPath, activeVfs.GetPath(), archive.Options{Xattrs: false})
				if err != nil {
					return err
				}
				defer a.Close()

				done := make(chan struct{})
				defer close(done)
				startTime := time.Now()

				showProgress := func() {
					bytes, entries := a.Written()
					elapsed := time.Since(startTime)
					speed := float64(0)
					if elapsed.Seconds() > 0 {
						speed = float64(bytes) / elapsed.Seconds()
					}

					pct := -1
					if totalBytes > 0 {
						pct = int((bytes * 100) / totalBytes)
					}
					if pct > 100 {
						pct = 100
					}

					speedStr := formatSize(int64(speed)) + "/s"

					etaStr := "Remaining: ??:??:??"
					if totalBytes > 0 && bytes > 0 && elapsed.Seconds() > 0.5 {
						ratio := float64(bytes) / float64(totalBytes)
						etaSecs := (elapsed.Seconds() / ratio) - elapsed.Seconds()
						if etaSecs >= 0 && etaSecs < 3600*100 {
							etaDur := time.Duration(etaSecs * float64(time.Second))
							etaStr = fmt.Sprintf("Remaining: %02d:%02d:%02d", int(etaDur.Hours()), int(etaDur.Minutes())%60, int(etaDur.Seconds())%60)
						}
					}

					elapsedStr := fmt.Sprintf("Time: %02d:%02d:%02d", int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
					timeSpeedText := fmt.Sprintf("%-16s %-21s %15s", elapsedStr, etaStr, speedStr)

					totalText := fmt.Sprintf("Total: %s / %s", formatSize(bytes), formatSize(totalBytes))

					reporter.UpdateTransfer("Archiving", fmt.Sprintf("%d files", entries), -1, totalText, pct, timeSpeedText)
				}
				showProgress()

				go func() {
					ticker := time.NewTicker(100 * time.Millisecond)
					defer ticker.Stop()
					for {
						select {
						case <-ctx.Done():
							return
						case <-done:
							return
						case <-ticker.C:
							showProgress()
						}
					}
				}()

				return a.Archive(ctx, fileMap)
			}, func(err error) {
				if err != nil && err != context.Canceled {
					go app.Message(" Error ", fmt.Sprintf("Archiving failed:\n%v", err), []string{"&Ok"})
				}
				if err == nil {
					app.SetPendingSelection(name)
				}
				app.RefreshAll()
			})
		}()
	})
}

func (p *ArchivePlugin) Close() error    { return nil }
func (p *ArchivePlugin) GetName() string { return "Archive Support" }
