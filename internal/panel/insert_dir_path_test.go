package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/cmdline"
)

func TestPathWithTrailingSeparator(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"/", "/"},
		{"/tmp", "/tmp/"},
		{"/tmp/", "/tmp/"},
		{`C:\`, `C:\`},
		{`C:\F4\build`, `C:\F4\build\`},
		{`C:`, `C:\`},
		{`C:/F4`, `C:/F4/`},
		{`\dir`, `\dir\`},
		{`\\server\share`, `\\server\share\`},
		{"sftp://host/tmp", "sftp://host/tmp/"},
		{"disks://", "disks://"},
	}
	for _, tc := range cases {
		if got := pathWithTrailingSeparator(tc.in); got != tc.want {
			t.Errorf("pathWithTrailingSeparator(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The path hotkeys must always leave the command line at a separator, even
// when shell quoting wraps the path: the separator is what tells the user
// (and the next keystroke) that a name can follow.
func TestInsertDirPathToCmdLineEndsWithSeparator(t *testing.T) {
	paths := []string{
		"/plain/dir",
		"/dir with spaces",
		`C:\F4\build`,
		`C:\dir with spaces`,
	}
	for _, path := range paths {
		pf := &PanelsFrame{CmdLine: cmdline.NewCommandLine(">")}
		pf.InsertDirPathToCmdLine(path)

		wantSep := pathWithTrailingSeparator(path)
		sep := wantSep[len(wantSep)-1]
		got := pf.CmdLine.Edit.GetText()
		if got == "" {
			t.Errorf("InsertDirPathToCmdLine(%q) left the command line empty", path)
			continue
		}
		if last := got[len(got)-1]; last != sep {
			t.Errorf("InsertDirPathToCmdLine(%q) = %q, want it to end with %q", path, got, string(sep))
		}
	}
}

// A path that already ends in a separator is quoted without swallowing it:
// a lone backslash before the closing quote escapes that quote for the
// Microsoft C runtime.
func TestInsertPathToCmdLineKeepsTrailingSeparatorOutOfQuotes(t *testing.T) {
	pf := &PanelsFrame{CmdLine: cmdline.NewCommandLine(">")}
	pf.InsertPathToCmdLine(`C:\dir with spaces\`)

	got := pf.CmdLine.Edit.GetText()
	if !strings.HasSuffix(got, `\`) {
		t.Fatalf("InsertPathToCmdLine = %q, want a trailing backslash", got)
	}
	if strings.Contains(got, `\"`) {
		t.Fatalf("InsertPathToCmdLine = %q, trailing backslash escaped the closing quote", got)
	}
}

func TestParseDirChangeCommandStripsQuotedTrailingSeparator(t *testing.T) {
	cases := []struct{ command, want string }{
		{`cd "C:\dir with spaces"\`, `C:\dir with spaces\`},
		{`cd '/tmp/a b'/`, `/tmp/a b/`},
		{`chdir 'folder with spaces'`, `folder with spaces`},
		{`cd "C:\plain"`, `C:\plain`},
		// Only a separator may follow the closing quote; anything else is
		// the user's own business and stays as typed.
		{`cd "C:\plain"X`, `"C:\plain"X`},
	}
	for _, tc := range cases {
		got, ok := parseDirChangeCommand(tc.command)
		if !ok {
			t.Errorf("parseDirChangeCommand(%q) was not recognized", tc.command)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDirChangeCommand(%q) = %q, want %q", tc.command, got, tc.want)
		}
	}
}
