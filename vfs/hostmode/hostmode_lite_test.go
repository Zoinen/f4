//go:build lite

package hostmode

import (
	"os"
	"testing"
)

func TestLitePosixIgnoresWineOverride(t *testing.T) {
	for _, override := range []string{"", "0", "1"} {
		t.Run(override, func(t *testing.T) {
			t.Setenv("F4_WINE_POSIX", override)
			if Posix() {
				t.Fatal("lite build must not enable the Wine personality")
			}
			if home, ok := HomeDir(); ok || home != "" {
				t.Fatalf("Wine home = %q, %v; want empty, false", home, ok)
			}
		})
	}
}

func TestLiteHostModeSettings(t *testing.T) {
	original := Allowed()
	t.Cleanup(func() { SetAllowed(original) })
	for _, allowed := range []bool{false, true} {
		SetAllowed(allowed)
		if Allowed() != allowed {
			t.Fatalf("Allowed() did not retain %v", allowed)
		}
	}
	want, wantErr := os.UserHomeDir()
	got, err := UserHomeDir()
	if got != want || (err == nil) != (wantErr == nil) {
		t.Fatalf("UserHomeDir() = %q, %v; want %q, %v", got, err, want, wantErr)
	}
}
