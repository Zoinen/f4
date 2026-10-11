package netfox

import (
	"context"
	"github.com/unxed/f4/vfs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNetFoxVFS_ConfigPersistence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_net.json")
	// Ensure the file is created for consistency in tests
	if err := os.WriteFile(dbPath, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	nf := NewNetFoxVFS(dbPath)

	// 1. Test Saving
	cfg := NetFoxConfig{Type: "sftp", Host: "1.2.3.4", User: "root", Pass: "plaintext_secret", Timeout: "15"}
	if err := nf.SaveConfig("My Server", cfg); err != nil {
		t.Fatal(err)
	}

	// Check if password was actually encrypted on disk
	rawJSON, _ := os.ReadFile(dbPath)
	if !strings.Contains(string(rawJSON), cryptoPrefix) {
		t.Error("Password was not encrypted on disk")
	}
	if strings.Contains(string(rawJSON), "plaintext_secret") {
		t.Error("Plaintext password leaked into JSON file")
	}

	// 2. Test Loading (via internal helper)
	configs := nf.getConfigs()
	if len(configs) != 1 {
		t.Fatalf("Expected 1 config, got %d", len(configs))
	}
	if configs["My Server"].Host != "1.2.3.4" {
		t.Errorf("Host mismatch. Got %s", configs["My Server"].Host)
	}
	if configs["My Server"].Pass != "plaintext_secret" {
		t.Errorf("Decryption during load failed. Expected 'plaintext_secret', got %q", configs["My Server"].Pass)
	}
	if configs["My Server"].Timeout != "15" {
		t.Errorf("Expected Timeout '15', got %q", configs["My Server"].Timeout)
	}

	// 3. Test ReadDir (visual representation)
	found := false
	if err := nf.ReadDir(context.Background(), "", func(items []vfs.VFSItem) {
		for _, itm := range items {
			if itm.Name == "My Server" {
				found = true
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Error("ReadDir failed to list saved connection")
	}

	// 4. Test Removal
	if err := nf.Remove(context.Background(), "My Server"); err != nil {
		t.Fatal(err)
	}
	if len(nf.getConfigs()) != 0 {
		t.Error("Config was not removed")
	}
}

func TestNetFoxVFS_DamagedConfigIsNeverOverwritten(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "connections.json")
	original := []byte("{not-json\n")
	if err := os.WriteFile(dbPath, original, 0o600); err != nil {
		t.Fatal(err)
	}
	nf := NewNetFoxVFS(dbPath)
	if err := nf.SaveConfig("new", NetFoxConfig{Type: "sftp", Host: "example"}); err == nil {
		t.Fatal("saving over damaged connections file unexpectedly succeeded")
	}
	got, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("damaged file changed after rejected save: %q", got)
	}
}

func TestNetFoxVFS_InvalidWriterDoesNotSaveEmptyConfig(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "connections.json")
	nf := NewNetFoxVFS(dbPath)
	if err := nf.SaveConfig("existing", NetFoxConfig{Type: "sftp", Host: "example"}); err != nil {
		t.Fatal(err)
	}
	writer, err := nf.Create(context.TODO(), "existing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("not-json")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err == nil {
		t.Fatal("invalid JSON writer unexpectedly succeeded")
	}
	configs := nf.getConfigs()
	if got := configs["existing"].Host; got != "example" {
		t.Fatalf("invalid writer changed existing profile: %q", got)
	}
}
