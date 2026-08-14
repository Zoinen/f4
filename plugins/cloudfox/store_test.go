package cloudfox

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func testConnection(name string, provider ProviderType) Connection {
	return Connection{Name: name, Provider: provider, Settings: json.RawMessage(`{"root":"safe"}`)}
}

func TestConnectionNamesCannotClaimManagerOrNativePathNamespaces(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		DriveName,
		strings.ToLower(DriveName),
		AddConnectionID,
		strings.ToLower(AddConnectionLabel),
		"C",
		"z",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			err := validateConnection(testConnection(name, ProviderWebDAV))
			if !errors.Is(err, ErrReservedName) {
				t.Fatalf("validateConnection(%q) = %v, want ErrReservedName", name, err)
			}
		})
	}

	for _, name := range []string{"team:archive", "account:/folder", "account:\\folder"} {
		name := name
		t.Run(name, func(t *testing.T) {
			if err := validateConnection(testConnection(name, ProviderWebDAV)); err == nil {
				t.Fatalf("validateConnection(%q) unexpectedly succeeded", name)
			}
		})
	}

	for _, name := range []string{"Google Drive", "S3 production", "Диск"} {
		name := name
		t.Run(name, func(t *testing.T) {
			if err := validateConnection(testConnection(name, ProviderWebDAV)); err != nil {
				t.Fatalf("validateConnection(%q) = %v", name, err)
			}
		})
	}
}

func TestConnectionStoreCRUDStableIDAndSort(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "CloudFox.json")
	store := NewConnectionStore(path)
	ctx := context.Background()

	zulu, err := store.Create(ctx, testConnection("Zulu", ProviderS3))
	if err != nil {
		t.Fatal(err)
	}
	if !validUUID(zulu.ID) {
		t.Fatalf("generated ID is invalid: %q", zulu.ID)
	}
	if _, err := store.Create(ctx, testConnection("zULu", ProviderWebDAV)); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("case-insensitive duplicate = %v", err)
	}
	if _, err := store.Create(ctx, testConnection(AddConnectionLabel, ProviderWebDAV)); !errors.Is(err, ErrReservedName) {
		t.Fatalf("manager pseudo-row name = %v, want ErrReservedName", err)
	}
	if _, err := store.Create(ctx, testConnection("Alpha", ProviderWebDAV)); err != nil {
		t.Fatal(err)
	}

	zulu.Name = "Bravo"
	updated, err := store.Update(ctx, zulu)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != zulu.ID {
		t.Fatalf("rename changed stable ID: %q -> %q", zulu.ID, updated.ID)
	}
	items, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "Alpha" || items[1].Name != "Bravo" {
		t.Fatalf("sorted profiles = %#v", items)
	}
	if _, err := store.Delete(ctx, zulu.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, zulu.ID); !errors.Is(err, ErrConnectionNotFound) {
		t.Fatalf("deleted Get = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("profile permissions are too broad: %v", info.Mode().Perm())
	}
}

func TestProfileStoreLockCannotBeStolenByAge(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "CloudFox.json.lock")
	release, err := acquireFileLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	old := time.Now().Add(-24 * time.Hour)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if secondRelease, err := acquireFileLock(ctx, lockPath); !errors.Is(err, context.DeadlineExceeded) {
		if secondRelease != nil {
			secondRelease()
		}
		t.Fatalf("second lock acquisition returned %v, want context deadline", err)
	}
}

func TestProfileStoreLockCanBeReacquiredAfterRelease(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "CloudFox.json.lock")
	release, err := acquireFileLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	release()
	release() // idempotent ownership release

	secondRelease, err := acquireFileLock(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	secondRelease()
}

func TestConnectionStorePreservesCorruptFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "CloudFox.json")
	original := []byte(`{"version":1,"connections":[`)
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewConnectionStore(path)
	if _, err := store.Create(context.Background(), testConnection("New", ProviderS3)); err == nil {
		t.Fatal("Create unexpectedly replaced corrupt metadata")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("corrupt file changed: %q", after)
	}
}

func TestConnectionStoreConcurrentCreatorsDoNotLoseUpdates(t *testing.T) {
	t.Parallel()
	store := NewConnectionStore(filepath.Join(t.TempDir(), "CloudFox.json"))
	ctx := context.Background()
	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Create(ctx, testConnection(string(rune('A'+i))+" profile", ProviderS3))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != count {
		t.Fatalf("stored %d profiles, want %d", len(items), count)
	}
}

func TestConnectionMetadataNeverContainsKnownSecret(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	store := NewConnectionStore(filepath.Join(dir, "CloudFox.json"))
	c := testConnection("Safe", ProviderWebDAV)
	c.SecretRef = "vault:v1:opaque-reference"
	if _, err := store.Create(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "correct horse battery staple") {
		t.Fatal("metadata contains a known secret")
	}
}
