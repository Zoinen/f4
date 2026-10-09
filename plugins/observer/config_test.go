package observer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeIni is a small test helper: it writes content to
// dir/name and returns dir, the way password_test.go's own helpers write a
// fixture straight into a t.TempDir().
func writeIni(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", name, err)
	}
}

func TestLoadModuleEntries_NoConfigFallsBackToDefault(t *testing.T) {
	dir := t.TempDir()

	got := loadModuleEntries(dir)
	want := defaultModuleEntries()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("loadModuleEntries(no config) = %+v, want defaultModuleEntries() = %+v", got, want)
	}
}

func TestLoadModuleEntries_OrderAndFilters(t *testing.T) {
	dir := t.TempDir()
	writeIni(t, dir, observerConfigFileName, `[Modules]
RPA=rpa.wasm
ISO=isoimg.wasm

[Filters]
RPA=*.rpa
ISO=*.iso

[RPA]
Key=Value
`)

	entries := loadModuleEntries(dir)
	want := []moduleEntry{
		{Name: "RPA", FileName: "rpa.wasm", Filter: "*.rpa", Settings: "Key=Value\x00"},
		{Name: "ISO", FileName: "isoimg.wasm", Filter: "*.iso"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("loadModuleEntries = %+v, want %+v (order from [Modules] must be preserved)", entries, want)
	}
}

func TestLoadModuleEntries_DashDisablesModule(t *testing.T) {
	dir := t.TempDir()
	writeIni(t, dir, observerConfigFileName, `[Modules]
RPA=rpa.wasm
ISO=-
`)

	entries := loadModuleEntries(dir)
	want := []moduleEntry{{Name: "RPA", FileName: "rpa.wasm"}}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("loadModuleEntries = %+v, want %+v (ISO=- must be skipped, like ModulesController::Init)", entries, want)
	}
}

func TestLoadModuleEntries_MissingModulesSectionYieldsNoEntries(t *testing.T) {
	dir := t.TempDir()
	writeIni(t, dir, observerConfigFileName, `[Filters]
ISO=*.iso
`)

	entries := loadModuleEntries(dir)
	if entries != nil {
		t.Errorf("loadModuleEntries = %+v, want nil: a config file without [Modules] is not the same as no config file at all", entries)
	}
}

func TestLoadModuleEntries_UserFileOverridesFilterInPlace(t *testing.T) {
	dir := t.TempDir()
	writeIni(t, dir, observerConfigFileName, `[Modules]
RPA=rpa.wasm
ISO=isoimg.wasm

[Filters]
RPA=*.rpa
ISO=*.iso
`)
	writeIni(t, dir, observerUserConfigFileName, `[Filters]
ISO=*.iso;*.nrg
`)

	entries := loadModuleEntries(dir)
	want := []moduleEntry{
		{Name: "RPA", FileName: "rpa.wasm", Filter: "*.rpa"},
		{Name: "ISO", FileName: "isoimg.wasm", Filter: "*.iso;*.nrg"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("loadModuleEntries = %+v, want %+v (observer_user.ini's [Filters] value must win, RPA's own position/value must survive untouched)", entries, want)
	}
}

func TestLoadModuleEntries_UserFileAddsModuleAfterBaseOnes(t *testing.T) {
	dir := t.TempDir()
	writeIni(t, dir, observerConfigFileName, `[Modules]
ISO=isoimg.wasm
`)
	writeIni(t, dir, observerUserConfigFileName, `[Modules]
RPA=rpa.wasm
`)

	entries := loadModuleEntries(dir)
	want := []moduleEntry{
		{Name: "ISO", FileName: "isoimg.wasm"},
		{Name: "RPA", FileName: "rpa.wasm"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("loadModuleEntries = %+v, want %+v (a module observer_user.ini adds must be appended, keeping observer.ini's own module first)", entries, want)
	}
}

func TestBuildSettingsString(t *testing.T) {
	if got := buildSettingsString(nil); got != "" {
		t.Errorf("buildSettingsString(nil) = %q, want \"\"", got)
	}
	if got := buildSettingsString(newOrderedSection()); got != "" {
		t.Errorf("buildSettingsString(empty) = %q, want \"\"", got)
	}

	sec := newOrderedSection()
	sec.set("Charset", "1")
	sec.set("RockRidge", "1")
	got := buildSettingsString(sec)
	want := "Charset=1\x00RockRidge=1\x00"
	if got != want {
		t.Errorf("buildSettingsString = %q, want %q", got, want)
	}
}

func TestOrderedSection_SetKeepsPositionOnUpdate(t *testing.T) {
	sec := newOrderedSection()
	sec.set("A", "1")
	sec.set("B", "2")
	sec.set("A", "3") // re-set: value changes, position must not move

	if got, want := sec.keys, []string{"A", "B"}; !reflect.DeepEqual(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	if sec.vals["A"] != "3" {
		t.Errorf("vals[A] = %q, want %q", sec.vals["A"], "3")
	}
}

func TestOrderedINI_MergeFile_MissingFileReturnsFalse(t *testing.T) {
	ini := newOrderedINI()
	if ini.mergeFile(filepath.Join(t.TempDir(), "does-not-exist.ini")) {
		t.Error("mergeFile(missing) = true, want false")
	}
}

func TestOrderedINI_MergeFile_CommentsAndBlankLinesIgnored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.ini")
	writeIni(t, dir, "x.ini", `[Modules]
; a comment
ISO=isoimg.wasm

;RPA=rpa.wasm
`)

	ini := newOrderedINI()
	if !ini.mergeFile(path) {
		t.Fatal("mergeFile(existing) = false, want true")
	}
	sec := ini.section("Modules")
	if sec == nil {
		t.Fatal("section(Modules) = nil")
	}
	want := []string{"ISO"}
	if !reflect.DeepEqual(sec.keys, want) {
		t.Errorf("keys = %v, want %v (commented-out RPA line must not be parsed)", sec.keys, want)
	}
}
