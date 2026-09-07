package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func applyCommandTestContext() ApplyCommandContext {
	return ApplyCommandContext{
		Dialect:    ApplyCommandDialectPOSIX,
		ActiveSide: ApplyCommandLeftSide,
		Active: ApplyCommandPanel{
			PathStyle:          ApplyCommandPathStylePOSIX,
			Directory:          "/active/dir",
			ShortDirectory:     "/ACTIVE",
			RealDirectory:      "/real/active",
			RealShortDirectory: "/REAL/A",
			Current: ApplyCommandFile{
				Name: "archive.tar.gz", ShortName: "ARCHIV~1.GZ", Description: "active description",
			},
		},
		Passive: ApplyCommandPanel{
			PathStyle: ApplyCommandPathStylePOSIX, Directory: "/passive/dir",
			Current: ApplyCommandFile{Name: "passive.txt", ShortName: "PASSIV~1.TXT"},
		},
	}
}

func expandApplyCommandForTest(t *testing.T, source string, ctx ApplyCommandContext) ApplyCommandExpansion {
	t.Helper()
	compiled, err := CompileApplyCommand(source)
	if err != nil {
		t.Fatalf("CompileApplyCommand(%q): %v", source, err)
	}
	expansion, err := compiled.Expand(ctx, nil)
	if err != nil {
		t.Fatalf("Expand(%q): %v", source, err)
	}
	return expansion
}

