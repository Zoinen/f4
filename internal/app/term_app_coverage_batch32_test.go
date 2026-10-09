package app

import (
	"reflect"
	"testing"
)

func TestTermApplicationVersionInfoCoverageBatch32(t *testing.T) {
	app := termApplication{}
	if got := app.VersionInfo(); got != getFormattedVersionInfo() {
		t.Fatalf("VersionInfo() = %q, want %q", got, getFormattedVersionInfo())
	}
}

func TestTermApplicationVersionInfoStableCoverageBatch32(t *testing.T) {
	app := termApplication{}
	first := app.VersionInfo()
	second := app.VersionInfo()
	if first != second {
		t.Fatalf("VersionInfo() changed between calls: %q then %q", first, second)
	}
}

func TestTermApplicationEditFilePathCoverageBatch32(t *testing.T) {
	app := termApplication{}
	if got := app.EditFilePath(); got != editFilePath {
		t.Fatalf("EditFilePath() = %q, want package value %q", got, editFilePath)
	}
}

func TestTermApplicationViewFilePathsCoverageBatch32(t *testing.T) {
	app := termApplication{}
	if got := app.ViewFilePaths(); !reflect.DeepEqual(got, viewFilePaths) {
		t.Fatalf("ViewFilePaths() = %#v, want package value %#v", got, viewFilePaths)
	}
}

func TestTermApplicationStartupDirsCoverageBatch32(t *testing.T) {
	app := termApplication{}
	left, right := app.StartupDirs()
	wantLeft, wantRight := startupDirs()
	if left != wantLeft || right != wantRight {
		t.Fatalf("StartupDirs() = (%q, %q), want (%q, %q)", left, right, wantLeft, wantRight)
	}
}

func TestTermApplicationStartupDirsEnvironmentCoverageBatch32(t *testing.T) {
	t.Setenv(startupDirEnv, "left-from-test")
	t.Setenv(startupDirRightEnv, "right-from-test")
	left, right := (termApplication{}).StartupDirs()
	if left != "left-from-test" || right != "right-from-test" {
		t.Fatalf("StartupDirs() = (%q, %q), want test environment", left, right)
	}
}

func TestTermApplicationDecodeImageEmptyCoverageBatch32(t *testing.T) {
	if _, err := (termApplication{}).DecodeImage(nil); err == nil {
		t.Fatal("DecodeImage(nil) returned nil error")
	}
}

func TestTermApplicationDecodeImageInvalidCoverageBatch32(t *testing.T) {
	if _, err := (termApplication{}).DecodeImage([]byte("not an image")); err == nil {
		t.Fatal("DecodeImage(invalid data) returned nil error")
	}
}

func TestTermApplicationOpenStartupFilesEmptyCoverageBatch32(t *testing.T) {
	oldEdit, oldViews := editFilePath, viewFilePaths
	editFilePath, viewFilePaths = "", nil
	t.Cleanup(func() { editFilePath, viewFilePaths = oldEdit, oldViews })
	(termApplication{}).OpenStartupFiles()
}

func TestTermApplicationOpenStartupFilesNoEditCoverageBatch32(t *testing.T) {
	oldEdit, oldViews := editFilePath, viewFilePaths
	editFilePath, viewFilePaths = "", []string{}
	t.Cleanup(func() { editFilePath, viewFilePaths = oldEdit, oldViews })
	(termApplication{}).OpenStartupFiles()
}
