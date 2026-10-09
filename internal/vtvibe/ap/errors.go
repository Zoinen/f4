package ap

// ErrCode identifies a specific failure condition, matching the reference
// implementation's error codes verbatim (§3.4). Two of them
// (snippet_tail_NOT_FOUND) intentionally keep the reference's inconsistent
// casing rather than "fixing" it, since these codes are meant to be a
// stable, checkable contract for callers (and for our own ported test
// cases).
type ErrCode string

const (
	ErrAnchorNotFound      ErrCode = "ANCHOR_NOT_FOUND"
	ErrAmbiguousAnchor     ErrCode = "AMBIGUOUS_ANCHOR"
	ErrAmbiguousMatch      ErrCode = "AMBIGUOUS_MATCH"
	ErrSnippetNotFound     ErrCode = "SNIPPET_NOT_FOUND"
	ErrSnippetTailNotFound ErrCode = "snippet_tail_NOT_FOUND"
	ErrLocatorConsumed     ErrCode = "LOCATOR_CONSUMED"
	ErrEmptyReplace        ErrCode = "EMPTY_REPLACE"
	ErrPatchTruncated      ErrCode = "PATCH_TRUNCATED"
	ErrMissingContent      ErrCode = "MISSING_CONTENT"
	ErrInvalidModification ErrCode = "INVALID_MODIFICATION"
	ErrFileNotFound        ErrCode = "FILE_NOT_FOUND"
	ErrFileExists          ErrCode = "FILE_EXISTS"
	ErrPathIsFile          ErrCode = "PATH_IS_FILE"
	ErrDestinationExists   ErrCode = "DESTINATION_EXISTS"
	ErrInvalidFilePath     ErrCode = "INVALID_FILE_PATH"
	ErrInvalidPatchFile    ErrCode = "INVALID_PATCH_FILE"
	ErrAfailedExists       ErrCode = "AFAILED_EXISTS"
	ErrNestingMismatch     ErrCode = "NESTING_MISMATCH"
	ErrFileWriteError      ErrCode = "FILE_WRITE_ERROR"
	ErrFileDeleteError     ErrCode = "FILE_DELETE_ERROR"
	ErrFileRenameError     ErrCode = "FILE_RENAME_ERROR"
	ErrDirCreateError      ErrCode = "DIR_CREATE_ERROR"
	// ErrSnapshotError: the undo snapshot of the paths a patch touches
	// (undo.go) could not be taken, so nothing was written.
	ErrSnapshotError ErrCode = "SNAPSHOT_ERROR"
)

// AppError is one failed modification or one fatal patch-level failure. It
// mirrors the reference's `{"code": ..., "message": ..., "context": ...}`
// error dicts.
type AppError struct {
	Code    ErrCode
	Message string
	Context map[string]any
}

func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return string(e.Code) + ": " + e.Message
}

// fixHints are the "How to fix" notes bundled into afailed.md for each
// error code (§ "LLM-oriented failure report" of the reference).
var fixHints = map[ErrCode]string{
	ErrAmbiguousMatch: "The `snippet` matches several places, so the patcher refuses to guess. Either extend the " +
		"`snippet` downwards until it covers a unique block, or add an `anchor` holding the nearest " +
		"unique construct above the target (a function signature, a class line, a unique comment). " +
		"If you really meant to change every occurrence, emit one modification per occurrence, all " +
		"with the same locator, in top-to-bottom order.",
	ErrLocatorConsumed: "The locator was present in the original file, but an earlier modification in THIS patch " +
		"replaced or deleted the region containing it. Do not restate a change you have already " +
		"made: fold the second edit into the `content` of the first one, or rewrite its locator " +
		"against the text as it looks AFTER the earlier modification.",
	ErrSnippetNotFound: "The `snippet` does not exist in the search scope. Copy the locator verbatim from the " +
		"'Current content' section below - do not retype it from memory. Remember that a locator " +
		"MUST cover whole lines and that the search starts after the previous modification of the " +
		"same file (top-to-bottom order is mandatory).",
	ErrAnchorNotFound: "The `anchor` does not exist in the file. Copy it verbatim from the 'Current content' " +
		"section, or drop the `anchor` entirely if the `snippet` is already unique.",
	ErrAmbiguousAnchor: "The `anchor` occurs several times and the `snippet` could not be tied to exactly one of " +
		"them. Choose a larger or more distinctive anchor.",
	ErrSnippetTailNotFound: "`snippet_tail` was not found after `snippet`. The two MUST be independent: `snippet_tail` " +
		"marks the *end* of the block and must appear strictly after `snippet` in the file. Do not " +
		"put the whole block into `snippet`, and do not repeat `content` in `snippet_tail`.",
	ErrEmptyReplace: "`REPLACE` with empty `content` is refused in strict mode. Use `DELETE` to remove code.",
	ErrPatchTruncated: "The patch file ends with an empty `content` block, which is what a cut-off answer looks " +
		"like. Re-emit the patch in full. Appending an `[ID] END` directive proves the patch is " +
		"complete and makes a deliberately empty `content` acceptable.",
	ErrMissingContent: "The action requires a `content` block and none was given. Check that the answer was not cut " +
		"off and that the `content` directive carries the patch ID prefix.",
	ErrInvalidModification: "The modification is structurally incomplete. Every REPLACE/DELETE/INSERT_* needs a `snippet`; " +
		"every REPLACE/INSERT_*/RECREATE needs a `content`.",
	ErrFileNotFound: "The target file does not exist. Check the path (it is relative to the project root) or use " +
		"`CREATE` if the file is meant to be new.",
	ErrFileExists: "`CREATE` refuses to overwrite a non-empty file. Use `RECREATE` to replace its whole content, " +
		"or ordinary REPLACE/INSERT modifications to edit it in place.",
	ErrPathIsFile:        "A file already exists where a directory was requested.",
	ErrDestinationExists: "The `RENAME` destination already exists. Delete it first or pick another name.",
	ErrInvalidFilePath: "The path escapes the project root or is malformed. Paths are relative to the project root " +
		"and MUST NOT contain `..`.",
	ErrInvalidPatchFile: "The patch itself could not be parsed. Re-read the directive syntax: every directive line is " +
		"`<patch-id> KEYWORD`, the ID is the same 8 hex characters everywhere, and no `#` comments " +
		"may appear inside the patch body.",
	ErrAfailedExists: "A previous failure report is still present. Remove it before applying a new patch.",
}
