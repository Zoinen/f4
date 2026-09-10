package cmdline

import (
	"github.com/unxed/vtui"
	"testing"
)

func TestCommandLineSemanticModel(t *testing.T) {
	cl := NewCommandLine("> ")
	cl.Edit.SetText("ls")

	model := cl.SemanticModel(nil)
	if model.ID != vtui.SemanticID(cl) {
		t.Fatalf("semantic ID = %v, want the command line ID", model.ID)
	}
	if !model.Visible || !model.Focused {
		t.Fatalf("semantic visibility/focus = %v/%v, want true/true", model.Visible, model.Focused)
	}
	if model.Prompt != "> " || model.Text != "ls" || model.Empty {
		t.Fatalf("semantic prompt/text/empty = %q/%q/%v, want %q/%q/false", model.Prompt, model.Text, model.Empty, "> ", "ls")
	}
}
