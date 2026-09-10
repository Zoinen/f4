package f4settings

import (
	"context"
	"errors"
	"testing"
)

func TestSearchMetadataOnly(t *testing.T) {
	f := Field{Label: Text{English: "Directory read timeout", Translations: map[string]string{"ru": "Время чтения каталога"}}, Description: Text{English: "Maximum seconds for path suggestions"}, Group: "Completion", Aliases: []string{"PathHintTimeout"}}
	for _, query := range []string{"", "DIRECTORY seconds", "completion", "pathhinttimeout", "каталога"} {
		if !Matches(query, f, "Navigation", "ru", nil) {
			t.Errorf("missing match %q", query)
		}
	}
	if Matches("password", f, "Navigation", "ru", nil) {
		t.Fatal("unexpected match")
	}
}
func TestDraftIsolationAndPartialApply(t *testing.T) {
	d := NewDraft(map[string]string{"a": "old", "b": "old"}, map[string][]Record{"records": {{ID: "r", Values: map[string]string{"name": "old"}}}})
	d.Values["a"] = "new"
	d.Values["b"] = "failed"
	d.Records["records"][0].Values["name"] = "new"
	if d.BaselineRecords["records"][0].Values["name"] != "old" {
		t.Fatal("record draft aliases baseline")
	}
	d.CommitFunc = func(context.Context, *Draft) Result {
		return Result{Applied: []string{"a", "r"}, Revisions: map[string]string{"r": "2"}, Errors: map[string]error{"b": errors.New("disk full")}}
	}
	d.Commit(context.Background())
	if d.Dirty("a") || !d.Dirty("b") || d.Dirty("records") {
		t.Fatalf("incorrect partial baseline: %#v", d.Changed())
	}
}
