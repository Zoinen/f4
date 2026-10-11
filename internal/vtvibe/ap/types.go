// Package ap is a Go port of github.com/unxed/ap, the "AI-friendly Patch"
// (ap) format: a declarative, snippet-located patch format designed to be
// resilient to the kind of mistakes LLMs make when generating diffs.
//
// This is a line-by-line port of the reference implementation
// (implementation/ap.py in github.com/unxed/ap), not a reinterpretation:
// the parsing rules, the search/uniqueness algorithm, the tolerant-mode
// heuristics and the error codes are meant to match it exactly, so that a
// patch which applies (or fails) a given way under the Python reference
// applies (or fails) the same way here. See
// https://github.com/unxed/ap/blob/main/ap.md for the format specification
// (AP 3.2), section numbers in comments below refer to it.
//
// f4#1606 tracks this port; it is a standalone package for now, with
// integration into internal/vtvibe (docs/VTVIBE.md section 17-18) as a
// following step.
package ap

// FormatVersion is the AP format version this package implements.
const FormatVersion = "3.2"

var supportedVersions = map[string]bool{"3.0": true, "3.1": true, "3.2": true}

// Canonical directive keywords of the format.
var actionKeys = map[string]bool{
	"REPLACE":       true,
	"INSERT_AFTER":  true,
	"INSERT_BEFORE": true,
	"DELETE":        true,
	"RECREATE":      true,
}

var valueKeys = map[string]bool{
	"snippet":      true,
	"anchor":       true,
	"content":      true,
	"snippet_tail": true,
}

var argKeys = map[string]bool{
	"include_leading_blank_lines":  true,
	"include_trailing_blank_lines": true,
	"scope_end":                    true,
}

var newlineVals = map[string]bool{"LF": true, "CRLF": true, "CR": true}

var canonicalKeys = buildCanonicalKeys()

func buildCanonicalKeys() map[string]bool {
	m := map[string]bool{"FILE": true, "CREATE": true, "RENAME": true, "END": true}
	for k := range actionKeys {
		m[k] = true
	}
	for k := range valueKeys {
		m[k] = true
	}
	for k := range argKeys {
		m[k] = true
	}
	return m
}

// keyAliases are tolerated misspellings/synonyms that weaker models invent.
// Resolving them is safe because directive lines are always prefixed with
// the patch ID, so they can never collide with the payload.
var keyAliases = map[string]string{
	"CREATE_FILE": "CREATE", "CREATE_DIR": "CREATE", "NEW_FILE": "CREATE", "ADD_FILE": "CREATE",
	"MOVE": "RENAME", "RENAME_TO": "RENAME", "MOVE_TO": "RENAME",
	"INSERT": "INSERT_AFTER", "APPEND_AFTER": "INSERT_AFTER", "ADD_AFTER": "INSERT_AFTER",
	"PREPEND_BEFORE": "INSERT_BEFORE", "ADD_BEFORE": "INSERT_BEFORE",
	"REWRITE": "RECREATE", "OVERWRITE": "RECREATE", "REPLACE_FILE": "RECREATE",
	"REMOVE":        "DELETE",
	"snippet_start": "snippet", "start_snippet": "snippet",
	"snippet_end": "snippet_tail", "end_snippet": "snippet_tail", "tail": "snippet_tail",
	"new_content": "content", "code": "content", "body": "content",
	"scope":       "anchor",
	"whole_block": "scope_end", "block_end": "scope_end", "to_scope_end": "scope_end",
}

// Modification is one change within a FILE block (§2.5). Optional string
// fields are nil when the directive was never given, and non-nil-but-empty
// when it was given with an empty value block - the two are semantically
// different (e.g. absent `content` is MISSING_CONTENT, empty `content` is
// treated as DELETE).
type Modification struct {
	Action string

	Snippet     *string
	Anchor      *string
	Content     *string
	SnippetTail *string

	// IncludeLeadingBlankLines/IncludeTrailingBlankLines mirror
	// mod.get(key, 0) in the reference: 0 means "absent or explicitly
	// zero", both are equivalent for the option's effect.
	IncludeLeadingBlankLines  int
	IncludeTrailingBlankLines int
	// ScopeEnd mirrors `if mod.get('scope_end')`: any non-zero value is
	// truthy, 0/absent is not.
	ScopeEnd int

	// Line is the patch source line number the action directive started
	// on (used for the "patch line N" hint in reports).
	Line int
	// eofValue records which VALUE_KEYS was still open when a value
	// block was closed by end-of-file rather than by a following
	// directive; see §2.5 on "an unterminated empty value block".
	eofValue string
}

// FileChange is one `FILE` block: either a rename, a set of content
// modifications, or (contextually) a whole-file/whole-directory delete.
type FileChange struct {
	HasFilePath bool
	FilePath    string

	RenameTo *string
	// Newline is "", "LF", "CRLF" or "CR" (the optional FILE argument).
	Newline string

	Modifications []*Modification
}

// PatchData is the parsed form of a patch file.
type PatchData struct {
	Version string
	PatchID string
	Changes []*FileChange
}
