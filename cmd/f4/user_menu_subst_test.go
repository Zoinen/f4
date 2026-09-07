package main

import (
	"os"
	"strings"
	"testing"
)

func baseCtx() *SubstContext {
	return &SubstContext{
		Active: PanelSnapshot{
			CurDir:      "/home/me/work",
			CurrentFile: "main.go",
			Marked:      []string{"main.go", "util.go"},
		},
		Passive: PanelSnapshot{
			CurDir:      "/tmp",
			CurrentFile: "report.txt",
			Marked:      []string{"report.txt"},
		},
	}
}

func subst(t *testing.T, cmd string, ctx *SubstContext) SubstResult {
	t.Helper()
	return SubstFileName(cmd, ctx)
}

func TestSubst_DotFile(t *testing.T) {
	r := subst(t, `cat !.!`, baseCtx())
	if r.Command != "cat main.go" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_LiteralBang(t *testing.T) {
	r := subst(t, `echo !! works`, baseCtx())
	if r.Command != "echo ! works" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_DoubleLiteralBang(t *testing.T) {
	// Adjacent !! pairs must not interfere with each other.
	r := subst(t, `!!!.!!!`, baseCtx())
	if r.Command != "!main.go!" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_Extension(t *testing.T) {
	r := subst(t, "echo !`!", baseCtx())
	if r.Command != "echo .go" {
		t.Fatalf("got %q", r.Command)
	}
}
func TestSubst_NameWithoutExtension(t *testing.T) {
	r := subst(t, "echo !~!", baseCtx())
	if r.Command != "echo main" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_DirNoTrailingSlash(t *testing.T) {
	ctx := baseCtx()
	ctx.Active.CurDir = "/home/me/work/"
	r := subst(t, "echo !/!", ctx)
	if r.Command != "echo /home/me/work" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_DriveLetter(t *testing.T) {
	ctx := baseCtx()
	ctx.Active.CurDir = "C:\\Windows\\System32"
	r := subst(t, "echo !:", ctx)
	if r.Command != "echo C:" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_ExtensionNoFile(t *testing.T) {
	ctx := baseCtx()
	ctx.Active.CurrentFile = ""
	r := subst(t, "echo !`!.", ctx)
	if r.Command != "echo ." {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_CurDir(t *testing.T) {
	r := subst(t, `cd !\!`, baseCtx())
	if r.Command != "cd /home/me/work" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_MarkedSpaceJoined(t *testing.T) {
	r := subst(t, "tar czf out.tar.gz !&", baseCtx())
	if r.Command != "tar czf out.tar.gz main.go util.go" {
		t.Fatalf("got %q", r.Command)
	}
	if !r.ListFiles {
		t.Errorf("ListFiles flag should be set")
	}
}

func TestSubst_MarkedFallbackToCurrent(t *testing.T) {
	ctx := baseCtx()
	ctx.Active.Marked = nil
	r := subst(t, "cat !&", ctx)
	if r.Command != "cat main.go" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_PassiveActiveToggle(t *testing.T) {
	r := subst(t, `cp !.! !#!\!/!.!`, baseCtx())
	// !.!=active main.go; then !# switches; !\!=passive /tmp; !.!=passive report.txt
	if r.Command != "cp main.go /tmp/report.txt" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_PassiveToggleAndBack(t *testing.T) {
	r := subst(t, `!.! !# !.! !^ !.!`, baseCtx())
	// active main.go, switch, passive report.txt, switch back, active main.go
	if r.Command != "main.go  report.txt  main.go" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_PromptAccepted(t *testing.T) {
	ctx := baseCtx()
	ctx.AskUser = func(title, init string) (string, bool) {
		if title != "name" {
			t.Errorf("title=%q", title)
		}
		if init != "default" {
			t.Errorf("init=%q", init)
		}
		return "answer", true
	}
	r := subst(t, `echo !?name?default!`, ctx)
	if r.Command != "echo answer" || r.Cancelled {
		t.Fatalf("got %q cancelled=%v", r.Command, r.Cancelled)
	}
}

func TestSubst_PromptCancelled(t *testing.T) {
	ctx := baseCtx()
	ctx.AskUser = func(title, init string) (string, bool) {
		return "", false
	}
	r := subst(t, `echo !?name?default!`, ctx)
	if !r.Cancelled {
		t.Fatalf("expected Cancelled, got %#v", r)
	}
}

func TestSubst_PromptNoCallbackUsesInit(t *testing.T) {
	r := subst(t, `echo !?name?defaultval!`, baseCtx())
	if r.Command != "echo defaultval" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_PromptTitleOnly(t *testing.T) {
	// "?init" half is optional; far2l treats the whole body as title.
	r := subst(t, `echo !?just title!`, baseCtx())
	if r.Command != "echo " {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_PromptUnterminatedPassesThrough(t *testing.T) {
	// "!?foo" with no closing '!' is malformed — the '!' must not be
	// silently eaten.
	r := subst(t, `echo !?broken`, baseCtx())
	if r.Command != "echo !?broken" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_EnvironmentReferencesPassThrough(t *testing.T) {
	t.Setenv("F4_TEST_VAR", "Hello")
	r := subst(t, `echo $F4_TEST_VAR ${F4_TEST_VAR}!`, baseCtx())
	if r.Command != `echo $F4_TEST_VAR ${F4_TEST_VAR}!` {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_EnvironmentReferencesRemainAfterTokens(t *testing.T) {
	// Shell variables must remain available after f4's own !X! tokens resolve.
	t.Setenv("F4_TEST_VAR", "real_main.go")
	r := subst(t, `cp !.! $F4_TEST_VAR`, baseCtx())
	if r.Command != `cp main.go $F4_TEST_VAR` {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_MarkedListTempFile(t *testing.T) {
	ctx := baseCtx()
	ctx.MarkedListTempDir = t.TempDir()
	r := subst(t, `xargs cat <!@!`, ctx)
	if len(r.TempFiles) != 1 {
		t.Fatalf("expected one temp file, got %d", len(r.TempFiles))
	}
	if !strings.Contains(r.Command, r.TempFiles[0]) {
		t.Fatalf("temp path not in command: %q", r.Command)
	}
	data, err := os.ReadFile(r.TempFiles[0])
	if err != nil {
		t.Fatalf("read temp: %v", err)
	}
	got := string(data)
	if got != "main.go\nutil.go\n" {
		t.Fatalf("temp file contents %q", got)
	}
}

func TestSubst_UnrecognizedTokenPassesThrough(t *testing.T) {
	// Tokens we don't support must NOT be eaten — pass them through so
	// they reach the shell verbatim (no surprising data loss).
	r := subst(t, `echo !~ !-! !+!`, baseCtx())
	if r.Command != "echo !~ !-! !+!" {
		t.Fatalf("got %q", r.Command)
	}
}

func TestSubst_NilContext(t *testing.T) {
	r := SubstFileName(`echo !.!`, nil)
	if r.Command != "echo !.!" {
		t.Fatalf("nil context should return command unchanged, got %q", r.Command)
	}
}

func TestSubst_RealWorldExample(t *testing.T) {
	// From the user's actual far2l user_menu.ini.
	ctx := baseCtx()
	ctx.AskUser = func(title, init string) (string, bool) {
		if title != "test path" {
			t.Errorf("title=%q", title)
		}
		return "tests/test_users.py::test_login", true
	}
	cmd := `source .venv/bin/activate && pytest -v "!?test path?tests/test_X.py::test_Y!"`
	r := subst(t, cmd, ctx)
	want := `source .venv/bin/activate && pytest -v "tests/test_users.py::test_login"`
	if r.Command != want {
		t.Fatalf("got %q", r.Command)
	}
}
