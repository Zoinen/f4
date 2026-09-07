//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type macOSLocationRow struct {
	ID        string
	Section   string
	Kind      string
	Label     string
	Icon      string
	Color     string
	Path      string
	URI       string
	QueryKind string
	Tag       string
	Action    string
	Order     int
}

var macOSLocationsCache struct {
	sync.RWMutex
	rows []macOSLocationRow
}

func cachedMacOSLocations() []macOSLocationRow {
	macOSLocationsCache.RLock()
	rows := append([]macOSLocationRow(nil), macOSLocationsCache.rows...)
	macOSLocationsCache.RUnlock()
	return rows
}

func storeMacOSLocations(rows []macOSLocationRow) {
	macOSLocationsCache.Lock()
	macOSLocationsCache.rows = append([]macOSLocationRow(nil), rows...)
	macOSLocationsCache.Unlock()
}

func decodeMacOSLocationRow(raw map[string]any) (macOSLocationRow, bool) {
	row := macOSLocationRow{
		ID:        platformAnyString(raw["id"]),
		Section:   platformAnyString(raw["section"]),
		Kind:      platformAnyString(raw["kind"]),
		Label:     platformAnyString(raw["label"]),
		Icon:      platformAnyString(raw["icon"]),
		Color:     platformAnyString(raw["color"]),
		Path:      platformAnyString(raw["path"]),
		URI:       platformAnyString(raw["uri"]),
		QueryKind: platformAnyString(raw["queryKind"]),
		Tag:       platformAnyString(raw["tag"]),
		Action:    platformAnyString(raw["action"]),
		Order:     int(platformInt64(raw["order"])),
	}
	return row, row.ID != "" && row.Label != ""
}

func macOSLocationsMenuItem(pf *PanelsFrame, panelIdx int) (vtui.MenuItem, bool) {
	return vtui.MenuItem{
		ID:   "macos-locations",
		Text: "macOS &Locations",
		Icon: "folder",
		Submenu: func() *vtui.VMenu {
			return newMacOSLocationsMenu(pf, panelIdx)
		},
	}, true
}

type macOSLocationsMenuState struct {
	mu         sync.Mutex
	generation uint64
	cancel     context.CancelFunc
}

func newMacOSLocationsMenu(pf *PanelsFrame, panelIdx int) *vtui.VMenu {
	menu := vtui.NewVMenu(Msg("MacOSLocations.Title"))
	menu.SetId(fmt.Sprintf("macos-locations-%d", panelIdx))
	state := &macOSLocationsMenuState{}
	rows := cachedMacOSLocations()
	if len(rows) == 0 {
		menu.ReplaceItems([]vtui.MenuItem{{
			ID: "loading", Text: "Loading macOS locations…",
			Icon: "loader-circle", Disabled: true,
		}})
	} else {
		menu.ReplaceItems(macOSLocationMenuItems(rows, pf, panelIdx))
	}
	menu.OnClose = func() {
		state.mu.Lock()
		if state.cancel != nil {
			state.cancel()
			state.cancel = nil
		}
		state.generation++
		state.mu.Unlock()
	}
	refreshMacOSLocationsMenu(menu, state, pf, panelIdx, len(rows) == 0)
	return menu
}

