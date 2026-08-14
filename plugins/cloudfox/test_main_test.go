package cloudfox

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// OAuth tests exercise URL construction and callbacks, never the user's
	// actual desktop/browser session.
	browserURLCommandRunner = func(string) error { return nil }
	os.Exit(m.Run())
}
