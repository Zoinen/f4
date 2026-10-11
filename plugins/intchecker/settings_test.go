package intchecker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestDefaultSettings(t *testing.T) {
	settings := DefaultSettings()
	want := Settings{
		Algorithm:              DefaultAlgorithm,
		Output:                 outputSingle,
		Recursive:              true,
		Absolute:               false,
		Encoding:               fileEncoding{Codepage: utf8Codepage},
		ValidateIgnoreMissing:  false,
		ValidateStopOnMismatch: false,
	}
	if settings != want {
		t.Fatalf("DefaultSettings() = %+v, want %+v", settings, want)
	}
}

// TestSettingsStoreRoundTrip covers f4#1623 points 1 and 2: everything the
// generate dialog remembers, plus the validate dialog's "Ignore missing
// files"/"Stop on first mismatch" checkboxes, saved by one store and read
// back by a fresh one, as happens across two runs of f4.
func TestSettingsStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := newSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.snapshot(); got != DefaultSettings() {
		t.Fatalf("fresh store = %+v, want defaults", got)
	}

	want := Settings{
		Algorithm:              AlgSHA256,
		Output:                 outputDirectory,
		Recursive:              false,
		Absolute:               true,
		Encoding:               fileEncoding{Codepage: 1251},
		ValidateIgnoreMissing:  true,
		ValidateStopOnMismatch: true,
	}
	if err := store.save(want); err != nil {
		t.Fatal(err)
	}
	if got := store.snapshot(); got != want {
		t.Fatalf("after save = %+v, want %+v", got, want)
	}

	reloaded, err := newSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.snapshot(); got != want {
		t.Fatalf("reloaded = %+v, want %+v", got, want)
	}
}

