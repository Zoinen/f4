package panel

import "testing"

func substCtx() *SubstContext {
	return &SubstContext{
		Active:  PanelSnapshot{CurDir: "/home/user/docs", CurrentFile: "report.txt"},
		Passive: PanelSnapshot{CurDir: "/tmp/dest", CurrentFile: "other.txt"},
	}
}

func TestSubstFileName_Tokens(t *testing.T) {
	cases := []struct {
		name string
		cmd  string
		want string
	}{
		{"filename", "cat !.!", "cat report.txt"},
		{"active dir bare, no closing bang", "ls -la !/ > dir.txt", "ls -la /home/user/docs > dir.txt"},
		{"active dir bare at end of command", "cd !/", "cd /home/user/docs"},
		{"active dir closed form unchanged", "cd !/!", "cd /home/user/docs"},
		{"backslash bare form", "cd !\\", "cd /home/user/docs"},
		{"backslash closed form unchanged", "cd !\\!", "cd /home/user/docs"},
		// f4 #1395: !# switches to the passive panel; the reporter's exact
		// template ("!#!/ ") needs the bare !/ form to work right after it.
		{"passive switch then bare dir (f4 #1395)", "ls -la !#!/ > dir.txt", "ls -la /tmp/dest > dir.txt"},
		{"literal bang", "echo !!", "echo !"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SubstFileName(tc.cmd, substCtx())
			if got.Command != tc.want {
				t.Errorf("SubstFileName(%q) = %q, want %q", tc.cmd, got.Command, tc.want)
			}
		})
	}
}

func TestTrimTrailingSlash(t *testing.T) {
	cases := map[string]string{
		"/mnt/data/": "/mnt/data",
		"/mnt/data":  "/mnt/data",
		"/":          "/",
		`C:\`:        `C:`,
	}
	for in, want := range cases {
		if got := trimTrailingSlash(in); got != want {
			t.Errorf("trimTrailingSlash(%q) = %q, want %q", in, got, want)
		}
	}
}
