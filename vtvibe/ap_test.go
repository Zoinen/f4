package vtvibe

import "testing"

const sampleReply = "Here you go:\n\n```ap\nf0cacc1a AP 3.1\n\nf0cacc1a FILE\ngreeter.py\n\nf0cacc1a REPLACE\n\nf0cacc1a snippet\nprint(\"a\")\n\nf0cacc1a content\nprint(\"b\")\n```\nDone.\n"

func TestExtractPatch_Fenced(t *testing.T) {
	p := ExtractPatch(sampleReply)
	if p == nil {
		t.Fatal("expected a patch")
	}
	if p.ID != "f0cacc1a" {
		t.Errorf("id = %q", p.ID)
	}
	if len(p.Files) != 1 || p.Files[0] != "greeter.py" {
		t.Errorf("files = %v", p.Files)
	}
	if got := p.Text; got[:8] != "f0cacc1a" || got[len(got)-1] != '\n' {
		t.Errorf("text = %q", got)
	}
}

func TestExtractPatch_Unfenced(t *testing.T) {
	reply := "Sure.\n\nf0cacc1a AP 3.1\n\nf0cacc1a FILE\na.go\n\nf0cacc1a DELETE\n"
	p := ExtractPatch(reply)
	if p == nil || len(p.Files) != 1 || p.Files[0] != "a.go" {
		t.Fatalf("unfenced patch not found: %+v", p)
	}
}

func TestExtractPatch_HeaderlessAndNegative(t *testing.T) {
	headerless := "```\nabcdef12 FILE\nx.txt\n\nabcdef12 RECREATE\n\nabcdef12 content\nhi\n```"
	if p := ExtractPatch(headerless); p == nil || p.ID != "abcdef12" {
		t.Errorf("headerless patch not detected: %+v", p)
	}
	if p := ExtractPatch("```go\nfunc main() {}\n```"); p != nil {
		t.Errorf("plain code taken for a patch: %+v", p)
	}
	if p := ExtractPatch("no patch here at all"); p != nil {
		t.Errorf("prose taken for a patch: %+v", p)
	}
}

func TestSession_PatchLandsInOut(t *testing.T) {
	s := NewSession()
	s.saveArtifacts(sampleReply)
	if s.LastPatch() == nil {
		t.Fatal("LastPatch is nil")
	}
	if _, ok := s.tree.readFile(PatchPath); !ok {
		t.Errorf("%s was not written", PatchPath)
	}
	s.saveArtifacts("plain answer, no patch")
	if s.LastPatch() != nil {
		t.Error("stale patch survived an answer without one")
	}
}