func TestSettingsStoreNormalizesBadData(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plugins", "intchecker.json")
	// An algorithm and output mode outside the valid range, and a codepage
	// nothing knows about, must all fall back to defaults rather than wedge
	// the dialog into an unselectable radio button or combo entry.
	if err := os.WriteFile(path, []byte(`{"algorithm":99,"output":-1,"recursive":true,"absolute":true,"encoding":{"codepage":424242,"bom":false}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := newSettingsStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := store.snapshot()
	if got.Algorithm != DefaultAlgorithm || got.Output != outputSingle || got.Encoding != (fileEncoding{Codepage: utf8Codepage}) {
		t.Fatalf("normalized = %+v", got)
	}
	// The bools were valid and survive normalization untouched.
	if !got.Recursive || !got.Absolute {
		t.Fatalf("normalized dropped valid bools: %+v", got)
	}
}

func TestSettingsStoreSurvivesCorruptFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "plugins", "intchecker.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := newSettingsStore(dir)
	if err == nil {
		t.Fatal("expected a decode error")
	}
	if got := store.snapshot(); got != DefaultSettings() {
		t.Fatalf("corrupt file left = %+v, want defaults", got)
	}
}

// TestNewGenerateDialogAppliesSettingsExceptNameAndMask is the dialog-level
// half of f4#1623 point 1: everything the dialog shows must come from the
// saved settings except the output file name (always dirBase-based) and the
// mask (always defaultMask). The encoding uses UTF-8 with BOM rather than a
// codepage such as 1251: writeEncodingChoices' system ANSI/OEM entries are
// dropped when they coincide with UTF-8 (systemEncodingChoices), which is
// exactly what happens on a UTF-8 CI runner, so a codepage not among the
// choices would silently fall back to the first one and defeat the test;
// UTF-8 with BOM is always offered, on every platform.
func TestNewGenerateDialogAppliesSettingsExceptNameAndMask(t *testing.T) {
	initValidateTestScreen(t)
	settings := Settings{
		Algorithm: AlgSHA256,
		Output:    outputDirectory,
		Recursive: false,
		Absolute:  true,
		Encoding:  fileEncoding{Codepage: utf8Codepage, BOM: true},
	}
	d := newGenerateDialog("photos", settings)
	if d.algorithm.Selected != int(AlgSHA256) {
		t.Errorf("algorithm = %d, want %d", d.algorithm.Selected, AlgSHA256)
	}
	if d.output.Selected != int(outputDirectory) {
		t.Errorf("output = %d, want %d", d.output.Selected, outputDirectory)
	}
	if d.recursive.State != 0 {
		t.Errorf("recursive = %d, want 0", d.recursive.State)
	}
	if d.absolute.State != 1 {
		t.Errorf("absolute = %d, want 1", d.absolute.State)
	}
	if got := d.encoding.selected(); got != settings.Encoding {
		t.Errorf("encoding = %+v, want %+v", got, settings.Encoding)
	}
	// Name and mask never come from settings.
	if d.editMask.GetText() != defaultMask {
		t.Errorf("mask = %q, want default %q", d.editMask.GetText(), defaultMask)
	}
	if got, want := d.editOutput.GetText(), defaultOutputName("photos", AlgSHA256); got != want {
		t.Errorf("output name = %q, want %q", got, want)
	}
}

// TestSubmitGeneratePersistsSettingsExceptNameAndMask drives the OK button's
// real handler (submitGenerate) end to end: builds a dialog from defaults,
// edits every field including the output name and the mask, submits it, and
// checks what actually landed on disk. Guards against a future edit widening
// the persisted Settings to include the file name or the mask by accident.
func TestSubmitGeneratePersistsSettingsExceptNameAndMask(t *testing.T) {
	initValidateTestScreen(t)
	configDir := t.TempDir()
	store, err := newSettingsStore(configDir)
	if err != nil {
		t.Fatal(err)
	}
	p := &Plugin{api: &hostMock{}, store: store}

	fsDir := t.TempDir()
	fs := vfs.NewOSVFS(fsDir)
	d := newGenerateDialog("photos", p.store.snapshot())
	d.algorithm.Selected = int(AlgSHA256)
	d.output.Selected = int(outputDisplay)
	d.recursive.State = 0
	d.absolute.State = 1
	// UTF-8 with BOM, not a codepage such as 1251: writeEncodingChoices' system
	// ANSI/OEM entries collapse into plain UTF-8 on a UTF-8 runner (see
	// TestNewGenerateDialogAppliesSettingsExceptNameAndMask), so a codepage not
	// among the actual choices would round-trip as UTF-8 and hide a real bug.
	d.encoding = newEncodingCombo(24, writeEncodingChoices(), fileEncoding{Codepage: utf8Codepage, BOM: true})
	d.editMask.SetText("*.jpg")
	d.editOutput.SetText("whatever-this-run-only.sha256")

	app := &appMock{fs: fs}
	p.submitGenerate(app, fs, fsDir, []string{"a.txt"}, d)

	want := Settings{Algorithm: AlgSHA256, Output: outputDisplay, Recursive: false, Absolute: true, Encoding: fileEncoding{Codepage: utf8Codepage, BOM: true}}
	if got := p.store.snapshot(); got != want {
		t.Fatalf("persisted = %+v, want %+v", got, want)
	}

	// A settings file on disk has no field at all for the output name or the
	// mask; only the JSON round trip proves that, a struct comparison in
	// memory would not catch a hidden field surviving through json:"-".
	data, err := os.ReadFile(filepath.Join(configDir, "plugins", "intchecker.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"whatever-this-run-only", "*.jpg"} {
		if strings.Contains(string(data), leaked) {
			t.Fatalf("settings file leaked a per-run value %q:\n%s", leaked, data)
		}
	}

	// The next dialog starts the name and the mask fresh, from dirBase and
	// defaultMask, not from what was just typed -- but keeps every other
	// remembered choice.
	next := newGenerateDialog("other", p.store.snapshot())
	if got, want := next.editOutput.GetText(), defaultOutputName("other", AlgSHA256); got != want {
		t.Fatalf("next dialog output name = %q, want %q", got, want)
	}
	if got := next.editMask.GetText(); got != defaultMask {
		t.Fatalf("next dialog mask = %q, want default %q", got, defaultMask)
	}
	if next.algorithm.Selected != int(AlgSHA256) || next.output.Selected != int(outputDisplay) {
		t.Fatalf("next dialog dropped remembered algorithm/output: algorithm=%d output=%d", next.algorithm.Selected, next.output.Selected)
	}
}

// TestSubmitGeneratePreservesValidateOptions guards against the same class
// of bug TestSubmitGeneratePersistsSettingsExceptNameAndMask already covers
// for the file name and mask, but the other way around: f4#1623 point 2
// added ValidateIgnoreMissing/ValidateStopOnMismatch to the very same
// Settings struct submitGenerate writes, so a "Generate hashes" run must not
// silently reset the "Validate files" dialog's remembered checkboxes.
func TestSubmitGeneratePreservesValidateOptions(t *testing.T) {
	initValidateTestScreen(t)
	configDir := t.TempDir()
	store, err := newSettingsStore(configDir)
	if err != nil {
		t.Fatal(err)
	}
	seed := DefaultSettings()
	seed.ValidateIgnoreMissing = true
	seed.ValidateStopOnMismatch = true
	if err := store.save(seed); err != nil {
		t.Fatal(err)
	}
	p := &Plugin{api: &hostMock{}, store: store}

	fsDir := t.TempDir()
	fs := vfs.NewOSVFS(fsDir)
	d := newGenerateDialog("photos", p.store.snapshot())
	d.algorithm.Selected = int(AlgSHA256)
	app := &appMock{fs: fs}
	p.submitGenerate(app, fs, fsDir, []string{"a.txt"}, d)

	got := p.store.snapshot()
	if !got.ValidateIgnoreMissing || !got.ValidateStopOnMismatch {
		t.Fatalf("submitGenerate reset the validate checkboxes: %+v", got)
	}
	if got.Algorithm != AlgSHA256 {
		t.Fatalf("submitGenerate did not apply its own change: %+v", got)
	}
}

// TestSaveValidateOptionsPreservesGenerateSettings is the mirror case: the
// "Validate files" dialog's Ok must not reset the "Generate hashes" choices
// that already live in the same Settings struct and file.
func TestSaveValidateOptionsPreservesGenerateSettings(t *testing.T) {
	configDir := t.TempDir()
	store, err := newSettingsStore(configDir)
	if err != nil {
		t.Fatal(err)
	}
	seed := DefaultSettings()
	seed.Algorithm = AlgSHA512
	seed.Absolute = true
	if err := store.save(seed); err != nil {
		t.Fatal(err)
	}
	p := &Plugin{api: &hostMock{}, store: store}

	if err := p.saveValidateOptions(validateOptions{ignoreMissing: true, stopOnMismatch: true}); err != nil {
		t.Fatal(err)
	}

	got := p.store.snapshot()
	if got.Algorithm != AlgSHA512 || !got.Absolute {
		t.Fatalf("saveValidateOptions reset the generate settings: %+v", got)
	}
	if !got.ValidateIgnoreMissing || !got.ValidateStopOnMismatch {
		t.Fatalf("saveValidateOptions did not apply its own change: %+v", got)
	}
}
