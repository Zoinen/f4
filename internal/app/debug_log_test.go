package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestF4DebugLogPath(t *testing.T) {
	configDir := filepath.Join("tmp", "f4", "Profile")
	want := filepath.Join(configDir, "logs", "debug.log")
	if got := f4DebugLogPath(configDir); got != want {
		t.Fatalf("debug log path = %q, want %q", got, want)
	}
}

func TestConfigureF4DebugLogPath_DefaultsToProfile(t *testing.T) {
	for _, value := range []string{"1", "true"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("VTUI_DEBUG", value)
			configDir := filepath.Join(t.TempDir(), "Profile")
			configureF4DebugLogPath(configDir)

			want := f4DebugLogPath(configDir)
			if got := os.Getenv("VTUI_DEBUG"); got != want {
				t.Fatalf("VTUI_DEBUG = %q, want %q", got, want)
			}
			if info, err := os.Stat(filepath.Dir(want)); err != nil || !info.IsDir() {
				t.Fatalf("debug log directory was not created: info=%v err=%v", info, err)
			}
		})
	}
}

func TestConfigureF4DebugLogPath_PreservesExplicitValue(t *testing.T) {
	if configDir := os.Getenv("F4_TEST_CONFIGURE_DEBUG_LOG"); configDir != "" {
		value := os.Getenv("F4_TEST_CONFIGURE_DEBUG_LOG_VALUE")
		configureF4DebugLogPath(configDir)
		if got := os.Getenv("VTUI_DEBUG"); got != value {
			t.Fatalf("VTUI_DEBUG = %q, want explicit value %q", got, value)
		}
		// #nosec G703 -- configDir is generated in this test's temporary directory.
		if _, err := os.Stat(filepath.Join(configDir, "logs")); !os.IsNotExist(err) {
			t.Fatalf("preserving explicit VTUI_DEBUG created profile logs directory: %v", err)
		}
		return
	}

	for _, value := range []string{"", "stderr", "test", filepath.Join(t.TempDir(), "custom.log")} {
		t.Run(value, func(t *testing.T) {
			configDir := filepath.Join(t.TempDir(), "Profile")
			// #nosec G204 G702 -- re-execute this test binary with a fixed test selector.
			cmd := exec.Command(os.Args[0], "-test.run=^TestConfigureF4DebugLogPath_PreservesExplicitValue$")
			for _, entry := range os.Environ() {
				if !strings.HasPrefix(entry, "VTUI_DEBUG=") &&
					!strings.HasPrefix(entry, "F4_TEST_CONFIGURE_DEBUG_LOG=") &&
					!strings.HasPrefix(entry, "F4_TEST_CONFIGURE_DEBUG_LOG_VALUE=") {
					cmd.Env = append(cmd.Env, entry)
				}
			}
			cmd.Env = append(cmd.Env,
				"F4_TEST_CONFIGURE_DEBUG_LOG="+configDir,
				"F4_TEST_CONFIGURE_DEBUG_LOG_VALUE="+value,
				"VTUI_DEBUG="+value,
			)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("isolated check failed: %v\n%s", err, output)
			}
		})
	}
}
