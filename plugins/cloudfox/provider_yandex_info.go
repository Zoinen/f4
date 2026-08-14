package cloudfox

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/unxed/f4/vfs"
)

func (b *yandexDiskBackend) refreshAbout(ctx context.Context) error {
	resp, err := b.apiRequest(ctx, http.MethodGet, "", nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return mapProviderHTTPError(resp, readSmallResponse(resp))
	}
	var about yandexDiskAbout
	if err := json.NewDecoder(resp.Body).Decode(&about); err != nil {
		return err
	}
	b.mu.Lock()
	b.about = &about
	b.aboutAt = time.Now()
	b.mu.Unlock()
	return nil
}

func (b *yandexDiskBackend) PanelInfoKey(req vfs.PanelInfoRequest) string {
	return "yandex:" + req.Path
}

func (b *yandexDiskBackend) CachedPanelInfo(vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	b.mu.RLock()
	about, refreshed := b.about, b.aboutAt
	b.mu.RUnlock()
	return yandexPanelSnapshot(about, refreshed), about != nil && time.Since(refreshed) < 5*time.Minute
}

func yandexPanelSnapshot(about *yandexDiskAbout, refreshed time.Time) vfs.PanelInfoSnapshot {
	snapshot := vfs.PanelInfoSnapshot{Authoritative: true, RefreshedAt: refreshed}
	if about == nil {
		return snapshot
	}
	section := vfs.PanelInfoSection{ID: "account", Title: "Yandex.Disk"}
	displayName := strings.TrimSpace(about.User.DisplayName)
	login := strings.TrimSpace(about.User.Login)
	user := displayName
	if user == "" {
		user = login
	} else if login != "" && !strings.EqualFold(displayName, login) {
		user += " <" + login + ">"
	}
	if user != "" {
		section.Fields = append(section.Fields, vfs.PanelInfoField{ID: "user", Label: "Account", Kind: vfs.PanelInfoText, Value: user})
	}
	if about.TotalSpace > 0 {
		available := uint64(0)
		if about.UsedSpace < about.TotalSpace {
			available = about.TotalSpace - about.UsedSpace
		}
		section.Fields = append(section.Fields, vfs.PanelInfoField{ID: "quota", Label: "Storage", Kind: vfs.PanelInfoUsage, TotalBytes: about.TotalSpace, AvailableBytes: available})
	}
	if about.TrashSize > 0 {
		section.Fields = append(section.Fields, vfs.PanelInfoField{ID: "trash", Label: "Trash", Kind: vfs.PanelInfoBytes, Bytes: about.TrashSize})
	}
	if about.MaxFileSize > 0 {
		section.Fields = append(section.Fields, vfs.PanelInfoField{ID: "max_file_size", Label: "Maximum file size", Kind: vfs.PanelInfoBytes, Bytes: about.MaxFileSize})
	}
	snapshot.Sections = []vfs.PanelInfoSection{section}
	return snapshot
}

func (b *yandexDiskBackend) RefreshPanelInfo(ctx context.Context, _ vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	if err := b.refreshAbout(ctx); err != nil {
		return vfs.PanelInfoSnapshot{}, err
	}
	snapshot, _ := b.CachedPanelInfo(vfs.PanelInfoRequest{})
	return snapshot, nil
}
