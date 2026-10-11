package vtui

import (
	"bytes"
	"os"
	"testing"
)

// describe must answer from the compiled-in vocabulary, wherever the host runs
// from, and the copy must be the file in the repository.
func TestVocabularyIsEmbeddedAndMatchesTheFile(t *testing.T) {
	onDisk, err := os.ReadFile("vocabulary.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, vocabularyJSON) {
		t.Fatal("the embedded vocabulary differs from vocabulary.json")
	}

	t.Chdir(t.TempDir()) // no vocabulary.json here
	vocab, ok := Vocabulary().(map[string]any)
	if !ok {
		t.Fatalf("Vocabulary() = %T, want the parsed document", Vocabulary())
	}
	widgets, ok := vocab["widgets"].(map[string]any)
	if !ok || len(widgets) == 0 || widgets["Widget"] == nil {
		t.Fatalf("Vocabulary() has no widgets: %v", vocab["widgets"])
	}
}
