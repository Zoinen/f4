package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStartupDirsForCoverageBatch34(t *testing.T) {
	cwd := t.TempDir()
	abs := filepath.Join(cwd, "absolute")
	cases := []struct {
		name      string
		args      []string
		wantLeft  string
		wantRight string
	}{
		{name: "none", wantLeft: cwd},
		{name: "one relative", args: []string{"left"}, wantLeft: filepath.Join(cwd, "left"), wantRight: cwd},
		{name: "one absolute", args: []string{abs}, wantLeft: abs, wantRight: cwd},
		{name: "two", args: []string{"left", abs}, wantLeft: filepath.Join(cwd, "left"), wantRight: abs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left, right := startupDirsFor(cwd, tc.args)
			if left != tc.wantLeft || right != tc.wantRight {
				t.Fatalf("startupDirsFor(%q, %q) = (%q, %q), want (%q, %q)", cwd, tc.args, left, right, tc.wantLeft, tc.wantRight)
			}
		})
	}
}

func TestStartupDirsOverrideCoverageBatch34(t *testing.T) {
	cwd := t.TempDir()
	if left, right, ok := startupDirsOverride(cwd, nil, false); ok || left != "" || right != "" {
		t.Fatalf("empty override disabled = (%q, %q, %v), want empty and false", left, right, ok)
	}
	if left, right, ok := startupDirsOverride(cwd, nil, true); !ok || left != cwd || right != "" {
		t.Fatalf("empty override enabled = (%q, %q, %v), want cwd and true", left, right, ok)
	}
	if left, right, ok := startupDirsOverride(cwd, []string{"dir"}, false); !ok || left != filepath.Join(cwd, "dir") || right != cwd {
		t.Fatalf("explicit override = (%q, %q, %v), want explicit paths", left, right, ok)
	}
}

func TestFarStartupDirsCoverageBatch34(t *testing.T) {
	cwd := t.TempDir()
	if left, right, ok := farStartupDirs(cwd, nil); ok || left != "" || right != "" {
		t.Fatalf("far empty = (%q, %q, %v), want empty and false", left, right, ok)
	}
	if left, right, ok := farStartupDirs(cwd, []string{"left"}); !ok || left != filepath.Join(cwd, "left") || right != "-" {
		t.Fatalf("far one = (%q, %q, %v), want left and keep marker", left, right, ok)
	}
	if left, right, ok := farStartupDirs(cwd, []string{"left", filepath.Join(cwd, "right")}); !ok || left != filepath.Join(cwd, "left") || right != filepath.Join(cwd, "right") {
		t.Fatalf("far two = (%q, %q, %v), want both paths", left, right, ok)
	}
}

func TestStartupDirsChoiceCoverageBatch34(t *testing.T) {
	cwd := t.TempDir()
	if left, right, ok := startupDirsChoice(cwd, []string{"dir"}, true); !ok || left != filepath.Join(cwd, "dir") || right != cwd {
		t.Fatalf("current-folder choice = (%q, %q, %v), want override", left, right, ok)
	}
	if left, right, ok := startupDirsChoice(cwd, []string{"dir"}, false); !ok || left != filepath.Join(cwd, "dir") || right != "-" {
		t.Fatalf("far choice = (%q, %q, %v), want far startup", left, right, ok)
	}
}

func TestStartupDirArgsCoverageBatch34(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{args: []string{"left", "right", "--gui", "ignored"}, want: []string{"left", "right"}},
		{args: []string{"--tty", "ignored", "left"}, want: nil},
		{args: []string{"--gui", "ignored", "--", "after", "--switch"}, want: []string{"after", "--switch"}},
	}
	for _, tc := range cases {
		if got := startupDirArgs(tc.args); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("startupDirArgs(%q) = %#v, want %#v", tc.args, got, tc.want)
		}
	}
}

func TestResolveStartupPathCoverageBatch34(t *testing.T) {
	cwd := t.TempDir()
	if got := resolveStartupPath(cwd, "a/../note.txt"); got != filepath.Join(cwd, "note.txt") {
		t.Fatalf("relative resolve = %q, want %q", got, filepath.Join(cwd, "note.txt"))
	}
	abs := filepath.Join(cwd, "a", "..", "absolute.txt")
	if got := resolveStartupPath(cwd, abs); got != filepath.Join(cwd, "absolute.txt") {
		t.Fatalf("absolute resolve = %q, want %q", got, filepath.Join(cwd, "absolute.txt"))
	}
}

func TestStartupViewFilesCoverageBatch34(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "note.txt")
	subdir := filepath.Join(dir, "folder")
	if err := os.WriteFile(file, []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(subdir, 0700); err != nil {
		t.Fatal(err)
	}
	want := []string{file}
	if got := startupViewFiles(dir, []string{"note.txt", "folder", "missing"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("startupViewFiles = %#v, want %#v", got, want)
	}
}

func TestSudoDispatcherPathCoverageBatch34(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{args: []string{"--sudo-dispatcher", "/tmp/dispatcher"}, want: "/tmp/dispatcher"},
		{args: []string{"--sudo-dispatcher"}, want: ""},
		{args: []string{"--sudo-dispatcher=/tmp/equal"}, want: "/tmp/equal"},
		{args: []string{"--tty"}, want: ""},
	}
	for _, tc := range cases {
		if got := sudoDispatcherPath(tc.args); got != tc.want {
			t.Fatalf("sudoDispatcherPath(%q) = %q, want %q", tc.args, got, tc.want)
		}
	}
}

func TestSudoStartupModeCoverageBatch34(t *testing.T) {
	if dispatcher, askpass := sudoStartupMode([]string{"--sudo-dispatcher", "/tmp/d"}, true); dispatcher != "/tmp/d" || askpass {
		t.Fatalf("dispatcher mode = (%q, %v), want dispatcher and no askpass", dispatcher, askpass)
	}
	if dispatcher, askpass := sudoStartupMode(nil, true); dispatcher != "" || !askpass {
		t.Fatalf("askpass mode = (%q, %v), want askpass", dispatcher, askpass)
	}
	if dispatcher, askpass := sudoStartupMode(nil, false); dispatcher != "" || askpass {
		t.Fatalf("normal mode = (%q, %v), want neither", dispatcher, askpass)
	}
}

func TestStartupDirsEnvironmentCoverageBatch34(t *testing.T) {
	t.Setenv(startupDirEnv, "left")
	t.Setenv(startupDirRightEnv, "right")
	if left, right := startupDirs(); left != "left" || right != "right" {
		t.Fatalf("startupDirs() = (%q, %q), want environment values", left, right)
	}
}
