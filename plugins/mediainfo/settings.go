package mediainfo

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

const maxTemplateSize = 64 << 10

var commandPrefixPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

// Settings are intentionally small and backend-independent so changing UI
// preferences never invalidates an analysis result.
type Settings struct {
	ShowInPluginMenu bool   `json:"showInPluginMenu"`
	EnableQuickView  bool   `json:"enableQuickView"`
	UseEditor        bool   `json:"useEditor"`
	Prefix           string `json:"prefix"`
	Language         string `json:"language"`
	Template         string `json:"template,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{
		ShowInPluginMenu: true,
		EnableQuickView:  true,
		Prefix:           "MediaInfo",
		Language:         "auto",
	}
}

func (settings Settings) validate() error {
	settings.Prefix = strings.TrimSpace(settings.Prefix)
	if settings.Prefix != "" && !commandPrefixPattern.MatchString(settings.Prefix) {
		return errors.New("prefix must start with a letter and contain only letters, digits, '_' or '-'")
	}
	switch strings.ToLower(strings.TrimSpace(settings.Language)) {
	case "auto", "en", "ru":
	default:
		return errors.New("language must be auto, en, or ru")
	}
	if len(settings.Template) > maxTemplateSize {
		return fmt.Errorf("template is larger than %d KiB", maxTemplateSize>>10)
	}
	if !utf8.ValidString(settings.Template) {
		return errors.New("template is not valid UTF-8")
	}
	if settings.Template != "" {
		if _, err := ExecuteTemplate(Report{}, settings.Template); err != nil {
			return fmt.Errorf("invalid Inform template: %w", err)
		}
	}
	return nil
}

func normalizeSettings(settings Settings) Settings {
	settings.Prefix = strings.TrimSpace(settings.Prefix)
	settings.Language = strings.ToLower(strings.TrimSpace(settings.Language))
	if settings.Language == "" {
		settings.Language = "auto"
	}
	return settings
}

type settingsStore struct {
	mu      sync.RWMutex
	saveMu  sync.Mutex
	path    string
	current Settings
}

func newSettingsStore(configDir string) (*settingsStore, error) {
	store := &settingsStore{
		path:    filepath.Join(configDir, "plugins", "mediainfo.json"),
		current: DefaultSettings(),
	}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("read MediaInfo settings: %w", err)
	}
	settings := DefaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return store, fmt.Errorf("decode MediaInfo settings: %w", err)
	}
	settings = normalizeSettings(settings)
	if err := settings.validate(); err != nil {
		return store, fmt.Errorf("validate MediaInfo settings: %w", err)
	}
	store.current = settings
	return store, nil
}

func (store *settingsStore) snapshot() Settings {
	if store == nil {
		return DefaultSettings()
	}
	store.mu.RLock()
	defer store.mu.RUnlock()
	return store.current
}

func (store *settingsStore) save(settings Settings) error {
	if store == nil {
		return errors.New("MediaInfo settings store is unavailable")
	}
	// Keep the on-disk rename and the in-memory publication in one save order.
	// Without this lock, two valid dialog submissions could publish A in memory
	// after B had already won the atomic rename on disk.
	store.saveMu.Lock()
	defer store.saveMu.Unlock()
	settings = normalizeSettings(settings)
	if err := settings.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode MediaInfo settings: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		return fmt.Errorf("create MediaInfo settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".mediainfo-*.json")
	if err != nil {
		return fmt.Errorf("create MediaInfo settings file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write MediaInfo settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync MediaInfo settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close MediaInfo settings: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("replace MediaInfo settings: %w", err)
	}
	committed = true
	store.mu.Lock()
	store.current = settings
	store.mu.Unlock()
	return nil
}
