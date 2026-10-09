package observer_test

// This file exercises Provider (provider.go) against a real
// observer.ini/observer_user.ini pair (config.go): part 9 of f4#1563's own
// module-selection mechanism, on top of the isoimg.wasm/target.iso fixtures
// provider_test.go and isoimg_e2e_test.go already build in CI.

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/plugins/observer"
	"github.com/unxed/f4/vfs"
)

// newTestModulesDirWithIni builds root/modules/isoimg.wasm plus
// root/observer.ini and returns modulesDir -- config.go reads the ini pair
// from filepath.Dir(modulesDir), one directory up from where the .wasm
// files themselves live.
func newTestModulesDirWithIni(t *testing.T, ini string) string {
	t.Helper()
	wasmBytes := loadOptionalFixture(t, isoimgWasmPath)
	root := t.TempDir()
	modulesDir := filepath.Join(root, "modules")
	if err := os.MkdirAll(modulesDir, 0o700); err != nil {
		t.Fatalf("creating modules dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(modulesDir, "isoimg.wasm"), wasmBytes, 0o600); err != nil {
		t.Fatalf("writing isoimg.wasm fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "observer.ini"), []byte(ini), 0o600); err != nil {
		t.Fatalf("writing observer.ini: %v", err)
	}
	return modulesDir
}

func TestProvider_ObserverIni_DisablesModule(t *testing.T) {
	modulesDir := newTestModulesDirWithIni(t, "[Modules]\nISO=-\n")
	parent, isoPath := newTestISO(t)
	p := observer.NewProvider(modulesDir)

	if p.CanOpen(context.Background(), parent, isoPath) {
		t.Error("CanOpen = true, want false: observer.ini disables ISO with \"ISO=-\"")
	}
}

func TestProvider_ObserverIni_EmptyModulesSectionDisablesEverything(t *testing.T) {
	// A config file exists but names no modules at all -- different from no
	// config file existing (see loadModuleEntries's own doc comment):
	// nothing is recognized, not even the stock default.
	modulesDir := newTestModulesDirWithIni(t, "[Filters]\nISO=*.iso\n")
	parent, isoPath := newTestISO(t)
	p := observer.NewProvider(modulesDir)

	if p.CanOpen(context.Background(), parent, isoPath) {
		t.Error("CanOpen = true, want false: an observer.ini without [Modules] loads no modules, matching upstream ModulesController::Init")
	}
}

func TestProvider_ObserverIni_CustomFilterExtension(t *testing.T) {
	modulesDir := newTestModulesDirWithIni(t, "[Modules]\nISO=isoimg.wasm\n\n[Filters]\nISO=*.img\n")
	isoBytes := loadOptionalFixture(t, isoimgIsoPath)

	imgDir := t.TempDir()
	imgPath := filepath.Join(imgDir, "target.img")
	if err := os.WriteFile(imgPath, isoBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	p := observer.NewProvider(modulesDir)

	if !p.CanOpen(context.Background(), vfs.NewOSVFS(imgDir), imgPath) {
		t.Errorf("CanOpen(%q) = false, want true: observer.ini's [Filters] ISO=*.img should widen the extension gate", imgPath)
	}

	// This ini's [Filters] ISO=*.img *replaces* the stock "*.iso" filter, it
	// does not add to it -- ".iso" no longer matches through the name alone.
	isoDir := t.TempDir()
	isoPath := filepath.Join(isoDir, "target.iso")
	if err := os.WriteFile(isoPath, isoBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if p.CanOpen(context.Background(), vfs.NewOSVFS(isoDir), isoPath) {
		t.Errorf("CanOpen(%q) = true, want false: [Filters] ISO=*.img replaces the default filter", isoPath)
	}
}

func TestProvider_ObserverIni_ModuleSettingsSectionDoesNotBreakOpen(t *testing.T) {
	// Charset/RockRidge are the real isoimg.cpp option names (see its own
	// LoadSubModule -> OptionsList(LoadParams->Settings), and the stock
	// observer.ini's own [ISO] section fetched straight from upstream) --
	// this proves buildSettingsString's wire format (config.go) round-trips
	// through LoadSubModule without upsetting a real module.
	modulesDir := newTestModulesDirWithIni(t,
		"[Modules]\nISO=isoimg.wasm\n\n[Filters]\nISO=*.iso\n\n[ISO]\nCharset=1\nRockRidge=1\n")
	parent, isoPath := newTestISO(t)
	p := observer.NewProvider(modulesDir)

	v, err := p.Open(context.Background(), parent, isoPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := v.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestProvider_ObserverUserIni_OverridesBaseFilter(t *testing.T) {
	wasmBytes := loadOptionalFixture(t, isoimgWasmPath)
	isoBytes := loadOptionalFixture(t, isoimgIsoPath)
	root := t.TempDir()
	modulesDir := filepath.Join(root, "modules")
	if err := os.MkdirAll(modulesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modulesDir, "isoimg.wasm"), wasmBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "observer.ini"),
		[]byte("[Modules]\nISO=isoimg.wasm\n\n[Filters]\nISO=*.iso\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "observer_user.ini"),
		[]byte("[Filters]\nISO=*.iso;*.img\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	imgDir := t.TempDir()
	imgPath := filepath.Join(imgDir, "target.img")
	if err := os.WriteFile(imgPath, isoBytes, 0o600); err != nil {
		t.Fatal(err)
	}

	p := observer.NewProvider(modulesDir)
	if !p.CanOpen(context.Background(), vfs.NewOSVFS(imgDir), imgPath) {
		t.Errorf("CanOpen(%q) = false, want true: observer_user.ini's [Filters] ISO=*.iso;*.img should win over observer.ini's own ISO=*.iso", imgPath)
	}
}
