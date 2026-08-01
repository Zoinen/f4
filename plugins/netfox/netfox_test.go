package netfox

import (
	"bytes"
	"github.com/unxed/f4/vfs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNetFoxVFS_ConfigPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_net.json")
	// Ensure the file is created for consistency in tests
	os.WriteFile(dbPath, []byte("{}"), 0644)
	nf := NewNetFoxVFS(dbPath)

	// 1. Test Saving
	cfg := NetFoxConfig{Type: "sftp", Host: "1.2.3.4", User: "root", Pass: "plaintext_secret", Timeout: "15"}
	nf.SaveConfig("My Server", cfg)

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
	nf.ReadDir(nil, "", func(items []vfs.VFSItem) {
		for _, itm := range items {
			if itm.Name == "My Server" {
				found = true
			}
		}
	})
	if !found {
		t.Error("ReadDir failed to list saved connection")
	}

	// 4. Test Removal
	nf.Remove(nil, "My Server")
	if len(nf.getConfigs()) != 0 {
		t.Error("Config was not removed")
	}
}

func TestNetFox_TimeoutAndDial(t *testing.T) {
	// 1. Start a local mock TCP server that does NOT send the FTP greeting (simulating wrong protocol)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start mock TCP server: %v", err)
	}
	defer l.Close()

	addr := l.Addr().String()
	host, port, _ := net.SplitHostPort(addr)

	// Accept connection in background but do nothing (simulating hang)
	go func() {
		conn, err := l.Accept()
		if err == nil {
			defer conn.Close()
			time.Sleep(2 * time.Second)
		}
	}()

	// 2. Attempt to connect using FTPVFS with a very short 1-second timeout
	start := time.Now()
	_, err = NewFTPVFS(nil, host, port, "user", "pass", 1, nil, "")
	duration := time.Since(start)

	if err == nil {
		t.Error("Expected connection to fail due to timeout, but it succeeded")
	}

	// The connection should fail and return within approx 1 second (plus small buffer), not hang
	if duration > 1500*time.Millisecond {
		t.Errorf("Timeout took too long: %v (expected ~1s)", duration)
	}
}
func TestNetFox_CodepageSupport(t *testing.T) {
	v := &FTPVFS{
		cwd: "/home",
	}
	dec, enc := vfs.GetCodepageDecoderEncoder("1251")
	v.decoder = dec
	v.encoder = enc

	encoded := v.encodePath("Привет")
	expected := []byte{0xcf, 0xf0, 0xe8, 0xe2, 0xe5, 0xf2}
	if !bytes.Equal([]byte(encoded), expected) {
		t.Errorf("encodePath failed: expected bytes %v, got %q", expected, []byte(encoded))
	}
}
