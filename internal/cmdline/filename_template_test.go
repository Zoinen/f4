package cmdline

import "testing"

func TestFilenameTemplateScalarsAndLiteralValues(t *testing.T) {
	template, err := CompileFilenameTemplate("!{prefix}!_!~!_!{seq}!_!?!")
	if err != nil {
		t.Fatal(err)
	}
	ctx := ApplyCommandContext{Active: ApplyCommandPanel{Current: ApplyCommandFile{Name: "photo.jpg", Description: "desc"}}}
	before, after, err := template.ExpandParts(ctx, nil, "literal!{seq}!")
	if err != nil || before != "literal!{seq}!_photo_" || after != "_desc" {
		t.Fatalf("parts = %q %q, %v", before, after, err)
	}
	// The normal command compiler must retain its previous interpretation.
	old, err := CompileApplyCommand("!{seq}!")
	if err != nil {
		t.Fatal(err)
	}
	if old.nodes[0].kind == applyNodeFilenameSequence {
		t.Fatal("filename token leaked into command parsing")
	}
}

func TestFilenameTemplatePrompts(t *testing.T) {
	template, err := CompileFilenameTemplate("!?Name?initial!_!{seq}!")
	if err != nil {
		t.Fatal(err)
	}
	prompts, err := template.ResolvePrompts(ApplyCommandContext{})
	if err != nil || len(prompts) != 1 || prompts[0].Initial != "initial" {
		t.Fatalf("prompts = %+v, %v", prompts, err)
	}
	before, after, err := template.ExpandParts(ApplyCommandContext{}, ApplyCommandPromptValues{0: "literal!{seq}!"}, "")
	if err != nil || before != "literal!{seq}!_" || after != "" {
		t.Fatalf("parts = %q %q, %v", before, after, err)
	}
	if _, _, err := template.ExpandParts(ApplyCommandContext{}, nil, ""); err == nil {
		t.Fatal("missing prompt accepted")
	}
}

func TestFilenameTemplateRejectsUnsupportedForms(t *testing.T) {
	for _, source := range []string{"plain", "!{seq}!!{seq}!", "!&!{seq}!", "!@!_!{seq}!", "../!{seq}!", "!?Title?(!&)!_!{seq}!", "!?Title?(!{seq}!)!_!{seq}!", "\x00!{seq}!"} {
		t.Run(source, func(t *testing.T) {
			if _, err := CompileFilenameTemplate(source); err == nil {
				t.Fatal("invalid template accepted")
			}
		})
	}
}
