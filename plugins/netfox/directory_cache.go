package netfox

import (
	"sort"
	"strconv"
	"strings"
)

// netFoxDirectoryCacheIdentity is intentionally based on connection
// settings, not a live protocol client. SFTP may fall back to FISH+ and both
// transports may be represented by a new VFS after a panel re-entry; the
// directory is still the same configured remote in that case.
func netFoxDirectoryCacheIdentity(connection string, cfg NetFoxConfig) string {
	protocol := strings.ToLower(strings.TrimSpace(cfg.Type))
	if protocol == "" {
		protocol = "sftp"
	}
	proxy := cfg.Proxy()
	parts := []string{
		"netfox", connection, protocol,
		strings.TrimSpace(cfg.Host), strings.TrimSpace(cfg.Port),
		strings.TrimSpace(cfg.User), strings.TrimSpace(cfg.KeyPath),
		strings.TrimSpace(cfg.Codepage), strings.TrimSpace(cfg.Timeout),
		strconv.Itoa(proxy.Mode), strings.TrimSpace(proxy.Host),
		strings.TrimSpace(proxy.Port), strings.TrimSpace(proxy.User),
	}
	optionNames := make([]string, 0, len(cfg.Options))
	for name := range cfg.Options {
		optionNames = append(optionNames, name)
	}
	sort.Strings(optionNames)
	for _, name := range optionNames {
		parts = append(parts, name, cfg.Options[name])
	}
	return strings.Join(parts, "\x1f")
}
