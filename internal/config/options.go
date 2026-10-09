package config

import (
	"bytes"
	"reflect"
	"strings"

	"github.com/unxed/f4/internal/ini"
)

// Option is one settings.ini key as SaveConfig writes it: f4:config lists
// these the way far2l's far:config lists its option table (#1177).
type Option struct {
	Section string
	Key     string
	Value   string
}

// builtinApp is App as compiled in, before any settings file was read.
var builtinApp = App

// Options lists every key SaveConfig writes for cfg, in the order it writes
// them. It reads SerializeSettingsConfig with the rules ini.Parse applies, so
// the list cannot drift from the file.
func Options(cfg F4Config) []Option {
	var options []Option
	section := ""
	for _, line := range strings.Split(string(SerializeSettingsConfig(cfg)), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		if idx := strings.Index(line, "="); idx != -1 && section != "" {
			options = append(options, Option{
				Section: section,
				Key:     strings.TrimSpace(line[:idx]),
				Value:   strings.TrimSpace(line[idx+1:]),
			})
		}
	}
	return options
}

// DefaultConfig is what f4 runs with when no settings file says anything:
// the compiled-in values as LoadConfig reads them from an empty file. That is
// not App's literal, which LoadConfig overrides in places (ColorStyle, for
// one), so it is computed rather than copied.
func DefaultConfig() F4Config {
	cfg := builtinApp
	parseConfigInto(&cfg, ini.New())
	return cfg
}

// WithOption returns cfg with one settings.ini key set, read back the way
// LoadConfig would read it: over the settings files, with cfg's own values on
// top. A value LoadConfig normalises or ignores comes back normalised or
// ignored, so Options of the result shows what f4 actually kept. Keys
// SaveConfig does not write come from the files, as they did at startup.
func WithOption(cfg F4Config, section, key, value string) F4Config {
	value = strings.NewReplacer("\r", "", "\n", "").Replace(value)
	merged := loadSettingsIni()
	merged.Merge(ini.Parse(bytes.NewReader(SerializeSettingsConfig(cfg))))
	merged.Merge(ini.Parse(strings.NewReader("[" + section + "]\n" + key + " = " + value + "\n")))
	next := cfg
	parseConfigInto(&next, merged)
	return next
}

// ChangedFields names the F4Config fields that differ between two
// configurations, in declaration order: what Settings Center hands its
// runtime refresh as the changed setting IDs.
func ChangedFields(before, after F4Config) []string {
	b, a := reflect.ValueOf(before), reflect.ValueOf(after)
	t := b.Type()
	var changed []string
	for i := 0; i < t.NumField(); i++ {
		if !reflect.DeepEqual(b.Field(i).Interface(), a.Field(i).Interface()) {
			changed = append(changed, t.Field(i).Name)
		}
	}
	return changed
}
