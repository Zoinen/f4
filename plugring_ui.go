package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtui"
)

type plugRingRow struct {
	item   PlugRingItem
	status string
}

func (r plugRingRow) GetCellText(col int) string {
	switch col {
	case 0:
		return r.item.Name
	case 1:
		return r.item.Version
	case 2:
		return r.status
	case 3:
		return r.item.Author
	case 4:
		return runewidth.Truncate(r.item.Description, 40, "...")
	}
	return ""
}

func (r plugRingRow) GetCellAttr(col int, def uint64) uint64 {
	if r.status == "Update" {
		return vtui.SetRGBFore(def, 0xFCE94F) // Yellow
	} else if r.status == "Installed" {
		return vtui.SetRGBFore(def, 0x8AE234) // Green
	}
	return def
}

func actionPlugRing(pf *PanelsFrame) {
	w, h := 76, 22
	dlg := vtui.NewCenteredDialog(w, h, " f4 PlugRing ")
	dlg.ShowClose = true

	table := vtui.NewTable(0, 0, w-4, h-5, []vtui.TableColumn{
		{Title: "Name", Width: 16},
		{Title: "Version", Width: 8},
		{Title: "Status", Width: 13},
		{Title: "Author", Width: 10},
		{Title: "Description", Width: w - 4 - 16 - 8 - 13 - 10 - 5}, // 5 is for borders and scrollbar
	})
	table.ShowScrollBar = true

	btnInstall := vtui.NewButton(0, 0, "&Install/Update")
	btnRemove := vtui.NewButton(0, 0, "&Remove")
	btnRefresh := vtui.NewButton(0, 0, "Re&fresh")
	btnClose := vtui.NewButton(0, 0, "&Close")

	dlg.AddItem(table)
	dlg.AddItem(btnInstall)
	dlg.AddItem(btnRemove)
	dlg.AddItem(btnRefresh)
	dlg.AddItem(btnClose)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, w-4, h-4)
	vbox.Add(table, vtui.Margins{Bottom: 1}, vtui.AlignFill)

	hbox := vtui.NewHBoxLayout(0, 0, w-4, 1)
	hbox.HorizontalAlign = vtui.AlignCenter
	hbox.Spacing = 2
	hbox.Add(btnInstall, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnRemove, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnRefresh, vtui.Margins{}, vtui.AlignTop)
	hbox.Add(btnClose, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(hbox, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	btnClose.OnClick = func() { dlg.Close() }

	var items []PlugRingItem

	refresh := func() {
		table.SetRows(nil)
		vtui.FrameManager.Redraw()

		vtui.RunAsync(func(ctx *vtui.TaskContext) {
			fetched, err := FetchCatalog(ctx.Context)
			ctx.RunOnUI(func() {
				if err != nil {
					vtui.ShowMessageOn(dlg, " Error ", fmt.Sprintf("Failed to fetch catalog:\n%v", err), []string{"&Ok"})
					return
				}
				items = fetched
				installed := GetInstalledPlugRingItems()
				rows := make([]vtui.TableRow, len(items))
				for i, itm := range items {
					status := "Not installed"
					if inst, ok := installed[itm.ID]; ok {
						if inst.Version != itm.Version {
							status = "Update"
						} else {
							status = "Installed"
						}
					}
					rows[i] = plugRingRow{item: itm, status: status}
				}
				table.SetRows(rows)
				vtui.FrameManager.Redraw()
			})
		})
	}

	btnRefresh.OnClick = refresh

	btnInstall.OnClick = func() {
		idx := table.SelectPos
		if idx >= 0 && idx < len(items) {
			actionInstallPlugRingItem(pf, dlg, items[idx], refresh)
		}
	}
	btnRemove.OnClick = func() {
		idx := table.SelectPos
		if idx >= 0 && idx < len(items) {
			actionRemovePlugRingItem(pf, dlg, items[idx], refresh)
		}
	}

	table.OnAction = func(idx int) {
		btnInstall.OnClick()
	}

	vtui.FrameManager.Push(dlg)
	refresh()
}

func actionInstallPlugRingItem(pf *PanelsFrame, parent *vtui.Window, item PlugRingItem, refresh func()) {
	// 1. Implicit dependency check from Entrypoint interpreter
	parts := strings.Fields(item.Entrypoint)
	if len(parts) > 0 {
		interpreter := parts[0]
		if !strings.ContainsAny(interpreter, "/\\") && !strings.HasPrefix(interpreter, ".") {
			if _, err := exec.LookPath(interpreter); err != nil {
				msg := fmt.Sprintf("Warning: This plugin requires '%s' to run, but it was not found in your system's PATH.\n\nPlease install '%s' first, or the plugin might fail to load.", interpreter, interpreter)
				if pf.Message(" Missing Dependency ", msg, []string{"&Install Anyway", "Cancel"}) != 0 {
					return
				}
			}
		}
	}

	// 2. Explicit dependencies check
	for _, dep := range item.Dependencies {
		if _, err := exec.LookPath(dep); err != nil {
			msg := fmt.Sprintf("Warning: This plugin requires '%s', but it was not found in your system's PATH.\n\nPlease install it, or the plugin might fail to load.", dep)
			if pf.Message(" Missing Dependency ", msg, []string{"&Install Anyway", "Cancel"}) != 0 {
				return
			}
		}
	}

	url := ResolveAssetURL(item.URL)
	isTarGz := strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz")
	isArchive := isTarGz || strings.HasSuffix(url, ".zip")

	plugringDir := filepath.Join(GetF4ConfigDir(), "plugring")
	pluginDir := filepath.Join(plugringDir, item.ID)

	pf.RunProgressTask(" Installing Plugin ", "Downloading "+item.Name+"...", false, func(ctx context.Context, update func(msg string, percent int)) error {
		var archiveBytes []byte
		if strings.HasPrefix(url, "file://") {
			localPath := strings.TrimPrefix(url, "file://")
			data, err := os.ReadFile(localPath)
			if err != nil {
				return fmt.Errorf("failed to read local test file %s: %w", localPath, err)
			}
			archiveBytes = data
		} else {
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return err
			}
			req.Header.Set("User-Agent", "f4-plugring")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				return fmt.Errorf("download failed with status %d", resp.StatusCode)
			}

			contentLength := resp.ContentLength
			var archiveData bytes.Buffer
			buf := make([]byte, 32*1024)
			var downloaded int64

			for {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				n, readErr := resp.Body.Read(buf)
				if n > 0 {
					archiveData.Write(buf[:n])
					downloaded += int64(n)
					pct := 0
					if contentLength > 0 {
						pct = int((downloaded * 100) / contentLength)
					}
					update("Downloading...", pct)
				}
				if readErr != nil {
					if readErr == io.EOF {
						break
					}
					return readErr
				}
			}
			archiveBytes = archiveData.Bytes()
		}

		update("Extracting files...", -1)

		os.RemoveAll(pluginDir)
		os.MkdirAll(pluginDir, 0755)

		if isArchive {
			var err error
			if isTarGz {
				err = extractTarGzToDir(archiveBytes, pluginDir)
			} else {
				err = extractZipToDir(archiveBytes, pluginDir)
			}
			if err != nil {
				os.RemoveAll(pluginDir)
				return fmt.Errorf("failed to extract archive: %w", err)
			}
		} else {
			filename := filepath.Base(url)
			err := os.WriteFile(filepath.Join(pluginDir, filename), archiveBytes, 0755)
			if err != nil {
				os.RemoveAll(pluginDir)
				return fmt.Errorf("failed to save plugin file: %w", err)
			}
		}

		if item.SetupCmd != "" {
			update("Running setup commands...", -1)
			var cmd *exec.Cmd
			if runtime.GOOS == "windows" {
				cmd = exec.CommandContext(ctx, "cmd.exe", "/c", item.SetupCmd)
			} else {
				cmd = exec.CommandContext(ctx, "sh", "-c", item.SetupCmd)
			}
			cmd.Dir = pluginDir
			out, cmdErr := cmd.CombinedOutput()
			if cmdErr != nil {
				os.RemoveAll(pluginDir)
				return fmt.Errorf("setup command failed: %v\nOutput: %s", cmdErr, string(out))
			}
		}

		manifestData, _ := json.MarshalIndent(item, "", "  ")
		os.WriteFile(filepath.Join(pluginDir, "manifest.json"), manifestData, 0644)

		return nil
	}, func(err error) {
		if err != nil {
			if err != context.Canceled {
				vtui.ShowMessageOn(parent, " Error ", fmt.Sprintf("Installation failed:\n%v", err), []string{"&Ok"})
			}
		} else {
			if GlobalPluginManager != nil {
				GlobalPluginManager.loadSinglePlugRingItem(item)
			}
			vtui.ShowMessageOn(parent, " Success ", "Plugin installed and loaded successfully!", []string{"&Ok"})
			refresh()
		}
	})
}

func actionRemovePlugRingItem(pf *PanelsFrame, parent *vtui.Window, item PlugRingItem, refresh func()) {
	plugringDir := filepath.Join(GetF4ConfigDir(), "plugring")
	pluginDir := filepath.Join(plugringDir, item.ID)

	if _, err := os.Stat(pluginDir); os.IsNotExist(err) {
		vtui.ShowMessageOn(parent, " Info ", "Plugin is not installed.", []string{"&Ok"})
		return
	}

	dlg := vtui.ShowMessageOn(parent, " Remove Plugin ", fmt.Sprintf("Do you want to completely remove %s?", item.Name), []string{"&Remove", "Cancel"})
	dlg.OnResult = func(code int) {
		if code == 0 {
			err := os.RemoveAll(pluginDir)
			if err != nil {
				vtui.ShowMessageOn(parent, " Error ", fmt.Sprintf("Removal failed:\n%v", err), []string{"&Ok"})
			} else {
				vtui.ShowMessageOn(parent, " Success ", "Plugin removed successfully.\nRestart f4 to fully unload.", []string{"&Ok"})
				refresh()
			}
		}
	}
}
