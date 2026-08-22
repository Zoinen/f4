package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReadAccessProfileStringsAndConservativeDefault(t *testing.T) {
	tests := map[ReadAccessProfile]string{
		ReadAccessUnknownExpensive: "unknownExpensive",
		ReadAccessDirectLocal:      "directLocal",
		ReadAccessNativeRange:      "nativeRange",
		ReadAccessHybridRange:      "hybridRange",
		ReadAccessMaterializeOnce:  "materializeOnce",
	}
	for profile, want := range tests {
		if got := profile.String(); got != want {
			t.Errorf("profile %d = %q, want %q", profile, got, want)
		}
	}
	if got := (VFSCapabilities{}).ReadAccess.String(); got != "unknownExpensive" {
		t.Fatalf("zero-value access profile = %q", got)
	}
	if got := (VFSCapabilities{}).StorageClass.String(); got != "unknown" {
		t.Fatalf("zero-value storage class = %q", got)
	}
}

func TestOSVFSReaderExposesDirectLocalBackingLease(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "image.jpg")
	if err := os.WriteFile(filePath, []byte("jpeg"), 0o600); err != nil {
		t.Fatal(err)
	}
	filesystem := NewOSVFS(filepath.Dir(filePath))
	reader, err := filesystem.Open(context.Background(), filePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	backing, ok := reader.(LocalBackingReader)
	if !ok {
		t.Fatal("OS reader does not implement LocalBackingReader")
	}
	if got, ok := backing.LocalPath(); !ok || got != filePath {
		t.Fatalf("local backing = %q, %v", got, ok)
	}
	profiler, ok := reader.(ReadAccessProfiler)
	if !ok || profiler.ReadAccessProfile() != ReadAccessDirectLocal {
		t.Fatalf("reader profile = %v, implements=%v", profiler, ok)
	}
	caps := filesystem.GetCapabilities()
	if caps.ReadAccess != ReadAccessDirectLocal || caps.StorageClass != StorageClassLocal {
		t.Fatalf("OS capabilities = %#v", caps)
	}
}
