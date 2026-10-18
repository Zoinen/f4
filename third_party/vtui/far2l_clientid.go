package vtui

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// The far2l terminal remembers the user's clipboard decision per client ID, so
// the ID has to identify one application on one machine: a constant shared by
// every vtui program would let any of them -- or any foreign process that knows
// the string -- reuse a permission granted to another. The ID is a random token
// created on first use and kept in the user's config directory, so the choice
// survives restarts but is not guessable by other programs.
const (
	far2lClientIDMinLen = 32  // far2l rejects shorter IDs
	far2lClientIDMaxLen = 256 // ... and longer ones
)

var (
	far2lClientIDOnce  sync.Once
	far2lClientIDValue string
)

// far2lClientID returns the ID of this application for the far2l clipboard
// handshake, creating and persisting it on first use.
func far2lClientID() string {
	far2lClientIDOnce.Do(func() {
		name := AppName
		if name == "" {
			name = "vtui"
		}
		path := ""
		if dir, err := os.UserConfigDir(); err == nil {
			path = filepath.Join(dir, name, "far2l-client-id")
		}
		far2lClientIDValue = loadOrCreateFar2lClientID(path)
	})
	return far2lClientIDValue
}

// loadOrCreateFar2lClientID reads the ID stored at path, or creates a new random
// one and stores it there. When path is empty or unusable the ID is still random
// but lives only as long as the process: the terminal then asks again on every
// start, which is safe, unlike falling back to a well-known value.
func loadOrCreateFar2lClientID(path string) string {
	if path != "" {
		// #nosec G304 -- the path is built from the user's config directory and the application name.
		if data, err := os.ReadFile(path); err == nil {
			id := strings.TrimSpace(string(data))
			if validFar2lClientID(id) {
				return id
			}
		}
	}
	id := newFar2lClientID()
	if path != "" {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err == nil {
			_ = os.WriteFile(path, []byte(id+"\n"), 0o600)
		}
	}
	return id
}

func validFar2lClientID(id string) bool {
	if len(id) < far2lClientIDMinLen || len(id) > far2lClientIDMaxLen {
		return false
	}
	for _, r := range id {
		if r <= ' ' || r > '~' {
			return false
		}
	}
	return true
}

func newFar2lClientID() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand does not fail on supported platforms; refuse to hand out
		// a predictable ID if it ever does.
		panic("vtui: no randomness for the far2l client ID: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
