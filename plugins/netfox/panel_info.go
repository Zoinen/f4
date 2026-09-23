package netfox

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const netFoxPanelInfoCacheTTL = 5 * time.Minute

// netFoxPanelInfoProvider keeps the connection facts that are already known
// when a NetFox session is opened. The information panel must be useful before
// any additional network probe completes, so its first read is local and the
// result is shared by every clone of the connection.
type netFoxPanelInfoProvider struct {
	mu         sync.RWMutex
	key        string
	connection string
	snapshot   vfs.PanelInfoSnapshot
	ttl        time.Duration
}

type netFoxManagerPanelInfoCache struct {
	mu        sync.RWMutex
	snapshots map[string]vfs.PanelInfoSnapshot
}

func newNetFoxManagerPanelInfoCache() *netFoxManagerPanelInfoCache {
	return &netFoxManagerPanelInfoCache{snapshots: make(map[string]vfs.PanelInfoSnapshot)}
}

func (c *netFoxManagerPanelInfoCache) replace(configs map[string]NetFoxConfig) {
	if c == nil {
		return
	}
	snapshots := make(map[string]vfs.PanelInfoSnapshot, len(configs))
	for name, cfg := range configs {
		protocol := strings.TrimSpace(cfg.Type)
		if protocol == "" {
			protocol = "sftp"
		}
		snapshots[name] = buildNetFoxPanelInfoSnapshot(name, protocol, cfg)
	}
	c.mu.Lock()
	c.snapshots = snapshots
	c.mu.Unlock()
}

func (c *netFoxManagerPanelInfoCache) snapshot(name string) (vfs.PanelInfoSnapshot, bool) {
	if c == nil {
		return vfs.PanelInfoSnapshot{}, false
	}
	c.mu.RLock()
	snapshot, ok := c.snapshots[name]
	c.mu.RUnlock()
	return snapshot, ok
}

func buildNetFoxPanelInfoSnapshot(connection, protocol string, cfg NetFoxConfig) vfs.PanelInfoSnapshot {
	fields := []vfs.PanelInfoField{
		{ID: "connection", Label: "Connection", Value: connection},
		{ID: "protocol", Label: "Protocol", Value: protocol},
	}
	if host := strings.TrimSpace(cfg.Host); host != "" {
		fields = append(fields, vfs.PanelInfoField{ID: "host", Label: "Host", Value: host})
	}
	if port := strings.TrimSpace(cfg.Port); port != "" {
		fields = append(fields, vfs.PanelInfoField{ID: "port", Label: "Port", Value: port})
	}
	if user := strings.TrimSpace(cfg.User); user != "" {
		fields = append(fields, vfs.PanelInfoField{ID: "user", Label: "User", Value: user})
	}
	return vfs.PanelInfoSnapshot{
		Authoritative: true,
		Sections: []vfs.PanelInfoSection{{
			ID: "netfox.connection", Title: "NetFox connection", Fields: fields,
		}},
		RefreshedAt: time.Now(),
	}
}

func newNetFoxPanelInfoProvider(connection, protocol string, cfg NetFoxConfig) *netFoxPanelInfoProvider {
	now := time.Now()
	fields := []vfs.PanelInfoField{
		{ID: "connection", Label: "Connection", Value: connection},
		{ID: "protocol", Label: "Protocol", Value: protocol},
	}
	if host := strings.TrimSpace(cfg.Host); host != "" {
		fields = append(fields, vfs.PanelInfoField{ID: "host", Label: "Host", Value: host})
	}
	if port := strings.TrimSpace(cfg.Port); port != "" {
		fields = append(fields, vfs.PanelInfoField{ID: "port", Label: "Port", Value: port})
	}
	if user := strings.TrimSpace(cfg.User); user != "" {
		fields = append(fields, vfs.PanelInfoField{ID: "user", Label: "User", Value: user})
	}
	return &netFoxPanelInfoProvider{
		key:        fmt.Sprintf("netfox:%s:%s", protocol, connection),
		connection: connection,
		snapshot: vfs.PanelInfoSnapshot{
			Authoritative: true,
			Sections: []vfs.PanelInfoSection{{
				ID: "netfox.connection", Title: "NetFox connection", Fields: fields,
			}},
			RefreshedAt: now,
		},
		ttl: netFoxPanelInfoCacheTTL,
	}
}

func (p *netFoxPanelInfoProvider) PanelInfoKey(req vfs.PanelInfoRequest) string {
	if p == nil {
		return ""
	}
	return p.key + ":" + req.Path
}

func (p *netFoxPanelInfoProvider) CachedPanelInfo(vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	if p == nil {
		return vfs.PanelInfoSnapshot{}, true
	}
	p.mu.RLock()
	snapshot, ttl := p.snapshot, p.ttl
	p.mu.RUnlock()
	fresh := ttl <= 0 || (!snapshot.RefreshedAt.IsZero() && time.Since(snapshot.RefreshedAt) < ttl)
	vtui.DebugLog("[FIX:netfox-cache] cached panel info connection=%q fresh=%t", p.connection, fresh)
	return snapshot, fresh
}

func (p *netFoxPanelInfoProvider) RefreshPanelInfo(ctx context.Context, _ vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return vfs.PanelInfoSnapshot{}, err
	}
	if p == nil {
		return vfs.PanelInfoSnapshot{}, nil
	}
	p.mu.Lock()
	p.snapshot.RefreshedAt = time.Now()
	snapshot := p.snapshot
	p.mu.Unlock()
	vtui.DebugLog("[FIX:netfox-cache] refreshed panel info connection=%q", p.connection)
	return snapshot, nil
}

func configureNetFoxConnection(fs vfs.VFS, connection, protocol string, cfg NetFoxConfig) {
	devicePath := vfs.DevicePath{Scheme: "net", Device: connection}
	provider := newNetFoxPanelInfoProvider(connection, protocol, cfg)
	directoryCacheKey := netFoxDirectoryCacheIdentity(connection, cfg)
	switch mounted := fs.(type) {
	case *FishVFS:
		mounted.SetDevicePath(devicePath)
		mounted.SetDirectoryCacheKey(directoryCacheKey)
		mounted.SetPanelInfoProvider(provider)
	case *SFTPVFS:
		mounted.SetDevicePath(devicePath)
		mounted.SetDirectoryCacheKey(directoryCacheKey)
		mounted.SetPanelInfoProvider(provider)
	case *FTPVFS:
		mounted.SetDevicePath(devicePath)
		mounted.SetDirectoryCacheKey(directoryCacheKey)
		mounted.SetPanelInfoProvider(provider)
	}
	vtui.DebugLog("[FIX:netfox-path] qualified connection=%q as %s://%s; directory cache identity enabled", connection, devicePath.Scheme, connection)
}

func netFoxConnectionName(raw string) string {
	if vfs.IsURIPath(raw) {
		if scheme, device, _, err := vfs.ParseDevicePath(raw); err == nil && strings.EqualFold(scheme, "net") {
			return device
		}
	}
	trimmed := strings.Trim(raw, "/")
	if slash := strings.LastIndexByte(trimmed, '/'); slash >= 0 {
		trimmed = trimmed[slash+1:]
	}
	return trimmed
}