func refreshMacOSLocationsMenu(
	menu *vtui.VMenu, state *macOSLocationsMenuState,
	pf *PanelsFrame, panelIdx int, showLoading bool,
) {
	client := currentPlatformIPC()
	if client == nil || !client.Available() {
		items := macOSLocationMenuItems(cachedMacOSLocations(), pf, panelIdx)
		items = append(items, macOSLocationsRetryItem(menu, state, pf, panelIdx,
			errPlatformServicesUnavailable))
		menu.ReplaceItems(items)
		return
	}
	state.mu.Lock()
	if state.cancel != nil {
		state.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	state.generation++
	generation := state.generation
	state.mu.Unlock()
	if showLoading {
		menu.ReplaceItems([]vtui.MenuItem{{
			ID: "loading", Text: "Loading macOS locations…",
			Icon: "loader-circle", Disabled: true,
		}})
	}

	frames := vtui.FrameManager
	go func() {
		var rows []macOSLocationRow
		err := client.Request(ctx, "macos.locations", nil, func(response platformResponse) error {
			for _, raw := range platformItems(response.Payload["items"]) {
				if row, ok := decodeMacOSLocationRow(raw); ok {
					rows = append(rows, row)
				}
			}
			snapshot := append([]macOSLocationRow(nil), rows...)
			if len(snapshot) != 0 {
				storeMacOSLocations(snapshot)
			}
			frames.PostTask(func() {
				state.mu.Lock()
				current := state.generation == generation
				state.mu.Unlock()
				if current && !menu.IsDone() {
					menu.ReplaceItems(macOSLocationMenuItems(snapshot, pf, panelIdx))
				}
			})
			return nil
		})
		if err == nil {
			storeMacOSLocations(rows)
		}
		frames.PostTask(func() {
			state.mu.Lock()
			current := state.generation == generation
			if current {
				state.cancel = nil
			}
			state.mu.Unlock()
			if !current || menu.IsDone() || err == nil || errors.Is(err, context.Canceled) {
				return
			}
			fallback := rows
			if len(fallback) == 0 {
				fallback = cachedMacOSLocations()
			}
			items := macOSLocationMenuItems(fallback, pf, panelIdx)
			items = append(items, macOSLocationsRetryItem(menu, state, pf, panelIdx, err))
			menu.ReplaceItems(items)
		})
	}()
}

func macOSLocationsRetryItem(
	menu *vtui.VMenu, state *macOSLocationsMenuState,
	pf *PanelsFrame, panelIdx int, err error,
) vtui.MenuItem {
	message := "Retry"
	if err != nil {
		message = "Retry — " + err.Error()
	}
	return vtui.MenuItem{
		ID: "retry", Text: message, Icon: "refresh-cw", KeepOpen: true,
		OnClick: func() {
			refreshMacOSLocationsMenu(menu, state, pf, panelIdx, true)
		},
	}
}

func macOSLocationMenuItems(
	rows []macOSLocationRow, pf *PanelsFrame, panelIdx int,
) []vtui.MenuItem {
	bySection := make(map[string][]macOSLocationRow)
	for _, row := range rows {
		bySection[row.Section] = append(bySection[row.Section], row)
	}
	items := make([]vtui.MenuItem, 0, len(rows)+5)
	sections := []struct {
		id, title string
	}{
		{"top", ""},
		{"favorites", "Favorites"},
		{"locations", "Locations"},
		{"tags", "Tags"},
	}
	for _, section := range sections {
		sectionRows := bySection[section.id]
		if section.id == "tags" {
			sectionRows = append(sectionRows, macOSLocationRow{
				ID: "all-tags", Section: "tags", Kind: "query",
				Label: "All Tags…", Icon: "folder", URI: "macos://tags",
				QueryKind: "allTags", Order: 10000,
			})
		}
		sort.SliceStable(sectionRows, func(i, j int) bool {
			left, right := sectionRows[i].Order, sectionRows[j].Order
			if left == 0 {
				left = int(^uint(0) >> 1)
			}
			if right == 0 {
				right = int(^uint(0) >> 1)
			}
			return left < right
		})
		if len(sectionRows) == 0 {
			continue
		}
		if section.title != "" {
			items = append(items, vtui.MenuItem{
				ID: "header-" + section.id, Text: section.title,
				Header: true, Disabled: true,
			})
		}
		for _, row := range sectionRows {
			row := row
			item := vtui.MenuItem{
				ID: row.ID, Text: row.Label, Icon: row.Icon,
				IconColor: row.Color,
			}
			switch row.Kind {
			case "path":
				item.OnClick = func() {
					fsp, ok := pf.panels[panelIdx].(*FileSystemPanel)
					if ok {
						pf.switchToVFS(fsp, vfs.NewOSVFS(row.Path))
					}
				}
			case "query":
				item.OnClick = func() {
					fsp, ok := pf.panels[panelIdx].(*FileSystemPanel)
					if !ok || fsp.vfs == nil {
						return
					}
					parent := fsp.vfs.Clone()
					query := newMacOSQueryVFS(row.URI, row.QueryKind, row.Tag,
						row.Label, parent)
					pf.switchToVFS(fsp, query)
				}
			case "action":
				if row.Action == "airdrop" {
					item.OnClick = func() {
						if fsp, ok := pf.panels[panelIdx].(*FileSystemPanel); ok {
							pf.startAirDrop(fsp)
						}
					}
				}
			default:
				item.Disabled = true
			}
			items = append(items, item)
		}
	}
	return items
}

func localSelectionPaths(fsp *FileSystemPanel, names []string) ([]string, bool) {
	provider, ok := fsp.vfs.(vfs.LocalPathProvider)
	if !ok {
		return nil, false
	}
	paths := make([]string, 0, len(names))
	for _, name := range names {
		localPath, err := provider.LocalPath(fsp.vfs.Join(fsp.vfs.GetPath(), name))
		if err != nil || localPath == "" {
			return nil, false
		}
		paths = append(paths, localPath)
	}
	return paths, true
}

func (pf *PanelsFrame) startAirDrop(fsp *FileSystemPanel) {
	if fsp == nil || fsp.vfs == nil {
		return
	}
	names := fsp.GetSelectedNames()
	if len(names) == 0 {
		vtui.ShowMessage(" AirDrop ", "Select a file or folder to share.", []string{"&Ok"})
		return
	}
	if paths, ok := localSelectionPaths(fsp, names); ok {
		pf.requestAirDrop(paths, "")
		return
	}
	tempDir, err := os.MkdirTemp("", "f4-airdrop-*")
	if err != nil {
		vtui.ShowMessage(" AirDrop ", err.Error(), []string{"&Ok"})
		return
	}
	source := fsp.vfs
	destination := vfs.NewOSVFS(tempDir)
	ExecuteFileOpWithResult(pf, source, destination, names, tempDir, false, 1, func(copyErr error) {
		if copyErr != nil {
			_ = os.RemoveAll(tempDir)
			return
		}
		entries, readErr := os.ReadDir(tempDir)
		if readErr != nil {
			_ = os.RemoveAll(tempDir)
			return
		}
		paths := make([]string, 0, len(entries))
		for _, entry := range entries {
			paths = append(paths, filepath.Join(tempDir, entry.Name()))
		}
		sort.Strings(paths)
		if len(paths) == 0 {
			_ = os.RemoveAll(tempDir)
			return
		}
		pf.requestAirDrop(paths, tempDir)
	})
}

func (pf *PanelsFrame) requestAirDrop(paths []string, cleanupDir string) {
	client := currentPlatformIPC()
	if client == nil || !client.Available() {
		if cleanupDir != "" {
			_ = os.RemoveAll(cleanupDir)
		}
		vtui.ShowMessage(" AirDrop ", errPlatformServicesUnavailable.Error(), []string{"&Ok"})
		return
	}
	frames := vtui.FrameManager
	go func() {
		values := make([]any, len(paths))
		for i := range paths {
			values[i] = paths[i]
		}
		err := client.Request(context.Background(), "macos.airdrop",
			map[string]any{"paths": values}, nil)
		if cleanupDir != "" {
			_ = os.RemoveAll(cleanupDir)
		}
		if err != nil {
			var structured *platformStructuredError
			if errors.As(err, &structured) && structured.Cancelled {
				return
			}
			frames.PostTask(func() {
				vtui.ShowMessage(" AirDrop ", strings.TrimSpace(err.Error()), []string{"&Ok"})
			})
		}
	}()
}
