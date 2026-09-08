//go:build windows

package vfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestRegistryVFSPathRoundTrip(t *testing.T) {
	v := NewRegistryVFS()
	p := v.Join(registryVFSRoot, "HKEY_CURRENT_USER", "Software", "name/with slash")
	if want := "registry://HKEY_CURRENT_USER/Software/name%2Fwith%20slash"; p != want {
		t.Fatalf("Join = %q, want %q", p, want)
	}
	if got := v.Base(p); got != "name/with slash" {
		t.Fatalf("Base = %q, want original registry name", got)
	}
	if got := v.Dir(p); got != "registry://HKEY_CURRENT_USER/Software" {
		t.Fatalf("Dir = %q, want parent key", got)
	}
}

func TestRegistryVFSListsHivesAndIsReadOnly(t *testing.T) {
	ctx := context.Background()
	v := NewRegistryVFS()
	var items []VFSItem
	if err := v.ReadDir(ctx, registryVFSRoot, func(chunk []VFSItem) { items = append(items, chunk...) }); err != nil {
		t.Fatalf("ReadDir root: %v", err)
	}
	if len(items) != len(registryHives) {
		t.Fatalf("root item count = %d, want %d: %#v", len(items), len(registryHives), items)
	}
	for _, hive := range registryHives {
		found := false
		for _, item := range items {
			if item.Name == hive.name && item.IsDir {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("root hive %q is missing", hive.name)
		}
	}
	if got := v.GetCapabilities(); got.HasWrite || !got.HasRandomAccess {
		t.Fatalf("capabilities = %#v, want read-only random access", got)
	}
	if err := v.MkDir(ctx, registryVFSRoot+"HKEY_CURRENT_USER/new"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("MkDir error = %v, want permission denied", err)
	}
	if _, err := v.Create(ctx, registryVFSRoot+"HKEY_CURRENT_USER/new"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Create error = %v, want permission denied", err)
	}
}

func TestRegistryVFSReadsStringAndDefaultValues(t *testing.T) {
	ctx := context.Background()
	keyPath := fmt.Sprintf(`Software\f4-registry-vfs-%d`, time.Now().UnixNano())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, keyPath, registry.ALL_ACCESS)
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if err := key.SetStringValue("Greeting", "hello"); err != nil {
		key.Close()
		t.Fatalf("SetStringValue: %v", err)
	}
	if err := key.SetStringValue("", "default"); err != nil {
		key.Close()
		t.Fatalf("SetStringValue default: %v", err)
	}
	if err := key.Close(); err != nil {
		t.Fatalf("close test key: %v", err)
	}
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, keyPath) })

	v := NewRegistryVFS()
	keyURI := v.Join(registryVFSRoot, "HKEY_CURRENT_USER", "Software", strings.TrimPrefix(keyPath, `Software\`))
	var items []VFSItem
	if err := v.ReadDir(ctx, keyURI, func(chunk []VFSItem) { items = append(items, chunk...) }); err != nil {
		t.Fatalf("ReadDir test key: %v", err)
	}
	for _, name := range []string{"Greeting", registryDefaultValueName} {
		found := false
		for _, item := range items {
			if item.Name == name && !item.IsDir && item.SizeKnown {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("value %q is missing from %#v", name, items)
		}
	}

	valueURI := v.Join(keyURI, "Greeting")
	stat, err := v.Stat(ctx, valueURI)
	if err != nil {
		t.Fatalf("Stat value: %v", err)
	}
	if stat.IsDir || stat.Mode != "REG_SZ" || stat.Revision == "" {
		t.Fatalf("Stat value = %#v, want REG_SZ file with revision", stat)
	}
	reader, err := v.Open(ctx, valueURI)
	if err != nil {
		t.Fatalf("Open value: %v", err)
	}
	data := make([]byte, reader.Size())
	if _, err := reader.ReadAt(ctx, data, 0); err != nil && !errors.Is(err, io.EOF) {
		reader.Close()
		t.Fatalf("ReadAt value: %v", err)
	}
	reader.Close()
	text := string(data)
	if !strings.Contains(text, "Type: REG_SZ") || !strings.Contains(text, `Value: "hello"`) {
		t.Fatalf("value text = %q, want type and data", text)
	}

	defaultURI := v.Join(keyURI, registryDefaultValueName)
	defaultReader, err := v.Open(ctx, defaultURI)
	if err != nil {
		t.Fatalf("Open default value: %v", err)
	}
	defaultData := make([]byte, defaultReader.Size())
	_, _ = defaultReader.ReadAt(ctx, defaultData, 0)
	_ = defaultReader.Close()
	if !strings.Contains(string(defaultData), `Value: "default"`) {
		t.Fatalf("default value text = %q", defaultData)
	}
}
