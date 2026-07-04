package vtui

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/vmihailenco/msgpack/v5"
)

const (
	qtProtocolVersion = 1
	qtMaxMessageSize  = 64 * 1024 * 1024
)

func qtNewNonce() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf[:]), nil
}

func qtSendMessage(w io.Writer, msg map[string]any) error {
	payload, err := msgpack.Marshal(msg)
	if err != nil {
		return err
	}
	if len(payload) > qtMaxMessageSize {
		return fmt.Errorf("qt message too large: %d bytes", len(payload))
	}

	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

type qtMessageSender struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *qtMessageSender) Send(msg map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return qtSendMessage(s.w, msg)
}

func qtReadMessage(r io.Reader) (map[string]any, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 {
		return nil, fmt.Errorf("empty qt message")
	}
	if n > qtMaxMessageSize {
		return nil, fmt.Errorf("qt message too large: %d bytes", n)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	var msg map[string]any
	if err := msgpack.Unmarshal(payload, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func qtString(msg map[string]any, key string) string {
	if v, ok := msg[key].(string); ok {
		return v
	}
	return ""
}

func qtBool(msg map[string]any, key string) bool {
	if v, ok := msg[key].(bool); ok {
		return v
	}
	return false
}

func qtInt(msg map[string]any, key string) int {
	return qtAnyInt(msg[key])
}

func qtAnyInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	}
	return 0
}

func qtHostExecutableName() string {
	if runtime.GOOS == "windows" {
		return "f4-qt-host.exe"
	}
	return "f4-qt-host"
}

func qtFileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func findQtHostPath() (string, error) {
	if env := os.Getenv("F4_QT_HOST_PATH"); env != "" {
		if qtFileExists(env) {
			return env, nil
		}
		return "", fmt.Errorf("F4_QT_HOST_PATH points to missing file: %s", env)
	}

	exeName := qtHostExecutableName()
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, exeName))
		if runtime.GOOS == "darwin" && strings.HasSuffix(exeDir, ".app/Contents/MacOS") {
			candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(exeDir)), "Resources", exeName))
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		for _, cfg := range []string{"RelWithDebInfo", "Release", "Debug"} {
			candidates = append(candidates, filepath.Join(cwd, "qt", "host", "build", "bin", cfg, exeName))
		}
		candidates = append(candidates,
			filepath.Join(cwd, "qt", "host", "build", "bin", exeName),
			filepath.Join(cwd, "qt", "host", "build", exeName),
		)
	}

	for _, path := range candidates {
		if qtFileExists(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("Qt host executable not found; set F4_QT_HOST_PATH or build qt/host")
}
