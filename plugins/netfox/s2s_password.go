package netfox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

// s2sBehaviorSettings holds netfox's own opt-in switches, as opposed to the
// per-connection fields in NetFox.json. Today it has exactly one: whether a
// server-to-server copy or move (internal/fileops) may authenticate its
// second hop with a password taken from f4's own saved connections instead
// of requiring the two hosts to already trust each other via SSH keys or an
// agent. It defaults to false (see f4#370): letting one server see another
// server's password is a deliberate, opt-in trade-off, not a default.
type s2sBehaviorSettings struct {
	AllowServerToServerPasswordAuth bool `json:"AllowServerToServerPasswordAuth"`
}

// netfoxConfigDir is the directory NetFox.json and this package's other
// small config files live in. It mirrors the fallback netfox.go's
// RegisterDrive and settings_center.go's newSettingsProvider already use.
func netfoxConfigDir() string {
	dir := vfs.CustomConfigDir
	if dir != "" {
		return dir
	}
	sysDir, _ := os.UserConfigDir()
	return filepath.Join(sysDir, "f4")
}

var s2sBehaviorMu sync.Mutex

// readS2SBehaviorSettingsAt and writeS2SBehaviorSettingsAt load and save the
// behavior file at an explicit path, defaulting to the safe (disabled) value
// whenever it is missing or damaged: a config problem must never silently
// turn a security-relevant opt-in on. The path is always explicit, never
// derived from the global vfs.CustomConfigDir internally, so a test can
// point these at a throwaway file the same way settingsProvider's own tests
// hand it a throwaway *NetFoxVFS rather than touching that global.
func readS2SBehaviorSettingsAt(path string) s2sBehaviorSettings {
	s2sBehaviorMu.Lock()
	defer s2sBehaviorMu.Unlock()
	var s s2sBehaviorSettings
	data, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s) // A damaged file falls back to the safe default above.
	return s
}

func writeS2SBehaviorSettingsAt(path string, s s2sBehaviorSettings) error {
	s2sBehaviorMu.Lock()
	defer s2sBehaviorMu.Unlock()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return writeNetFoxFile(path, append(data, '\n'))
}

// posixSingleQuote wraps s in single quotes for a POSIX shell command line,
// closing and reopening the quote around any single quote already in s. It
// is used for values (like a mktemp'd path) that this client generated
// itself rather than trusted user input, but stays defensive regardless.
// Kept here rather than reusing sftp_vfs.go's equivalent because that file
// is built with "!lite" while fish_vfs.go (and this file) are not.
func posixSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// secondHopPassword is what a FISH+ site asks on its own behalf when a
// server-to-server transfer wants to authenticate to it with a password
// (see vfs.SecondHopPasswordProvider). It answers yes only when the opt-in
// setting above is on AND f4's own saved connections carry a fish+ entry for
// this exact host, port and (if given) user with a non-empty password --
// exactly the "если у нас в списке соединений встречаются пароли ... от всех
// нужных серверов" scenario the ticket describes, never a password that
// happens to be sitting on the live session for some other reason.
func secondHopPassword(host, port, user string) (string, bool) {
	dir := netfoxConfigDir()
	return secondHopPasswordAt(filepath.Join(dir, "NetFoxBehavior.json"), filepath.Join(dir, "NetFox.json"), host, port, user)
}

// secondHopPasswordAt is secondHopPassword with explicit file paths, so
// tests can exercise the real lookup logic against throwaway files instead
// of the global vfs.CustomConfigDir.
func secondHopPasswordAt(behaviorPath, connectionsPath, host, port, user string) (string, bool) {
	if host == "" {
		return "", false
	}
	if !readS2SBehaviorSettingsAt(behaviorPath).AllowServerToServerPasswordAuth {
		return "", false
	}
	wantPort := port
	if wantPort == "" {
		wantPort = "22"
	}
	store := &NetFoxVFS{path: connectionsPath}
	for _, cfg := range store.getConfigs() {
		if !fishTypeMatches(cfg.Type) {
			continue
		}
		if !strings.EqualFold(cfg.Host, host) {
			continue
		}
		cfgPort := cfg.Port
		if cfgPort == "" {
			cfgPort = "22"
		}
		if cfgPort != wantPort {
			continue
		}
		if user != "" && cfg.User != user {
			continue
		}
		if cfg.Pass == "" {
			continue
		}
		return cfg.Pass, true
	}
	return "", false
}
