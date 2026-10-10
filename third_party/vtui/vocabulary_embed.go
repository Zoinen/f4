package vtui

import (
	_ "embed"
	"encoding/json"
)

// vocabularyJSON is the widget vocabulary the protocol's describe operation
// answers with. It is compiled in: the host used to read vocabulary.json from
// its working directory, so a binary run anywhere but the source tree answered
// describe with null, and a client that validates against it saw an empty
// vocabulary (vtui#174, proposal 3).
//
//go:embed vocabulary.json
var vocabularyJSON []byte

// Vocabulary returns the parsed vocabulary document, or nil if it does not
// parse.
func Vocabulary() any {
	var vocab any
	if err := json.Unmarshal(vocabularyJSON, &vocab); err != nil {
		return nil
	}
	return vocab
}
