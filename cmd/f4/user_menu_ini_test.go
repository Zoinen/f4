package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "main_menu.ini")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestLoadMainMenu_MissingFile(t *testing.T) {
	items, err := LoadMainMenu(filepath.Join(t.TempDir(), "does_not_exist.ini"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected empty slice for missing file, got %v", items)
	}
}

func TestLoadMainMenu_FlatLeafItem(t *testing.T) {
	p := writeTemp(t, `[UserMenu/MainMenu/Item0]
Command0=code .
HotKey=c
Label=open VSCode
Submenu=0
`)
	items, err := LoadMainMenu(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []UserMenuItem{{
		HotKey:   "c",
		Label:    "open VSCode",
		Commands: []string{"code ."},
	}}
	if !reflect.DeepEqual(items, want) {
		t.Fatalf("got %#v, want %#v", items, want)
	}
}

func TestLoadMainMenu_MultipleCommands(t *testing.T) {
	p := writeTemp(t, `[UserMenu/MainMenu/Item0]
Command0=cd /tmp
Command1=ls -la
Command2=pwd
HotKey=m
Label=multi
Submenu=0
`)
	items, _ := LoadMainMenu(p)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	got := items[0].Commands
	want := []string{"cd /tmp", "ls -la", "pwd"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commands: got %v, want %v", got, want)
	}
}

func TestLoadMainMenu_Submenu(t *testing.T) {
	p := writeTemp(t, `[UserMenu/MainMenu/Item0]
HotKey=t
Label=tmux
Submenu=1

[UserMenu/MainMenu/Item0/Item0]
Command0=tmux ls
HotKey=l
Label=list
Submenu=0

[UserMenu/MainMenu/Item0/Item1]
Command0=tmux attach
HotKey=a
Label=attach
Submenu=0
`)
	items, _ := LoadMainMenu(p)
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	parent := items[0]
	if parent.HotKey != "t" || parent.Label != "tmux" {
		t.Fatalf("parent fields wrong: %#v", parent)
	}
	if !parent.IsSubmenu() {
		t.Fatalf("expected submenu, got leaf")
	}
	if len(parent.Submenu) != 2 {
		t.Fatalf("want 2 children, got %d", len(parent.Submenu))
	}
	if parent.Submenu[0].Label != "list" || parent.Submenu[1].Label != "attach" {
		t.Fatalf("children order/labels wrong: %#v", parent.Submenu)
	}
	if parent.Submenu[0].Commands[0] != "tmux ls" {
		t.Fatalf("child[0] command: %q", parent.Submenu[0].Commands[0])
	}
}

func TestLoadMainMenu_NestedSubmenus(t *testing.T) {
	p := writeTemp(t, `[UserMenu/MainMenu/Item0]
HotKey=a
Label=A
Submenu=1

[UserMenu/MainMenu/Item0/Item0]
HotKey=b
Label=B
Submenu=1

[UserMenu/MainMenu/Item0/Item0/Item0]
Command0=echo deep
HotKey=c
Label=C
Submenu=0
`)
	items, _ := LoadMainMenu(p)
	if len(items) != 1 || !items[0].IsSubmenu() {
		t.Fatalf("top: %#v", items)
	}
	if !items[0].Submenu[0].IsSubmenu() {
		t.Fatalf("mid: %#v", items[0].Submenu)
	}
	leaf := items[0].Submenu[0].Submenu[0]
	if leaf.IsSubmenu() {
		t.Fatalf("expected leaf at depth 3")
	}
	if leaf.Label != "C" || leaf.Commands[0] != "echo deep" {
		t.Fatalf("leaf wrong: %#v", leaf)
	}
}

func TestLoadMainMenu_SkipsForeignSections(t *testing.T) {
	p := writeTemp(t, `[Some/Other/Section]
Key=value

[UserMenu/MainMenu/Item0]
Command0=ok
HotKey=x
Label=mine
Submenu=0

[Unrelated]
Foo=bar
`)
	items, _ := LoadMainMenu(p)
	if len(items) != 1 || items[0].Label != "mine" {
		t.Fatalf("foreign sections not skipped: %#v", items)
	}
}

func TestLoadMainMenu_StopsAtMissingIndex(t *testing.T) {
	// Item0 and Item2 present, Item1 missing => Item2 should NOT be picked up.
	p := writeTemp(t, `[UserMenu/MainMenu/Item0]
Command0=a
HotKey=a
Label=A
Submenu=0

[UserMenu/MainMenu/Item2]
Command0=c
HotKey=c
Label=C
Submenu=0
`)
	items, _ := LoadMainMenu(p)
	if len(items) != 1 {
		t.Fatalf("indexing must be contiguous; got %d items: %#v", len(items), items)
	}
}

func TestLoadMainMenu_EmptySubmenu(t *testing.T) {
	p := writeTemp(t, `[UserMenu/MainMenu/Item0]
HotKey=e
Label=empty
Submenu=1
`)
	items, _ := LoadMainMenu(p)
	if len(items) != 1 || !items[0].IsSubmenu() {
		t.Fatalf("expected one submenu item, got %#v", items)
	}
	if len(items[0].Submenu) != 0 {
		t.Fatalf("expected zero children, got %d", len(items[0].Submenu))
	}
}

func TestLoadMainMenu_Separator(t *testing.T) {
	p := writeTemp(t, `[UserMenu/MainMenu/Item0]
HotKey=--
Label=
Submenu=0
`)
	items, _ := LoadMainMenu(p)
	if len(items) != 1 || !items[0].IsSeparator() {
		t.Fatalf("expected separator, got %#v", items)
	}
}

func TestSaveMainMenu_RoundTrip(t *testing.T) {
	in := []UserMenuItem{
		{HotKey: "c", Label: "VSCode", Commands: []string{"code ."}},
		{HotKey: "t", Label: "tmux", Submenu: []UserMenuItem{
			{HotKey: "l", Label: "list", Commands: []string{"tmux ls"}},
			{HotKey: "a", Label: "attach", Commands: []string{"tmux attach -t main"}},
		}},
		{HotKey: "--", Label: "", Commands: []string{}},
		{HotKey: "x", Label: "exit", Commands: []string{"exit"}},
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "out.ini")
	if err := SaveMainMenu(p, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := LoadMainMenu(p)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	// Normalize the empty-Commands separator: load returns nil for
	// "no Command keys present", we wrote with an empty slice.
	in[2].Commands = nil
	if !reflect.DeepEqual(out, in) {
		t.Fatalf("round-trip mismatch:\ngot  %#v\nwant %#v", out, in)
	}
}

func TestSaveMainMenu_DeterministicAndAlphabetical(t *testing.T) {
	in := []UserMenuItem{
		{HotKey: "c", Label: "VSCode", Commands: []string{"code ."}},
	}
	p := filepath.Join(t.TempDir(), "det.ini")
	if err := SaveMainMenu(p, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	data, _ := os.ReadFile(p)
	want := `[UserMenu/MainMenu/Item0]
Command0=code .
HotKey=c
Label=VSCode
Submenu=0
`
	if string(data) != want {
		t.Fatalf("output not deterministic/alphabetical:\nGOT:\n%s\nWANT:\n%s", data, want)
	}
}

func TestSaveMainMenu_BlankLineBetweenSections(t *testing.T) {
	in := []UserMenuItem{
		{HotKey: "a", Label: "A", Commands: []string{"a"}},
		{HotKey: "b", Label: "B", Commands: []string{"b"}},
	}
	p := filepath.Join(t.TempDir(), "blanks.ini")
	_ = SaveMainMenu(p, in)
	data, _ := os.ReadFile(p)
	if !strings.Contains(string(data), "Submenu=0\n\n[UserMenu/MainMenu/Item1]") {
		t.Fatalf("expected blank line between sibling sections, got:\n%s", data)
	}
}

func TestSaveMainMenu_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "deeper", "main_menu.ini")
	if err := SaveMainMenu(p, []UserMenuItem{{HotKey: "a", Label: "a", Commands: []string{"a"}}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}

// Real-world fixture: a verbatim slice of the user's actual far2l user_menu.ini.
// This is the bug-for-bug compatibility check.
const realFar2lFixture = `[UserMenu/MainMenu/Item0]
Command0=code .
HotKey=c
Label=открыть VSCode в текущем пути в WSL-режиме
Submenu=0

[UserMenu/MainMenu/Item1]
Command0=claude
HotKey=a
Label=claude code
Submenu=0

[UserMenu/MainMenu/Item2]
HotKey=t
Label=tmux
Submenu=1

[UserMenu/MainMenu/Item2/Item0]
Command0=bash ~/sledy/start-all.sh
HotKey=S
Label=start ALL (redis + бэк + фронт в tmux, без блокировки)
Submenu=0

[UserMenu/MainMenu/Item2/Item1]
Command0=bash ~/sledy/stop-all.sh
HotKey=K
Label=kill ALL (остановить всё)
Submenu=0

[UserMenu/MainMenu/Item3]
HotKey=b
Label=backend
Submenu=1

[UserMenu/MainMenu/Item3/Item0]
Command0=cd /home/sogonov/sledy/go-basa.backend
Command1=source .venv/bin/activate && uvicorn app.main:app --reload
HotKey=r
Label=run uvicorn dev
Submenu=0

[UserMenu/MainMenu/Item3/Item1]
Command0=cd /home/sogonov/sledy/go-basa.backend
Command1=source .venv/bin/activate && pytest -v "!?test path?tests/test_X.py::test_Y!"
HotKey=o
Label=pytest один тест (с подсказкой)
Submenu=0
`

func TestLoadMainMenu_RealFar2lFile(t *testing.T) {
	p := writeTemp(t, realFar2lFixture)
	items, err := LoadMainMenu(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("want 4 top-level items, got %d", len(items))
	}

	// Spot-check unicode label.
	if items[0].Label != "открыть VSCode в текущем пути в WSL-режиме" {
		t.Fatalf("unicode label corrupted: %q", items[0].Label)
	}

	// Submenu structure.
	if !items[2].IsSubmenu() || len(items[2].Submenu) != 2 {
		t.Fatalf("tmux submenu: %#v", items[2])
	}

	// Multi-command and embedded "!?...!" substitution token must survive.
	pytest := items[3].Submenu[1]
	if pytest.HotKey != "o" || len(pytest.Commands) != 2 {
		t.Fatalf("pytest item: %#v", pytest)
	}
	if !strings.Contains(pytest.Commands[1], `"!?test path?tests/test_X.py::test_Y!"`) {
		t.Fatalf("substitution token mangled: %q", pytest.Commands[1])
	}
}

func TestSaveMainMenu_RealFar2lFile_RoundTripSemantics(t *testing.T) {
	// Save -> load -> compare. We don't require byte-identical output to
	// the original (far2l's whitespace varies), only structural equality.
	src := writeTemp(t, realFar2lFixture)
	items, err := LoadMainMenu(src)
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "out.ini")
	if err := SaveMainMenu(dst, items); err != nil {
		t.Fatalf("save: %v", err)
	}
	items2, err := LoadMainMenu(dst)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !reflect.DeepEqual(items, items2) {
		t.Fatalf("structural round-trip failed:\nfirst:  %#v\nsecond: %#v", items, items2)
	}
}

func TestUserMenuItem_IsSeparator(t *testing.T) {
	cases := []struct {
		it   UserMenuItem
		want bool
	}{
		{UserMenuItem{HotKey: "--"}, true},
		{UserMenuItem{HotKey: "-"}, false},
		{UserMenuItem{HotKey: ""}, false},
		{UserMenuItem{HotKey: "a"}, false},
	}
	for _, c := range cases {
		if got := c.it.IsSeparator(); got != c.want {
			t.Errorf("IsSeparator(%q) = %v, want %v", c.it.HotKey, got, c.want)
		}
	}
}

func TestUserMenuItem_IsSubmenu(t *testing.T) {
	leaf := UserMenuItem{HotKey: "a", Commands: []string{"x"}}
	emptySub := UserMenuItem{HotKey: "s", Submenu: []UserMenuItem{}}
	fullSub := UserMenuItem{HotKey: "s", Submenu: []UserMenuItem{{HotKey: "x"}}}
	if leaf.IsSubmenu() {
		t.Errorf("leaf reported as submenu")
	}
	if !emptySub.IsSubmenu() {
		t.Errorf("empty (non-nil) submenu must report true")
	}
	if !fullSub.IsSubmenu() {
		t.Errorf("populated submenu must report true")
	}
}

func TestLoadMainMenu_SubmenuFlagTolerance(t *testing.T) {
	// Whitespace around '1' shouldn't trip the flag parser.
	p := writeTemp(t, `[UserMenu/MainMenu/Item0]
HotKey=s
Label=sub
Submenu= 1

[UserMenu/MainMenu/Item0/Item0]
Command0=ok
HotKey=o
Label=ok
Submenu=0
`)
	items, _ := LoadMainMenu(p)
	if len(items) != 1 || !items[0].IsSubmenu() || len(items[0].Submenu) != 1 {
		t.Fatalf("submenu flag whitespace not tolerated: %#v", items)
	}
}