func TestApplyCommandCanonicalScalarMetasymbols(t *testing.T) {
	ctx := applyCommandTestContext()
	tests := []struct {
		source string
		want   string
	}{
		{"!!", "!"},
		{"!", "archive.tar"},
		{"!.!", "archive.tar.gz"},
		{"!~", "ARCHIV~1"},
		{"!`", "gz"},
		{"!`~", "GZ"},
		{"!-!", "ARCHIV~1.GZ"},
		{"!+!", "ARCHIV~1.GZ"},
		{"!:", "/"},
		{"!\\", "/active/dir/"},
		{"!/", "/ACTIVE/"},
		{"!=\\", "/real/active/"},
		{"!=/", "/REAL/A/"},
		{"!?!", "active description"},
	}

	for _, test := range tests {
		t.Run(test.source, func(t *testing.T) {
			got := expandApplyCommandForTest(t, test.source, ctx).Command
			if got != test.want {
				t.Fatalf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestApplyCommandScalarPreservesPOSIXBackslashFilename(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.Current = ApplyCommandFile{Name: `foo\bar.txt`}
	got := expandApplyCommandForTest(t, "!|!.!|!`|!~", ctx).Command
	if want := `foo\bar|foo\bar.txt|txt|foo\bar`; got != want {
		t.Fatalf("backslash filename expansion = %q, want %q", got, want)
	}
}

func TestApplyCommandExistingF4AliasesRemainAvailable(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.Directory = "/active/dir/"

	tests := []struct {
		source string
		want   string
	}{
		{"!`!", ".gz"},
		{"!~!", "archive.tar"},
		{"!\\!", "/active/dir/"},
		{"!/!", "/active/dir"},
	}
	for _, test := range tests {
		if got := expandApplyCommandForTest(t, test.source, ctx).Command; got != test.want {
			t.Errorf("%q: got %q, want %q", test.source, got, test.want)
		}
	}
}

func TestApplyCommandScalarValuesAreRawAndEnvironmentIsNotExpanded(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.Current.Name = "a $HOME 'quoted'.txt"

	got := expandApplyCommandForTest(t, "$HOME ! %PATH%", ctx).Command
	want := "$HOME a $HOME 'quoted' %PATH%"
	if got != want {
		t.Fatalf("got %q, want raw substitution %q", got, want)
	}
}

func TestApplyCommandSelectorsUseActivePassiveLeftAndRightSnapshots(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.ActiveSide = ApplyCommandRightSide
	ctx.Active.Current.Name = "right.txt"
	ctx.Passive.Current.Name = "left.txt"

	got := expandApplyCommandForTest(t, "!.!|!#!.!|![!.!|!]!.!|!^!.!", ctx).Command
	if want := "right.txt|left.txt|left.txt|right.txt|right.txt"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyCommandShortFormsFallBackToLongAndDescriptionMayBeEmpty(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.ShortDirectory = ""
	ctx.Active.RealShortDirectory = ""
	ctx.Active.Current.ShortName = ""
	ctx.Active.Current.Description = ""

	got := expandApplyCommandForTest(t, "!~|!-!|!`~|!/|!=/|!?!", ctx).Command
	want := "archive.tar|archive.tar.gz|gz|/active/dir/|/real/active/|"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestApplyCommandPromptsCompileResolveAndExpandPreflightValues(t *testing.T) {
	ctx := applyCommandTestContext()
	compiled, err := CompileApplyCommand("run !?$find$Find in (!.!):?(!)! -- !?Literal (parentheses)?raw!")
	if err != nil {
		t.Fatal(err)
	}

	wantSpecs := []ApplyCommandPromptSpec{
		{Index: 0, History: "find", TitleTemplate: "Find in (!.!):", InitialTemplate: "(!)"},
		{Index: 1, TitleTemplate: "Literal (parentheses)", InitialTemplate: "raw"},
	}
	if got := compiled.Prompts(); !reflect.DeepEqual(got, wantSpecs) {
		t.Fatalf("Prompts() = %#v, want %#v", got, wantSpecs)
	}

	resolved, err := compiled.ResolvePrompts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantResolved := []ApplyCommandResolvedPrompt{
		{Index: 0, History: "find", Title: "Find in archive.tar.gz:", Initial: "archive.tar"},
		{Index: 1, Title: "Literal (parentheses)", Initial: "raw"},
	}
	if !reflect.DeepEqual(resolved, wantResolved) {
		t.Fatalf("ResolvePrompts() = %#v, want %#v", resolved, wantResolved)
	}

	if _, err := compiled.Expand(ctx, nil); err == nil || !strings.Contains(err.Error(), "prompt 0") {
		t.Fatalf("missing prompt value error = %v", err)
	}
	expansion, err := compiled.Expand(ctx, ApplyCommandPromptValues{0: "needle $HOME", 1: ""})
	if err != nil {
		t.Fatal(err)
	}
	if want := "run needle $HOME -- "; expansion.Command != want {
		t.Fatalf("Command = %q, want %q", expansion.Command, want)
	}
}

func TestApplyCommandMalformedSyntaxIsRejected(t *testing.T) {
	tests := []string{
		"!?",
		"!?title(unclosed?value!",
		"!?title)?value!",
		"!?title(!?nested?value!)?value!",
		"!?title(!@Q!)?value!",
		"!@Q",
		"!@Z!",
		"!=x",
		"bad\x00template",
	}
	for _, source := range tests {
		t.Run(source, func(t *testing.T) {
			_, err := CompileApplyCommand(source)
			if err == nil {
				t.Fatalf("CompileApplyCommand(%q) unexpectedly succeeded", source)
			}
			var syntax *ApplyCommandSyntaxError
			if !errors.As(err, &syntax) {
				t.Fatalf("error %T (%v) is not ApplyCommandSyntaxError", err, err)
			}
		})
	}
}

func TestApplyCommandInlineListsUseExecutionDialect(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.Selected = []ApplyCommandFile{
		{Name: "plain", ShortName: "PLAIN"},
		{Name: "two words", ShortName: "TWO~1"},
		{Name: "it's", ShortName: "ITS"},
	}

	if got := expandApplyCommandForTest(t, "!&", ctx).Command; got != `'plain' 'two words' 'it'\''s'` {
		t.Fatalf("POSIX default = %q", got)
	}
	if got := expandApplyCommandForTest(t, "!&Q", ctx).Command; got != `'plain' 'two words' 'it'\''s'` {
		t.Fatalf("POSIX Q = %q", got)
	}
	if got := expandApplyCommandForTest(t, "!&q", ctx).Command; got != `plain 'two words' 'it'\''s'` {
		t.Fatalf("POSIX q = %q", got)
	}
	if got := expandApplyCommandForTest(t, "!&~", ctx).Command; got != `'PLAIN' 'TWO~1' 'ITS'` {
		t.Fatalf("POSIX short list = %q", got)
	}

	ctx.Dialect = ApplyCommandDialectCMD
	ctx.Active.Selected = append(ctx.Active.Selected[:2], ApplyCommandFile{Name: "%PATH% report"})
	if got := expandApplyCommandForTest(t, "!&", ctx).Command; got != `"plain" "two words" "%F4_APPLY_LITERAL_PERCENT_8C1E%PATH%F4_APPLY_LITERAL_PERCENT_8C1E% report"` {
		t.Fatalf("CMD = %q", got)
	}

	ctx.Dialect = ApplyCommandDialectPowerShell
	ctx.Active.Selected = append(ctx.Active.Selected[:2], ApplyCommandFile{Name: "it's"})
	if got := expandApplyCommandForTest(t, "!&", ctx).Command; got != `'plain' 'two words' 'it''s'` {
		t.Fatalf("PowerShell = %q", got)
	}
	ctx.Active.Selected = []ApplyCommandFile{{Name: "a,b"}, {Name: "@arguments"}, {Name: "plain"}}
	if got := expandApplyCommandForTest(t, "!&q", ctx).Command; got != `'a,b' '@arguments' plain` {
		t.Fatalf("PowerShell q = %q", got)
	}
}

func TestApplyCommandAggregateMetadataAndCardinality(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.Selected = []ApplyCommandFile{{Name: "a"}, {Name: "b"}}
	ctx.Passive.Selected = []ApplyCommandFile{{Name: "p"}}

	compiled, err := CompileApplyCommand("!& !#!@F!")
	if err != nil {
		t.Fatal(err)
	}
	metadata := compiled.Metadata()
	if !metadata.HasAggregate || !metadata.RequiresListFiles {
		t.Fatalf("metadata = %#v", metadata)
	}
	if want := []ApplyCommandPanelSelector{ApplyCommandPanelActive, ApplyCommandPanelPassive}; !reflect.DeepEqual(metadata.AggregatePanels, want) {
		t.Fatalf("aggregate panels = %#v, want %#v", metadata.AggregatePanels, want)
	}
	expansion, err := compiled.Expand(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if expansion.Cardinality != ApplyCommandOnce {
		t.Fatalf("active aggregate cardinality = %v", expansion.Cardinality)
	}

	passiveOnly := expandApplyCommandForTest(t, "!#!&", ctx)
	if passiveOnly.Cardinality != ApplyCommandPerTarget {
		t.Fatalf("passive aggregate cardinality = %v", passiveOnly.Cardinality)
	}

	ctx.ActiveSide = ApplyCommandRightSide
	if got := expandApplyCommandForTest(t, "![!&", ctx).Cardinality; got != ApplyCommandPerTarget {
		t.Fatalf("left/passive aggregate cardinality = %v", got)
	}
	if got := expandApplyCommandForTest(t, "!]!&", ctx).Cardinality; got != ApplyCommandOnce {
		t.Fatalf("right/active aggregate cardinality = %v", got)
	}
}

func TestApplyCommandInlineAggregateRequiresDialect(t *testing.T) {
	minimal, err := CompileApplyCommand("echo !&q")
	if err != nil {
		t.Fatal(err)
	}
	if !minimal.Metadata().HasAggregate || !minimal.Metadata().RequiresDialect {
		t.Fatalf("conditionally quoted aggregate metadata = %+v", minimal.Metadata())
	}
	quoted, err := CompileApplyCommand("echo !&")
	if err != nil {
		t.Fatal(err)
	}
	if !quoted.Metadata().RequiresDialect {
		t.Fatalf("quoted aggregate metadata = %+v", quoted.Metadata())
	}
}

func TestApplyCommandListFilesAreResourceRequests(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Dialect = ApplyCommandDialectPowerShell
	ctx.Active.PathStyle = ApplyCommandPathStyleWindows
	ctx.Active.Directory = `C:\work`
	ctx.Active.ShortDirectory = `C:\WORK`
	ctx.Active.Selected = []ApplyCommandFile{
		{Name: "one.txt", ShortName: "ONE.TXT"},
		{Name: `sub\two name.txt`},
	}

	compiled, err := CompileApplyCommand("tool !@FQSU! then !$AFW!")
	if err != nil {
		t.Fatal(err)
	}
	expansion, err := compiled.Expand(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(expansion.Resources) != 2 {
		t.Fatalf("got %d resources, want 2", len(expansion.Resources))
	}
	if !strings.ContainsRune(expansion.Command, '\x00') {
		t.Fatalf("unmaterialized command has no protected placeholder: %q", expansion.Command)
	}

	first := expansion.Resources[0]
	wantFirstEntries := []string{"C:/work/one.txt", "C:/work/sub/two name.txt"}
	if !reflect.DeepEqual(first.ListFile.Entries, wantFirstEntries) {
		t.Fatalf("first entries = %#v, want %#v", first.ListFile.Entries, wantFirstEntries)
	}
	if !first.ListFile.FullPaths || !first.ListFile.QuoteEntries || !first.ListFile.ForwardSlashes || first.ListFile.ShortNames {
		t.Fatalf("first modifiers = %#v", first.ListFile)
	}
	if first.ListFile.Encoding != ApplyCommandListUTF8BOM || !first.ListFile.BOM {
		t.Fatalf("first encoding = %v, BOM = %v", first.ListFile.Encoding, first.ListFile.BOM)
	}
	wantLines := []string{`"C:/work/one.txt"`, `"C:/work/sub/two name.txt"`}
	if got := first.ListFile.Lines(); !reflect.DeepEqual(got, wantLines) {
		t.Fatalf("first lines = %#v, want %#v", got, wantLines)
	}

	second := expansion.Resources[1]
	wantSecondEntries := []string{`C:\WORK\ONE.TXT`, `C:\WORK\sub\two name.txt`}
	if !reflect.DeepEqual(second.ListFile.Entries, wantSecondEntries) {
		t.Fatalf("second entries = %#v, want %#v", second.ListFile.Entries, wantSecondEntries)
	}
	if !second.ListFile.ShortNames || second.ListFile.Encoding != ApplyCommandListUTF16LE || !second.ListFile.BOM {
		t.Fatalf("second spec = %#v", second.ListFile)
	}

	if _, err := expansion.Render(map[int]string{0: "/tmp/list one"}); err == nil {
		t.Fatal("Render unexpectedly accepted a missing resource path")
	}
	rendered, err := expansion.Render(map[int]string{0: "/tmp/list one", 1: "/tmp/list2"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "tool '/tmp/list one' then '/tmp/list2'"; rendered != want {
		t.Fatalf("Render = %q, want %q", rendered, want)
	}
}

func TestApplyCommandPOSIXPathsPreserveBackslashes(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.Directory = `/tmp/foo\`
	ctx.Active.Selected = []ApplyCommandFile{{Name: `bar\baz`}}
	if got, want := expandApplyCommandForTest(t, `!\|!/!`, ctx).Command, `/tmp/foo\/|/tmp/foo\`; got != want {
		t.Fatalf("directory tokens = %q, want %q", got, want)
	}
	expansion := expandApplyCommandForTest(t, `!@FS!`, ctx)
	if got, want := expansion.Resources[0].ListFile.Entries, []string{`/tmp/foo\/bar\baz`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("POSIX F/S entries = %#v, want %#v", got, want)
	}
}

func TestApplyCommandPathStyleIsIndependentFromShellDialect(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Dialect = ApplyCommandDialectPowerShell
	ctx.Active.Directory = "/work"
	ctx.Active.Selected = []ApplyCommandFile{{Name: "name"}}
	if got := expandApplyCommandForTest(t, `!\`, ctx).Command; got != "/work/" {
		t.Fatalf("PowerShell with POSIX path = %q", got)
	}
	if got := expandApplyCommandForTest(t, `!@F!`, ctx).Resources[0].ListFile.Entries; !reflect.DeepEqual(got, []string{"/work/name"}) {
		t.Fatalf("PowerShell POSIX F entries = %#v", got)
	}

	ctx.Dialect = ApplyCommandDialectPOSIX
	ctx.Active.PathStyle = ApplyCommandPathStyleWindows
	ctx.Active.Directory = `C:\work`
	if got := expandApplyCommandForTest(t, `!\`, ctx).Command; got != `C:\work\` {
		t.Fatalf("POSIX shell with Windows path = %q", got)
	}
	if got := expandApplyCommandForTest(t, `!@F!`, ctx).Resources[0].ListFile.Entries; !reflect.DeepEqual(got, []string{`C:\work\name`}) {
		t.Fatalf("POSIX-shell Windows F entries = %#v", got)
	}
}

func TestApplyCommandVolumeUsesPanelPathStyle(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.Directory = "//server/share/path"
	ctx.Active.PathStyle = ApplyCommandPathStylePOSIX
	if got := expandApplyCommandForTest(t, "!:", ctx).Command; got != "/" {
		t.Fatalf("POSIX double-slash volume = %q", got)
	}
	ctx.Active.PathStyle = ApplyCommandPathStyleWindows
	if got := expandApplyCommandForTest(t, "!:", ctx).Command; got != "//server/share" {
		t.Fatalf("Windows UNC volume = %q", got)
	}
}

func TestApplyCommandResourcePathsAreAlwaysShellQuoted(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Dialect = ApplyCommandDialectPowerShell
	expansion := expandApplyCommandForTest(t, "tool !@!", ctx)
	rendered, err := expansion.Render(map[int]string{0: `C:\Users\A,B\list.tmp`})
	if err != nil {
		t.Fatal(err)
	}
	if want := `tool 'C:\Users\A,B\list.tmp'`; rendered != want {
		t.Fatalf("Render = %q, want %q", rendered, want)
	}
}

func TestApplyCommandRejectsGeneratedCmdArgumentsContainingQuotes(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Dialect = ApplyCommandDialectCMD
	ctx.Active.Selected = []ApplyCommandFile{{Name: `x"&whoami&"`}}
	compiled, err := CompileApplyCommand("tool !&")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := compiled.Expand(ctx, nil); err == nil {
		t.Fatal("quoted CMD aggregate accepted a shell-closing quote")
	}
	conditional, err := CompileApplyCommand("tool !&q")
	if err != nil {
		t.Fatal(err)
	} else if _, err := conditional.Expand(ctx, nil); err == nil {
		t.Fatal("conditionally quoted CMD aggregate accepted a shell-closing quote")
	}

	expansion := expandApplyCommandForTest(t, "tool !@!", ctx)
	if _, err := expansion.Render(map[int]string{0: `C:\tmp\x"&whoami&".lst`}); err == nil {
		t.Fatal("CMD resource path accepted a shell-closing quote")
	}
	ctx.Active.Selected = []ApplyCommandFile{{Name: "safe\r\nwhoami"}}
	if _, err := compiled.Expand(ctx, nil); err == nil {
		t.Fatal("CMD aggregate accepted a command line break")
	}
	if _, err := expansion.Render(map[int]string{0: "C:\\tmp\\safe\r\nwhoami.lst"}); err == nil {
		t.Fatal("CMD resource path accepted a command line break")
	}
}

func TestApplyCommandFullListUsesPanelPathStyleForAbsolutePaths(t *testing.T) {
	ctx := applyCommandTestContext()
	ctx.Active.Directory = "/work"
	ctx.Active.Selected = []ApplyCommandFile{{Name: "a:b"}, {Name: `\name`}, {Name: "/already/full"}}
	expansion := expandApplyCommandForTest(t, "!@F!", ctx)
	if got, want := expansion.Resources[0].ListFile.Entries, []string{"/work/a:b", `/work/\name`, "/already/full"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("POSIX full entries = %#v, want %#v", got, want)
	}

	ctx.Dialect = ApplyCommandDialectCMD
	ctx.Active.PathStyle = ApplyCommandPathStyleWindows
	ctx.Active.Directory = `C:\work`
	ctx.Active.Selected = []ApplyCommandFile{{Name: `C:\already\full`}, {Name: `\rooted`}, {Name: "relative"}}
	expansion = expandApplyCommandForTest(t, "!@F!", ctx)
	if got, want := expansion.Resources[0].ListFile.Entries, []string{`C:\already\full`, `\rooted`, `C:\work\relative`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CMD full entries = %#v, want %#v", got, want)
	}
}

func TestApplyCommandListLinesQuoteEmbeddedDoubleQuotes(t *testing.T) {
	spec := ApplyCommandListFileSpec{Entries: []string{`a"b`}, QuoteEntries: true}
	if got, want := spec.Lines(), []string{`"a""b"`}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Lines = %#v, want %#v", got, want)
	}
}
