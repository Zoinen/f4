package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/vtinput"
)

func TestShellSingleQuoteCoverageBatch22(t *testing.T) {
	if got, want := ShellSingleQuote("a'b"), "'a'\\''b'"; got != want {
		t.Fatalf("ShellSingleQuote = %q, want %q", got, want)
	}
}

func TestWorkspaceCommandNameCoverageBatch22(t *testing.T) {
	cases := []struct {
		command, want string
	}{
		{"", "Terminal"},
		{"python3 --version", "Python"},
		{"\"/opt/tools/worker.py\" --check", "worker"},
		{"/usr/bin/f4.exe --debug", "f4"},
	}
	for _, tc := range cases {
		if got := workspaceCommandName(tc.command); got != tc.want {
			t.Errorf("workspaceCommandName(%q) = %q, want %q", tc.command, got, tc.want)
		}
	}
}

func TestSameFolderHistoryPathCoverageBatch22(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"", "/tmp", false},
		{"/tmp/one/../folder", "/tmp/folder", true},
		{"/tmp/one", "/tmp/two", false},
	}
	for _, tc := range cases {
		if got := SameFolderHistoryPath(tc.a, tc.b); got != tc.want {
			t.Errorf("SameFolderHistoryPath(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestFolderHistoryStepCoverageBatch22(t *testing.T) {
	history := []string{"/three", "/two", "/one"}
	if pos, path, ok := FolderHistoryStep(history, "/two", 1, -1); !ok || pos != 2 || path != "/one" {
		t.Fatalf("backward history step = (%d, %q, %v)", pos, path, ok)
	}
	if pos, path, ok := FolderHistoryStep(history, "/two", 1, 1); !ok || pos != 0 || path != "/three" {
		t.Fatalf("forward history step = (%d, %q, %v)", pos, path, ok)
	}
	if _, _, ok := FolderHistoryStep(history, "/missing", -1, 1); ok {
		t.Fatal("history step unexpectedly found a missing current path")
	}
	if _, _, ok := FolderHistoryStep(history, "/two", 1, 0); ok {
		t.Fatal("zero-direction history step unexpectedly succeeded")
	}
}

func TestParseDirChangeCommandCoverageBatch22(t *testing.T) {
	cases := []struct {
		command, want string
		ok            bool
	}{
		{"cd /tmp/work", "/tmp/work", true},
		{"chdir 'folder with spaces'", "folder with spaces", true},
		{"cd..", "..", true},
		{"cd/", string(os.PathSeparator), true},
		{"echo /tmp", "", false},
	}
	for _, tc := range cases {
		got, ok := parseDirChangeCommand(tc.command)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseDirChangeCommand(%q) = (%q, %v), want (%q, %v)", tc.command, got, ok, tc.want, tc.ok)
		}
	}
}

func TestParsePlainEditCommandCoverageBatch22(t *testing.T) {
	cases := []struct {
		command, want string
		ok            bool
	}{
		{" edit: notes.txt ", "notes.txt", true},
		{"EDIT:/tmp/report", "/tmp/report", true},
		{"edit:", "", false},
		{"edit:<<capture", "", false},
		{"view:notes.txt", "", false},
	}
	for _, tc := range cases {
		got, ok := parsePlainEditCommand(tc.command)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parsePlainEditCommand(%q) = (%q, %v), want (%q, %v)", tc.command, got, ok, tc.want, tc.ok)
		}
	}
}

func TestExpandPathEnvCoverageBatch22(t *testing.T) {
	const name = "F4_COVERAGE_BATCH22"
	t.Setenv(name, filepath.Join("workspace", "value"))
	if got, want := ExpandPathEnv("$F4_COVERAGE_BATCH22/${F4_COVERAGE_BATCH22}/%F4_COVERAGE_BATCH22%"), filepath.Join("workspace", "value")+"/"+filepath.Join("workspace", "value")+"/"+filepath.Join("workspace", "value"); got != want {
		t.Fatalf("ExpandPathEnv with environment forms = %q, want %q", got, want)
	}
	if got := ExpandPathEnv("$F4_COVERAGE_BATCH22_UNKNOWN"); got != "$F4_COVERAGE_BATCH22_UNKNOWN" {
		t.Fatalf("unknown environment variable was changed to %q", got)
	}
}

func TestTerminalWantsMouseEventCoverageBatch22(t *testing.T) {
	if terminalWantsMouseEvent(0, nil) {
		t.Fatal("disabled mouse tracking accepted a nil event")
	}
	moved := &vtinput.InputEvent{MouseEventFlags: vtinput.MouseMoved}
	if terminalWantsMouseEvent(1000, moved) || terminalWantsMouseEvent(1002, moved) {
		t.Fatal("button-only tracking accepted hover motion")
	}
	if !terminalWantsMouseEvent(1002, &vtinput.InputEvent{MouseEventFlags: vtinput.MouseMoved, ButtonState: 1}) {
		t.Fatal("button tracking rejected motion with a button held")
	}
	if !terminalWantsMouseEvent(1003, moved) || !terminalWantsMouseEvent(1000, &vtinput.InputEvent{}) {
		t.Fatal("mouse tracking rejected a supported event")
	}
}

func TestCommandFocusToggleKeyCoverageBatch22(t *testing.T) {
	cases := []*vtinput.InputEvent{
		{VirtualKeyCode: vtinput.VK_OEM_3},
		{Char: '`'},
		{Char: 'ё'},
	}
	for _, event := range cases {
		if !isCommandFocusToggleKey(event) {
			t.Errorf("event %#v was not recognized as command-focus toggle", event)
		}
	}
	if isCommandFocusToggleKey(&vtinput.InputEvent{Char: 'x'}) {
		t.Fatal("ordinary character was recognized as command-focus toggle")
	}
}

func TestMenuCheckmarkTextCoverageBatch22(t *testing.T) {
	if got := getMenuText(ViewModeBrief, ViewModeBrief, "Brief"); got != "√Brief" {
		t.Fatalf("selected view menu text = %q", got)
	}
	if got := getSortMenuText(SortName, SortSize, "Size"); got != " Size" {
		t.Fatalf("unselected sort menu text = %q", got)
	}
	if got := getToggleMenuText(false, "Groups"); got != " Groups" {
		t.Fatalf("off toggle menu text = %q", got)
	}
	if got := getToggleMenuText(true, "Groups"); got != "√Groups" {
		t.Fatalf("on toggle menu text = %q", got)
	}
}
