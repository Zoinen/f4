package visren

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/unxed/f4/vfs"
)

type config struct {
	WordDiv string `json:"word_div"`
}

func configPath() string {
	dir := vfs.CustomConfigDir
	if dir == "" {
		if userDir, err := os.UserConfigDir(); err == nil {
			dir = filepath.Join(userDir, "f4")
		} else {
			dir = "."
		}
	}
	return filepath.Join(dir, "visren.json")
}

func loadConfig() config {
	cfg := config{WordDiv: "-. _&"}
	data, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg.WordDiv == "" {
		cfg.WordDiv = "-. _&"
	}
	if runes := []rune(cfg.WordDiv); len(runes) > 18 {
		cfg.WordDiv = string(runes[:18])
	}
	return cfg
}

func saveConfig(cfg config) error {
	path := configPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
