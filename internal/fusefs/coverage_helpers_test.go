package fusefs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type titledFakeVFS struct {
	*fakeVFS
	title string
}

func (v *titledFakeVFS) GetTitle() string { return v.title }

func TestDescribeSourceUsesExplicitSourceOrTitle(t *testing.T) {
	v := &titledFakeVFS{fakeVFS: newFakeVFS(false), title: "Archive"}
	if got := describeSource("  shown source  ", v, "/root"); got != "  shown source  " {
		t.Fatalf("explicit source = %q, want it preserved", got)
	}
	if got := describeSource("", v, "/root"); got != "Archive/root" {
		t.Fatalf("titled source = %q, want %q", got, "Archive/root")
	}
	v.title = ""
	if got := describeSource("", v, "/root"); got != "/root" {
		t.Fatalf("untitled source = %q, want root path", got)
	}
}

func TestMountRootUsesRuntimeDirectory(t *testing.T) {
	old, had := os.LookupEnv("XDG_RUNTIME_DIR")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("XDG_RUNTIME_DIR", old)
		} else {
			_ = os.Unsetenv("XDG_RUNTIME_DIR")
		}
	})
	if err := os.Setenv("XDG_RUNTIME_DIR", "/run/user/test"); err != nil {
		t.Fatal(err)
	}
	// MountRoot uses filepath.Join, so the expectation is OS-specific
	// (backslashes on Windows).
	want := filepath.Join("/run/user/test", "f4", "mnt")
	if got := MountRoot(); got != want {
		t.Fatalf("MountRoot() = %q, want %q", got, want)
	}
}

func TestSanitizeNameKeepsRemoteHostAndBoundsResult(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{name: "local path", source: " /srv/backups/ ", want: "backups"},
		{name: "remote path", source: "sftp://user@example.com/srv/backups/", want: "example.com-backups"},
		{name: "host only", source: "https://user@example.com", want: "example.com"},
		{name: "empty", source: "   ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeName(tt.source); got != tt.want {
				t.Fatalf("sanitizeName(%q) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
	long := strings.Repeat("x", 100)
	if got := sanitizeName(long); len(got) != 64 {
		t.Fatalf("sanitizeName(long) length = %d, want 64", len(got))
	}
}

func TestEnsureMountPointCoversCreationAndSafetyChecks(t *testing.T) {
	root := t.TempDir()
	createdPoint := filepath.Join(root, "created")
	created, err := ensureMountPoint(createdPoint)
	if err != nil || !created {
		t.Fatalf("ensureMountPoint(missing) = %v, %v; want created", created, err)
	}
	if created, err := ensureMountPoint(createdPoint); err != nil || created {
		t.Fatalf("ensureMountPoint(empty existing) = %v, %v; want false, nil", created, err)
	}
	if err := os.WriteFile(filepath.Join(createdPoint, "hidden"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureMountPoint(createdPoint); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("ensureMountPoint(non-empty) error = %v, want not-empty error", err)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureMountPoint(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("ensureMountPoint(file) error = %v, want not-directory error", err)
	}
}

func TestRegistryDirHonoursExplicitEnvironment(t *testing.T) {
	old, had := os.LookupEnv("F4_FUSE_REGISTRY")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("F4_FUSE_REGISTRY", old)
		} else {
			_ = os.Unsetenv("F4_FUSE_REGISTRY")
		}
	})
	if err := os.Setenv("F4_FUSE_REGISTRY", filepath.Join(t.TempDir(), "registry")); err != nil {
		t.Fatal(err)
	}
	if got := RegistryDir(); !strings.HasSuffix(got, filepath.Join("registry")) {
		t.Fatalf("RegistryDir() = %q, want explicit directory", got)
	}
	if err := Deregister(""); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Deregister(\"\") = %v, want nil", err)
	}
}
