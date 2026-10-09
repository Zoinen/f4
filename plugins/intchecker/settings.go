package intchecker

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/unxed/f4/vfs"
)

// Settings are the "Generate hashes" dialog choices that persist between
// runs, so the next run of f4 reopens the dialog the way the user last left
// it. The output file name and the file mask are deliberately left out: the
// author asked (f4#1623, review of build d1a1b7d, 28-09-2026) to keep those
// fresh every time -- the file name usually follows the working directory's
// name and the mask defaults back to "every file".
type Settings struct {
	Algorithm Algorithm    `json:"algorithm"`
	Output    outputMode   `json:"output"`
	Recursive bool         `json:"recursive"`
	Absolute  bool         `json:"absolute"`
	Encoding  fileEncoding `json:"encoding"`
	// ValidateIgnoreMissing and ValidateStopOnMismatch are the "Validate
	// files" dialog's "Ignore missing files"/"Stop on first mismatch"
	// checkboxes (f4#1623, review of build d1a1b7d, point 2). They persist
	// between runs the same way the generate settings above do, through the
	// same store and file; the checksum file encoding is deliberately not
	// stored here -- the dialog's default for it always tracks Encoding
	// above (see Plugin.showValidate in validate_ui.go), which is the
	// literal ask ("by default, the same encoding as for generation").
	ValidateIgnoreMissing  bool `json:"validateIgnoreMissing"`
	ValidateStopOnMismatch bool `json:"validateStopOnMismatch"`
}

// DefaultSettings is what a fresh install (or a settings file this version
// cannot make sense of) starts with -- the same defaults the dialog offered
// before it remembered anything. The validate checkboxes both default to
// off, so a user who never opens the "Validate files" dialog keeps today's
// behaviour: missing files are reported, and a mismatch does not stop the
// run.
func DefaultSettings() Settings {
	return Settings{
		Algorithm:              DefaultAlgorithm,
		Output:                 outputSingle,
		Recursive:              true,
		Absolute:               false,
		Encoding:               fileEncoding{Codepage: utf8Codepage},
		ValidateIgnoreMissing:  false,
		ValidateStopOnMismatch: false,
	}
}

// normalizeSettings clamps whatever came from disk (an older or a hand-edited
// settings file) to values the dialog can actually show.
func normalizeSettings(settings Settings) Settings {
	if !settings.Algorithm.valid() {
		settings.Algorithm = DefaultAlgorithm
	}
	if !settings.Output.valid() {
		settings.Output = outputSingle
	}
	settings.Encoding.Codepage = vfs.NormalizeCodepageID(settings.Encoding.Codepage)
	if settings.Encoding.Codepage != utf8Codepage {
		if cp, ok := vfs.FindCodepage(settings.Encoding.Codepage); !ok || cp.Enc == nil {
			settings.Encoding = fileEncoding{Codepage: utf8Codepage}
		}
	}
	return settings
}

// settingsStore reads and atomically rewrites the generate settings file. It
// is nil-safe on every method, the same way plugins/mediainfo's does it, so a
// plugin whose Init has not run yet (or failed) can still ask for defaults.
type settingsStore struct {
	mu      sync.RWMutex
	saveMu  sync.Mutex
	path    string
	current Settings
}

func newSettingsStore(configDir string) (*settingsStore, error) {
	store := &settingsStore{
		path:    filepath.Join(configDir, "plugins", "intchecker.json"),
		current: DefaultSettings(),
	}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("read Integrity Checker settings: %w", err)
	}
	settings := DefaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return store, fmt.Errorf("decode Integrity Checker settings: %w", err)
	}
	store.current = normalizeSettings(settings)
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

// save writes the generate settings that should carry over to the next run.
// It follows the same write-then-rename order as plugins/mediainfo's store,
// so a crash mid-save never leaves a half-written file behind.
func (store *settingsStore) save(settings Settings) error {
	if store == nil {
		return errors.New("Integrity Checker settings store is unavailable")
	}
	store.saveMu.Lock()
	defer store.saveMu.Unlock()
	settings = normalizeSettings(settings)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode Integrity Checker settings: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		return fmt.Errorf("create Integrity Checker settings directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".intchecker-*.json")
	if err != nil {
		return fmt.Errorf("create Integrity Checker settings file: %w", err)
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
		return fmt.Errorf("write Integrity Checker settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync Integrity Checker settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Integrity Checker settings: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("replace Integrity Checker settings: %w", err)
	}
	committed = true
	store.mu.Lock()
	store.current = settings
	store.mu.Unlock()
	return nil
}
