package netfox

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestLoadSSHProfilesAppliesIncludesAndHostDefaults(t *testing.T) {
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	includeDir := filepath.Join(sshDir, "config.d")
	if err := os.MkdirAll(includeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	identity := filepath.Join(sshDir, "id_de_zoin")
	if err := os.WriteFile(identity, []byte("placeholder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(includeDir, "10-defaults.conf"), []byte("Host *\n  IdentityFile ~/.ssh/id_missing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "Include config.d/*.conf\n\nHost de_zoin\n  HostName 202.61.255.60\n  User root\n  Port 2202\n  IdentityFile ~/.ssh/id_de_zoin\n  ConnectTimeout 7\n\nHost web-*\n  HostName web.example\n"
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}

	profiles, err := loadSSHProfiles(home)
	if err != nil {
		t.Fatal(err)
	}
	profile, ok := profiles["de_zoin"]
	if !ok {
		t.Fatalf("de_zoin profile missing: %#v", profiles)
	}
	if profile.Host != "202.61.255.60" || profile.Port != "2202" || profile.User != "root" {
		t.Fatalf("unexpected profile connection fields: %#v", profile)
	}
	if profile.KeyPath != identity || profile.Timeout != "7" {
		t.Fatalf("unexpected profile identity/timeout: %#v", profile)
	}
	if len(profile.IdentityFiles) != 2 {
		t.Fatalf("identity files = %#v, want included and host-specific entries", profile.IdentityFiles)
	}
	if _, ok := profiles["web-*"]; ok {
		t.Fatal("wildcard Host pattern was exposed as a connection")
	}
}

func TestNetFoxVFSImportsAndDisablesSSHProfiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host imported\n  HostName imported.example\n  User test-user\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	configs := store.getConfigs()
	imported, ok := configs["imported"]
	if !ok {
		t.Fatalf("imported SSH profile missing from NetFox: %#v", configs)
	}
	if imported.Type != "sftp" || imported.Host != "imported.example" || imported.User != "test-user" {
		t.Fatalf("unexpected imported connection: %#v", imported)
	}

	var listed bool
	if err := store.ReadDir(context.Background(), "net://", func(items []vfs.VFSItem) {
		for _, item := range items {
			if item.Name == "imported" {
				listed = true
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !listed {
		t.Fatal("imported SSH profile missing from ReadDir")
	}
	if err := store.Remove(context.Background(), "imported"); err == nil {
		t.Fatal("auto-imported SSH profile was removable")
	}

	if err := store.savePreferences(netFoxPreferences{ImportSSHProfiles: boolPtr(false)}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.getConfigs()["imported"]; ok {
		t.Fatal("disabled SSH profile import still exposed a profile")
	}
}

func TestNetFoxSettingsImportSSHProfilesDefaultsOnAndCanBeDisabled(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host imported\n  HostName imported.example\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	draft, err := (&settingsProvider{store: store}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer draft.Close()
	if got := draft.Values[importSSHProfilesSettingID]; got != "true" {
		t.Fatalf("OpenSSH profile import default = %q, want true", got)
	}
	draft.Values[importSSHProfilesSettingID] = "false"
	if result := draft.Commit(context.Background()); len(result.Errors) != 0 {
		t.Fatalf("disabling OpenSSH profile import failed: %v", result.Errors)
	}
	if store.importSSHProfilesLocked() {
		t.Fatal("OpenSSH profile import remained enabled after Settings Center commit")
	}
}

func TestNetFoxVFSOpenMergesSSHProfileIntoSavedConnection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "config"), []byte("Host de_zoin\n  HostName 202.61.255.60\n  User root\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if err := store.SaveConfig("de_zoin", NetFoxConfig{Type: "sftp", Host: "202.61.255.60", Port: "22", KeyPath: "/tmp/key"}); err != nil {
		t.Fatal(err)
	}
	reader, err := store.Open(context.Background(), "de_zoin")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	data := make([]byte, reader.Size())
	if _, err := reader.Read(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	var cfg NetFoxConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.User != "root" {
		t.Fatalf("saved connection did not inherit SSH-config user: %#v", cfg)
	}
	resolved, ok := netFoxConfigAt(context.Background(), &netFoxVFSWrapper{NetFoxVFS: store}, "net://de_zoin")
	if !ok || !resolved.autoSSHProfile {
		t.Fatalf("saved OpenSSH alias was not marked for transport fallback: ok=%v cfg=%#v", ok, resolved)
	}
}

func boolPtr(value bool) *bool { return &value }
