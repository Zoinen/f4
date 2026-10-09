package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ColorStyleConfigured is what tells a first start from a user who picked the
// built-in default on purpose (issue #513): only a ColorStyle actually present
// in a settings.ini counts.
func TestColorStyleConfigured(t *testing.T) {
	tests := []struct {
		name string
		ini  *string // nil = no settings.ini at all
		want bool
	}{
		{"no settings.ini", nil, false},
		{"settings.ini without an Interface section", ptr("[Panel]\nShowHiddenFiles = 1\n"), false},
		{"Interface section without ColorStyle", ptr("[Interface]\nLanguage = en\n"), false},
		{"ColorStyle set to a value", ptr("[Interface]\nColorStyle = Radiola\n"), true},
		{"ColorStyle set to Classic", ptr("[Interface]\nColorStyle = Classic\n"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.ini")
			if tc.ini != nil {
				if err := os.WriteFile(path, []byte(*tc.ini), 0o600); err != nil {
					t.Fatal(err)
				}
			}

			origUser, origPaths, oldCfg := GetUserConfigIniPath, GetConfigIniPaths, App
			GetUserConfigIniPath = func() string { return path }
			GetConfigIniPaths = func() []string { return []string{path} }
			t.Cleanup(func() {
				GetUserConfigIniPath, GetConfigIniPaths, App = origUser, origPaths, oldCfg
				LoadConfig()
			})

			LoadConfig()
			if got := ColorStyleConfigured(); got != tc.want {
				t.Fatalf("ColorStyleConfigured() = %v, want %v", got, tc.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }
