package config

import (
	"github.com/unxed/f4/internal/ini"
	"os"
	"strings"
)

func SaveAppliedConfiguration() error {
	path := GetUserConfigIniPath()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	parsed := ini.Parse(strings.NewReader(string(SerializeSettingsConfig(App))))
	for section, values := range parsed.Sections() {
		data = UpdateIniValues(data, section, values)
	}
	return WriteUserFileAtomically(path, data, 0600)
}

func WriteSettingsCandidate(before, after F4Config) error {
	path := GetUserConfigIniPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return WriteUserFileAtomically(path, SerializeSettingsConfig(after), 0600)
	}
	if err != nil {
		return err
	}
	oldIni := ini.Parse(strings.NewReader(string(SerializeSettingsConfig(before))))
	newIni := ini.Parse(strings.NewReader(string(SerializeSettingsConfig(after))))
	for section, values := range newIni.Sections() {
		changes := map[string]string{}
		for key, value := range values {
			if oldIni.GetString(section, key, "") != value {
				changes[key] = value
			}
		}
		if len(changes) > 0 {
			data = UpdateIniValues(data, section, changes)
		}
	}
	return WriteUserFileAtomically(path, data, 0600)
}
